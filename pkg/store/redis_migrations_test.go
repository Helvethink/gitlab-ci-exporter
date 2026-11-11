package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

const legacySecret = "glpat-SENSITIVEVALUE0001"

func TestRedisPurgeLegacyPipelineVariables(t *testing.T) {
	mr, r := newTestRedisStore(t)

	// Data as written by an earlier version: raw "KEY:VALUE" pairs everywhere.
	legacy := "API_TOKEN:" + legacySecret
	require.NoError(t, mr.Set("unrelated", "kept"))
	mr.HSet(redisLegacyPipelineVariablesKey, "42", legacy)

	pipeline := schemas.Pipeline{ID: 42, Variables: legacy}
	require.NoError(t, r.SetPipeline(testCtx, pipeline))

	ref := schemas.NewRef(schemas.NewProject("foo/bar"), schemas.RefKindBranch, "main")
	ref.LatestPipeline = pipeline
	require.NoError(t, r.SetRef(testCtx, ref))

	cleanRef := schemas.NewRef(schemas.NewProject("foo/baz"), schemas.RefKindBranch, "main")
	require.NoError(t, r.SetRef(testCtx, cleanRef))

	leaky := schemas.Metric{Kind: schemas.MetricKindID, Labels: map[string]string{"project": "foo/bar", "variables": legacy}, Value: 1}
	clean := schemas.Metric{Kind: schemas.MetricKindID, Labels: map[string]string{"project": "foo/baz", "variables": ""}, Value: 2}
	require.NoError(t, r.SetMetric(testCtx, leaky))
	require.NoError(t, r.SetMetric(testCtx, clean))

	require.Contains(t, mr.Dump(), "SENSITIVEVALUE", "fixture must contain the secret before the purge")
	require.NoError(t, r.PurgeLegacyPipelineVariables(testCtx))

	assert.False(t, mr.Exists(redisLegacyPipelineVariablesKey))
	assert.False(t, mr.Exists(redisPipelinesKey))
	assert.True(t, mr.Exists(redisMigrationPipelineVariableKeysOnly))
	got, err := mr.Get("unrelated")
	require.NoError(t, err)
	assert.Equal(t, "kept", got)

	refs, err := r.Refs(testCtx)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	for _, stored := range refs {
		assert.Empty(t, stored.LatestPipeline.Variables)
	}

	metrics, err := r.Metrics(testCtx)
	require.NoError(t, err)
	assert.Len(t, metrics, 1)
	assert.Contains(t, metrics, clean.Key())

	// Nothing in the database may still contain the secret value.
	assert.NotContains(t, mr.Dump(), "SENSITIVEVALUE")

	// Idempotent: once the marker is set, later data is left untouched.
	require.NoError(t, r.SetMetric(testCtx, leaky))
	require.NoError(t, r.PurgeLegacyPipelineVariables(testCtx))
	metrics, err = r.Metrics(testCtx)
	require.NoError(t, err)
	assert.Len(t, metrics, 2)
}

func TestRedisPipelineVariablesUseNewKey(t *testing.T) {
	mr, r := newTestRedisStore(t)

	require.NoError(t, r.SetPipelineVariables(testCtx, schemas.Pipeline{ID: 7}, "DEPLOY_ENV"))
	assert.True(t, mr.Exists(redisPipelineVariablesKey))
	assert.False(t, mr.Exists(redisLegacyPipelineVariablesKey))
}
