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

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func registerNoopPullRefMetricsTask(t *testing.T, c *Controller) {
	t.Helper()

	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullRefMetrics), &taskq.TaskConfig{
		Handler: func(context.Context, schemas.Ref) {
		},
	})
	require.NoError(t, err)
}

func TestGetRefs(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/repository/branches"):
			_, _ = w.Write([]byte(`[
				{"name":"main"},
				{"name":"develop"}
			]`))
		case strings.Contains(r.URL.Path, "/repository/tags"):
			_, _ = w.Write([]byte(`[
				{"name":"v1.0.0"},
				{"name":"v2.0.0"}
			]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	p := schemas.NewProject("group/project")
	p.Pull.Refs.Branches.Enabled = true
	p.Pull.Refs.Branches.Regexp = "^main$"
	p.Pull.Refs.Branches.ExcludeDeleted = true
	p.Pull.Refs.Tags.Enabled = true
	p.Pull.Refs.Tags.Regexp = "^v1\\."
	p.Pull.Refs.Tags.ExcludeDeleted = true
	p.Pull.Refs.MergeRequests.Enabled = false

	refs, err := c.GetRefs(ctx, p)
	require.NoError(t, err)
	require.Len(t, refs, 2)

	_, branchExists := refs[schemas.NewRef(p, schemas.RefKindBranch, "main").Key()]
	assert.True(t, branchExists)

	_, tagExists := refs[schemas.NewRef(p, schemas.RefKindTag, "v1.0.0").Key()]
	assert.True(t, tagExists)
}

func TestPullRefsFromProject(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/repository/branches"):
			_, _ = w.Write([]byte(`[
				{"name":"main"}
			]`))
		case strings.Contains(r.URL.Path, "/repository/tags"):
			_, _ = w.Write([]byte(`[
				{"name":"v1.0.0"}
			]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	c.TaskController = NewTaskController(ctx, nil, 10)
	c.UUID = uuid.New()
	registerNoopPullRefMetricsTask(t, c)

	p := schemas.NewProject("group/project")
	p.Pull.Refs.Branches.Enabled = true
	p.Pull.Refs.Branches.Regexp = "^main$"
	p.Pull.Refs.Branches.ExcludeDeleted = true
	p.Pull.Refs.Tags.Enabled = true
	p.Pull.Refs.Tags.Regexp = "^v1\\."
	p.Pull.Refs.Tags.ExcludeDeleted = true
	p.Pull.Refs.MergeRequests.Enabled = false

	err := c.PullRefsFromProject(ctx, p)
	require.NoError(t, err)

	storedBranch := schemas.NewRef(p, schemas.RefKindBranch, "main")
	require.NoError(t, c.Store.GetRef(ctx, &storedBranch))
	assert.Equal(t, "main", storedBranch.Name)

	storedTag := schemas.NewRef(p, schemas.RefKindTag, "v1.0.0")
	require.NoError(t, c.Store.GetRef(ctx, &storedTag))
	assert.Equal(t, "v1.0.0", storedTag.Name)

	queued, err := c.Store.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), queued)
}
