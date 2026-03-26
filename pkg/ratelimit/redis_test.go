package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRedisLimiter(t *testing.T, maxRPS int) *Redis {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	l, ok := NewRedisLimiter(client, maxRPS).(Redis)
	require.True(t, ok)

	return &l
}

func TestNewRedisLimiter(t *testing.T) {
	l := newTestRedisLimiter(t, 5)

	require.NotNil(t, l)
	require.NotNil(t, l.Limiter)
	assert.Equal(t, 5, l.MaxRPS)
}

func TestRedisTake_FirstCallAllowed(t *testing.T) {
	l := newTestRedisLimiter(t, 10)

	d := l.Take(context.Background())

	assert.GreaterOrEqual(t, d, time.Duration(0))
	assert.Less(t, d, 200*time.Millisecond)
}

func TestRedisTake_SecondCallIsRateLimited(t *testing.T) {
	l := newTestRedisLimiter(t, 1)

	ctx := context.Background()

	d1 := l.Take(ctx)
	d2 := l.Take(ctx)

	assert.GreaterOrEqual(t, d1, time.Duration(0))
	assert.Less(t, d1, 200*time.Millisecond)

	assert.GreaterOrEqual(t, d2, 900*time.Millisecond)
	assert.Less(t, d2, 2*time.Second)
}
