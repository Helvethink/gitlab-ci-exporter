package controller

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/taskq/v4"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

type publicationQueue struct {
	taskq.Queue
	addJob func(context.Context, *taskq.Job) error
}

func (q publicationQueue) AddJob(ctx context.Context, job *taskq.Job) error {
	return q.addJob(ctx, job)
}

type cancelingStore struct {
	store.Store
	cancel context.CancelFunc
}

func (s cancelingStore) QueueTask(ctx context.Context, task schemas.TaskType, id, owner string) (bool, error) {
	queued, err := s.Store.QueueTask(ctx, task, id, owner)
	s.cancel()
	return queued, err
}

func TestScheduleTaskQueuesTaskOnlyOnce(t *testing.T) {
	ctx := context.Background()
	var handled atomic.Int32

	c := &Controller{
		Store:          store.NewLocalStore(),
		TaskController: NewTaskController(ctx, nil, 10),
		UUID:           uuid.New(),
	}
	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error {
			handled.Add(1)

			return nil
		},
	})
	require.NoError(t, err)

	c.ScheduleTask(ctx, schemas.TaskTypePullProject, "group/project", "group/project")
	c.ScheduleTask(ctx, schemas.TaskTypePullProject, "group/project", "group/project")

	require.Eventually(t, func() bool {
		return handled.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)

	queued, err := c.Store.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), queued)
}

func TestScheduleTaskRollsBackFailedPublication(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	c := &Controller{Store: s, TaskController: NewTaskController(ctx, nil, 1), UUID: uuid.New()}
	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error { return nil },
	})
	require.NoError(t, err)
	c.TaskController.Queue = publicationQueue{Queue: c.TaskController.Queue, addJob: func(context.Context, *taskq.Job) error {
		return errors.New("publication failed")
	}}

	c.ScheduleTask(ctx, schemas.TaskTypePullProject, "one", "one")
	count, err := s.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
	executed, err := s.ExecutedTasksCount(ctx)
	require.NoError(t, err)
	assert.Zero(t, executed)
	assert.Empty(t, c.TaskController.admission.slots)
}

func TestScheduleTaskRollsBackCanceledReservation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := store.NewLocalStore()
	c := &Controller{Store: cancelingStore{Store: s, cancel: cancel}, TaskController: NewTaskController(ctx, nil, 1), UUID: uuid.New()}
	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error { return nil },
	})
	require.NoError(t, err)
	c.TaskController.Queue = publicationQueue{Queue: c.TaskController.Queue, addJob: func(context.Context, *taskq.Job) error {
		t.Fatal("AddJob called after cancellation")
		return nil
	}}

	c.ScheduleTask(ctx, schemas.TaskTypePullProject, "one", "one")
	count, err := s.CurrentlyQueuedTasksCount(context.Background())
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Empty(t, c.TaskController.admission.slots)
}

func TestScheduleTaskCapacityIsAtomic(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	c := &Controller{Store: s, TaskController: NewTaskController(ctx, nil, 1), UUID: uuid.New()}
	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error { return nil },
	})
	require.NoError(t, err)
	started := make(chan struct{})
	release := make(chan struct{})
	c.TaskController.Queue = publicationQueue{Queue: c.TaskController.Queue, addJob: func(context.Context, *taskq.Job) error {
		close(started)
		<-release
		return nil
	}}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.ScheduleTask(ctx, schemas.TaskTypePullProject, "first", "first")
	}()
	<-started
	for i := range 100 {
		c.ScheduleTask(ctx, schemas.TaskTypePullProject, strconv.Itoa(i), strconv.Itoa(i))
	}
	count, err := s.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
	close(release)
	wg.Wait()
}

func TestRedisTwoControllersPreserveAndCompletePendingJobs(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	publisher := &Controller{Redis: client, Store: store.NewRedisStore(client), UUID: uuid.New()}
	publisher.TaskController = NewTaskController(ctx, client, 1)
	_, err = publisher.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error { t.Error("publisher consumed its own job"); return nil },
	})
	require.NoError(t, err)
	_, err = publisher.Store.(*store.Redis).SetKeepalive(ctx, publisher.UUID.String(), time.Minute)
	require.NoError(t, err)
	publisher.ScheduleTask(ctx, schemas.TaskTypePullProject, "one", "one")

	// The second controller is constructed after publication; startup must preserve the job.
	consumer := &Controller{Redis: client, Store: store.NewRedisStore(client), UUID: uuid.New()}
	consumer.TaskController = NewTaskController(ctx, client, 1)
	completed := make(chan string, 20)
	_, err = consumer.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(ctx context.Context, id string) error {
			consumer.dequeueTask(ctx, schemas.TaskTypePullProject, id)
			completed <- id
			return nil
		},
	})
	require.NoError(t, err)
	_, err = consumer.Store.(*store.Redis).SetKeepalive(ctx, consumer.UUID.String(), time.Minute)
	require.NoError(t, err)
	require.NoError(t, consumer.TaskController.Factory.StartConsumers(ctx))
	defer func() {
		_ = consumer.TaskController.Factory.StopConsumers()
		_ = consumer.TaskController.Factory.Close()
	}()

	for i := 0; i < 10; i++ {
		id := "one"
		if i > 0 {
			id = strconv.Itoa(i)
			publisher.ScheduleTask(ctx, schemas.TaskTypePullProject, id, id)
		}
		select {
		case got := <-completed:
			require.Equal(t, id, got)
		case <-time.After(5 * time.Second):
			t.Fatalf("job %s was not consumed", id)
		}
		require.Eventually(t, func() bool {
			count, err := publisher.Store.CurrentlyQueuedTasksCount(ctx)
			return err == nil && count == 0
		}, time.Second, 10*time.Millisecond)
	}
}

