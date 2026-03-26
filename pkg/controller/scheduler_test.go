package controller

import (
	"context"
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

func TestScheduleTaskQueuesTaskOnlyOnce(t *testing.T) {
	ctx := context.Background()
	var handled atomic.Int32

	c := &Controller{
		Store: store.NewLocalStore(),
		TaskController: NewTaskController(ctx, nil, 10),
		UUID: uuid.New(),
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
