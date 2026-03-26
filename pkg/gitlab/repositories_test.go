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
)

type noopLimiterRepositoriesTests struct{}

func (noopLimiterRepositoriesTests) Take(ctx context.Context) time.Duration {
	return 0
}

func newTestGitLabClientForRepositories(t *testing.T, handler http.HandlerFunc) *Client {
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
		RateLimiter: noopLimiterRepositoriesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestGetCommitCountBetweenRefs(t *testing.T) {
	c := newTestGitLabClientForRepositories(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/repository/compare"))
		assert.Equal(t, "main", r.URL.Query().Get("from"))
		assert.Equal(t, "release", r.URL.Query().Get("to"))
		assert.Equal(t, "true", r.URL.Query().Get("straight"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`{
			"commit": null,
			"commits": [
				{"id":"111"},
				{"id":"222"},
				{"id":"333"}
			],
			"diffs": []
		}`))
	})

	count, err := c.GetCommitCountBetweenRefs(context.Background(), "group/project", "main", "release")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestGetCommitCountBetweenRefs_NoCommits(t *testing.T) {
	c := newTestGitLabClientForRepositories(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/repository/compare"))
		assert.Equal(t, "main", r.URL.Query().Get("from"))
		assert.Equal(t, "release", r.URL.Query().Get("to"))
		assert.Equal(t, "true", r.URL.Query().Get("straight"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`{
			"commit": null,
			"commits": [],
			"diffs": []
		}`))
	})

	count, err := c.GetCommitCountBetweenRefs(context.Background(), "group/project", "main", "release")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestGetCommitCountBetweenRefs_APIError(t *testing.T) {
	c := newTestGitLabClientForRepositories(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/repository/compare"))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
	})

	count, err := c.GetCommitCountBetweenRefs(context.Background(), "group/project", "main", "release")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
}
