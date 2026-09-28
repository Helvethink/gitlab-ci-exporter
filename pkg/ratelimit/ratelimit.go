package ratelimit

import (
	"context" // Package for managing context and cancellation
	"time"    // Package for time-related operations
)

// Limiter is an interface for rate limiting functionality.
// It defines a method for taking a rate-limited action.
type Limiter interface {
	Take(ctx context.Context) (time.Duration, error)
	// Take attempts to allow an action under the rate limit and returns the duration taken.
	// It blocks until the action is allowed, the context is canceled, or the backend fails.
}

// Take is a helper function that calls the Take method on a Limiter.
// It is used to apply rate limiting to an operation.
func Take(ctx context.Context, l Limiter) (time.Duration, error) {
	return l.Take(ctx)
}
