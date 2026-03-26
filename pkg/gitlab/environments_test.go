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

type noopLimiterEnvironmentsTests struct{}

func (noopLimiterEnvironmentsTests) Take(ctx context.Context) time.Duration {
	return 0
}

func newTestGitLabClientForEnvironments(t *testing.T, handler http.HandlerFunc) *Client {
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
		RateLimiter: noopLimiterEnvironmentsTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestGetProjectEnvironments_InvalidRegexp(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterEnvironmentsTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	p.Pull.Environments.Regexp = "["

	envs, err := c.GetProjectEnvironments(context.Background(), p)

	assert.Error(t, err)
	assert.Nil(t, envs)
}

func TestGetProjectEnvironments_FiltersAndPaginates(t *testing.T) {
	c := newTestGitLabClientForEnvironments(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/environments"))
		w.Header().Set("Content-Type", "application/json")

		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[
				{"id":1,"name":"production","state":"available"},
				{"id":2,"name":"review/123","state":"stopped"}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{"id":3,"name":"staging","state":"available"}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	p := schemas.NewProject("group/project")
	p.Pull.Environments.Regexp = `^(production|staging)$`
	p.OutputSparseStatusMetrics = true

	envs, err := c.GetProjectEnvironments(context.Background(), p)
	require.NoError(t, err)

	expected1 := schemas.Environment{
		ProjectName:               p.Name,
		ID:                        int64(1),
		Name:                      "production",
		Available:                 true,
		OutputSparseStatusMetrics: true,
	}
	expected2 := schemas.Environment{
		ProjectName:               p.Name,
		ID:                        int64(3),
		Name:                      "staging",
		Available:                 true,
		OutputSparseStatusMetrics: true,
	}

	require.Len(t, envs, 2)
	assert.Contains(t, envs, expected1.Key())
	assert.Contains(t, envs, expected2.Key())
	assert.Equal(t, expected1, envs[expected1.Key()])
	assert.Equal(t, expected2, envs[expected2.Key()])
}

func TestGetProjectEnvironments_ExcludeStopped(t *testing.T) {
	c := newTestGitLabClientForEnvironments(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/environments"))
		assert.Equal(t, "available", r.URL.Query().Get("states"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id":1,"name":"production","state":"available"}
		]`))
	})

	p := schemas.NewProject("group/project")
	p.Pull.Environments.Regexp = `^production$`
	p.Pull.Environments.ExcludeStopped = true

	envs, err := c.GetProjectEnvironments(context.Background(), p)
	require.NoError(t, err)
	require.Len(t, envs, 1)

	expected := schemas.Environment{
		ProjectName:               p.Name,
		ID:                        int64(1),
		Name:                      "production",
		Available:                 true,
		OutputSparseStatusMetrics: true,
	}
	assert.Contains(t, envs, expected.Key())
	assert.Equal(t, expected, envs[expected.Key()])
}

func TestGetEnvironment_NoLastDeployment(t *testing.T) {
	c := newTestGitLabClientForEnvironments(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/environments/42"))
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 42,
			"name": "production",
			"external_url": "https://example.com",
			"state": "available",
			"last_deployment": null
		}`))
	})

	env, err := c.GetEnvironment(context.Background(), "group/project", 42)
	require.NoError(t, err)

	assert.Equal(t, "group/project", env.ProjectName)
	assert.Equal(t, int64(42), env.ID)
	assert.Equal(t, "production", env.Name)
	assert.Equal(t, "https://example.com", env.ExternalURL)
	assert.True(t, env.Available)
	assert.Equal(t, schemas.Deployment{}, env.LatestDeployment)
}

func TestGetEnvironment_WithLastDeployment_Branch(t *testing.T) {
	c := newTestGitLabClientForEnvironments(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/environments/99"))
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 99,
			"name": "staging",
			"external_url": "https://staging.example.com",
			"state": "available",
			"last_deployment": {
				"ref": "main",
				"created_at": "2024-03-09T16:00:00Z",
				"deployable": {
					"id": 1234,
					"tag": false,
					"duration": 12.5,
					"status": "success",
					"user": {
						"username": "jdoe"
					},
					"commit": {
						"short_id": "abc123"
					}
				}
			}
		}`))
	})

	env, err := c.GetEnvironment(context.Background(), "group/project", 99)
	require.NoError(t, err)

	assert.Equal(t, "group/project", env.ProjectName)
	assert.Equal(t, int64(99), env.ID)
	assert.Equal(t, "staging", env.Name)
	assert.Equal(t, "https://staging.example.com", env.ExternalURL)
	assert.True(t, env.Available)

	assert.Equal(t, schemas.RefKindBranch, env.LatestDeployment.RefKind)
	assert.Equal(t, "main", env.LatestDeployment.RefName)
	assert.Equal(t, int64(1234), env.LatestDeployment.JobID)
	assert.Equal(t, 12.5, env.LatestDeployment.DurationSeconds)
	assert.Equal(t, "success", env.LatestDeployment.Status)
	assert.Equal(t, "jdoe", env.LatestDeployment.Username)
	assert.Equal(t, "abc123", env.LatestDeployment.CommitShortID)
	assert.Equal(t, float64(1710000000), env.LatestDeployment.Timestamp)
}

func TestGetEnvironment_WithLastDeployment_Tag(t *testing.T) {
	c := newTestGitLabClientForEnvironments(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/environments/100"))
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 100,
			"name": "release",
			"external_url": "https://release.example.com",
			"state": "stopped",
			"last_deployment": {
				"ref": "v1.2.3",
				"created_at": "2024-03-10T10:00:00Z",
				"deployable": {
					"id": 555,
					"tag": true,
					"duration": 20,
					"status": "failed"
				}
			}
		}`))
	})

	env, err := c.GetEnvironment(context.Background(), "group/project", 100)
	require.NoError(t, err)

	assert.Equal(t, "group/project", env.ProjectName)
	assert.Equal(t, int64(100), env.ID)
	assert.Equal(t, "release", env.Name)
	assert.Equal(t, "https://release.example.com", env.ExternalURL)
	assert.False(t, env.Available)

	assert.Equal(t, schemas.RefKindTag, env.LatestDeployment.RefKind)
	assert.Equal(t, "v1.2.3", env.LatestDeployment.RefName)
	assert.Equal(t, int64(555), env.LatestDeployment.JobID)
	assert.Equal(t, 20.0, env.LatestDeployment.DurationSeconds)
	assert.Equal(t, "failed", env.LatestDeployment.Status)
	assert.Equal(t, "", env.LatestDeployment.Username)
	assert.Equal(t, "", env.LatestDeployment.CommitShortID)
	assert.Equal(t, float64(1710064800), env.LatestDeployment.Timestamp)
}
