package ratelimit

import (
	"net/http" // Package for HTTP client and server implementations
	"time"     // Package for time-related operations

	"golang.org/x/time/rate" // Package for rate limiting
)

// ThrottledTransport is a custom HTTP transport that implements rate limiting.
type ThrottledTransport struct {
	roundTripper http.RoundTripper // The underlying HTTP transport to use for making requests
	rateLimiter  *rate.Limiter     // The rate limiter to control the rate of requests
}

// RoundTrip implements the RoundTripper interface for ThrottledTransport.
// It ensures that requests are made according to the rate limit.
func (t *ThrottledTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Wait until the rate limiter allows the request to proceed
	err := t.rateLimiter.Wait(req.Context())
	if err != nil {
		// Return an error if the rate limiter fails
		return nil, err
	}

	// Use the underlying round tripper to make the HTTP request
	return t.roundTripper.RoundTrip(req)
}

// NewThrottledTransport creates a new ThrottledTransport with the specified rate limit.
func NewThrottledTransport(limitPeriod time.Duration, requestCount int, transportWrap http.RoundTripper) http.RoundTripper {
	// Create and return a new ThrottledTransport with the specified rate limit
	// Example usage: client := &http.Client{Transport: NewThrottledTransport(10*time.Second, 60, http.DefaultTransport)}
	// This allows 60 requests every 10 seconds.
	return &ThrottledTransport{
		roundTripper: transportWrap,                                          // The underlying transport to use for HTTP requests
		rateLimiter:  rate.NewLimiter(rate.Every(limitPeriod), requestCount), // Create a new rate limiter
	}
}