func TestRedisShutdownCompletesActiveTaskReservation(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	publisher := &Controller{Redis: client, Store: store.NewRedisStore(client), UUID: uuid.New()}
	publisher.TaskController = NewTaskController(ctx, client, 1)
	_, err = publisher.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error { return nil },
	})
	require.NoError(t, err)
	_, err = publisher.Store.(*store.Redis).SetKeepalive(ctx, publisher.UUID.String(), time.Minute)
	require.NoError(t, err)

	consumer := &Controller{Redis: client, Store: store.NewRedisStore(client), UUID: uuid.New()}
	consumer.TaskController = NewTaskController(ctx, client, 1)
	started := make(chan struct{})
	finished := make(chan struct{})
	release := make(chan struct{})
	_, err = consumer.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(ctx context.Context, id string) error {
			defer close(finished)
			defer consumer.dequeueTask(ctx, schemas.TaskTypePullProject, id)
			close(started)
			<-release
			return nil
		},
	})
	require.NoError(t, err)
	require.NoError(t, consumer.TaskController.Factory.StartConsumers(ctx))
	defer func() {
		_ = consumer.TaskController.Factory.StopConsumers()
		_ = consumer.TaskController.Factory.Close()
	}()
	publisher.ScheduleTask(ctx, schemas.TaskTypePullProject, "active", "active")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start")
	}
	cancel()
	close(release)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not finish after shutdown")
	}
	require.Eventually(t, func() bool {
		count, err := publisher.Store.CurrentlyQueuedTasksCount(context.Background())
		return err == nil && count == 0
	}, time.Second, 10*time.Millisecond)
}

func TestRedisHandlerSkipsSupersededJob(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	tasks := &taskq.TaskMap{}
	var calls atomic.Int32
	task, err := tasks.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error {
			calls.Add(1)
			return nil
		},
	})
	require.NoError(t, err)
	st := store.NewRedisStore(client)
	old := "process|old"
	replacement := "process|replacement"
	ok, err := st.QueueTask(context.Background(), schemas.TaskTypePullProject, "same", old)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = st.QueueTask(context.Background(), schemas.TaskTypePullProject, "same", replacement)
	require.NoError(t, err)
	require.True(t, ok)
	handler := ownedTaskHandler{tasks: tasks, redis: client}
	stale := task.NewJob("same", reservationMarker+old+":same")
	require.NoError(t, handler.HandleJob(context.Background(), stale))
	require.Zero(t, calls.Load())
	current := task.NewJob("same", reservationMarker+replacement+":same")
	require.NoError(t, handler.HandleJob(context.Background(), current))
	require.Equal(t, int32(1), calls.Load())
	// The same message can be retried without losing its reservation metadata.
	require.NoError(t, handler.HandleJob(context.Background(), current))
	require.Equal(t, int32(2), calls.Load())
}

func TestRedisRestartTakeoverSkipsStalePendingJob(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	old := &Controller{Redis: client, Store: store.NewRedisStore(client), UUID: uuid.New()}
	old.TaskController = NewTaskController(ctx, client, 2)
	_, err = old.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error { return nil },
	})
	require.NoError(t, err)
	_, err = old.Store.(*store.Redis).SetKeepalive(ctx, old.UUID.String(), time.Second)
	require.NoError(t, err)
	old.ScheduleTask(ctx, schemas.TaskTypePullProject, "same", "same")
	mr.FastForward(2 * time.Second)

	replacement := &Controller{Redis: client, Store: store.NewRedisStore(client), UUID: uuid.New()}
	replacement.TaskController = NewTaskController(ctx, client, 2)
	var calls atomic.Int32
	done := make(chan struct{}, 1)
	_, err = replacement.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(ctx context.Context, id string) error {
			defer replacement.dequeueTask(ctx, schemas.TaskTypePullProject, id)
			calls.Add(1)
			done <- struct{}{}
			return nil
		},
	})
	require.NoError(t, err)
	_, err = replacement.Store.(*store.Redis).SetKeepalive(ctx, replacement.UUID.String(), time.Minute)
	require.NoError(t, err)
	replacement.ScheduleTask(ctx, schemas.TaskTypePullProject, "same", "same")
	require.NoError(t, replacement.TaskController.Factory.StartConsumers(ctx))
	defer func() {
		_ = replacement.TaskController.Factory.StopConsumers()
		_ = replacement.TaskController.Factory.Close()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement job was not consumed")
	}
	require.Eventually(t, func() bool {
		count, err := replacement.Store.CurrentlyQueuedTasksCount(ctx)
		return err == nil && count == 0
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), calls.Load())
}

func TestKeepaliveFailureReturnsAndRecovers(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()
	c := &Controller{
		Redis: redisClient,
		Store: store.NewRedisStore(redisClient),
		UUID:  uuid.New(),
	}
	mr.SetError("redis unavailable")
	err = c.setKeepaliveWithRetry(context.Background())
	require.ErrorContains(t, err, "redis unavailable")
	mr.SetError("")
	require.NoError(t, c.setKeepaliveWithRetry(context.Background()))
}
