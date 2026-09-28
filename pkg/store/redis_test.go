package store

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

var testCtx = context.Background()

func newTestRedisStore(t *testing.T) (mr *miniredis.Miniredis, r *Redis) {
	mr, err := miniredis.Run()
	if err != nil {
		panic(err)
	}

	t.Cleanup(func() {
		mr.Close()
	})

	return mr, NewRedisStore(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
}

func TestRedisProjectFunctions(t *testing.T) {
	_, r := newTestRedisStore(t)

	p := schemas.NewProject("foo/bar")
	p.OutputSparseStatusMetrics = false

	assert.NoError(t, r.SetProject(testCtx, p))

	projects, err := r.Projects(testCtx)
	assert.NoError(t, err)
	assert.Contains(t, projects, p.Key())
	assert.Equal(t, p, projects[p.Key()])

	exists, err := r.ProjectExists(testCtx, p.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	newProject := schemas.NewProject("foo/bar")
	assert.NoError(t, r.GetProject(testCtx, &newProject))
	assert.Equal(t, p, newProject)

	count, err := r.ProjectsCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, r.DelProject(testCtx, p.Key()))

	projects, err = r.Projects(testCtx)
	assert.NoError(t, err)
	assert.NotContains(t, projects, p.Key())

	exists, err = r.ProjectExists(testCtx, p.Key())
	assert.NoError(t, err)
	assert.False(t, exists)

	newProject = schemas.NewProject("foo/bar")
	assert.NoError(t, r.GetProject(testCtx, &newProject))
	assert.NotEqual(t, p, newProject)
}

func TestRedisEnvironmentFunctions(t *testing.T) {
	_, r := newTestRedisStore(t)

	environment := schemas.Environment{
		ProjectName: "foo",
		ID:          1,
		ExternalURL: "bar",
	}

	assert.NoError(t, r.SetEnvironment(testCtx, environment))

	environments, err := r.Environments(testCtx)
	assert.NoError(t, err)
	assert.Contains(t, environments, environment.Key())
	assert.Equal(t, environment.ProjectName, environments[environment.Key()].ProjectName)
	assert.Equal(t, environment.ID, environments[environment.Key()].ID)

	exists, err := r.EnvironmentExists(testCtx, environment.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	newEnvironment := schemas.Environment{
		ProjectName: "foo",
		ID:          1,
	}
	assert.NoError(t, r.GetEnvironment(testCtx, &newEnvironment))
	assert.Equal(t, environment.ExternalURL, newEnvironment.ExternalURL)

	count, err := r.EnvironmentsCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, r.DelEnvironment(testCtx, environment.Key()))

	environments, err = r.Environments(testCtx)
	assert.NoError(t, err)
	assert.NotContains(t, environments, environment.Key())

	exists, err = r.EnvironmentExists(testCtx, environment.Key())
	assert.NoError(t, err)
	assert.False(t, exists)

	newEnvironment = schemas.Environment{
		ProjectName: "foo",
		ID:          1,
	}
	assert.NoError(t, r.GetEnvironment(testCtx, &newEnvironment))
	assert.NotEqual(t, environment, newEnvironment)
}

func TestRedisRefFunctions(t *testing.T) {
	_, r := newTestRedisStore(t)

	p := schemas.NewProject("foo/bar")
	p.Topics = "salty"
	ref := schemas.NewRef(p, schemas.RefKindBranch, "sweet")

	assert.NoError(t, r.SetRef(testCtx, ref))

	projectsRefs, err := r.Refs(testCtx)
	assert.NoError(t, err)
	assert.Contains(t, projectsRefs, ref.Key())
	assert.Equal(t, ref, projectsRefs[ref.Key()])

	exists, err := r.RefExists(testCtx, ref.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	newRef := schemas.Ref{
		Project: schemas.NewProject("foo/bar"),
		Kind:    schemas.RefKindBranch,
		Name:    "sweet",
	}
	assert.NoError(t, r.GetRef(testCtx, &newRef))
	assert.Equal(t, ref, newRef)

	count, err := r.RefsCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, r.DelRef(testCtx, ref.Key()))

	projectsRefs, err = r.Refs(testCtx)
	assert.NoError(t, err)
	assert.NotContains(t, projectsRefs, ref.Key())

	exists, err = r.RefExists(testCtx, ref.Key())
	assert.NoError(t, err)
	assert.False(t, exists)

	newRef = schemas.Ref{
		Kind:    schemas.RefKindBranch,
		Project: schemas.NewProject("foo/bar"),
		Name:    "sweet",
	}
	assert.NoError(t, r.GetRef(testCtx, &newRef))
	assert.NotEqual(t, ref, newRef)
}

func TestRedisMetricFunctions(t *testing.T) {
	_, r := newTestRedisStore(t)

	m := schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: prometheus.Labels{
			"foo": "bar",
		},
		Value: 5,
	}

	assert.NoError(t, r.SetMetric(testCtx, m))

	metrics, err := r.Metrics(testCtx)
	assert.NoError(t, err)
	assert.Contains(t, metrics, m.Key())
	assert.Equal(t, m, metrics[m.Key()])

	exists, err := r.MetricExists(testCtx, m.Key())
	assert.NoError(t, err)
	assert.True(t, exists)

	newMetric := schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: prometheus.Labels{
			"foo": "bar",
		},
	}
	assert.NoError(t, r.GetMetric(testCtx, &newMetric))
	assert.Equal(t, m, newMetric)

	count, err := r.MetricsCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.NoError(t, r.DelMetric(testCtx, m.Key()))

	metrics, err = r.Metrics(testCtx)
	assert.NoError(t, err)
	assert.NotContains(t, metrics, m.Key())

	exists, err = r.MetricExists(testCtx, m.Key())
	assert.NoError(t, err)
	assert.False(t, exists)

	newMetric = schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: prometheus.Labels{
			"foo": "bar",
		},
	}
	assert.NoError(t, r.GetMetric(testCtx, &newMetric))
	assert.NotEqual(t, m, newMetric)
}

