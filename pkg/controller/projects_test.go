package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/taskq/v4"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func registerProjectFollowUpTasks(t *testing.T, c *Controller) {
	t.Helper()

	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullRefsFromProject), &taskq.TaskConfig{
		Handler: func(context.Context, schemas.Project) {
		},
	})
	require.NoError(t, err)

	_, err = c.TaskController.TaskMap.Register(string(schemas.TaskTypePullEnvironmentsFromProject), &taskq.TaskConfig{
		Handler: func(context.Context, schemas.Project) {
		},
	})
	require.NoError(t, err)

	_, err = c.TaskController.TaskMap.Register(string(schemas.TaskTypePullRunnersFromProject), &taskq.TaskConfig{
		Handler: func(context.Context, schemas.Project) {
		},
	})
	require.NoError(t, err)
}

func TestPullProject(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/projects/group/project")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 101,
			"path_with_namespace": "group/project"
		}`))
	})
	c.TaskController = NewTaskController(ctx, nil, 10)
	c.UUID = uuid.New()
	registerProjectFollowUpTasks(t, c)

	pull := config.ProjectPull{}
	pull.Environments.Enabled = true
	pull.Refs.Branches.Enabled = true
	pull.Runners.Enabled = true

	err := c.PullProject(ctx, "group/project", pull)
	require.NoError(t, err)

	storedProject := schemas.NewProject("group/project")
	require.NoError(t, c.Store.GetProject(ctx, &storedProject))
	assert.Equal(t, "group/project", storedProject.Name)
	assert.True(t, storedProject.Pull.Environments.Enabled)
	assert.True(t, storedProject.Pull.Refs.Branches.Enabled)
	assert.True(t, storedProject.Pull.Runners.Enabled)

	queued, err := c.Store.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), queued)
}

func TestPullProjectsFromWildcard(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/groups/platform/projects"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")
		_, _ = w.Write([]byte(`[
			{"id": 1, "path_with_namespace": "platform/app-one"},
			{"id": 2, "path_with_namespace": "platform/app-two"}
		]`))
	})
	c.TaskController = NewTaskController(ctx, nil, 10)
	c.UUID = uuid.New()
	registerProjectFollowUpTasks(t, c)

	wildcard := config.Wildcard{
		Search: "app",
		Owner: config.WildcardOwner{
			Kind: "group",
			Name: "platform",
		},
	}
	wildcard.Pull.Environments.Enabled = true
	wildcard.Pull.Refs.Branches.Enabled = true
	wildcard.Pull.Runners.Enabled = true

	err := c.PullProjectsFromWildcard(ctx, wildcard)
	require.NoError(t, err)

	projectOne := schemas.NewProject("platform/app-one")
	require.NoError(t, c.Store.GetProject(ctx, &projectOne))
	assert.True(t, projectOne.Pull.Environments.Enabled)

	projectTwo := schemas.NewProject("platform/app-two")
	require.NoError(t, c.Store.GetProject(ctx, &projectTwo))
	assert.True(t, projectTwo.Pull.Refs.Branches.Enabled)

	queued, err := c.Store.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(6), queued)
}
