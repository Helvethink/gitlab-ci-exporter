package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalLimiter(t *testing.T) {
	l := NewLocalLimiter(10, 2)

	require.NotNil(t, l)

	start := time.Now()
	d, err := l.Take(context.Background())
	require.NoError(t, err)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, d, time.Duration(0))
	assert.Less(t, elapsed, 100*time.Millisecond)
}

func TestLocalTake_RespectsRateLimit(t *testing.T) {
	l := NewLocalLimiter(5, 1) // 1 token every 200ms, burst = 1
	ctx := context.Background()

	d1, err := l.Take(ctx)
	require.NoError(t, err)
	d2, err := l.Take(ctx)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, d1, time.Duration(0))
	assert.Less(t, d1, 50*time.Millisecond)

	assert.GreaterOrEqual(t, d2, 150*time.Millisecond)
	assert.Less(t, d2, 1*time.Second)
}

func TestLocalTake_AllowsBurst(t *testing.T) {
	l := NewLocalLimiter(5, 2) // burst = 2
	ctx := context.Background()

	d1, err := l.Take(ctx)
	require.NoError(t, err)
	d2, err := l.Take(ctx)
	require.NoError(t, err)
	d3, err := l.Take(ctx)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, d1, time.Duration(0))
	assert.Less(t, d1, 50*time.Millisecond)

	assert.GreaterOrEqual(t, d2, time.Duration(0))
	assert.Less(t, d2, 50*time.Millisecond)

	assert.GreaterOrEqual(t, d3, 150*time.Millisecond)
	assert.Less(t, d3, 1*time.Second)
}