func TestRedisKeepalive(t *testing.T) {
	mr, r := newTestRedisStore(t)

	uuidString := uuid.New().String()

	resp, err := r.SetKeepalive(testCtx, uuidString, time.Second)
	assert.Equal(t, "OK", resp)
	assert.NoError(t, err)

	exists, err := r.KeepaliveExists(testCtx, uuidString)
	assert.True(t, exists)
	assert.NoError(t, err)

	mr.FastForward(2 * time.Second)

	exists, err = r.KeepaliveExists(testCtx, uuidString)
	assert.False(t, exists)
	assert.NoError(t, err)
}

func TestGetRedisQueueKey(t *testing.T) {
	assert.Equal(t, "task:GarbageCollectEnvironments:foo", getRedisQueueKey(schemas.TaskTypeGarbageCollectEnvironments, "foo"))
}

func TestRedisQueueTask(t *testing.T) {
	mr, r := newTestRedisStore(t)

	_, err := r.SetKeepalive(testCtx, "controller1", time.Second)
	assert.NoError(t, err)

	ok, err := r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "foo", "controller1")
	assert.True(t, ok)
	assert.NoError(t, err)

	ok, err = r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "foo", "controller2")
	assert.False(t, ok)
	assert.NoError(t, err)

	mr.FastForward(2 * time.Second)

	ok, err = r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "foo", "controller2")
	assert.True(t, ok)
	assert.NoError(t, err)
}

func TestRedisDequeueTask(t *testing.T) {
	_, r := newTestRedisStore(t)

	_, err := r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "foo", "")
	assert.NoError(t, err)

	count, err := r.ExecutedTasksCount(testCtx)
	assert.Error(t, err)
	assert.Equal(t, uint64(0), count)

	assert.NoError(t, r.DequeueTask(testCtx, schemas.TaskTypePullMetrics, "foo", ""))

	count, err = r.ExecutedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestRedisCurrentlyQueuedTasksCount(t *testing.T) {
	_, r := newTestRedisStore(t)

	_, err := r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "foo", "")
	assert.NoError(t, err)
	_, err = r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "bar", "")
	assert.NoError(t, err)
	_, err = r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "baz", "")
	assert.NoError(t, err)

	count, err := r.CurrentlyQueuedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), count)

	assert.NoError(t, r.DequeueTask(testCtx, schemas.TaskTypePullMetrics, "foo", ""))

	count, err = r.CurrentlyQueuedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

func TestRedisExecutedTasksCount(t *testing.T) {
	_, r := newTestRedisStore(t)

	_, err := r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "foo", "")
	assert.NoError(t, err)
	_, err = r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "bar", "")
	assert.NoError(t, err)

	assert.NoError(t, r.DequeueTask(testCtx, schemas.TaskTypePullMetrics, "foo", ""))
	assert.NoError(t, r.DequeueTask(testCtx, schemas.TaskTypePullMetrics, "foo", ""))

	count, err := r.ExecutedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestRedisCancelTaskOnlyRemovesOwnedReservation(t *testing.T) {
	_, r := newTestRedisStore(t)
	queued, err := r.QueueTask(testCtx, schemas.TaskTypePullMetrics, "foo", "owner-a")
	assert.NoError(t, err)
	assert.True(t, queued)
	assert.NoError(t, r.CancelTask(testCtx, schemas.TaskTypePullMetrics, "foo", "owner-b"))
	count, err := r.CurrentlyQueuedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), count)
	assert.NoError(t, r.CancelTask(testCtx, schemas.TaskTypePullMetrics, "foo", "owner-a"))
	count, err = r.CurrentlyQueuedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Zero(t, count)
	executed, err := r.ExecutedTasksCount(testCtx)
	assert.ErrorIs(t, err, redis.Nil)
	assert.Zero(t, executed)
}

