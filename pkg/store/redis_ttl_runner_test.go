package store

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// rsTTLConfig uses a distinct duration per entity so a test can tell which
// configured TTL was applied to which key.
func rsTTLConfig() *RedisTTLConfig {
	return &RedisTTLConfig{
		Project:     1 * time.Minute,
		Environment: 2 * time.Minute,
		Runner:      3 * time.Minute,
		Refs:        4 * time.Minute,
		Metrics:     5 * time.Minute,
	}
}

// rsNewTTLStore returns a miniredis-backed store configured with rsTTLConfig.
func rsNewTTLStore(t *testing.T) (*miniredis.Miniredis, *Redis) {
	t.Helper()
	mr, r := newTestRedisStore(t)
	WithTTLConfig(rsTTLConfig())(r.StoreConfig)
	return mr, r
}

// rsEntity describes one entity kind whose TTL tracking is exercised the same way.
type rsEntity struct {
	name    string
	ttlKey  string
	ttl     time.Duration
	set     func(r *Redis) error
	expired func(r *Redis) bool
}

func rsEntities() []rsEntity {
	cfg := rsTTLConfig()

	project := schemas.NewProject("group/project")
	env := schemas.Environment{ProjectName: "group/project", Name: "production", ID: 7}
	runner := schemas.Runner{ID: 42, Description: "shared runner"}
	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")
	metric := schemas.Metric{
		Kind:   schemas.MetricKindCoverage,
		Labels: prometheus.Labels{"project": "group/project"},
		Value:  1,
	}

	return []rsEntity{
		{
			name:    "project",
			ttlKey:  "projects:" + string(project.Key()),
			ttl:     cfg.Project,
			set:     func(r *Redis) error { return r.SetProject(testCtx, project) },
			expired: func(r *Redis) bool { return r.HasProjectExpired(testCtx, project.Key()) },
		},
		{
			name:    "environment",
			ttlKey:  "environments:" + string(env.Key()),
			ttl:     cfg.Environment,
			set:     func(r *Redis) error { return r.SetEnvironment(testCtx, env) },
			expired: func(r *Redis) bool { return r.HasEnvExpired(testCtx, env.Key()) },
		},
		{
			name:    "runner",
			ttlKey:  "runner:" + string(runner.Key()),
			ttl:     cfg.Runner,
			set:     func(r *Redis) error { return r.SetRunner(testCtx, runner) },
			expired: func(r *Redis) bool { return r.HasRunnerExpired(testCtx, runner.Key()) },
		},
		{
			name:    "ref",
			ttlKey:  "refs:" + string(ref.Key()),
			ttl:     cfg.Refs,
			set:     func(r *Redis) error { return r.SetRef(testCtx, ref) },
			expired: func(r *Redis) bool { return r.HasRefExpired(testCtx, ref.Key()) },
		},
		{
			name:    "metric",
			ttlKey:  "metrics:" + string(metric.Key()),
			ttl:     cfg.Metrics,
			set:     func(r *Redis) error { return r.SetMetric(testCtx, metric) },
			expired: func(r *Redis) bool { return r.HasMetricExpired(testCtx, metric.Key()) },
		},
	}
}

func TestRedisGetTTLKeys(t *testing.T) {
	assert.Equal(t, "projects:p", getTTLProjectKey("p"))
	assert.Equal(t, "environments:e", getTTLEnvKey("e"))
	assert.Equal(t, "runner:r", getTTLRunnerKey("r"))
	assert.Equal(t, "refs:f", getTTLRefKey("f"))
	assert.Equal(t, "metrics:m", getTTLMetricKey("m"))
}

// TestRedisSetWritesTTLKeyWithConfiguredTTL asserts that each Set* call writes
// its TTL marker key with the duration configured for that entity kind.
func TestRedisSetWritesTTLKeyWithConfiguredTTL(t *testing.T) {
	for _, e := range rsEntities() {
		t.Run(e.name, func(t *testing.T) {
			mr, r := rsNewTTLStore(t)
			require.NoError(t, e.set(r))
			require.True(t, mr.Exists(e.ttlKey), "TTL key %q missing", e.ttlKey)
			assert.Equal(t, e.ttl, mr.TTL(e.ttlKey))
		})
	}
}

// TestRedisSetWithoutTTLConfigWritesNoTTLKey asserts that no marker key is
// written when the store has no TTL configuration.
func TestRedisSetWithoutTTLConfigWritesNoTTLKey(t *testing.T) {
	for _, e := range rsEntities() {
		t.Run(e.name, func(t *testing.T) {
			mr, r := newTestRedisStore(t)
			require.NoError(t, e.set(r))
			assert.False(t, mr.Exists(e.ttlKey))
			assert.False(t, e.expired(r))
		})
	}
}

// TestRedisHasExpiredTracksTTLKeyLifetime pins the current behavior: the
// Has*Expired methods report true while the TTL marker key is alive and false
// once miniredis has expired it. See TestRedisHasExpiredSemantics for why this
// is inverted relative to the method names.
// TestRedisHasExpiredTracksTTLKeyLifetime is a regression test: Has*Expired used
// to return true while the TTL key was still alive, so garbage collection deleted
// fresh refs and metrics and kept the expired ones.
func TestRedisHasExpiredTracksTTLKeyLifetime(t *testing.T) {
	for _, e := range rsEntities() {
		t.Run(e.name, func(t *testing.T) {
			mr, r := rsNewTTLStore(t)
			assert.True(t, e.expired(r), "an entry without a TTL key is expired")

			require.NoError(t, e.set(r))
			assert.False(t, e.expired(r), "fresh entry must not be expired")

			mr.FastForward(e.ttl - time.Second)
			assert.False(t, e.expired(r), "entry just before its TTL must not be expired")

			mr.FastForward(2 * time.Second)
			assert.False(t, mr.Exists(e.ttlKey))
			assert.True(t, e.expired(r), "entry past its TTL must be expired")

			require.NoError(t, e.set(r))
			assert.False(t, e.expired(r), "writing the entry again renews its TTL")
		})
	}
}

