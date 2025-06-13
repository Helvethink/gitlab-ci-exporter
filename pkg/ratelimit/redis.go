package ratelimit

import (
	"context"
	"time"

	"github.com/go-redis/redis_rate/v10" // Redis rate limiting library
	"github.com/redis/go-redis/v9"       // Redis client library
	log "github.com/sirupsen/logrus"     // Logging library
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
func (r Redis) Take(ctx context.Context) time.Duration {
	start := time.Now() // Record the start time

	// Loop until a request is allowed
	for {
		// Check if a request is allowed under the rate limit
		res, err := r.Allow(ctx, redisKey, redis_rate.PerSecond(r.MaxRPS))
		if err != nil {
			// Log a fatal error if there is an issue with the rate limiter
			log.WithContext(ctx).
				WithError(err).
				Fatal()
		}

		// If the request is allowed, break out of the loop
		if res.Allowed > 0 {
			break
		} else {
			// Log a debug message indicating that the request is being throttled
			log.WithFields(
				log.Fields{
					"for": res.RetryAfter.String(), // Duration to wait before retrying
				},
			).Debug("throttled GitLab requests")

			// Sleep for the duration specified by the rate limiter
			time.Sleep(res.RetryAfter)
		}
	}

	// Return the duration taken to allow the request
	return time.Until(start)
}