func TestRedisQueueTaskConcurrentTakeover(t *testing.T) {
	mr, r := newTestRedisStore(t)
	const task = schemas.TaskTypePullMetrics
	queued, err := r.QueueTask(testCtx, task, "same", "dead")
	assert.NoError(t, err)
	assert.True(t, queued)
	mr.FastForward(time.Second)
	const contenders = 24
	for i := 0; i < contenders; i++ {
		_, err := r.SetKeepalive(testCtx, fmt.Sprintf("owner-%d", i), time.Minute)
		assert.NoError(t, err)
	}
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := r.QueueTask(testCtx, task, "same", fmt.Sprintf("owner-%d", i))
			if err != nil {
				t.Errorf("QueueTask: %v", err)
			}
			if ok {
				won.Add(1)
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, int32(1), won.Load())
}

func TestRedisDelayedOwnerCannotCompleteReplacement(t *testing.T) {
	_, r := newTestRedisStore(t)
	const task = schemas.TaskTypePullMetrics
	ok, err := r.QueueTask(testCtx, task, "same", "old")
	assert.NoError(t, err)
	assert.True(t, ok)
	ok, err = r.QueueTask(testCtx, task, "same", "new")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NoError(t, r.DequeueTask(testCtx, task, "same", "old"))
	assert.NoError(t, r.CancelTask(testCtx, task, "same", "old"))
	count, err := r.CurrentlyQueuedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), count)
	assert.NoError(t, r.DequeueTask(testCtx, task, "same", "new"))
	count, err = r.CurrentlyQueuedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Zero(t, count)
	executed, err := r.ExecutedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), executed)
}

func TestRedisSharedCapacityAfterCrossReplicaCompletion(t *testing.T) {
	_, r := newTestRedisStore(t)
	const task = schemas.TaskTypePullMetrics
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("task-%d", i)
		ok, err := r.QueueTaskLimited(testCtx, task, id, "publisher", 1)
		assert.NoError(t, err)
		assert.True(t, ok)
		blocked, err := r.QueueTaskLimited(testCtx, task, "overflow", "other", 1)
		assert.NoError(t, err)
		assert.False(t, blocked)
		// Any replica can complete the publisher's job using its carried owner.
		assert.NoError(t, r.DequeueTask(testCtx, task, id, "publisher"))
	}
}

func TestRedisSameProcessOldDeliveryCannotDeleteNewReservation(t *testing.T) {
	mr, r := newTestRedisStore(t)
	const task = schemas.TaskTypePullMetrics
	_, err := r.SetKeepalive(testCtx, "process", time.Second)
	assert.NoError(t, err)
	ok, err := r.QueueTask(testCtx, task, "same", "process|old")
	assert.NoError(t, err)
	assert.True(t, ok)
	ok, err = r.QueueTask(testCtx, task, "same", "process|new")
	assert.NoError(t, err)
	assert.False(t, ok)
	mr.FastForward(2 * time.Second)
	ok, err = r.QueueTask(testCtx, task, "same", "process|new")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NoError(t, r.DequeueTask(testCtx, task, "same", "process|old"))
	count, err := r.CurrentlyQueuedTasksCount(testCtx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestRedisGetPipelineVariables(t *testing.T) {
	pipeline := schemas.Pipeline{ID: 88}

	t.Run("stored value", func(t *testing.T) {
		_, r := newTestRedisStore(t)
		variables := `{"ENV":"prod"}`
		require.NoError(t, r.SetPipelineVariables(testCtx, pipeline, variables))
		got, err := r.GetPipelineVariables(testCtx, pipeline)
		require.NoError(t, err)
		assert.Equal(t, variables, got)
	})

	t.Run("missing value", func(t *testing.T) {
		_, r := newTestRedisStore(t)
		got, err := r.GetPipelineVariables(testCtx, pipeline)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("corrupt value", func(t *testing.T) {
		_, r := newTestRedisStore(t)
		require.NoError(t, r.HSet(testCtx, redisPipelineVariablesKey, "88", []byte{0xc1}).Err())
		got, err := r.GetPipelineVariables(testCtx, pipeline)
		require.ErrorContains(t, err, "decode pipeline variables")
		assert.Empty(t, got)
	})

	t.Run("Redis error", func(t *testing.T) {
		mr, r := newTestRedisStore(t)
		mr.SetError("redis unavailable")
		got, err := r.GetPipelineVariables(testCtx, pipeline)
		require.ErrorContains(t, err, "redis unavailable")
		assert.Empty(t, got)
	})
}
