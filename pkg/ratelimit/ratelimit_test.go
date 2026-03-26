package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockLimiter struct {
	called bool
	ctx    context.Context
	delay  time.Duration
}

func (m *mockLimiter) Take(ctx context.Context) time.Duration {
	m.called = true
	m.ctx = ctx
	return m.delay
}

func TestTake(t *testing.T) {
	ctx := context.Background()
	m := &mockLimiter{
		delay: 10 * time.Millisecond,
	}

	Take(ctx, m)

	assert.True(t, m.called)
	assert.Equal(t, ctx, m.ctx)
}
