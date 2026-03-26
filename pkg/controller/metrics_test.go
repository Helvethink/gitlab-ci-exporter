package controller

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitlabclient "github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

func metricFamilyValue(t *testing.T, family *dto.MetricFamily) float64 {
	t.Helper()
	require.Len(t, family.GetMetric(), 1)

	switch family.GetType() {
	case dto.MetricType_GAUGE:
		return family.GetMetric()[0].GetGauge().GetValue()
	case dto.MetricType_COUNTER:
		return family.GetMetric()[0].GetCounter().GetValue()
	default:
		t.Fatalf("unsupported metric family type %v", family.GetType())
		return 0
	}
}

func TestNewRegistryInitializesCollectors(t *testing.T) {
	r := NewRegistry(context.Background())

	require.NotNil(t, r)
	require.NotNil(t, r.InternalCollectors.CurrentlyQueuedTasksCount)
	require.NotNil(t, r.InternalCollectors.ProjectsCount)
	require.NotNil(t, r.GetCollector(schemas.MetricKindCoverage))
	require.NotNil(t, r.GetCollector(schemas.MetricKindRunCount))
	require.NotNil(t, r.GetCollector(schemas.MetricKindRunner))
}

func TestRegisterCollectorsReturnsErrorOnDuplicateCollector(t *testing.T) {
	collector := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "test_duplicate_collector", Help: "test"},
		[]string{},
	)

	r := &Registry{
		Registry: prometheus.NewRegistry(),
		Collectors: RegistryCollectors{
			schemas.MetricKindCoverage:        collector,
			schemas.MetricKindDurationSeconds: collector,
		},
	}

	err := r.RegisterCollectors()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not add provided collector")
}

func TestExportInternalMetrics(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()

	require.NoError(t, s.SetProject(ctx, schemas.NewProject("group/project")))
	require.NoError(t, s.SetEnvironment(ctx, schemas.Environment{ProjectName: "group/project", Name: "production"}))
	require.NoError(t, s.SetRef(ctx, schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")))
	require.NoError(t, s.SetRunner(ctx, schemas.Runner{ID: 42, ProjectName: "group/project"}))
	require.NoError(t, s.SetMetric(ctx, schemas.Metric{
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
		Value: 1,
	}))

	ok, err := s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-1", "")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-2", "")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, s.DequeueTask(ctx, schemas.TaskTypePullMetrics, "task-1"))

	g := &gitlabclient.Client{
		RequestsRemaining: 17,
		RequestsLimit:     50,
	}
	g.RequestsCounter.Add(9)

	r := NewRegistry(ctx)
	require.NoError(t, r.ExportInternalMetrics(ctx, g, s))

	families, err := r.Gather()
	require.NoError(t, err)

	assert.Equal(t, float64(1), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_currently_queued_tasks_count")))
	assert.Equal(t, float64(1), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_executed_tasks_count")))
	assert.Equal(t, float64(1), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_projects_count")))
	assert.Equal(t, float64(1), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_environments_count")))
	assert.Equal(t, float64(1), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_refs_count")))
	assert.Equal(t, float64(1), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_runners_count")))
	assert.Equal(t, float64(1), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_metrics_count")))
	assert.Equal(t, float64(9), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_gitlab_api_requests_count")))
	assert.Equal(t, float64(17), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_gitlab_api_requests_remaining")))
	assert.Equal(t, float64(50), metricFamilyValue(t, metricFamilyByName(t, families, "gcpe_gitlab_api_requests_limit")))
}

func TestExportMetricsSetsGaugeAndCounterValues(t *testing.T) {
	r := NewRegistry(context.Background())

	r.ExportMetrics(schemas.Metrics{
		schemas.Metric{
			Kind: schemas.MetricKindCoverage,
			Labels: map[string]string{
				"project":     "group/project",
				"topics":      "",
				"kind":        "branch",
				"ref":         "main",
				"source":      "push",
				"variables":   "",
				"pipeline_id": "123",
				"status":      "success",
			},
			Value: 97.5,
		}.Key(): {
			Kind: schemas.MetricKindCoverage,
			Labels: map[string]string{
				"project":     "group/project",
				"topics":      "",
				"kind":        "branch",
				"ref":         "main",
				"source":      "push",
				"variables":   "",
				"pipeline_id": "123",
				"status":      "success",
			},
			Value: 97.5,
		},
		schemas.Metric{
			Kind: schemas.MetricKindRunCount,
			Labels: map[string]string{
				"project":     "group/project",
				"topics":      "",
				"kind":        "branch",
				"ref":         "main",
				"source":      "push",
				"variables":   "",
				"pipeline_id": "123",
				"status":      "success",
			},
			Value: 3,
		}.Key(): {
			Kind: schemas.MetricKindRunCount,
			Labels: map[string]string{
				"project":     "group/project",
				"topics":      "",
				"kind":        "branch",
				"ref":         "main",
				"source":      "push",
				"variables":   "",
				"pipeline_id": "123",
				"status":      "success",
			},
			Value: 3,
		},
	})

	families, err := r.Gather()
	require.NoError(t, err)

	assert.Equal(t, float64(97.5), metricFamilyValue(t, metricFamilyByName(t, families, "gitlab_ci_pipeline_coverage")))
	assert.Equal(t, float64(3), metricFamilyValue(t, metricFamilyByName(t, families, "gitlab_ci_pipeline_run_count")))
}

func TestEmitStatusMetricDense(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()

	emitStatusMetric(ctx, s, schemas.MetricKindStatus, map[string]string{
		"project":     "group/project",
		"kind":        "branch",
		"ref":         "main",
		"source":      "push",
		"variables":   "",
		"pipeline_id": "123",
	}, []string{"success", "failed"}, "success", false)

	successMetric := schemas.Metric{
		Kind: schemas.MetricKindStatus,
		Labels: map[string]string{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
	}
	require.NoError(t, s.GetMetric(ctx, &successMetric))
	assert.Equal(t, float64(1), successMetric.Value)

	failedMetric := schemas.Metric{
		Kind: schemas.MetricKindStatus,
		Labels: map[string]string{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "failed",
		},
	}
	require.NoError(t, s.GetMetric(ctx, &failedMetric))
	assert.Equal(t, float64(0), failedMetric.Value)
}

func TestEmitStatusMetricSparse(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()

	otherMetric := schemas.Metric{
		Kind: schemas.MetricKindStatus,
		Labels: map[string]string{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "failed",
		},
		Value: 1,
	}
	require.NoError(t, s.SetMetric(ctx, otherMetric))

	emitStatusMetric(ctx, s, schemas.MetricKindStatus, map[string]string{
		"project":     "group/project",
		"kind":        "branch",
		"ref":         "main",
		"source":      "push",
		"variables":   "",
		"pipeline_id": "123",
	}, []string{"success", "failed"}, "success", true)

	successMetric := schemas.Metric{
		Kind: schemas.MetricKindStatus,
		Labels: map[string]string{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
	}
	require.NoError(t, s.GetMetric(ctx, &successMetric))
	assert.Equal(t, float64(1), successMetric.Value)

	exists, err := s.MetricExists(ctx, otherMetric.Key())
	require.NoError(t, err)
	assert.False(t, exists)
}
