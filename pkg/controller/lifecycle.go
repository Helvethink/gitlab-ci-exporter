package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
)

type backgroundTasks struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	cancel context.CancelFunc
	closed bool
}

func (b *backgroundTasks) start(fn func()) bool {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return false
	}
	b.wg.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.wg.Done()
		fn()
	}()
	return true
}

func (b *backgroundTasks) stop() {
	b.mu.Lock()
	b.closed = true
	if b.cancel != nil {
		b.cancel()
	}
	b.mu.Unlock()
}

func (b *backgroundTasks) wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for background work: %w", ctx.Err())
	}
}

func (c *Controller) startBackground(fn func()) bool {
	if c.background == nil {
		go fn()
		return true
	}
	return c.background.start(fn)
}

// Close stops background work and closes the task queue, Redis client, and tracer.
func (c *Controller) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c.closeOnce.Do(func() {
		c.closeErr = c.close(shutdownCtx)
	})
	return c.closeErr
}

func (c *Controller) close(ctx context.Context) error {
	if c.background != nil {
		c.background.stop()
	}
	var shutdownErr error
	if c.TaskController.Queue != nil {
		done := make(chan error, 1)
		go func() {
			if queue, ok := c.TaskController.Queue.(interface{ CloseTimeout(time.Duration) error }); ok {
				timeout := 5 * time.Second
				if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
					timeout = min(timeout, max(0, time.Until(deadline)))
				}
				done <- queue.CloseTimeout(timeout)
				return
			}
			done <- c.TaskController.Queue.Close()
		}()
		select {
		case err := <-done:
			shutdownErr = errors.Join(shutdownErr, err)
		case <-ctx.Done():
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close task queue: %w", ctx.Err()))
		}
	}
	if c.background != nil {
		shutdownErr = errors.Join(shutdownErr, c.background.wait(ctx))
	}
	if c.Gitlab != nil {
		c.Gitlab.CloseIdleConnections()
	}
	if c.Redis != nil {
		shutdownErr = errors.Join(shutdownErr, c.Redis.Close())
	}
	if c.tracerProvider != nil {
		if c.previousTracerProvider != nil {
			otel.SetTracerProvider(c.previousTracerProvider)
		}
		shutdownErr = errors.Join(shutdownErr, c.tracerProvider.Shutdown(ctx))
	}
	return shutdownErr
}
