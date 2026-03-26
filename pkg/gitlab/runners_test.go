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

type noopLimiterForRunnersTests struct{}

func (noopLimiterForRunnersTests) Take(ctx context.Context) time.Duration {
	return 0
}

func newTestGitLabClientForRunners(t *testing.T, handler http.HandlerFunc) *Client {
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
		RateLimiter: noopLimiterForRunnersTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestGetProjectRunners_InvalidRegexp(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterForRunnersTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	p.Pull.Runners.Regexp = "["

	runners, err := c.GetProjectRunners(context.Background(), p)

	assert.Error(t, err)
	assert.Nil(t, runners)
}

func TestGetProjectRunners_FiltersAndPaginates(t *testing.T) {
	c := newTestGitLabClientForRunners(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/runners"))
		w.Header().Set("Content-Type", "application/json")

		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[
				{"id":1,"name":"runner-linux-1","description":"Linux runner"},
				{"id":2,"name":"other-runner","description":"Should be filtered out"}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{"id":3,"name":"runner-linux-2","description":"Second Linux runner"}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	p := schemas.NewProject("group/project")
	p.Pull.Runners.Regexp = `^runner-linux-`
	p.OutputSparseStatusMetrics = true

	runners, err := c.GetProjectRunners(context.Background(), p)
	require.NoError(t, err)

	expected1 := schemas.Runner{
		ProjectName:               p.Name,
		ID:                        1,
		Name:                      "runner-linux-1",
		Description:               "Linux runner",
		OutputSparseStatusMetrics: true,
	}
	expected2 := schemas.Runner{
		ProjectName:               p.Name,
		ID:                        3,
		Name:                      "runner-linux-2",
		Description:               "Second Linux runner",
		OutputSparseStatusMetrics: true,
	}

	require.Len(t, runners, 2)
	assert.Contains(t, runners, expected1.Key())
	assert.Contains(t, runners, expected2.Key())
	assert.Equal(t, expected1, runners[expected1.Key()])
	assert.Equal(t, expected2, runners[expected2.Key()])
}

func TestGetRunner_FullDetails(t *testing.T) {
	c := newTestGitLabClientForRunners(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/runners/123"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`{
			"id": 123,
			"name": "runner-123",
			"description": "Main shared runner",
			"paused": true,
			"is_shared": true,
			"runner_type": "instance_type",
			"contacted_at": "2024-03-09T16:00:00Z",
			"maintenance_note": "maintenance window",
			"online": true,
			"status": "online",
			"token": "tok123",
			"tag_list": ["docker", "linux"],
			"run_untagged": true,
			"locked": false,
			"access_level": "not_protected",
			"maximum_timeout": 3600,
			"groups": [
				{
					"id": 10,
					"name": "group-a",
					"web_url": "https://gitlab.example.com/groups/group-a"
				}
			],
			"projects": [
				{
					"id": 20,
					"name": "project-a",
					"name_with_namespace": "group-a/project-a",
					"path": "project-a",
					"path_with_namespace": "group-a/project-a"
				}
			]
		}`))
	})

	got, err := c.GetRunner(context.Background(), "group/project", 123)
	require.NoError(t, err)

	assert.Equal(t, "group/project", got.ProjectName)
	assert.Equal(t, int64(123), got.ID)
	assert.Equal(t, "runner-123", got.Name)
	assert.Equal(t, "Main shared runner", got.Description)
	assert.True(t, got.Paused)
	assert.True(t, got.IsShared)
	assert.Equal(t, "instance_type", got.RunnerType)
	require.NotNil(t, got.ContactedAt)
	assert.Equal(t, int64(1710000000), got.ContactedAt.Unix())
	assert.Equal(t, "maintenance window", got.MaintenanceNote)
	assert.True(t, got.Online)
	assert.Equal(t, "online", got.Status)
	assert.Equal(t, "tok123", got.Token)
	assert.Equal(t, []string{"docker", "linux"}, got.TagList)
	assert.True(t, got.RunUntagged)
	assert.False(t, got.Locked)
	assert.Equal(t, "not_protected", got.AccessLevel)
	assert.Equal(t, int64(3600), got.MaximumTimeout)

	require.Len(t, got.Groups, 1)
	assert.Equal(t, int64(10), got.Groups[0].ID)
	assert.Equal(t, "group-a", got.Groups[0].Name)
	assert.Equal(t, "https://gitlab.example.com/groups/group-a", got.Groups[0].WebURL)

	require.Len(t, got.Projects, 1)
	assert.Equal(t, int64(20), got.Projects[0].ID)
	assert.Equal(t, "project-a", got.Projects[0].Name)
	assert.Equal(t, "group-a/project-a", got.Projects[0].NameWithNamespace)
	assert.Equal(t, "project-a", got.Projects[0].Path)
	assert.Equal(t, "group-a/project-a", got.Projects[0].PathWithNamespace)
}

func TestGetRunner_NoGroups_StillReturnsDetails(t *testing.T) {
	c := newTestGitLabClientForRunners(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/runners/456"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`{
			"id": 456,
			"name": "runner-456",
			"description": "Runner without groups",
			"paused": false,
			"is_shared": false,
			"runner_type": "project_type",
			"contacted_at": "2024-03-10T10:00:00Z",
			"maintenance_note": "",
			"online": false,
			"status": "offline",
			"token": "tok456",
			"tag_list": ["shell"],
			"run_untagged": false,
			"locked": true,
			"access_level": "ref_protected",
			"maximum_timeout": 1800,
			"groups": null,
			"projects": [
				{
					"id": 30,
					"name": "project-b",
					"name_with_namespace": "group-b/project-b",
					"path": "project-b",
					"path_with_namespace": "group-b/project-b"
				}
			]
		}`))
	})

	got, err := c.GetRunner(context.Background(), "group/project", 456)
	require.NoError(t, err)

	assert.Equal(t, "group/project", got.ProjectName)
	assert.Equal(t, int64(456), got.ID)
	assert.Equal(t, "runner-456", got.Name)
	assert.Equal(t, "Runner without groups", got.Description)
	assert.False(t, got.Paused)
	assert.False(t, got.IsShared)
	assert.Equal(t, "project_type", got.RunnerType)
	require.NotNil(t, got.ContactedAt)
	assert.Equal(t, int64(1710064800), got.ContactedAt.Unix())
	assert.Equal(t, "", got.MaintenanceNote)
	assert.False(t, got.Online)
	assert.Equal(t, "offline", got.Status)
	assert.Equal(t, "tok456", got.Token)
	assert.Equal(t, []string{"shell"}, got.TagList)
	assert.False(t, got.RunUntagged)
	assert.True(t, got.Locked)
	assert.Equal(t, "ref_protected", got.AccessLevel)
	assert.Equal(t, int64(1800), got.MaximumTimeout)

	assert.Nil(t, got.Groups)

	require.Len(t, got.Projects, 1)
	assert.Equal(t, int64(30), got.Projects[0].ID)
	assert.Equal(t, "project-b", got.Projects[0].Name)
	assert.Equal(t, "group-b/project-b", got.Projects[0].NameWithNamespace)
	assert.Equal(t, "project-b", got.Projects[0].Path)
	assert.Equal(t, "group-b/project-b", got.Projects[0].PathWithNamespace)
}
