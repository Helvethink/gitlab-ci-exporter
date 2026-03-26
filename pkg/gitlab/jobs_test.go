package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

type noopLimiterJobsTests struct{}

func (noopLimiterJobsTests) Take(ctx context.Context) time.Duration {
	return 0
}

func newTestGitLabClientForJobs(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	gl, err := goGitlab.NewClient(
		"test-token",
		goGitlab.WithBaseURL(server.URL+"/api/v4"),
	)
	require.NoError(t, err)

	return &Client{
		Client:      gl,
		RateLimiter: noopLimiterJobsTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestListRefPipelineJobs_EmptyLatestPipeline(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterJobsTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	jobs, err := c.ListRefPipelineJobs(context.Background(), ref)
	require.NoError(t, err)
	assert.Nil(t, jobs)
}

func TestListPipelineJobs_Paginates(t *testing.T) {
	c := newTestGitLabClientForJobs(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines/123/jobs"))
		w.Header().Set("Content-Type", "application/json")

		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			w.Header().Set("X-Total", "2")
			_, _ = w.Write([]byte(`[
				{
					"id": 101,
					"name": "build",
					"stage": "build",
					"created_at": "2024-03-09T16:00:00Z",
					"duration": 10,
					"queued_duration": 1,
					"status": "success",
					"ref": "main",
					"tag_list": ["docker"],
					"failure_reason": "",
					"pipeline": {"id": 123},
					"artifacts": [{"size": 100}],
					"runner": {"description": "runner-1"}
				}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			w.Header().Set("X-Total", "2")
			_, _ = w.Write([]byte(`[
				{
					"id": 102,
					"name": "test",
					"stage": "test",
					"created_at": "2024-03-09T16:01:00Z",
					"duration": 20,
					"queued_duration": 2,
					"status": "failed",
					"ref": "main",
					"tag_list": ["linux"],
					"failure_reason": "script_failure",
					"pipeline": {"id": 123},
					"artifacts": [{"size": 200}],
					"runner": {"description": "runner-2"}
				}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	jobs, err := c.ListPipelineJobs(context.Background(), "group/project", 123)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	assert.Equal(t, 101, jobs[0].ID)
	assert.Equal(t, "build", jobs[0].Name)
	assert.Equal(t, 100.0, jobs[0].ArtifactSize)

	assert.Equal(t, 102, jobs[1].ID)
	assert.Equal(t, "test", jobs[1].Name)
	assert.Equal(t, "script_failure", jobs[1].FailureReason)
}

func TestListPipelineBridges_Paginates(t *testing.T) {
	c := newTestGitLabClientForJobs(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines/123/bridges"))
		w.Header().Set("Content-Type", "application/json")

		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			w.Header().Set("X-Total", "2")
			_, _ = w.Write([]byte(`[
				{"id": 1, "name": "bridge-1"}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			w.Header().Set("X-Total", "2")
			_, _ = w.Write([]byte(`[
				{"id": 2, "name": "bridge-2"}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	bridges, err := c.ListPipelineBridges(context.Background(), "group/project", 123)
	require.NoError(t, err)
	require.Len(t, bridges, 2)

	assert.Equal(t, 1, bridges[0].ID)
	assert.Equal(t, 2, bridges[1].ID)
}

func TestListPipelineChildJobs(t *testing.T) {
	c := newTestGitLabClientForJobs(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/pipelines/1000/bridges"):
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{
					"id": 11,
					"name": "trigger-child",
					"downstream_pipeline": {
						"id": 2001,
						"project_id": 222
					}
				},
				{
					"id": 12,
					"name": "not-run-yet",
					"downstream_pipeline": null
				}
			]`))

		case strings.Contains(r.URL.Path, "/projects/222/pipelines/2001/jobs"):
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{
					"id": 201,
					"name": "child-job",
					"stage": "test",
					"created_at": "2024-03-09T16:00:00Z",
					"duration": 12,
					"queued_duration": 1,
					"status": "success",
					"ref": "main",
					"tag_list": [],
					"failure_reason": "",
					"pipeline": {"id": 2001},
					"artifacts": [],
					"runner": {"description": "child-runner"}
				}
			]`))

		case strings.Contains(r.URL.Path, "/projects/222/pipelines/2001/bridges"):
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[]`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	jobs, err := c.ListPipelineChildJobs(context.Background(), "group/project", 1000)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	assert.Equal(t, 201, jobs[0].ID)
	assert.Equal(t, "child-job", jobs[0].Name)
	assert.Equal(t, 2001, jobs[0].PipelineID)
}

func TestListRefPipelineJobs_WithChildPipelinesEnabled(t *testing.T) {
	c := newTestGitLabClientForJobs(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/pipelines/1000/jobs"):
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{
					"id": 101,
					"name": "parent-job",
					"stage": "build",
					"created_at": "2024-03-09T16:00:00Z",
					"duration": 10,
					"queued_duration": 1,
					"status": "success",
					"ref": "main",
					"tag_list": [],
					"failure_reason": "",
					"pipeline": {"id": 1000},
					"artifacts": [],
					"runner": {"description": "runner-parent"}
				}
			]`))

		case strings.Contains(r.URL.Path, "/pipelines/1000/bridges"):
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{
					"id": 11,
					"name": "trigger-child",
					"downstream_pipeline": {
						"id": 2001,
						"project_id": 222
					}
				}
			]`))

		case strings.Contains(r.URL.Path, "/projects/222/pipelines/2001/jobs"):
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{
					"id": 201,
					"name": "child-job",
					"stage": "test",
					"created_at": "2024-03-09T16:01:00Z",
					"duration": 20,
					"queued_duration": 2,
					"status": "success",
					"ref": "main",
					"tag_list": [],
					"failure_reason": "",
					"pipeline": {"id": 2001},
					"artifacts": [],
					"runner": {"description": "runner-child"}
				}
			]`))

		case strings.Contains(r.URL.Path, "/projects/222/pipelines/2001/bridges"):
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[]`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	p := schemas.NewProject("group/project")
	p.Pull.Pipeline.Jobs.FromChildPipelines.Enabled = true

	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{ID: 1000}

	jobs, err := c.ListRefPipelineJobs(context.Background(), ref)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	assert.Equal(t, "parent-job", jobs[0].Name)
	assert.Equal(t, "child-job", jobs[1].Name)
}

func TestListRefMostRecentJobs_NoJobsInMemory(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterJobsTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	jobs, err := c.ListRefMostRecentJobs(context.Background(), ref)
	require.NoError(t, err)
	assert.Nil(t, jobs)
}

func TestListRefMostRecentJobs_FindsAllJobsOnFirstPage(t *testing.T) {
	c := newTestGitLabClientForJobs(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/projects/"))
		assert.True(t, strings.Contains(r.URL.Path, "/jobs"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")
		w.Header().Set("X-Total", "2")

		_, _ = w.Write([]byte(`[
			{
				"id": 101,
				"name": "build",
				"stage": "build",
				"created_at": "2024-03-09T16:00:00Z",
				"duration": 10,
				"queued_duration": 1,
				"status": "success",
				"ref": "main",
				"tag_list": [],
				"failure_reason": "",
				"pipeline": {"id": 1000},
				"artifacts": [],
				"runner": {"description": "runner-1"}
			},
			{
				"id": 102,
				"name": "test",
				"stage": "test",
				"created_at": "2024-03-09T16:01:00Z",
				"duration": 20,
				"queued_duration": 2,
				"status": "failed",
				"ref": "main",
				"tag_list": [],
				"failure_reason": "script_failure",
				"pipeline": {"id": 1000},
				"artifacts": [],
				"runner": {"description": "runner-2"}
			}
		]`))
	})

	c.version = NewGitLabVersion("15.8.0")

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")
	ref.LatestJobs = schemas.Jobs{
		"build": {Name: "build"},
		"test":  {Name: "test"},
	}

	jobs, err := c.ListRefMostRecentJobs(context.Background(), ref)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	assert.Equal(t, "build", jobs[0].Name)
	assert.Equal(t, "test", jobs[1].Name)
}
