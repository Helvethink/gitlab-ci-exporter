package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/taskq/v4"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

func TestControllerCloseWaitsForWebhookWork(t *testing.T) {
	appCtx, cancel := context.WithCancel(context.Background())
	c := &Controller{background: &backgroundTasks{cancel: cancel}}
	h := c.NewWebhookHandler(appCtx).(*webhookHandler)
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	h.process = func(context.Context, any) {
		close(started)
		<-release
		close(finished)
	}
	require.True(t, h.submit(struct{}{}))
	<-started

	closeResult := make(chan error, 1)
	go func() {
		shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		closeResult <- c.Close(shutdownCtx)
	}()
	<-appCtx.Done()
	select {
	case err := <-closeResult:
		t.Fatalf("Close returned while webhook was active: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-closeResult)
	<-finished
	assert.False(t, h.submit(struct{}{}))
	require.NoError(t, c.Close(context.Background()))
}

func TestControllerCloseWaitsForActiveTask(t *testing.T) {
	appCtx, cancel := context.WithCancel(context.Background())
	c := &Controller{
		Store:          store.NewLocalStore(),
		TaskController: NewTaskController(appCtx, nil, 10),
		UUID:           uuid.New(),
		background:     &backgroundTasks{cancel: cancel},
	}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	_, err := c.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(context.Context, string) error {
			close(started)
			<-release
			close(finished)
			return nil
		},
	})
	require.NoError(t, err)
	c.ScheduleTask(appCtx, schemas.TaskTypePullProject, "group/project", "group/project")
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start")
	}

	closeResult := make(chan error, 1)
	go func() {
		shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		closeResult <- c.Close(shutdownCtx)
	}()
	<-appCtx.Done()
	select {
	case err := <-closeResult:
		t.Fatalf("Close returned while task was active: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-closeResult)
	<-finished
}

func TestControllerCloseStopsRedisAndBackgroundWork(t *testing.T) {
	mr := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	appCtx, cancel := context.WithCancel(context.Background())
	c := &Controller{
		Redis:          redisClient,
		Store:          store.NewRedisStore(redisClient),
		TaskController: NewTaskController(appCtx, redisClient, 10),
		UUID:           uuid.New(),
		background:     &backgroundTasks{cancel: cancel},
	}
	require.NoError(t, c.TaskController.Factory.StartConsumers(appCtx))
	c.ScheduleRedisSetKeepalive(appCtx)
	exited := make(chan struct{})
	require.True(t, c.startBackground(func() {
		<-appCtx.Done()
		close(exited)
	}))

	shutdownCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	require.NoError(t, c.Close(shutdownCtx))
	<-exited
	assert.False(t, c.startBackground(func() {}))
	assert.ErrorIs(t, redisClient.Ping(context.Background()).Err(), redis.ErrClosed)
	require.NoError(t, c.Close(context.Background()))
}

type shutdownSpanExporter struct {
	spans   int
	stopped bool
}

func (e *shutdownSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.spans += len(spans)
	return nil
}

func (e *shutdownSpanExporter) Shutdown(context.Context) error {
	e.stopped = true
	return nil
}

func TestControllerCloseFlushesTracer(t *testing.T) {
	exporter := &shutdownSpanExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	c := &Controller{tracerProvider: provider}
	_, span := provider.Tracer("test").Start(context.Background(), "completed")
	span.End()
	require.NoError(t, c.Close(context.Background()))
	assert.True(t, exporter.stopped)
	assert.Equal(t, 1, exporter.spans)
}

func TestControllerCloseHonorsDeadline(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	c := &Controller{background: &backgroundTasks{cancel: cancel}}
	release := make(chan struct{})
	exited := make(chan struct{})
	require.True(t, c.startBackground(func() {
		<-release
		close(exited)
	}))
	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	err := c.Close(shutdownCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	<-exited
	assert.True(t, errors.Is(c.Close(context.Background()), context.DeadlineExceeded))
}
