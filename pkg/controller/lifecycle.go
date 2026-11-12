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
	ctx    context.Context // lifecycle context cancelled by stop; nil when not owned
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

// bindLifecycle returns a context cancelled when either ctx or the controller's
// lifecycle context is done, so work started from externally supplied contexts
// still stops when Close is called.
func (c *Controller) bindLifecycle(ctx context.Context) context.Context {
	if c.background == nil || c.background.ctx == nil {
		return ctx
	}
	bound, cancel := context.WithCancel(ctx)
	context.AfterFunc(c.background.ctx, cancel)
	return bound
}

// Drain stops producers and drains in-flight work: it stops the schedulers and
// webhook intake, closes the task queue (stopping its consumers), and waits for
// background work to finish. It does not close clients, so servers that read the
// store (such as the monitoring server) can keep serving until they are stopped.
// Drain is idempotent; later calls return the first result.
func (c *Controller) Drain(ctx context.Context) error {
	if c == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c.drainOnce.Do(func() {
		c.drainErr = c.drain(shutdownCtx)
	})
	return c.drainErr
}

// Close drains work if Drain was not called, then closes the GitLab, Redis,
// and tracing clients, flushing pending spans. Close is idempotent.
func (c *Controller) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c.closeOnce.Do(func() {
		c.closeErr = errors.Join(c.Drain(shutdownCtx), c.closeClients(shutdownCtx))
	})
	return c.closeErr
}

func (c *Controller) drain(ctx context.Context) error {
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
	return shutdownErr
}

func (c *Controller) closeClients(ctx context.Context) error {
	var shutdownErr error
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
