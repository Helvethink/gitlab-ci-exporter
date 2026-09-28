package ratelimit

import (
	"context" // Package for managing context and cancellation
	"time"    // Package for time-related operations

	"golang.org/x/time/rate" // Package for rate limiting functionality
)

// Local represents a local rate limiter using the golang.org/x/time/rate package.
type Local struct {
	*rate.Limiter // Embedded rate limiter from the rate package
}

// NewLocalLimiter creates a new local rate limiter with specified maximum and burstable requests per second.
func NewLocalLimiter(maximumRPS int, burstableRPS int) Limiter {
	// Create and return a new Local rate limiter with the specified rate limits
	return Local{
		Limiter: rate.NewLimiter(rate.Limit(maximumRPS), burstableRPS),
		// maximumRPS: The maximum average rate allowed in requests per second
		// burstableRPS: The maximum burst size allowed
	}
}

// Take attempts to allow an action under the rate limit and returns the duration taken.
func (l Local) Take(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	if err := l.Wait(ctx); err != nil {
		return time.Since(start), err
	}
	return time.Since(start), nil
}
