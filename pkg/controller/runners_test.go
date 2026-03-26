package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	gitlabclient "github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/ratelimit"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

type noopLimiterControllerRunnersTests struct{}

func (noopLimiterControllerRunnersTests) Take(ctx context.Context) time.Duration {
	return 0
}

var _ ratelimit.Limiter = noopLimiterControllerRunnersTests{}

func newTestRunnerController(t *testing.T, handler http.HandlerFunc) *Controller {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	gl, err := goGitlab.NewClient(
		"test-token",
		goGitlab.WithBaseURL(server.URL+"/api/v4"),
	)
	require.NoError(t, err)

	client := &gitlabclient.Client{
		Client:      gl,
		RateLimiter: noopLimiterControllerRunnersTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	return &Controller{
		Store:  store.NewLocalStore(),
		Gitlab: client,
	}
}

func metricsByKind(metrics schemas.Metrics, kind schemas.MetricKind) []schemas.Metric {
	out := make([]schemas.Metric, 0)
	for _, m := range metrics {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

func findMetricByKindAndLabel(metrics schemas.Metrics, kind schemas.MetricKind, labelKey, labelValue string) (schemas.Metric, bool) {
	for _, m := range metrics {
		if m.Kind == kind && m.Labels[labelKey] == labelValue {
			return m, true
		}
	}
	return schemas.Metric{}, false
}

func TestUniqueSortedNonEmpty(t *testing.T) {
	got := uniqueSortedNonEmpty([]string{"beta", "", "alpha", "beta", "gamma", ""})
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, got)
}

func TestRunnerGroupNames(t *testing.T) {
	runner := schemas.Runner{
		Groups: []struct {
			ID     int64
			Name   string
			WebURL string
		}{
			{ID: 1, Name: "platform"},
			{ID: 2, Name: "devops"},
			{ID: 3, Name: "platform"},
			{ID: 4, Name: ""},
		},
	}

	got := runnerGroupNames(runner)
	assert.Equal(t, []string{"devops", "platform"}, got)
}

func TestRunnerProjectNames(t *testing.T) {
	runner := schemas.Runner{
		Projects: []struct {
			ID                int64
			Name              string
			NameWithNamespace string
			Path              string
			PathWithNamespace string
		}{
			{ID: 1, PathWithNamespace: "group-a/proj-a"},
			{ID: 2, NameWithNamespace: "group-b/proj-b"},
			{ID: 3, Name: "proj-c"},
			{ID: 4, PathWithNamespace: "group-a/proj-a"},
			{ID: 5},
		},
	}

	got := runnerProjectNames(runner)
	assert.Equal(t, []string{"group-a/proj-a", "group-b/proj-b", "proj-c"}, got)
}

func TestRunnerTagNames(t *testing.T) {
	runner := schemas.Runner{
		TagList: []string{"docker", "linux", "docker", "", "arm64"},
	}

	got := runnerTagNames(runner)
	assert.Equal(t, []string{"arm64", "docker", "linux"}, got)
}

func TestUpdateRunner(t *testing.T) {
	ctx := context.Background()
	contactedAt := time.Unix(1710000000, 0).UTC()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/runners/101")
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 101,
			"name": "runner-101",
			"description": "shared-runner",
			"paused": false,
			"is_shared": true,
			"runner_type": "instance_type",
			"contacted_at": "2024-03-09T16:00:00Z",
			"maintenance_note": "maintenance window",
			"online": true,
			"status": "online",
			"tag_list": ["docker", "linux"],
			"groups": [],
			"projects": []
		}`))
	})

	runner := schemas.Runner{
		ProjectName:               "group/project",
		ID:                        101,
		OutputSparseStatusMetrics: true,
	}

	err := c.UpdateRunner(ctx, &runner)
	require.NoError(t, err)

	assert.Equal(t, "group/project", runner.ProjectName)
	assert.True(t, runner.OutputSparseStatusMetrics)
	assert.Equal(t, "runner-101", runner.Name)
	assert.Equal(t, "shared-runner", runner.Description)
	assert.True(t, runner.IsShared)
	assert.Equal(t, "instance_type", runner.RunnerType)
	assert.True(t, runner.Online)
	assert.Equal(t, "online", runner.Status)
	assert.False(t, runner.Paused)
	require.NotNil(t, runner.ContactedAt)
	assert.Equal(t, contactedAt.Unix(), runner.ContactedAt.Unix())
	assert.Equal(t, "maintenance window", runner.MaintenanceNote)

	storedRunner := schemas.Runner{
		ProjectName: "group/project",
		ID:          101,
	}
	err = c.Store.GetRunner(ctx, &storedRunner)
	require.NoError(t, err)

	assert.Equal(t, "runner-101", storedRunner.Name)
	assert.True(t, storedRunner.OutputSparseStatusMetrics)
}

func TestDeleteRunnerMetrics(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindRunner,
		Labels: map[string]string{
			"runner_id": "42",
		},
		Value: 1,
	}))

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindRunnerContactedAtSeconds,
		Labels: map[string]string{
			"runner_id": "42",
		},
		Value: 1710000000,
	}))

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindRunnerProjectInfo,
		Labels: map[string]string{
			"runner_id": "42",
			"project":   "group/project",
		},
		Value: 1,
	}))

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindRunnerTagInfo,
		Labels: map[string]string{
			"runner_id": "999",
			"tag":       "docker",
		},
		Value: 1,
	}))

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: map[string]string{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 99,
	}))

	err := c.deleteRunnerMetrics(ctx, 42)
	require.NoError(t, err)

	metrics, err := c.Store.Metrics(ctx)
	require.NoError(t, err)

	assert.Len(t, metricsByKind(metrics, schemas.MetricKindRunner), 0)
	assert.Len(t, metricsByKind(metrics, schemas.MetricKindRunnerContactedAtSeconds), 0)
	assert.Len(t, metricsByKind(metrics, schemas.MetricKindRunnerProjectInfo), 0)

	remainingTagMetrics := metricsByKind(metrics, schemas.MetricKindRunnerTagInfo)
	require.Len(t, remainingTagMetrics, 1)
	assert.Equal(t, "999", remainingTagMetrics[0].Labels["runner_id"])

	remainingCoverage := metricsByKind(metrics, schemas.MetricKindCoverage)
	require.Len(t, remainingCoverage, 1)
}

func TestProcessRunnerMetrics(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/runners/202")
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 202,
			"name": "runner-202",
			"description": "linux-runner",
			"paused": false,
			"is_shared": true,
			"runner_type": "instance_type",
			"contacted_at": "2024-03-09T16:00:00Z",
			"maintenance_note": "maintenance",
			"online": true,
			"status": "online",
			"tag_list": ["docker", "linux", "docker"],
			"groups": [
				{"id": 1, "name": "platform", "web_url": "https://gitlab.example.com/groups/platform"},
				{"id": 2, "name": "devops", "web_url": "https://gitlab.example.com/groups/devops"},
				{"id": 3, "name": "platform", "web_url": "https://gitlab.example.com/groups/platform"}
			],
			"projects": [
				{"id": 1, "name": "proj-a", "name_with_namespace": "group-a/proj-a", "path": "proj-a", "path_with_namespace": "group-a/proj-a"},
				{"id": 2, "name": "proj-b", "name_with_namespace": "group-b/proj-b", "path": "proj-b", "path_with_namespace": "group-b/proj-b"},
				{"id": 3, "name": "proj-a", "name_with_namespace": "group-a/proj-a", "path": "proj-a", "path_with_namespace": "group-a/proj-a"}
			]
		}`))
	})

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindRunnerTagInfo,
		Labels: map[string]string{
			"runner_id": "202",
			"tag":       "old-tag",
		},
		Value: 1,
	}))

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindRunner,
		Labels: map[string]string{
			"runner_id":          "999",
			"runner_name":        "other-runner",
			"runner_description": "other-desc",
		},
		Value: 1,
	}))

	inputRunner := schemas.Runner{
		ProjectName:               "group/project",
		ID:                        202,
		OutputSparseStatusMetrics: true,
	}

	err := c.ProcessRunnerMetrics(ctx, inputRunner)
	require.NoError(t, err)

	metrics, err := c.Store.Metrics(ctx)
	require.NoError(t, err)

	infoMetric, ok := findMetricByKindAndLabel(metrics, schemas.MetricKindRunner, "runner_id", "202")
	require.True(t, ok)
	assert.Equal(t, "runner-202", infoMetric.Labels["runner_name"])
	assert.Equal(t, "linux-runner", infoMetric.Labels["runner_description"])
	assert.Equal(t, "true", infoMetric.Labels["is_shared"])
	assert.Equal(t, "instance_type", infoMetric.Labels["runner_type"])
	assert.Equal(t, "true", infoMetric.Labels["online"])
	assert.Equal(t, "true", infoMetric.Labels["active"])
	assert.Equal(t, "false", infoMetric.Labels["paused"])
	assert.Equal(t, "online", infoMetric.Labels["status"])
	assert.Equal(t, "maintenance", infoMetric.Labels["runner_maintenance_note"])
	assert.Equal(t, 1.0, infoMetric.Value)

	contactMetric, ok := findMetricByKindAndLabel(metrics, schemas.MetricKindRunnerContactedAtSeconds, "runner_id", "202")
	require.True(t, ok)
	assert.Equal(t, float64(1710000000), contactMetric.Value)

	projectMetrics := metricsByKind(metrics, schemas.MetricKindRunnerProjectInfo)
	sort.Slice(projectMetrics, func(i, j int) bool {
		return projectMetrics[i].Labels["project"] < projectMetrics[j].Labels["project"]
	})
	require.Len(t, projectMetrics, 2)
	assert.Equal(t, "group-a/proj-a", projectMetrics[0].Labels["project"])
	assert.Equal(t, "group-b/proj-b", projectMetrics[1].Labels["project"])

	tagMetrics := metricsByKind(metrics, schemas.MetricKindRunnerTagInfo)
	sort.Slice(tagMetrics, func(i, j int) bool {
		return tagMetrics[i].Labels["tag"] < tagMetrics[j].Labels["tag"]
	})
	require.Len(t, tagMetrics, 2)
	assert.Equal(t, "docker", tagMetrics[0].Labels["tag"])
	assert.Equal(t, "linux", tagMetrics[1].Labels["tag"])

	groupMetrics := metricsByKind(metrics, schemas.MetricKindRunnerGroupInfo)
	sort.Slice(groupMetrics, func(i, j int) bool {
		return groupMetrics[i].Labels["group"] < groupMetrics[j].Labels["group"]
	})
	require.Len(t, groupMetrics, 2)
	assert.Equal(t, "devops", groupMetrics[0].Labels["group"])
	assert.Equal(t, "platform", groupMetrics[1].Labels["group"])

	otherRunnerMetric, ok := findMetricByKindAndLabel(metrics, schemas.MetricKindRunner, "runner_id", "999")
	require.True(t, ok)
	assert.Equal(t, "other-runner", otherRunnerMetric.Labels["runner_name"])

	_, ok = findMetricByKindAndLabel(metrics, schemas.MetricKindRunnerTagInfo, "tag", "old-tag")
	assert.False(t, ok)
}
