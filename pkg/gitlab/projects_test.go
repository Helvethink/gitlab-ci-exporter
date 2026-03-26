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

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

type noopLimiterProjectsTests struct{}

func (noopLimiterProjectsTests) Take(ctx context.Context) time.Duration {
	return 0
}

func newTestGitLabClientForProjects(t *testing.T, handler http.HandlerFunc) *Client {
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
		RateLimiter: noopLimiterProjectsTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestGetProject(t *testing.T) {
	c := newTestGitLabClientForProjects(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/projects/"))
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 101,
			"path_with_namespace": "group/project",
			"name": "project"
		}`))
	})

	p, err := c.GetProject(context.Background(), "group/project")
	require.NoError(t, err)
	require.NotNil(t, p)

	assert.Equal(t, int64(101), p.ID)
	assert.Equal(t, "group/project", p.PathWithNamespace)
	assert.Equal(t, "project", p.Name)
}

func TestListProjects_UserOwner(t *testing.T) {
	c := newTestGitLabClientForProjects(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/users/john/projects"))
		assert.Equal(t, "myapp", r.URL.Query().Get("search"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id":1,"path_with_namespace":"john/myapp-api"},
			{"id":2,"path_with_namespace":"john/myapp-web"},
			{"id":3,"path_with_namespace":"other/myapp-other"}
		]`))
	})

	w := config.Wildcard{}
	w.Search = "myapp"
	w.Owner.Kind = "user"
	w.Owner.Name = "john"

	projects, err := c.ListProjects(context.Background(), w)
	require.NoError(t, err)

	require.Len(t, projects, 2)
	assert.Equal(t, "john/myapp-api", projects[0].Name)
	assert.Equal(t, "john/myapp-web", projects[1].Name)
}

func TestListProjects_GroupOwner_Paginates(t *testing.T) {
	c := newTestGitLabClientForProjects(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/groups/team/projects"))
		w.Header().Set("Content-Type", "application/json")

		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[
				{"id":1,"path_with_namespace":"team/service-a"},
				{"id":2,"path_with_namespace":"team/service-b"}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{"id":3,"path_with_namespace":"team/sub/service-c"}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	w := config.Wildcard{}
	w.Owner.Kind = "group"
	w.Owner.Name = "team"
	w.Owner.IncludeSubgroups = true

	projects, err := c.ListProjects(context.Background(), w)
	require.NoError(t, err)

	require.Len(t, projects, 3)
	assert.Equal(t, "team/service-a", projects[0].Name)
	assert.Equal(t, "team/service-b", projects[1].Name)
	assert.Equal(t, "team/sub/service-c", projects[2].Name)
}

func TestListProjects_DefaultListsVisibleProjects(t *testing.T) {
	c := newTestGitLabClientForProjects(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/projects"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id":1,"path_with_namespace":"group/project-a"},
			{"id":2,"path_with_namespace":"other/project-b"}
		]`))
	})

	w := config.Wildcard{}
	w.Search = "project"

	projects, err := c.ListProjects(context.Background(), w)
	require.NoError(t, err)

	require.Len(t, projects, 2)
	assert.Equal(t, "group/project-a", projects[0].Name)
	assert.Equal(t, "other/project-b", projects[1].Name)
}

func TestListProjects_FiltersByOwnerName(t *testing.T) {
	c := newTestGitLabClientForProjects(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/groups/team/projects"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id":1,"path_with_namespace":"team/service-a"},
			{"id":2,"path_with_namespace":"other/service-b"}
		]`))
	})

	w := config.Wildcard{}
	w.Owner.Kind = "group"
	w.Owner.Name = "team"

	projects, err := c.ListProjects(context.Background(), w)
	require.NoError(t, err)

	require.Len(t, projects, 1)
	assert.Equal(t, "team/service-a", projects[0].Name)
}

func TestListProjects_APIError(t *testing.T) {
	c := newTestGitLabClientForProjects(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	w := config.Wildcard{}
	w.Search = "myapp"

	projects, err := c.ListProjects(context.Background(), w)
	assert.Error(t, err)
	assert.Empty(t, projects)
	assert.Contains(t, err.Error(), "unable to list projects with search pattern")
}

func TestListProjects_ReturnsSchemaProjects(t *testing.T) {
	c := newTestGitLabClientForProjects(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id":1,"path_with_namespace":"team/service-a"}
		]`))
	})

	w := config.Wildcard{}

	projects, err := c.ListProjects(context.Background(), w)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	expected := schemas.NewProject("team/service-a")
	assert.Equal(t, expected.Name, projects[0].Name)
}
