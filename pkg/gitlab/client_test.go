package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/ratelimit"
)

type mockLimiter struct {
	called int32
}

func (m *mockLimiter) Take(ctx context.Context) time.Duration {
	atomic.AddInt32(&m.called, 1)
	return 0
}

var _ ratelimit.Limiter = (*mockLimiter)(nil)

func TestNewHTTPClient(t *testing.T) {
	client := NewHTTPClient(true)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestNewHTTPClient_DisableTLSFalse(t *testing.T) {
	client := NewHTTPClient(false)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestNewClient(t *testing.T) {
	limiter := &mockLimiter{}

	cfg := ClientConfig{
		URL:              "https://gitlab.example.com/api/v4",
		Token:            "test-token",
		UserAgentVersion: "1.2.3",
		DisableTLSVerify: true,
		ReadinessURL:     "https://gitlab.example.com/-/health",
		RateLimiter:      limiter,
	}

	c, err := NewClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, c)

	require.NotNil(t, c.Client)
	assert.Equal(t, "gitlab-ci-pipelines-exporter-1.2.3", c.UserAgent)
	assert.Same(t, limiter, c.RateLimiter)

	assert.Equal(t, "https://gitlab.example.com/-/health", c.Readiness.URL)
	require.NotNil(t, c.Readiness.HTTPClient)
	assert.Equal(t, 5*time.Second, c.Readiness.HTTPClient.Timeout)

	require.NotNil(t, c.RateCounter)
}

func TestReadinessCheck_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	c := &Client{}
	c.Readiness.URL = server.URL
	c.Readiness.HTTPClient = server.Client()

	check := c.ReadinessCheck(context.Background())
	err := check()
	assert.NoError(t, err)
}

func TestReadinessCheck_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	c := &Client{}
	c.Readiness.URL = server.URL
	c.Readiness.HTTPClient = server.Client()

	check := c.ReadinessCheck(context.Background())
	err := check()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP error: 503")
}

func TestReadinessCheck_NoHTTPClient(t *testing.T) {
	c := &Client{}
	c.Readiness.URL = "https://gitlab.example.com/-/health"
	c.Readiness.HTTPClient = nil

	check := c.ReadinessCheck(context.Background())
	err := check()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "readiness http client not configured")
}

func TestRateLimit(t *testing.T) {
	limiter := &mockLimiter{}
	c := &Client{
		RateLimiter: limiter,
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	before := c.RequestsCounter.Load()
	c.rateLimit(context.Background())

	assert.Equal(t, int32(1), atomic.LoadInt32(&limiter.called))
	assert.Equal(t, before+1, c.RequestsCounter.Load())
	assert.Equal(t, int64(1), c.RateCounter.Rate())
}

func TestUpdateVersionAndVersion(t *testing.T) {
	c := &Client{}

	assert.Equal(t, GitLabVersion{}, c.Version())

	v := NewGitLabVersion("15.9.0")
	c.UpdateVersion(v)

	assert.Equal(t, v, c.Version())
}

func TestRequestsRemaining(t *testing.T) {
	resp := &goGitlab.Response{
		Response: &http.Response{
			Header: http.Header{
				"Ratelimit-Remaining": []string{"42"},
				"Ratelimit-Limit":     []string{"100"},
			},
		},
	}

	c := &Client{}
	c.requestsRemaining(resp)

	assert.Equal(t, 42, c.RequestsRemaining)
	assert.Equal(t, 100, c.RequestsLimit)
}

func TestRequestsRemaining_NilResponse(t *testing.T) {
	c := &Client{
		RequestsRemaining: 7,
		RequestsLimit:     9,
	}

	c.requestsRemaining(nil)

	assert.Equal(t, 7, c.RequestsRemaining)
	assert.Equal(t, 9, c.RequestsLimit)
}

func TestRequestsRemaining_InvalidHeaders(t *testing.T) {
	resp := &goGitlab.Response{
		Response: &http.Response{
			Header: http.Header{
				"Ratelimit-Remaining": []string{"abc"},
				"Ratelimit-Limit":     []string{"def"},
			},
		},
	}

	c := &Client{
		RequestsRemaining: 11,
		RequestsLimit:     22,
	}

	c.requestsRemaining(resp)

	// strconv.Atoi errors are ignored by production code, so values fall back to zero.
	assert.Equal(t, 0, c.RequestsRemaining)
	assert.Equal(t, 0, c.RequestsLimit)
}
