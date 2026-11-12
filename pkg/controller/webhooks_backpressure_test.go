package controller

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

const whPipelineMainBody = `{"object_kind":"pipeline","object_attributes":{"id":1,"ref":"main","tag":false},` +
	`"project":{"id":123,"path_with_namespace":"group/project"}}`

type whCtxKey string

// whWithLifecycle gives the controller the same lifecycle ownership as New:
// a lifecycle context cancelled by Close and a background work tracker.
func whWithLifecycle(c *Controller) context.Context {
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	c.background = &backgroundTasks{ctx: lifecycleCtx, cancel: cancel}
	return lifecycleCtx
}

func whWebhookConfig(maxConcurrent int) config.Config {
	return config.Config{Server: config.Server{Webhook: config.ServerWebhook{
		SecretToken:                 whSecretToken,
		MaximumConcurrentProcessing: maxConcurrent,
	}}}
}

func TestWebhookBackpressureRejectsBeyondCapacityAndReleasesSlots(t *testing.T) {
	const capacity = 3
	ctx := context.Background()
	c := &Controller{Config: whWebhookConfig(capacity)}
	h := c.NewWebhookHandler(ctx).(*webhookHandler)

	var calls atomic.Int32
	started := make(chan struct{}, 16)
	release := make(chan struct{})
	h.process = func(context.Context, any) {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}
	post := func() int { return whPostWebhook(ctx, h, "Pipeline Hook", whPipelineMainBody).Code }

	for range capacity {
		require.Equal(t, http.StatusOK, post())
	}
	for range capacity {
		<-started
	}

	rr := whPostWebhook(ctx, h, "Pipeline Hook", whPipelineMainBody)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Equal(t, "1", rr.Header().Get("Retry-After"))
	assert.EqualValues(t, capacity, calls.Load(), "the rejected event must not be processed")
	assert.Len(t, h.processingSlots, capacity, "a rejected event must not hold a slot")

	// Finishing one event frees exactly one slot.
	release <- struct{}{}
	require.Eventually(t, func() bool { return post() == http.StatusOK }, 2*time.Second, 5*time.Millisecond)
	<-started
	assert.Equal(t, http.StatusServiceUnavailable, post())

	// Once everything finished, the full capacity is available again.
	close(release)
	require.Eventually(t, func() bool { return len(h.processingSlots) == 0 }, 2*time.Second, 5*time.Millisecond)
	for range capacity {
		require.Equal(t, http.StatusOK, post())
	}
	require.Eventually(t, func() bool { return len(h.processingSlots) == 0 }, 2*time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 2*capacity+1, calls.Load())
}

func TestWebhookHandlerDefaultLimits(t *testing.T) {
	c := &Controller{}
	h := c.NewWebhookHandler(context.Background()).(*webhookHandler)

	assert.Equal(t, defaultWebhookMaximumConcurrentProcessing, cap(h.processingSlots))
	assert.EqualValues(t, defaultWebhookMaximumBodyBytes, h.maximumBodyBytes)
}

func TestWebhookProcessingSurvivesRequestCancellation(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, nil)
	f.c.Config.Server.Webhook.SecretToken = whSecretToken
	whWithLifecycle(f.c)
	t.Cleanup(func() { _ = f.c.Close(context.Background()) })
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))

	h := f.c.NewWebhookHandler(ctx).(*webhookHandler)
	realProcess := h.process
	gate := make(chan struct{})
	h.process = func(ctx context.Context, event any) {
		<-gate
		realProcess(ctx, event)
	}

	reqCtx, cancelRequest := context.WithCancel(ctx)
	rr := whPostWebhook(reqCtx, h, "Pipeline Hook", whPipelineMainBody)
	require.Equal(t, http.StatusOK, rr.Code)

	// The client goes away before processing even starts.
	cancelRequest()
	close(gate)

	ref := whRef(whProject, schemas.RefKindBranch, "main")
	require.Eventually(t, func() bool {
		return len(f.store.tasks()) == 1 && whRefExists(t, f.c.Store, ref)
	}, 2*time.Second, 5*time.Millisecond)
	// ScheduleTask drops tasks when its context is cancelled: a queued task
	// proves processing did not inherit the request context.
	assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullRefMetrics, ID: string(ref.Key())}}, f.store.tasks())
}

