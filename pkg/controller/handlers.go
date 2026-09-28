package controller

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultWebhookMaximumBodyBytes            = 1 << 20
	defaultWebhookMaximumConcurrentProcessing = 8
)

type webhookHandler struct {
	applicationContext context.Context
	controller         *Controller
	maximumBodyBytes   int64
	processingSlots    chan struct{}
	process            func(context.Context, any)
}

// HealthCheckHandler creates and returns a health check handler for the controller.
func (c *Controller) HealthCheckHandler(ctx context.Context) (h healthcheck.Handler) {
	// Initialize a new health check handler
	h = healthcheck.NewHandler()

	// If GitLab health checks are enabled in the config, add a readiness check for GitLab connectivity
	if c.Config.Gitlab.EnableHealthCheck {
		h.AddReadinessCheck("gitlab-reachable", c.Gitlab.ReadinessCheck(ctx))
	} else {
		// Otherwise, log a warning indicating that GitLab readiness checks are disabled
		log.WithContext(ctx).
			Warn("GitLab health check has been disabled. Readiness checks won't be operated.")
	}

	if c.Redis != nil {
		h.AddReadinessCheck("redis-available", func() error {
			if !c.redisReady.Load() {
				return fmt.Errorf("redis keepalive unavailable")
			}
			checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := c.Redis.Ping(checkCtx).Err(); err != nil {
				return fmt.Errorf("ping redis: %w", err)
			}
			return nil
		})
	}

	// Return the configured health check handler
	return
}

// MetricsHandler serves the /metrics HTTP endpoint to expose Prometheus metrics.
func (c *Controller) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the request's context and get the tracing span for observability
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)

	// Ensure the span is ended when this handler returns
	defer span.End()

	// Create a new Prometheus metrics registry specific to this request/context
	registry := NewRegistry(ctx)

	// Retrieve all stored metrics from the data store
	metrics, err := c.Store.Metrics(ctx)
	if err != nil {
		// Log an error if metrics retrieval failed
		log.WithContext(ctx).
			WithError(err).
			Error()
	}

	// Export internal metrics such as GitLab and store-related metrics into the registry
	if err := registry.ExportInternalMetrics(ctx, c.Gitlab, c.Store); err != nil {
		// Log a warning if exporting internal metrics failed
		log.WithContext(ctx).
			WithError(err).
			Warn()
	}

	// Export the retrieved metrics into the registry for exposure
	registry.ExportMetrics(metrics)

	// Wrap the Prometheus handler with OpenTelemetry instrumentation,
	// and serve the HTTP response with metrics data
	otelhttp.NewHandler(
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			Registry:          registry,
			EnableOpenMetrics: c.Config.Server.Metrics.EnableOpenmetricsEncoding,
		}),
		"/metrics",
	).ServeHTTP(w, r)
}

// NewWebhookHandler creates a webhook handler with bounded request bodies and
// event-processing concurrency. Accepted events are processed asynchronously
// using the application context, so request cancellation does not abandon them.
func (c *Controller) NewWebhookHandler(ctx context.Context) http.Handler {
	maximumBodyBytes := c.Config.Server.Webhook.MaximumBodyBytes
	if maximumBodyBytes <= 0 {
		maximumBodyBytes = defaultWebhookMaximumBodyBytes
	}

	maximumConcurrentProcessing := c.Config.Server.Webhook.MaximumConcurrentProcessing
	if maximumConcurrentProcessing <= 0 {
		maximumConcurrentProcessing = defaultWebhookMaximumConcurrentProcessing
	}

	h := &webhookHandler{
		applicationContext: ctx,
		controller:         c,
		maximumBodyBytes:   maximumBodyBytes,
		processingSlots:    make(chan struct{}, maximumConcurrentProcessing),
	}
	h.process = h.processWebhookEvent

	return h
}

// ServeHTTP handles incoming GitLab webhook HTTP requests.
func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Prepare a logger with context and fields including the remote IP and user agent
	logger := log.
		WithContext(ctx).
		WithFields(log.Fields{
			"ip-address": r.RemoteAddr,
			"user-agent": r.UserAgent(),
		})

	logger.Debug("webhook request received")

	// Validate the webhook secret token from the request header
	providedToken := []byte(r.Header.Get("X-Gitlab-Token"))
	expectedToken := []byte(h.controller.Config.Server.Webhook.SecretToken)
	if subtle.ConstantTimeCompare(providedToken, expectedToken) != 1 {
		logger.Debug("invalid token provided for webhook request")
		http.Error(w, "{\"error\":\"invalid token\"}", http.StatusForbidden)
		return
	}

	// Limit the body before reading it so an authenticated client cannot cause
	// unbounded memory consumption.
	r.Body = http.MaxBytesReader(w, r.Body, h.maximumBodyBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			logger.WithError(err).Warn("webhook request body exceeds configured limit")
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		logger.
			WithError(err).
			Warn("unable to read body of a received webhook")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(payload) == 0 {
		logger.Warn("unable to read empty body of a received webhook")
		http.Error(w, "empty request body", http.StatusBadRequest)
		return
	}

	// Parse the webhook event from the payload according to the event type header
	event, err := gitlab.ParseHook(gitlab.HookEventType(r), payload)
	if err != nil {
		logger.
			WithError(err).
			Warn("unable to parse webhook payload")
		http.Error(w, "invalid webhook payload", http.StatusBadRequest)
		return
	}
	if !isSupportedWebhookEvent(event) {
		eventType := "<nil>"
		if typ := reflect.TypeOf(event); typ != nil {
			eventType = typ.String()
		}
		logger.
			WithField("event-type", eventType).
			Warn("received unsupported webhook event type")
		http.Error(w, "unsupported webhook event type", http.StatusUnprocessableEntity)
		return
	}

	if !h.submit(event) {
		logger.Warn("webhook processing capacity exhausted")
		w.Header().Set("Retry-After", "1")
		http.Error(w, "webhook processing capacity exhausted", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func isSupportedWebhookEvent(event any) bool {
	switch event.(type) {
	case *gitlab.PipelineEvent,
		*gitlab.JobEvent,
		*gitlab.DeploymentEvent,
		*gitlab.PushEvent,
		*gitlab.TagEvent,
		*gitlab.MergeEvent:
		return true
	default:
		return false
	}
}

func (h *webhookHandler) submit(event any) bool {
	select {
	case h.processingSlots <- struct{}{}:
		go func() {
			defer func() { <-h.processingSlots }()

			ctx, span := otel.Tracer(tracerName).Start(h.applicationContext, "controller:processWebhookEvent")
			defer span.End()

			h.process(ctx, event)
		}()
		return true
	default:
		return false
	}
}

func (h *webhookHandler) processWebhookEvent(ctx context.Context, event any) {
	switch event := event.(type) {
	case *gitlab.PipelineEvent:
		h.controller.processPipelineEvent(ctx, *event)
	case *gitlab.JobEvent:
		h.controller.processJobEvent(ctx, *event)
	case *gitlab.DeploymentEvent:
		h.controller.processDeploymentEvent(ctx, *event)
	case *gitlab.PushEvent:
		h.controller.processPushEvent(ctx, *event)
	case *gitlab.TagEvent:
		h.controller.processTagEvent(ctx, *event)
	case *gitlab.MergeEvent:
		h.controller.processMergeEvent(ctx, *event)
	default:
		eventType := "<nil>"
		if typ := reflect.TypeOf(event); typ != nil {
			eventType = typ.String()
		}
		log.WithContext(ctx).
			WithField("event-type", eventType).
			Warn("received unsupported webhook event type")
	}
}
