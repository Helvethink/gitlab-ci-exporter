package controller

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
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
