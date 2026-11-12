package controller

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

// schedWaitBackground asserts that every loop started through startBackground
// has returned, which proves the loop observed cancellation.
func schedWaitBackground(t *testing.T, c *Controller) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, c.background.wait(waitCtx), "background loop did not stop after cancellation")
}

func TestSchedTickerSchedulesUntilCancelled(t *testing.T) {
	t.Parallel()

	h := schedNewHarness(t, nil)
	h.c.background = &backgroundTasks{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	h.c.ScheduleTaskWithTicker(ctx, schemas.TaskTypeGarbageCollectMetrics, 1)

	firstNext := h.c.TaskController.TaskSchedulingMonitoring.Snapshot()[schemas.TaskTypeGarbageCollectMetrics].Next
	assert.WithinDuration(t, start.Add(time.Second), firstNext, 250*time.Millisecond)
	assert.Empty(t, h.published(), "the ticker must not schedule before its first tick")

	require.Eventually(t, func() bool {
		return len(h.published()) == 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, []string{string(schemas.TaskTypeGarbageCollectMetrics)}, h.publishedNames())
	assert.True(t, schedAdmissionHolds(h.c, schemas.TaskTypeGarbageCollectMetrics, "_"))
	require.Eventually(t, func() bool {
		next := h.c.TaskController.TaskSchedulingMonitoring.Snapshot()[schemas.TaskTypeGarbageCollectMetrics].Next
		return next.After(firstNext)
	}, time.Second, 10*time.Millisecond, "Next was not advanced after the tick")

	cancel()
	schedWaitBackground(t, h.c)
	assert.Len(t, h.published(), 1)
}

func TestSchedTickerRejectsNonPositiveInterval(t *testing.T) {
	h := schedNewHarness(t, nil)
	h.c.background = &backgroundTasks{}

	h.c.ScheduleTaskWithTicker(context.Background(), schemas.TaskTypePullMetrics, 0)
	h.c.ScheduleTaskWithTicker(context.Background(), schemas.TaskTypePullMetrics, -5)

	_, monitored := h.c.TaskController.TaskSchedulingMonitoring.Snapshot()[schemas.TaskTypePullMetrics]
	assert.False(t, monitored)
	// No loop may have been started: wait returns immediately.
	schedWaitBackground(t, h.c)
}

func TestSchedScheduleRunsOnInitTasksAndStopsLoops(t *testing.T) {
	t.Parallel()

	var metadataCalls atomic.Int32
	h := schedNewHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/metadata" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		metadataCalls.Add(1)
		schedJSON(w, `{"version": "17.0.0"}`)
	})
	h.c.background = &backgroundTasks{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pull := config.Pull{}
	pull.Metrics.OnInit = true
	pull.RefsFromProjects.Scheduled = true
	pull.RefsFromProjects.IntervalSeconds = 3600
	gc := config.GarbageCollect{}
	gc.Projects.OnInit = true

	h.c.Schedule(ctx, pull, gc)

	assert.ElementsMatch(t, []string{
		string(schemas.TaskTypePullMetrics),
		string(schemas.TaskTypeGarbageCollectProjects),
	}, h.publishedNames())
	status := h.c.TaskController.TaskSchedulingMonitoring.Snapshot()
	assert.Contains(t, status, schemas.TaskTypePullRefsFromProjects)
	assert.NotContains(t, status, schemas.TaskTypePullEnvironmentsFromProjects)
	require.Eventually(t, func() bool {
		return h.c.Gitlab.Version().Version == "v17.0.0"
	}, time.Second, 10*time.Millisecond, "GitLab metadata was not refreshed")

	cancel()
	schedWaitBackground(t, h.c)
	assert.Equal(t, int32(1), metadataCalls.Load())
}

func TestSchedRedisKeepaliveLoopTracksRedisHealth(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisStore := store.NewRedisStore(client)
	c := &Controller{
		Redis:      client,
		Store:      redisStore,
		UUID:       uuid.New(),
		background: &backgroundTasks{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.ScheduleRedisSetKeepalive(ctx)

	require.Eventually(t, func() bool {
		exists, err := redisStore.KeepaliveExists(context.Background(), c.UUID.String())
		return err == nil && exists && c.redisReady.Load()
	}, 2*time.Second, 10*time.Millisecond, "keepalive was not written")

	mr.SetError("sched test: redis down")
	require.Eventually(t, func() bool {
		return !c.redisReady.Load()
	}, 2*time.Second, 10*time.Millisecond, "readiness was not degraded on keepalive failure")

	cancel()
	schedWaitBackground(t, c)
}

func TestSchedRedisKeepaliveLoopStopsDuringRetry(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	c := &Controller{
		Redis:      client,
		Store:      store.NewRedisStore(client),
		UUID:       uuid.New(),
		background: &backgroundTasks{},
	}
	c.redisReady.Store(true)
	mr.SetError("sched test: redis down")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.ScheduleRedisSetKeepalive(ctx)

	// The first tick fires after 1s and then backs off between retries;
	// cancel while it is retrying.
	time.Sleep(1050 * time.Millisecond)
	cancel()
	schedWaitBackground(t, c)
	assert.True(t, c.redisReady.Load(), "a cancelled keepalive must not flip readiness")
}