// TestRedisSetRefreshesTTL asserts that re-setting an entity resets its TTL.
func TestRedisSetRefreshesTTL(t *testing.T) {
	mr, r := rsNewTTLStore(t)
	ru := schemas.Runner{ID: 1}
	key := getTTLRunnerKey(ru.Key())

	require.NoError(t, r.SetRunner(testCtx, ru))
	mr.FastForward(2 * time.Minute)
	assert.Equal(t, time.Minute, mr.TTL(key))

	require.NoError(t, r.SetRunner(testCtx, ru))
	assert.Equal(t, rsTTLConfig().Runner, mr.TTL(key))
}

func rsRunner(id int64) schemas.Runner {
	return schemas.Runner{
		ID:          id,
		Description: "runner description",
		Name:        "runner-name",
		Online:      true,
		Status:      "online",
		RunnerType:  "instance_type",
		TagList:     []string{"docker", "linux"},
		ProjectName: "group/project",
	}
}

// TestRedisRunnerFunctions covers the full runner CRUD round-trip.
func TestRedisRunnerFunctions(t *testing.T) {
	_, r := newTestRedisStore(t)
	ru := rsRunner(42)

	count, err := r.RunnersCount(testCtx)
	require.NoError(t, err)
	assert.Zero(t, count)

	require.NoError(t, r.SetRunner(testCtx, ru))

	exists, err := r.RunnerExists(testCtx, ru.Key())
	require.NoError(t, err)
	assert.True(t, exists)

	got := schemas.Runner{ID: 42}
	require.NoError(t, r.GetRunner(testCtx, &got))
	assert.Equal(t, ru, got)

	runners, err := r.Runners(testCtx)
	require.NoError(t, err)
	assert.Equal(t, schemas.Runners{ru.Key(): ru}, runners)

	count, err = r.RunnersCount(testCtx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	require.NoError(t, r.DelRunner(testCtx, ru.Key()))

	exists, err = r.RunnerExists(testCtx, ru.Key())
	require.NoError(t, err)
	assert.False(t, exists)

	runners, err = r.Runners(testCtx)
	require.NoError(t, err)
	assert.Empty(t, runners)

	count, err = r.RunnersCount(testCtx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

// TestRedisRunnersMultiple asserts counts and keys with several runners and
// that overwriting an existing runner does not add an entry.
func TestRedisRunnersMultiple(t *testing.T) {
	_, r := newTestRedisStore(t)
	for _, id := range []int64{1, 2, 3} {
		require.NoError(t, r.SetRunner(testCtx, rsRunner(id)))
	}
	updated := rsRunner(2)
	updated.Status = "offline"
	require.NoError(t, r.SetRunner(testCtx, updated))

	count, err := r.RunnersCount(testCtx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	runners, err := r.Runners(testCtx)
	require.NoError(t, err)
	require.Len(t, runners, 3)
	assert.Equal(t, "offline", runners[updated.Key()].Status)
	assert.Equal(t, "online", runners[rsRunner(1).Key()].Status)
}

// TestRedisGetRunnerMissing asserts that a missing runner is not an error and
// leaves the target untouched.
func TestRedisGetRunnerMissing(t *testing.T) {
	_, r := newTestRedisStore(t)
	got := schemas.Runner{ID: 99, Description: "keep"}
	require.NoError(t, r.GetRunner(testCtx, &got))
	assert.Equal(t, schemas.Runner{ID: 99, Description: "keep"}, got)

	exists, err := r.RunnerExists(testCtx, got.Key())
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, r.DelRunner(testCtx, got.Key()), "deleting a missing runner is a no-op")
}

// TestRedisPipelineCache covers GetPipeline/PipelineExists round-trip and
// missing keys.
func TestRedisPipelineCache(t *testing.T) {
	_, r := newTestRedisStore(t)
	p := schemas.Pipeline{
		ID:              1234,
		Coverage:        87.5,
		Timestamp:       1710000000,
		DurationSeconds: 60,
		Source:          "push",
		Status:          "success",
	}

	exists, err := r.PipelineExists(testCtx, p.Key())
	require.NoError(t, err)
	assert.False(t, exists)

	missing := schemas.Pipeline{ID: 1234}
	require.NoError(t, r.GetPipeline(testCtx, &missing))
	assert.Equal(t, schemas.Pipeline{ID: 1234}, missing)

	require.NoError(t, r.SetPipeline(testCtx, p))

	exists, err = r.PipelineExists(testCtx, p.Key())
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = r.PipelineExists(testCtx, schemas.PipelineKey(4321))
	require.NoError(t, err)
	assert.False(t, exists)

	got := schemas.Pipeline{ID: 1234}
	require.NoError(t, r.GetPipeline(testCtx, &got))
	assert.Equal(t, p, got)
}

// TestRedisPipelineVariablesExists asserts existence tracks SetPipelineVariables
// per pipeline ID.
func TestRedisPipelineVariablesExists(t *testing.T) {
	_, r := newTestRedisStore(t)
	p := schemas.Pipeline{ID: 55}

	exists, err := r.PipelineVariablesExists(testCtx, p)
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, r.SetPipelineVariables(testCtx, p, `{"K":"v"}`))

	exists, err = r.PipelineVariablesExists(testCtx, p)
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = r.PipelineVariablesExists(testCtx, schemas.Pipeline{ID: 56})
	require.NoError(t, err)
	assert.False(t, exists)
}
