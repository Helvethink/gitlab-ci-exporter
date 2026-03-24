package store

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func newTestLocalStore(t *testing.T) *Local {
	t.Helper()

	s, ok := NewLocalStore().(*Local)
	require.True(t, ok)

	return s
}

func TestLocalHasExpiredAlwaysFalse(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	assert.False(t, s.HasProjectExpired(ctx, schemas.ProjectKey("p1")))
	assert.False(t, s.HasEnvExpired(ctx, schemas.EnvironmentKey("e1")))
	assert.False(t, s.HasRunnerExpired(ctx, schemas.RunnerKey("r1")))
	assert.False(t, s.HasRefExpired(ctx, schemas.RefKey("ref1")))
	assert.False(t, s.HasMetricExpired(ctx, schemas.MetricKey("m1")))
}

func TestLocalProjectFunctions(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	p := schemas.NewProject("foo/bar")
	p.Topics = "go,ci"

	assert.NoError(t, s.SetProject(ctx, p))

	projects, err := s.Projects(ctx)
	assert.NoError(t, err)
	assert.Contains(t, projects, p.Key())
	assert.Equal(t, p, projects[p.Key()])

	exists, err := s.ProjectExists(ctx, p.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	got := schemas.NewProject("foo/bar")
	assert.NoError(t, s.GetProject(ctx, &got))
	assert.Equal(t, p, got)

	count, err := s.ProjectsCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, s.DelProject(ctx, p.Key()))

	projects, err = s.Projects(ctx)
	assert.NoError(t, err)
	assert.NotContains(t, projects, p.Key())

	exists, err = s.ProjectExists(ctx, p.Key())
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalEnvironmentFunctions(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	env := schemas.Environment{
		ProjectName: "foo/bar",
		ID:          1,
		Name:        "production",
		ExternalURL: "https://example.com",
		Available:   true,
	}

	assert.NoError(t, s.SetEnvironment(ctx, env))

	envs, err := s.Environments(ctx)
	assert.NoError(t, err)
	assert.Contains(t, envs, env.Key())
	assert.Equal(t, env, envs[env.Key()])

	exists, err := s.EnvironmentExists(ctx, env.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	got := schemas.Environment{
		ProjectName: "foo/bar",
		Name:        "production",
	}
	assert.NoError(t, s.GetEnvironment(ctx, &got))
	assert.Equal(t, env, got)

	count, err := s.EnvironmentsCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, s.DelEnvironment(ctx, env.Key()))

	envs, err = s.Environments(ctx)
	assert.NoError(t, err)
	assert.NotContains(t, envs, env.Key())

	exists, err = s.EnvironmentExists(ctx, env.Key())
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalRunnerFunctions(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	runner := schemas.Runner{
		ID:          123,
		Description: "shared-runner",
		Name:        "runner-01",
		ProjectName: "foo/bar",
		Online:      true,
		Status:      "online",
		TagList:     []string{"docker", "linux"},
	}

	assert.NoError(t, s.SetRunner(ctx, runner))

	runners, err := s.Runners(ctx)
	assert.NoError(t, err)
	assert.Contains(t, runners, runner.Key())
	assert.Equal(t, runner, runners[runner.Key()])

	exists, err := s.RunnerExists(ctx, runner.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	got := schemas.Runner{ID: 123}
	assert.NoError(t, s.GetRunner(ctx, &got))
	assert.Equal(t, runner, got)

	count, err := s.RunnersCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, s.DelRunner(ctx, runner.Key()))

	runners, err = s.Runners(ctx)
	assert.NoError(t, err)
	assert.NotContains(t, runners, runner.Key())

	exists, err = s.RunnerExists(ctx, runner.Key())
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalRefFunctions(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	p := schemas.NewProject("foo/bar")
	p.Topics = "topic1"

	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{
		ID:        100,
		Status:    "success",
		Source:    "push",
		Variables: `{"foo":"bar"}`,
	}

	assert.NoError(t, s.SetRef(ctx, ref))

	refs, err := s.Refs(ctx)
	assert.NoError(t, err)
	assert.Contains(t, refs, ref.Key())
	assert.Equal(t, ref, refs[ref.Key()])

	exists, err := s.RefExists(ctx, ref.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	got := schemas.NewRef(p, schemas.RefKindBranch, "main")
	assert.NoError(t, s.GetRef(ctx, &got))
	assert.Equal(t, ref, got)

	count, err := s.RefsCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, s.DelRef(ctx, ref.Key()))

	refs, err = s.Refs(ctx)
	assert.NoError(t, err)
	assert.NotContains(t, refs, ref.Key())

	exists, err = s.RefExists(ctx, ref.Key())
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalMetricFunctions(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	m := schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: prometheus.Labels{
			"project":     "foo/bar",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   `{"foo":"bar"}`,
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 99.9,
	}

	assert.NoError(t, s.SetMetric(ctx, m))

	metrics, err := s.Metrics(ctx)
	assert.NoError(t, err)
	assert.Contains(t, metrics, m.Key())
	assert.Equal(t, m, metrics[m.Key()])

	exists, err := s.MetricExists(ctx, m.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	got := schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: prometheus.Labels{
			"project":     "foo/bar",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   `{"foo":"bar"}`,
			"pipeline_id": "123",
			"status":      "success",
		},
	}
	assert.NoError(t, s.GetMetric(ctx, &got))
	assert.Equal(t, m, got)

	count, err := s.MetricsCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, s.DelMetric(ctx, m.Key()))

	metrics, err = s.Metrics(ctx)
	assert.NoError(t, err)
	assert.NotContains(t, metrics, m.Key())

	exists, err = s.MetricExists(ctx, m.Key())
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalPipelineFunctions(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	pipeline := schemas.Pipeline{
		ID:        77,
		Coverage:  85.5,
		Source:    "push",
		Status:    "success",
		Variables: `{"foo":"bar"}`,
	}

	assert.NoError(t, s.SetPipeline(ctx, pipeline))

	exists, err := s.PipelineExists(ctx, pipeline.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	got := schemas.Pipeline{ID: 77}
	assert.NoError(t, s.GetPipeline(ctx, &got))
	assert.Equal(t, pipeline, got)
}

func TestLocalPipelineVariablesFunctions(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	pipeline := schemas.Pipeline{ID: 88}
	vars := `{"ENV":"prod"}`

	exists, err := s.PipelineVariablesExists(ctx, pipeline)
	assert.NoError(t, err)
	assert.False(t, exists)

	value, err := s.GetPipelineVariables(ctx, pipeline)
	assert.NoError(t, err)
	assert.Equal(t, "", value)

	assert.NoError(t, s.SetPipelineVariables(ctx, pipeline, vars))

	exists, err = s.PipelineVariablesExists(ctx, pipeline)
	assert.NoError(t, err)
	assert.True(t, exists)

	value, err = s.GetPipelineVariables(ctx, pipeline)
	assert.NoError(t, err)
	assert.Equal(t, vars, value)
}

func TestLocalQueueTask(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	ok, err := s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-1", "")
	assert.NoError(t, err)
	assert.True(t, ok)

	ok, err = s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-1", "")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestLocalDequeueTaskAndExecutedCount(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	ok, err := s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-1", "")
	assert.NoError(t, err)
	assert.True(t, ok)

	count, err := s.ExecutedTasksCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), count)

	assert.NoError(t, s.DequeueTask(ctx, schemas.TaskTypePullMetrics, "task-1"))

	count, err = s.ExecutedTasksCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), count)

	queued, err := s.CurrentlyQueuedTasksCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), queued)
}

func TestLocalCurrentlyQueuedTasksCount(t *testing.T) {
	s := newTestLocalStore(t)
	ctx := context.Background()

	_, err := s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-1", "")
	assert.NoError(t, err)
	_, err = s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-2", "")
	assert.NoError(t, err)
	_, err = s.QueueTask(ctx, schemas.TaskTypeGarbageCollectEnvironments, "task-3", "")
	assert.NoError(t, err)

	count, err := s.CurrentlyQueuedTasksCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), count)

	assert.NoError(t, s.DequeueTask(ctx, schemas.TaskTypePullMetrics, "task-1"))

	count, err = s.CurrentlyQueuedTasksCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}
