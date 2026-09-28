package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis_rate/v10" // Redis rate limiting library
	"github.com/redis/go-redis/v9"       // Redis client library
)

const redisKey string = `gcpe:gitlab:api` // Redis key used for rate limiting

// Redis represents a rate limiter using Redis.
type Redis struct {
	*redis_rate.Limiter     // Embedded Redis rate limiter
	MaxRPS              int // Maximum requests per second allowed
}

// NewRedisLimiter creates a new Redis-based rate limiter.
func NewRedisLimiter(redisClient *redis.Client, maxRPS int) Limiter {
	// Create and return a new Redis rate limiter with the given Redis client and maximum requests per second
	return Redis{
		Limiter: redis_rate.NewLimiter(redisClient), // Initialize the Redis rate limiter
		MaxRPS:  maxRPS,                             // Set the maximum requests per second
	}
}

// Take attempts to allow a request under the rate limit and blocks until allowed.
const (
	redisLimitAttempts = 3
	redisLimitBackoff  = 100 * time.Millisecond
)

func (r Redis) Take(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	failures := 0
	for {
		if err := ctx.Err(); err != nil {
			return time.Since(start), err
		}
		res, err := r.Allow(ctx, redisKey, redis_rate.PerSecond(r.MaxRPS))
		if err != nil {
			failures++
			if failures >= redisLimitAttempts {
				return time.Since(start), fmt.Errorf("redis rate limiter: %w", err)
			}
			if err := wait(ctx, time.Duration(failures)*redisLimitBackoff); err != nil {
				return time.Since(start), err
			}
			continue
		}
		failures = 0
		if res.Allowed > 0 {
			return time.Since(start), nil
		}
		if err := wait(ctx, res.RetryAfter); err != nil {
			return time.Since(start), err
		}
	}
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
