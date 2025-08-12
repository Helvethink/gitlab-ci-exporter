package controller

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/taskq/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"google.golang.org/grpc"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/ratelimit"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

const tracerName = "gitlab-ci-pipelines-exporter"

// Controller holds the necessary clients and components to run the application and handle its operations.
// It includes configuration, connections to Redis, GitLab client, storage interface, and task management.
// The UUID field uniquely identifies this controller instance, especially useful in clustered deployments
// where multiple exporter instances share Redis.
type Controller struct {
	Config         config.Config  // Application configuration settings
	Redis          *redis.Client  // Redis client for caching and coordination
	Gitlab         *gitlab.Client // GitLab API client
	Store          store.Store    // Storage interface to persist data (backed by Redis)
	TaskController TaskController // Manages background tasks and job queues

	// UUID uniquely identifies this controller instance among others when running
	// in clustered mode, facilitating coordination via Redis.
	UUID uuid.UUID
}

// New creates and initializes a new Controller instance.
// It sets up tracing, Redis connection, task controller, storage, GitLab client, and starts the scheduler.
//
// Parameters:
// - ctx: Context for cancellation and deadlines.
// - cfg: Configuration object with all needed parameters.
// - version: Version string of the running application (used in GitLab client configuration).
//
// Returns:
// - c: Initialized Controller instance.
// - err: Any error encountered during setup.
func New(ctx context.Context, cfg config.Config, version string) (c Controller, err error) {
	c.Config = cfg      // Store configuration
	c.UUID = uuid.New() // Generate a new UUID for this controller instance

	// Configure distributed tracing if an OpenTelemetry gRPC endpoint is specified
	if err = configureTracing(ctx, cfg.OpenTelemetry.GRPCEndpoint); err != nil {
		return
	}

	// Initialize Redis connection with provided URL
	if err = c.configureRedis(ctx, cfg.Redis.URL); err != nil {
		return
	}

	// Create a task controller to manage job queues with a maximum size from config
	c.TaskController = NewTaskController(ctx, c.Redis, cfg.Gitlab.MaximumJobsQueueSize)
	c.registerTasks() // Register the tasks that the controller can run

	// Initialize the storage interface which will use Redis and configured projects
	c.Store = store.New(ctx, c.Redis, c.Config.Projects)

	// Configure GitLab client, passing the app version for client identification
	if err = c.configureGitlab(cfg.Gitlab, version); err != nil {
		return
	}

	// Start background schedulers for pulling data and garbage collection based on config
	c.Schedule(ctx, cfg.Pull, cfg.GarbageCollect)

	return
}

// registerTasks registers all task handlers with the TaskController's task map.
// It iterates over a map where each key is a task type and each value is the corresponding handler method.
// For each task type, it registers the handler with a retry limit of 1, meaning the task will be retried once on failure.
// This setup enables the Controller to handle various asynchronous tasks related to garbage collection, pulling metrics, environments, projects, and refs.
func (c *Controller) registerTasks() {
	for n, h := range map[schemas.TaskType]interface{}{
		schemas.TaskTypeGarbageCollectEnvironments:   c.TaskHandlerGarbageCollectEnvironments,
		schemas.TaskTypeGarbageCollectMetrics:        c.TaskHandlerGarbageCollectMetrics,
		schemas.TaskTypeGarbageCollectProjects:       c.TaskHandlerGarbageCollectProjects,
		schemas.TaskTypeGarbageCollectRefs:           c.TaskHandlerGarbageCollectRefs,
		schemas.TaskTypeGarbageCollectRunners:        c.TaskHandlerGarbageCollectRunners,
		schemas.TaskTypePullEnvironmentMetrics:       c.TaskHandlerPullEnvironmentMetrics,
		schemas.TaskTypePullEnvironmentsFromProject:  c.TaskHandlerPullEnvironmentsFromProject,
		schemas.TaskTypePullEnvironmentsFromProjects: c.TaskHandlerPullEnvironmentsFromProjects,
		schemas.TaskTypePullMetrics:                  c.TaskHandlerPullMetrics,
		schemas.TaskTypePullProject:                  c.TaskHandlerPullProject,
		schemas.TaskTypePullProjectsFromWildcard:     c.TaskHandlerPullProjectsFromWildcard,
		schemas.TaskTypePullProjectsFromWildcards:    c.TaskHandlerPullProjectsFromWildcards,
		schemas.TaskTypePullRefMetrics:               c.TaskHandlerPullRefMetrics,
		schemas.TaskTypePullRefsFromProject:          c.TaskHandlerPullRefsFromProject,
		schemas.TaskTypePullRefsFromProjects:         c.TaskHandlerPullRefsFromProjects,
		schemas.TaskTypePullRunnersMetrics:           c.TaskHandlerPullRunnerMetrics,
		schemas.TaskTypePullRunnersFromProject:       c.TaskHandlerPullRunnersFromProject,
		schemas.TaskTypePullRunnersFromProjects:      c.TaskHandlerPullRunnersFromProjects,
	} {
		_, _ = c.TaskController.TaskMap.Register(string(n), &taskq.TaskConfig{
			Handler:    h,
			RetryLimit: 1,
		})
	}
}

