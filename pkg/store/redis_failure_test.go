package store

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// rsCorruptMsgpack is a reserved MessagePack byte that never decodes.
var rsCorruptMsgpack = []byte{0xc1}

// rsClosedRedisStore returns a TTL-configured store whose server has been shut
// down. Retries are disabled so every call fails fast.
func rsClosedRedisStore(t *testing.T) *Redis {
	t.Helper()
	mr, _ := newTestRedisStore(t)
	addr := mr.Addr()
	mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr:               addr,
		MaxRetries:         -1,
		DialTimeout:        200 * time.Millisecond,
		DialerRetries:      1,
		DialerRetryTimeout: time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisStore(client, WithTTLConfig(rsTTLConfig()))
}

// rsErrorCalls lists every error-returning store method touched by this suite.
func rsErrorCalls() map[string]func(r *Redis) error {
	project := schemas.NewProject("group/project")
	env := schemas.Environment{ProjectName: "group/project", Name: "production"}
	runner := schemas.Runner{ID: 42}
	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")
	metric := schemas.Metric{Kind: schemas.MetricKindCoverage, Labels: prometheus.Labels{"project": "group/project"}}
	pipeline := schemas.Pipeline{ID: 1}

	return map[string]func(r *Redis) error{
		"SetRunner":     func(r *Redis) error { return r.SetRunner(testCtx, runner) },
		"GetRunner":     func(r *Redis) error { ru := runner; return r.GetRunner(testCtx, &ru) },
		"DelRunner":     func(r *Redis) error { return r.DelRunner(testCtx, runner.Key()) },
		"RunnerExists":  func(r *Redis) error { _, err := r.RunnerExists(testCtx, runner.Key()); return err },
		"Runners":       func(r *Redis) error { _, err := r.Runners(testCtx); return err },
		"RunnersCount":  func(r *Redis) error { _, err := r.RunnersCount(testCtx); return err },
		"SetProject":    func(r *Redis) error { return r.SetProject(testCtx, project) },
		"GetProject":    func(r *Redis) error { p := project; return r.GetProject(testCtx, &p) },
		"Projects":      func(r *Redis) error { _, err := r.Projects(testCtx); return err },
		"ProjectsCount": func(r *Redis) error { _, err := r.ProjectsCount(testCtx); return err },
		"SetEnvironment": func(r *Redis) error {
			return r.SetEnvironment(testCtx, env)
		},
		"GetEnvironment": func(r *Redis) error { e := env; return r.GetEnvironment(testCtx, &e) },
		"Environments":   func(r *Redis) error { _, err := r.Environments(testCtx); return err },
		"SetRef":         func(r *Redis) error { return r.SetRef(testCtx, ref) },
		"GetRef":         func(r *Redis) error { rf := ref; return r.GetRef(testCtx, &rf) },
		"Refs":           func(r *Redis) error { _, err := r.Refs(testCtx); return err },
		"SetMetric":      func(r *Redis) error { return r.SetMetric(testCtx, metric) },
		"GetMetric":      func(r *Redis) error { m := metric; return r.GetMetric(testCtx, &m) },
		"Metrics":        func(r *Redis) error { _, err := r.Metrics(testCtx); return err },
		"SetPipeline":    func(r *Redis) error { return r.SetPipeline(testCtx, pipeline) },
		"GetPipeline":    func(r *Redis) error { p := pipeline; return r.GetPipeline(testCtx, &p) },
		"PipelineExists": func(r *Redis) error { _, err := r.PipelineExists(testCtx, pipeline.Key()); return err },
		"PipelineVariablesExists": func(r *Redis) error {
			_, err := r.PipelineVariablesExists(testCtx, pipeline)
			return err
		},
		"SetPipelineVariables": func(r *Redis) error { return r.SetPipelineVariables(testCtx, pipeline, "{}") },
		"GetPipelineVariables": func(r *Redis) error { _, err := r.GetPipelineVariables(testCtx, pipeline); return err },
	}
}

// TestRedisUnavailableReturnsErrors asserts that every error-returning method
// surfaces a connection failure instead of panicking or reporting success.
func TestRedisUnavailableReturnsErrors(t *testing.T) {
	r := rsClosedRedisStore(t)
	for name, call := range rsErrorCalls() {
		t.Run(name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() { err = call(r) })
			assert.Error(t, err)
		})
	}
}