func TestWebhookProcessingUsesApplicationContext(t *testing.T) {
	appCtx, cancelApp := context.WithCancel(context.WithValue(context.Background(), whCtxKey("scope"), "application"))
	defer cancelApp()
	c := &Controller{Config: whWebhookConfig(1)}
	h := c.NewWebhookHandler(appCtx).(*webhookHandler)

	processingCtx := make(chan context.Context, 1)
	h.process = func(ctx context.Context, _ any) {
		processingCtx <- ctx
		<-ctx.Done()
	}

	reqCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), whCtxKey("request"), "yes"))
	require.Equal(t, http.StatusOK, whPostWebhook(reqCtx, h, "Pipeline Hook", whPipelineMainBody).Code)
	cancelRequest()

	ctx := <-processingCtx
	assert.Equal(t, "application", ctx.Value(whCtxKey("scope")))
	assert.Nil(t, ctx.Value(whCtxKey("request")), "processing must not derive from the request context")
	assert.NoError(t, ctx.Err(), "request cancellation must not cancel processing")

	cancelApp()
	require.Eventually(t, func() bool { return ctx.Err() != nil }, time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return len(h.processingSlots) == 0 }, time.Second, time.Millisecond)
}

func TestWebhookCloseWaitsForInFlightEventAndRejectsNewOnes(t *testing.T) {
	c := &Controller{Config: whWebhookConfig(4)}
	lifecycleCtx := whWithLifecycle(c)
	h := c.NewWebhookHandler(context.Background()).(*webhookHandler)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	h.process = func(context.Context, any) {
		if calls.Add(1) > 1 {
			t.Error("no event may start after Close")
			return
		}
		close(started)
		<-release // deliberately ignores cancellation
		close(finished)
	}

	require.Equal(t, http.StatusOK, whPostWebhook(context.Background(), h, "Pipeline Hook", whPipelineMainBody).Code)
	<-started

	closeResult := make(chan error, 1)
	go func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		closeResult <- c.Close(shutdownCtx)
	}()
	<-lifecycleCtx.Done() // intake is closed before the lifecycle is cancelled

	rr := whPostWebhook(context.Background(), h, "Pipeline Hook", whPipelineMainBody)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Equal(t, "1", rr.Header().Get("Retry-After"))
	assert.Len(t, h.processingSlots, 1, "only the in-flight event holds a slot")

	select {
	case err := <-closeResult:
		t.Fatalf("Close returned while a webhook was in flight: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-closeResult)
	select {
	case <-finished:
	default:
		t.Fatal("Close returned before the in-flight event finished")
	}

	assert.Equal(t, http.StatusServiceUnavailable, whPostWebhook(context.Background(), h, "Pipeline Hook", whPipelineMainBody).Code)
	assert.EqualValues(t, 1, calls.Load())
	assert.Empty(t, h.processingSlots)
}

func TestWebhookCloseCancelsInFlightGitLabCall(t *testing.T) {
	ctx := context.Background()
	gitlabCalled := make(chan struct{}, 1)
	unblock := make(chan struct{})
	f := whNewFixture(t, func(w http.ResponseWriter, r *http.Request) {
		gitlabCalled <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-unblock:
		}
		http.Error(w, "too late", http.StatusGatewayTimeout)
	})
	t.Cleanup(func() { close(unblock) })
	f.c.Config.Server.Webhook.SecretToken = whSecretToken
	whWithLifecycle(f.c)
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))
	h := f.c.NewWebhookHandler(ctx)

	rr := whPostWebhook(ctx, h, "Job Hook", `{"object_kind":"build","ref":"main","tag":false,"project_id":123}`)
	require.Equal(t, http.StatusOK, rr.Code)
	<-gitlabCalled

	shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	require.NoError(t, f.c.Close(shutdownCtx), "cancellation must let the worker finish within the deadline")
	assert.Less(t, time.Since(start), time.Second)

	assert.Empty(t, f.store.tasks(), "an aborted event must not schedule work")
	count, err := f.c.Store.RefsCount(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestWebhookCloseHonorsDeadlineForWorkIgnoringCancellation(t *testing.T) {
	c := &Controller{Config: whWebhookConfig(2)}
	whWithLifecycle(c)
	h := c.NewWebhookHandler(context.Background()).(*webhookHandler)

	processingCtx := make(chan context.Context, 1)
	release := make(chan struct{})
	exited := make(chan struct{})
	h.process = func(ctx context.Context, _ any) {
		processingCtx <- ctx
		<-release
		close(exited)
	}
	require.Equal(t, http.StatusOK, whPostWebhook(context.Background(), h, "Pipeline Hook", whPipelineMainBody).Code)
	ctx := <-processingCtx

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.Close(shutdownCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorIs(t, ctx.Err(), context.Canceled, "outliving work must have been told to stop")

	close(release)
	<-exited
	require.Eventually(t, func() bool { return len(h.processingSlots) == 0 }, time.Second, time.Millisecond)
}

func TestWebhookEachEventRunsInItsOwnSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	ctx := context.Background()
	f := whNewFixture(t, nil)
	f.c.Config.Server.Webhook.SecretToken = whSecretToken
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))
	h := f.c.NewWebhookHandler(ctx)

	reqCtx, reqSpan := provider.Tracer("wh-test").Start(ctx, "inbound-request")
	bodies := []string{
		whPipelineMainBody,
		`{"object_kind":"pipeline","object_attributes":{"id":2,"ref":"v1.0.0","tag":true},"project":{"id":123,"path_with_namespace":"group/project"}}`,
	}
	for _, body := range bodies {
		require.Equal(t, http.StatusOK, whPostWebhook(reqCtx, h, "Pipeline Hook", body).Code)
	}
	reqSpan.End()

	spansNamed := func(name string) []sdktrace.ReadOnlySpan {
		var out []sdktrace.ReadOnlySpan
		for _, s := range recorder.Ended() {
			if s.Name() == name {
				out = append(out, s)
			}
		}
		return out
	}
	require.Eventually(t, func() bool {
		return len(spansNamed("controller:processWebhookEvent")) == len(bodies) &&
			len(spansNamed("controller:ScheduleTask")) == len(bodies)
	}, 2*time.Second, 5*time.Millisecond)

	eventSpans := spansNamed("controller:processWebhookEvent")
	eventSpanIDs := map[string]bool{}
	traceIDs := map[string]bool{}
	for _, s := range eventSpans {
		eventSpanIDs[s.SpanContext().SpanID().String()] = true
		traceIDs[s.SpanContext().TraceID().String()] = true
		assert.False(t, s.Parent().IsValid(), "event spans are roots, detached from the request span")
		assert.NotEqual(t, reqSpan.SpanContext().TraceID(), s.SpanContext().TraceID())
	}
	assert.Len(t, eventSpanIDs, len(bodies), "one span per event")
	assert.Len(t, traceIDs, len(bodies), "events do not share a trace")

	// The work done for each event is nested under that event's span.
	parents := map[string]bool{}
	for _, s := range spansNamed("controller:ScheduleTask") {
		parent := s.Parent().SpanID().String()
		assert.True(t, eventSpanIDs[parent], "ScheduleTask must be a child of a processWebhookEvent span")
		parents[parent] = true
	}
	assert.Len(t, parents, len(bodies))
}