// unqueueTask attempts to remove a task identified by its type and unique ID from the task queue in the store.
// If the operation fails, it logs a warning with the task details and the error encountered.
// This helps ensure that tasks are properly cleaned up from the queue to avoid duplicate processing or stale tasks.
func (c *Controller) unqueueTask(ctx context.Context, tt schemas.TaskType, uniqueID string) {
	if err := c.Store.UnqueueTask(ctx, tt, uniqueID); err != nil {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"task_type":      tt,
				"task_unique_id": uniqueID,
			}).
			WithError(err).
			Warn("unqueuing task")
	}
}

// configureTracing sets up OpenTelemetry tracing via a gRPC endpoint.
// If no endpoint is provided, tracing support is skipped.
func configureTracing(ctx context.Context, grpcEndpoint string) error {
	// If no gRPC endpoint is specified, log that tracing will be skipped and return nil
	if len(grpcEndpoint) == 0 {
		log.Debug("open-telemetry.grpc_endpoint is not configured, skipping open telemetry support")
		return nil
	}

	// Log that a gRPC endpoint is configured and tracing initialization is starting
	log.WithFields(log.Fields{
		"open-telemetry_grpc_endpoint": grpcEndpoint,
	}).Info("open-telemetry gRPC endpoint provided, initializing connection..")

	// Create a new OpenTelemetry gRPC trace client with insecure connection, connecting to the given endpoint,
	// and block until the connection is established
	traceClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(grpcEndpoint),
		otlptracegrpc.WithDialOption(grpc.WithBlock()), // nolint: staticcheck
	)

	// Create a new trace exporter using the gRPC trace client
	traceExp, err := otlptrace.New(ctx, traceClient)
	if err != nil {
		// Return error if exporter creation fails
		return err
	}

	// Create a resource describing this application with metadata from environment,
	// process info, telemetry SDK, host info, and set the service name attribute
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("gitlab-ci-pipelines-exporter"),
		),
	)
	if err != nil {
		// Return error if resource creation fails
		return err
	}

	// Create a batch span processor to buffer and send spans efficiently to the exporter
	bsp := sdktrace.NewBatchSpanProcessor(traceExp)

	// Create a tracer provider configured to always sample all traces,
	// associate the resource metadata, and use the batch span processor
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// Set the global tracer provider so it will be used by the OpenTelemetry API
	otel.SetTracerProvider(tracerProvider)

	// Return nil to indicate successful setup
	return nil
}

// configureGitlab initializes the GitLab client with the given configuration and version.
// It sets up a rate limiter using Redis if available, otherwise uses a local rate limiter.
func (c *Controller) configureGitlab(cfg config.Gitlab, version string) (err error) {
	var rl ratelimit.Limiter

	// If Redis client is available, create a Redis-based rate limiter
	if c.Redis != nil {
		rl = ratelimit.NewRedisLimiter(c.Redis, cfg.MaximumRequestsPerSecond)
	} else {
		// Otherwise, create a local in-memory rate limiter with configured limits
		rl = ratelimit.NewLocalLimiter(cfg.MaximumRequestsPerSecond, cfg.BurstableRequestsPerSecond)
	}

	// Create a new GitLab client with the provided configuration parameters:
	// - URL to the GitLab API
	// - Personal Access Token or OAuth token for authentication
	// - Option to disable TLS verification if specified
	// - User-Agent header version string to identify the client
	// - The configured rate limiter (Redis or local)
	// - Optional readiness/health check URL
	c.Gitlab, err = gitlab.NewClient(gitlab.ClientConfig{
		URL:              cfg.URL,
		Token:            cfg.Token,
		DisableTLSVerify: !cfg.EnableTLSVerify,
		UserAgentVersion: version,
		RateLimiter:      rl,
		ReadinessURL:     cfg.HealthURL,
	})

	// Return any error from GitLab client creation
	return
}

// configureRedis initializes the Redis client using the provided URL and sets up OpenTelemetry tracing instrumentation.
// It returns an error if any step of the configuration or connection fails.
func (c *Controller) configureRedis(ctx context.Context, url string) (err error) {
	// Start a new OpenTelemetry trace span for monitoring this function
	ctx, span := otel.Tracer(tracerName).Start(ctx, "controller:configureRedis")
	defer span.End()

	// If no Redis URL is provided, skip Redis configuration and use local (in-memory) alternatives
	if len(url) <= 0 {
		log.Debug("redis url is not configured, skipping configuration & using local driver")
		return
	}

	log.Info("redis url configured, initializing connection..")

	var opt *redis.Options

	// Parse the Redis URL into options; return early on error
	if opt, err = redis.ParseURL(url); err != nil {
		return
	}

	// Create a new Redis client instance with the parsed options
	c.Redis = redis.NewClient(opt)

	// Instrument the Redis client with OpenTelemetry tracing for monitoring Redis operations
	if err = redisotel.InstrumentTracing(c.Redis); err != nil {
		return
	}

	// Test the Redis connection by sending a PING command; wrap any error with context
	if _, err := c.Redis.Ping(ctx).Result(); err != nil {
		return errors.Wrap(err, "connecting to redis")
	}

	log.Info("connected to redis")

	// Return nil error on successful initialization
	return
}