// TestRedisServerErrorReturnsErrors asserts that Redis error replies are
// propagated unchanged by every error-returning method.
func TestRedisServerErrorReturnsErrors(t *testing.T) {
	mr, r := rsNewTTLStore(t)
	// A plain ERR reply is not retried by go-redis, keeping this test fast.
	mr.SetError("ERR simulated outage")
	for name, call := range rsErrorCalls() {
		t.Run(name, func(t *testing.T) {
			assert.ErrorContains(t, call(r), "simulated outage")
		})
	}
}

// TestRedisUnavailableHasExpiredFailsSafe asserts the bool-only Has*Expired
// methods do not panic and report false when Redis cannot be reached; they have
// no error return, so false is the only failure signal available.
func TestRedisUnavailableHasExpiredFailsSafe(t *testing.T) {
	r := rsClosedRedisStore(t)
	for _, e := range rsEntities() {
		t.Run(e.name, func(t *testing.T) {
			var got bool
			require.NotPanics(t, func() { got = e.expired(r) })
			assert.False(t, got)
		})
	}
}

// TestRedisCorruptValuesReturnDecodeErrors asserts that undecodable msgpack
// payloads stored in each hash are reported by the Get* and list methods.
func TestRedisCorruptValuesReturnDecodeErrors(t *testing.T) {
	runner := schemas.Runner{ID: 42}
	pipeline := schemas.Pipeline{ID: 9}
	project := schemas.NewProject("group/project")
	env := schemas.Environment{ProjectName: "group/project", Name: "production"}
	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")
	metric := schemas.Metric{Kind: schemas.MetricKindCoverage, Labels: prometheus.Labels{"project": "group/project"}}

	cases := []struct {
		name  string
		hash  string
		field string
		call  func(r *Redis) error
	}{
		{"GetRunner", redisRunnerKey, string(runner.Key()), func(r *Redis) error { ru := runner; return r.GetRunner(testCtx, &ru) }},
		{"Runners", redisRunnerKey, string(runner.Key()), func(r *Redis) error { _, err := r.Runners(testCtx); return err }},
		{"GetPipeline", redisPipelinesKey, "9", func(r *Redis) error { p := pipeline; return r.GetPipeline(testCtx, &p) }},
		{"GetProject", redisProjectsKey, string(project.Key()), func(r *Redis) error { p := project; return r.GetProject(testCtx, &p) }},
		{"Projects", redisProjectsKey, string(project.Key()), func(r *Redis) error { _, err := r.Projects(testCtx); return err }},
		{"GetEnvironment", redisEnvironmentsKey, string(env.Key()), func(r *Redis) error { e := env; return r.GetEnvironment(testCtx, &e) }},
		{"Environments", redisEnvironmentsKey, string(env.Key()), func(r *Redis) error { _, err := r.Environments(testCtx); return err }},
		{"GetRef", redisRefsKey, string(ref.Key()), func(r *Redis) error { rf := ref; return r.GetRef(testCtx, &rf) }},
		{"Refs", redisRefsKey, string(ref.Key()), func(r *Redis) error { _, err := r.Refs(testCtx); return err }},
		{"GetMetric", redisMetricsKey, string(metric.Key()), func(r *Redis) error { m := metric; return r.GetMetric(testCtx, &m) }},
		{"Metrics", redisMetricsKey, string(metric.Key()), func(r *Redis) error { _, err := r.Metrics(testCtx); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, r := newTestRedisStore(t)
			require.NoError(t, r.HSet(testCtx, tc.hash, tc.field, rsCorruptMsgpack).Err())
			err := tc.call(r)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "msgpack")
		})
	}
}

// TestRedisRunnersCorruptEntryDoesNotHideValidOnes asserts that a single corrupt
// runner makes Runners fail rather than silently returning a partial map as
// success.
func TestRedisRunnersCorruptEntryDoesNotHideValidOnes(t *testing.T) {
	_, r := newTestRedisStore(t)
	require.NoError(t, r.SetRunner(testCtx, rsRunner(1)))
	require.NoError(t, r.HSet(testCtx, redisRunnerKey, "corrupt", rsCorruptMsgpack).Err())

	_, err := r.Runners(testCtx)
	require.Error(t, err)

	count, err := r.RunnersCount(testCtx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}
