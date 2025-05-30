package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

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

// WebhookHandler handles incoming GitLab webhook HTTP requests.
func (c *Controller) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	// Get the tracing span from the request context for observability
	span := trace.SpanFromContext(r.Context())
	defer span.End()

	// Create a new background context with the span,
	// instead of using the request context which may have a short cancellation TTL
	ctx := trace.ContextWithSpan(context.Background(), span)

	// Prepare a logger with context and fields including the remote IP and user agent
	logger := log.
		WithContext(ctx).
		WithFields(log.Fields{
			"ip-address": r.RemoteAddr,
			"user-agent": r.UserAgent(),
		})

	logger.Debug("webhook request received")

	// Validate the webhook secret token from the request header
	if r.Header.Get("X-Gitlab-Token") != c.Config.Server.Webhook.SecretToken {
		logger.Debug("invalid token provided for webhook request")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "{\"error\": \"invalid token\"}")
		return
	}

	// Check if the request body is empty (no content)
	if r.Body == http.NoBody {
		logger.
			WithError(fmt.Errorf("empty request body")).
			Warn("unable to read body of a received webhook")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Read the entire request body payload
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		logger.
			WithError(err).
			Warn("unable to read body of a received webhook")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Parse the webhook event from the payload according to the event type header
	event, err := gitlab.ParseHook(gitlab.HookEventType(r), payload)
	if err != nil {
		logger.
			WithError(err).
			Warn("unable to parse webhook payload")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Handle different types of GitLab webhook events asynchronously in separate goroutines
	switch event := event.(type) {
	case *gitlab.PipelineEvent:
		go c.processPipelineEvent(ctx, *event)
	case *gitlab.JobEvent:
		go c.processJobEvent(ctx, *event)
	case *gitlab.DeploymentEvent:
		go c.processDeploymentEvent(ctx, *event)
	case *gitlab.PushEvent:
		go c.processPushEvent(ctx, *event)
	case *gitlab.TagEvent:
		go c.processTagEvent(ctx, *event)
	case *gitlab.MergeEvent:
		go c.processMergeEvent(ctx, *event)
	default:
		// Log and respond with an error for unsupported event types
		logger.
			WithField("event-type", reflect.TypeOf(event).String()).
			Warn("received unsupported webhook event type")
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
}
