package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/ratelimit"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

type noopLimiter struct{}

func (noopLimiter) Take(ctx context.Context) time.Duration {
	return 0
}

var _ ratelimit.Limiter = noopLimiter{}

func newTestGitLabClientForTags(t *testing.T, handler http.HandlerFunc) *Client {
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
		RateLimiter: noopLimiter{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestGetProjectTags_InvalidRegexp(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiter{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	p.Pull.Refs.Tags.Regexp = "["

	refs, err := c.GetProjectTags(context.Background(), p)

	assert.Error(t, err)
	assert.Empty(t, refs)
}

func TestGetProjectTags_FiltersAndPaginates(t *testing.T) {
	c := newTestGitLabClientForTags(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repository/tags")

		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")

		switch page {
		case "1", "":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[
				{"name":"v1.0.0","commit":{"short_id":"abc111","committed_date":"2024-03-09T16:00:00Z"}},
				{"name":"dev-snapshot","commit":{"short_id":"def222","committed_date":"2024-03-08T16:00:00Z"}}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{"name":"v2.0.0","commit":{"short_id":"ghi333","committed_date":"2024-03-10T16:00:00Z"}},
				{"name":"test-tag","commit":{"short_id":"jkl444","committed_date":"2024-03-07T16:00:00Z"}}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	p := schemas.NewProject("group/project")
	p.Pull.Refs.Tags.Regexp = `^v`

	refs, err := c.GetProjectTags(context.Background(), p)
	require.NoError(t, err)

	expected1 := schemas.NewRef(p, schemas.RefKindTag, "v1.0.0")
	expected2 := schemas.NewRef(p, schemas.RefKindTag, "v2.0.0")

	assert.Len(t, refs, 2)
	assert.Contains(t, refs, expected1.Key())
	assert.Contains(t, refs, expected2.Key())
	assert.Equal(t, expected1, refs[expected1.Key()])
	assert.Equal(t, expected2, refs[expected2.Key()])
}

func TestGetProjectMostRecentTagCommit_InvalidRegexp(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiter{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	sha, ts, err := c.GetProjectMostRecentTagCommit(context.Background(), "group/project", "[")

	assert.Error(t, err)
	assert.Equal(t, "", sha)
	assert.Equal(t, float64(0), ts)
}

func TestGetProjectMostRecentTagCommit_ReturnsFirstMatchingTag(t *testing.T) {
	c := newTestGitLabClientForTags(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repository/tags")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"name":"not-a-release","commit":{"short_id":"zzz999","committed_date":"2024-03-08T16:00:00Z"}},
			{"name":"v2.1.0","commit":{"short_id":"abc123","committed_date":"2024-03-09T16:00:00Z"}},
			{"name":"v2.2.0","commit":{"short_id":"def456","committed_date":"2024-03-10T16:00:00Z"}}
		]`))
	})

	sha, ts, err := c.GetProjectMostRecentTagCommit(context.Background(), "group/project", `^v`)
	require.NoError(t, err)

	assert.Equal(t, "abc123", sha)
	assert.Equal(t, float64(1710000000), ts)
}

func TestGetProjectMostRecentTagCommit_PaginatesUntilMatch(t *testing.T) {
	c := newTestGitLabClientForTags(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repository/tags")

		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")

		switch page {
		case "1", "":
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[
				{"name":"snapshot-1","commit":{"short_id":"aaa111","committed_date":"2024-03-08T16:00:00Z"}}
			]`))
		case "2":
			w.Header().Set("X-Page", "2")
			w.Header().Set("X-Next-Page", "0")
			_, _ = w.Write([]byte(`[
				{"name":"v3.0.0","commit":{"short_id":"bbb222","committed_date":"2024-03-11T16:00:00Z"}}
			]`))
		default:
			t.Fatalf("unexpected page: %s", page)
		}
	})

	sha, ts, err := c.GetProjectMostRecentTagCommit(context.Background(), "group/project", `^v`)
	require.NoError(t, err)

	assert.Equal(t, "bbb222", sha)
	assert.Equal(t, float64(1710172800), ts)
}

func TestGetProjectMostRecentTagCommit_NoMatch(t *testing.T) {
	c := newTestGitLabClientForTags(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/repository/tags")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"name":"snapshot-1","commit":{"short_id":"aaa111","committed_date":"2024-03-08T16:00:00Z"}},
			{"name":"snapshot-2","commit":{"short_id":"bbb222","committed_date":"2024-03-09T16:00:00Z"}}
		]`))
	})

	sha, ts, err := c.GetProjectMostRecentTagCommit(context.Background(), "group/project", `^v`)
	require.NoError(t, err)
	assert.Equal(t, "", sha)
	assert.Equal(t, float64(0), ts)
}
