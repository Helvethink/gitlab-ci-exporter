package ratelimit

import (
	"context" // Package for managing context and cancellation
	"time"    // Package for time-related operations
)

// Limiter is an interface for rate limiting functionality.
// It defines a method for taking a rate-limited action.
type Limiter interface {
	Take(ctx context.Context) time.Duration
	// Take attempts to allow an action under the rate limit and returns the duration taken.
	// It blocks until the action is allowed or the context is canceled.
}

// Take is a helper function that calls the Take method on a Limiter.
// It is used to apply rate limiting to an operation.
func Take(ctx context.Context, l Limiter) {
	// Call the Take method on the provided Limiter to enforce rate limiting.
	l.Take(ctx)
}
