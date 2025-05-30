package gitlab

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/paulbellamy/ratecounter"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/otel"

	"github.com/helvethink/gitlab-ci-exporter/pkg/ratelimit"
)

const (
	userAgent  = "gitlab-ci-pipelines-exporter"
	tracerName = "gitlab-ci-pipelines-exporter"
)

// Client is a wrapper around the official go-gitlab client,
// adding support for rate limiting, request counting, readiness checks,
// and GitLab version tracking with concurrency safety.
type Client struct {
	*goGitlab.Client // Embedded GitLab API client

	// Readiness contains configuration to check if the GitLab instance
	// is responsive and healthy via an HTTP endpoint.
	Readiness struct {
		URL        string       // URL for readiness checks
		HTTPClient *http.Client // HTTP client used to perform readiness requests
	}

	RateLimiter       ratelimit.Limiter        // RateLimiter controls the rate of API requests to avoid hitting GitLab rate limits.
	RateCounter       *ratecounter.RateCounter // RateCounter tracks the number of requests over time for monitoring or throttling.
	RequestsCounter   atomic.Uint64            // RequestsCounter is an atomic counter for total requests sent.
	RequestsLimit     int                      // RequestsLimit is the maximum allowed number of requests within a certain period.
	RequestsRemaining int                      // RequestsRemaining tracks how many requests can still be sent before hitting the limit.
	version           GitLabVersion            // version stores the detected GitLab API version to enable version-aware behavior.
	mutex             sync.RWMutex             // mutex protects concurrent access to mutable shared fields like version and counters.
}

// ClientConfig holds configuration options needed to instantiate a new Client.
type ClientConfig struct {
	URL              string            // Base URL of the GitLab instance
	Token            string            // API token for authentication
	UserAgentVersion string            // User agent string for client identification
	DisableTLSVerify bool              // Whether to skip TLS verification (e.g., for self-signed certs)
	ReadinessURL     string            // URL used for readiness checks
	RateLimiter      ratelimit.Limiter // Optional custom rate limiter implementation
}

// NewHTTPClient creates an HTTP client with optional TLS verification disabling.
// It clones the default transport to preserve proxy settings and other defaults,
// then modifies TLS configuration as requested.
func NewHTTPClient(disableTLSVerify bool) *http.Client {
	// Clone the default transport, which includes proxy and connection settings.
	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Configure TLS to skip verification if requested.
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: disableTLSVerify}

	// Return a new HTTP client using the customized transport.
	return &http.Client{
		Transport: transport,
	}
}

// NewClient creates and returns a new Client instance configured with
// the provided ClientConfig. It initializes the underlying GitLab client,
// sets up the HTTP clients, readiness check, rate limiting, and request counting.
func NewClient(cfg ClientConfig) (*Client, error) {
	// Prepare options for the go-gitlab client:
	// - Use a custom HTTP client that optionally disables TLS verification.
	// - Set the base URL of the GitLab instance.
	// - Disable automatic retries to handle errors explicitly.
	opts := []goGitlab.ClientOptionFunc{
		goGitlab.WithHTTPClient(NewHTTPClient(cfg.DisableTLSVerify)),
		goGitlab.WithBaseURL(cfg.URL),
		goGitlab.WithoutRetries(),
	}

	// Create a new OAuth GitLab client using the provided API token and options.
	gc, err := goGitlab.NewOAuthClient(cfg.Token, opts...)
	if err != nil {
		return nil, err // Return error if client creation fails.
	}

	// Set a custom user agent string combining a base userAgent with the configured version.
	gc.UserAgent = fmt.Sprintf("%s-%s", userAgent, cfg.UserAgentVersion)

	// Create an HTTP client specifically for readiness checks with a 5-second timeout,
	// optionally disabling TLS verification based on config.
	readinessCheckHTTPClient := NewHTTPClient(cfg.DisableTLSVerify)
	readinessCheckHTTPClient.Timeout = 5 * time.Second

	// Construct and return the wrapped Client, including:
	// - the underlying go-gitlab client,
	// - the configured rate limiter,
	// - the readiness check HTTP client and URL,
	// - and a rate counter measuring request rate per second.
	return &Client{
		Client:      gc,
		RateLimiter: cfg.RateLimiter,
		Readiness: struct {
			URL        string
			HTTPClient *http.Client
		}{
			URL:        cfg.ReadinessURL,
			HTTPClient: readinessCheckHTTPClient,
		},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}, nil
}

// ReadinessCheck returns a healthcheck.Check function that performs
// an HTTP GET request to the configured readiness URL to verify if
// the GitLab service is ready to accept requests.
func (c *Client) ReadinessCheck(ctx context.Context) healthcheck.Check {
	// Start a tracing span for observability
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ReadinessCheck")
	defer span.End()

	// Return a closure that performs the actual readiness check when called
	return func() error {
		// Ensure the HTTP client for readiness checks is configured
		if c.Readiness.HTTPClient == nil {
			return fmt.Errorf("readiness http client not configured")
		}

		// Create a new HTTP GET request with the provided context
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			c.Readiness.URL,
			nil,
		)
		if err != nil {
			// Return error if request creation failed
			return err
		}

		// Execute the HTTP request using the readiness HTTP client
		resp, err := c.Readiness.HTTPClient.Do(req)
		if err != nil {
			// Return error if the request failed to execute
			return err
		}

		// Defensive check: response should not be nil
		if resp == nil {
			return fmt.Errorf("HTTP error: empty response")
		}

		// If the response status code is not 200 OK, return an error
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}

		// Return nil indicating readiness check passed successfully
		return nil
	}
}

// rateLimit enforces rate limiting by blocking until a token
// is available from the configured RateLimiter. It also increments
// internal counters for monitoring requests made.
func (c *Client) rateLimit(ctx context.Context) {
	// Start a tracing span for observability
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:rateLimit")
	defer span.End()

	// Block until allowed by the RateLimiter (e.g., token bucket)
	ratelimit.Take(ctx, c.RateLimiter)

	// Increment the rate counter for monitoring the number of requests per second
	c.RateCounter.Incr(1)

	// Increment the atomic requests counter (total requests made)
	c.RequestsCounter.Add(1)
}

// UpdateVersion safely updates the GitLab version stored in the client.
// It locks the mutex for writing to prevent concurrent access issues.
func (c *Client) UpdateVersion(version GitLabVersion) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.version = version
}

// Version safely returns the current GitLab version stored in the client.
// It uses a read lock to allow concurrent readers.
func (c *Client) Version() GitLabVersion {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.version
}

// requestsRemaining parses rate limit headers from the GitLab API response
// and updates the client's fields to track remaining requests and limit.
func (c *Client) requestsRemaining(response *goGitlab.Response) {
	if response == nil {
		return
	}

	// Extract "ratelimit-remaining" header and parse it to int
	if remaining := response.Header.Get("ratelimit-remaining"); remaining != "" {
		c.RequestsRemaining, _ = strconv.Atoi(remaining)
	}

	// Extract "ratelimit-limit" header and parse it to int
	if limit := response.Header.Get("ratelimit-limit"); limit != "" {
		c.RequestsLimit, _ = strconv.Atoi(limit)
	}
}
