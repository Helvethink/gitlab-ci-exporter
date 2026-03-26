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

type noopLimiterBranchesTests struct{}

func (noopLimiterBranchesTests) Take(ctx context.Context) time.Duration {
	return 0
}

func newTestGitLabClientForBranches(t *testing.T, handler http.HandlerFunc) *Client {
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
		RateLimiter: noopLimiterBranchesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestGetProjectBranches_InvalidRegexp(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterBranchesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	p.Pull.Refs.Branches.Regexp = "["

	refs, err := c.GetProjectBranches(context.Background(), p)

	assert.Error(t, err)
	assert.Empty(t, refs)
}

func TestGetProjectBranches_FiltersAndPaginates(t *testing.T) {
	c := newTestGitLabClientForBranches(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/repository/branches"))
		w.Header().Set("Content-Type", "application/json")

		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[
				{"name":"main"},
				{"name":"feature/test"}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{"name":"develop"},
				{"name":"release/1.0"}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	p := schemas.NewProject("group/project")
	p.Pull.Refs.Branches.Regexp = `^(main|develop)$`

	refs, err := c.GetProjectBranches(context.Background(), p)
	require.NoError(t, err)

	expected1 := schemas.NewRef(p, schemas.RefKindBranch, "main")
	expected2 := schemas.NewRef(p, schemas.RefKindBranch, "develop")

	require.Len(t, refs, 2)
	assert.Contains(t, refs, expected1.Key())
	assert.Contains(t, refs, expected2.Key())
	assert.Equal(t, expected1, refs[expected1.Key()])
	assert.Equal(t, expected2, refs[expected2.Key()])
}

func TestGetBranchLatestCommit(t *testing.T) {
	c := newTestGitLabClientForBranches(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/repository/branches/main"))
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"name": "main",
			"commit": {
				"short_id": "abc123",
				"committed_date": "2024-03-09T16:00:00Z"
			}
		}`))
	})

	shortID, ts, err := c.GetBranchLatestCommit(context.Background(), "group/project", "main")
	require.NoError(t, err)

	assert.Equal(t, "abc123", shortID)
	assert.Equal(t, float64(1710000000), ts)
}

func TestGetBranchLatestCommit_APIError(t *testing.T) {
	c := newTestGitLabClientForBranches(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	shortID, ts, err := c.GetBranchLatestCommit(context.Background(), "group/project", "main")
	assert.Error(t, err)
	assert.Equal(t, "", shortID)
	assert.Equal(t, float64(0), ts)
}
