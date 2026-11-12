package controller

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"go.uber.org/goleak"
)

// TestMain fails the package if any test leaves a goroutine running.
//
// Two library goroutines are ignored because they stop by themselves:
//   - ratecounter's decay goroutine exits once the counted rate falls back to
//     zero, within one interval;
//   - redisq's scheduler loops exit within about 1.5s of Queue.Close, which does
//     not wait for them.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/paulbellamy/ratecounter.(*RateCounter).run.func1"),
		goleak.IgnoreAnyFunction("github.com/vmihailenco/taskq/redisq/v4.(*Queue).scheduler"),
	)
}

// newTestTaskController creates a task controller whose queue consumers are
// stopped when the test ends.
func newTestTaskController(t testing.TB, ctx context.Context, r *redis.Client, maximumJobsQueueSize int) TaskController {
	t.Helper()
	tc := NewTaskController(ctx, r, maximumJobsQueueSize)
	t.Cleanup(func() { _ = tc.Factory.Close() })
	return tc
}
