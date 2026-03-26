package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/taskq/v4"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func registerNoopPullEnvironmentMetricsTask(t *testing.T, c *Controller) {
	t.Helper()

	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullEnvironmentMetrics), &taskq.TaskConfig{
		Handler: func(context.Context, schemas.Environment) {
		},
	})
	require.NoError(t, err)
}

func TestUpdateEnvironment(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Unix(1710000000, 0).UTC()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/projects/group/project/environments/7")
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 7,
			"name": "production",
			"state": "available",
			"external_url": "https://prod.example.com",
			"last_deployment": {
				"ref": "main",
				"created_at": "2024-03-09T16:00:00Z",
				"deployable": {
					"id": 321,
					"duration": 12,
					"status": "success",
					"tag": false,
					"user": {"username": "alice"},
					"commit": {"short_id": "abc123"}
				}
			}
		}`))
	})

	env := schemas.Environment{
		ProjectName: "group/project",
		ID:          int64(7),
		Name:        "production",
	}

	err := c.UpdateEnvironment(ctx, &env)
	require.NoError(t, err)

	assert.True(t, env.Available)
	assert.Equal(t, "https://prod.example.com", env.ExternalURL)
	assert.Equal(t, int64(321), env.LatestDeployment.JobID)
	assert.Equal(t, schemas.RefKindBranch, env.LatestDeployment.RefKind)
	assert.Equal(t, "main", env.LatestDeployment.RefName)
	assert.Equal(t, "alice", env.LatestDeployment.Username)
	assert.Equal(t, "abc123", env.LatestDeployment.CommitShortID)
	assert.Equal(t, float64(createdAt.Unix()), env.LatestDeployment.Timestamp)

	storedEnv := schemas.Environment{
		ProjectName: "group/project",
		Name:        "production",
	}
	err = c.Store.GetEnvironment(ctx, &storedEnv)
	require.NoError(t, err)
	assert.Equal(t, env.ExternalURL, storedEnv.ExternalURL)
	assert.Equal(t, env.LatestDeployment.JobID, storedEnv.LatestDeployment.JobID)
}

func TestPullEnvironmentsFromProject(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/projects/group/project/environments/7"):
			_, _ = w.Write([]byte(`{
				"id": 7,
				"name": "production",
				"state": "available",
				"external_url": "https://prod.example.com",
				"last_deployment": {
					"ref": "main",
					"created_at": "2024-03-09T16:00:00Z",
					"deployable": {
						"id": 321,
						"duration": 12,
						"status": "success",
						"tag": false
					}
				}
			}`))
		case strings.Contains(r.URL.Path, "/projects/group/project/environments"):
			_, _ = w.Write([]byte(`[{"id":7,"name":"production","state":"available"}]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	c.TaskController = NewTaskController(ctx, nil, 10)
	c.UUID = uuid.New()
	registerNoopPullEnvironmentMetricsTask(t, c)

	p := schemas.NewProject("group/project")
	p.Pull.Environments.Enabled = true
	p.Pull.Environments.Regexp = ".*"

	err := c.PullEnvironmentsFromProject(ctx, p)
	require.NoError(t, err)

	storedEnv := schemas.Environment{
		ProjectName: "group/project",
		Name:        "production",
	}
	err = c.Store.GetEnvironment(ctx, &storedEnv)
	require.NoError(t, err)

	assert.Equal(t, int64(7), storedEnv.ID)
	assert.True(t, storedEnv.Available)
	assert.Equal(t, "https://prod.example.com", storedEnv.ExternalURL)

	queued, err := c.Store.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), queued)
}

func TestPullEnvironmentMetrics(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/projects/group/project/environments/7"):
			_, _ = w.Write([]byte(`{
				"id": 7,
				"name": "production",
				"state": "available",
				"external_url": "https://prod.example.com",
				"last_deployment": {
					"ref": "main",
					"created_at": "2024-03-09T16:00:00Z",
					"deployable": {
						"id": 12,
						"duration": 15,
						"status": "success",
						"tag": false,
						"user": {"username": "alice"},
						"commit": {"short_id": "currsha"}
					}
				}
			}`))
		case strings.Contains(r.URL.Path, "/repository/branches/main"):
			_, _ = w.Write([]byte(`{
				"name": "main",
				"commit": {
					"short_id": "newsha",
					"committed_date": "2024-03-09T16:05:00Z"
				}
			}`))
		case strings.Contains(r.URL.Path, "/repository/compare"):
			assert.Equal(t, "currsha", r.URL.Query().Get("from"))
			assert.Equal(t, "newsha", r.URL.Query().Get("to"))
			_, _ = w.Write([]byte(`{
				"commits": [
					{"id": "1"},
					{"id": "2"}
				]
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	env := schemas.Environment{
		ProjectName:               "group/project",
		ID:                        7,
		Name:                      "production",
		OutputSparseStatusMetrics: false,
		LatestDeployment: schemas.Deployment{
			JobID:         10,
			RefKind:       schemas.RefKindBranch,
			RefName:       "main",
			CommitShortID: "oldsha",
			Timestamp:     float64(time.Unix(1709999900, 0).Unix()),
			Status:        "running",
		},
	}
	require.NoError(t, c.Store.SetEnvironment(ctx, env))

	err := c.PullEnvironmentMetrics(ctx, env)
	require.NoError(t, err)

	defaultLabels := env.DefaultLabelsValues()

	behindCommits := schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentBehindCommitsCount,
		Labels: defaultLabels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &behindCommits))
	assert.Equal(t, float64(2), behindCommits.Value)

	behindDuration := schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentBehindDurationSeconds,
		Labels: defaultLabels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &behindDuration))
	assert.Equal(t, float64(300), behindDuration.Value)

	deploymentCount := schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentDeploymentCount,
		Labels: defaultLabels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &deploymentCount))
	assert.Equal(t, float64(1), deploymentCount.Value)

	deploymentDuration := schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentDeploymentDurationSeconds,
		Labels: defaultLabels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &deploymentDuration))
	assert.Equal(t, float64(15), deploymentDuration.Value)

	deploymentJobID := schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentDeploymentJobID,
		Labels: defaultLabels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &deploymentJobID))
	assert.Equal(t, float64(12), deploymentJobID.Value)

	statusMetric := schemas.Metric{
		Kind: schemas.MetricKindEnvironmentDeploymentStatus,
		Labels: map[string]string{
			"project":     "group/project",
			"environment": "production",
			"status":      "success",
		},
	}
	require.NoError(t, c.Store.GetMetric(ctx, &statusMetric))
	assert.Equal(t, float64(1), statusMetric.Value)

	infoMetric := schemas.Metric{
		Kind: schemas.MetricKindEnvironmentInformation,
		Labels: map[string]string{
			"project":                 "group/project",
			"environment":             "production",
			"environment_id":          "7",
			"external_url":            "https://prod.example.com",
			"kind":                    "branch",
			"ref":                     "main",
			"latest_commit_short_id":  "newsha",
			"current_commit_short_id": "currsha",
			"available":               "true",
			"username":                "alice",
		},
	}
	require.NoError(t, c.Store.GetMetric(ctx, &infoMetric))
	assert.Equal(t, float64(1), infoMetric.Value)
}
