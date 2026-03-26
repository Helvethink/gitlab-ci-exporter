package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

func TestDeleteEnv(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	env := schemas.Environment{
		ProjectName: "group/project",
		Name:        "production",
	}
	require.NoError(t, s.SetEnvironment(ctx, env))

	require.NoError(t, deleteEnv(ctx, s, env, "test"))

	exists, err := s.EnvironmentExists(ctx, env.Key())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDeleteRunner(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	runner := schemas.Runner{
		ID:          42,
		Name:        "runner-42",
		ProjectName: "group/project",
	}
	require.NoError(t, s.SetRunner(ctx, runner))

	require.NoError(t, deleteRunner(ctx, s, runner, "test"))

	exists, err := s.RunnerExists(ctx, runner.Key())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDeleteRef(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	ref := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	require.NoError(t, s.SetRef(ctx, ref))

	require.NoError(t, deleteRef(ctx, s, ref, "test"))

	exists, err := s.RefExists(ctx, ref.Key())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDeleteMetric(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	metric := schemas.Metric{
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
		Value: 98.5,
	}
	require.NoError(t, s.SetMetric(ctx, metric))

	require.NoError(t, deleteMetric(ctx, s, metric, "test"))

	exists, err := s.MetricExists(ctx, metric.Key())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestGarbageCollectProjectsDeletesUnconfiguredProjects(t *testing.T) {
	ctx := context.Background()
	c := &Controller{Store: store.NewLocalStore()}

	require.NoError(t, c.Store.SetProject(ctx, schemas.NewProject("group/project")))

	require.NoError(t, c.GarbageCollectProjects(ctx))

	count, err := c.Store.ProjectsCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGarbageCollectEnvironmentsDeletesEnvironmentWithoutProject(t *testing.T) {
	ctx := context.Background()
	c := &Controller{Store: store.NewLocalStore()}

	env := schemas.Environment{
		ProjectName: "group/project",
		Name:        "production",
		ID:          7,
	}
	require.NoError(t, c.Store.SetEnvironment(ctx, env))

	require.NoError(t, c.GarbageCollectEnvironments(ctx))

	count, err := c.Store.EnvironmentsCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGarbageCollectRefsDeletesRefWithoutProject(t *testing.T) {
	ctx := context.Background()
	c := &Controller{Store: store.NewLocalStore()}

	ref := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	require.NoError(t, c.Store.SetRef(ctx, ref))

	require.NoError(t, c.GarbageCollectRefs(ctx))

	count, err := c.Store.RefsCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGarbageCollectRunnersDeletesRunnerWithoutProject(t *testing.T) {
	ctx := context.Background()
	c := &Controller{Store: store.NewLocalStore()}

	runner := schemas.Runner{
		ID:          42,
		Name:        "runner-42",
		ProjectName: "group/project",
	}
	require.NoError(t, c.Store.SetRunner(ctx, runner))

	require.NoError(t, c.GarbageCollectRunners(ctx))

	count, err := c.Store.RunnersCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGarbageCollectMetricsDeletesOrphanAndSparseMetrics(t *testing.T) {
	ctx := context.Background()
	c := &Controller{Store: store.NewLocalStore()}

	refProject := schemas.NewProject("group/project")
	refProject.OutputSparseStatusMetrics = true
	ref := schemas.NewRef(refProject, schemas.RefKindBranch, "main")
	require.NoError(t, c.Store.SetRef(ctx, ref))

	env := schemas.Environment{
		ProjectName:               "group/project",
		Name:                      "production",
		OutputSparseStatusMetrics: true,
	}
	require.NoError(t, c.Store.SetEnvironment(ctx, env))

	runner := schemas.Runner{
		ID:                        42,
		Name:                      "runner-42",
		ProjectName:               "group/project",
		OutputSparseStatusMetrics: true,
	}
	require.NoError(t, c.Store.SetRunner(ctx, runner))

	orphanMetric := schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: map[string]string{
			"project":     "missing/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 1,
	}
	require.NoError(t, c.Store.SetMetric(ctx, orphanMetric))

	sparseRefMetric := schemas.Metric{
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
		Value: 0,
	}
	require.NoError(t, c.Store.SetMetric(ctx, sparseRefMetric))

	sparseEnvMetric := schemas.Metric{
		Kind: schemas.MetricKindEnvironmentDeploymentStatus,
		Labels: map[string]string{
			"project":     "group/project",
			"environment": "production",
			"status":      "failed",
		},
		Value: 0,
	}
	require.NoError(t, c.Store.SetMetric(ctx, sparseEnvMetric))

	sparseRunnerMetric := schemas.Metric{
		Kind: schemas.MetricKindRunner,
		Labels: map[string]string{
			"runner_id":               "42",
			"runner_name":             "runner-42",
			"runner_description":      "shared-runner",
			"is_shared":               "true",
			"runner_type":             "instance_type",
			"online":                  "true",
			"active":                  "true",
			"status":                  "offline",
			"runner_maintenance_note": "",
			"paused":                  "false",
		},
		Value: 0,
	}
	require.NoError(t, c.Store.SetMetric(ctx, sparseRunnerMetric))

	preservedMetric := schemas.Metric{
		Kind: schemas.MetricKindRunnerContactedAtSeconds,
		Labels: map[string]string{
			"runner_id": "42",
		},
		Value: 1710000000,
	}
	require.NoError(t, c.Store.SetMetric(ctx, preservedMetric))

	require.NoError(t, c.GarbageCollectMetrics(ctx))

	exists, err := c.Store.MetricExists(ctx, orphanMetric.Key())
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = c.Store.MetricExists(ctx, sparseRefMetric.Key())
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = c.Store.MetricExists(ctx, sparseEnvMetric.Key())
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = c.Store.MetricExists(ctx, sparseRunnerMetric.Key())
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = c.Store.MetricExists(ctx, preservedMetric.Key())
	require.NoError(t, err)
	assert.True(t, exists)
}
