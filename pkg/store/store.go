package store

import (
	"context"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// Store is an interface that defines methods for interacting with storage.
// It includes methods for manipulating projects, environments, references, and metrics.
type Store interface {
	// Methods for manipulating projects
	SetProject(ctx context.Context, p schemas.Project) error                // Stores a project
	DelProject(ctx context.Context, pk schemas.ProjectKey) error            // Deletes a project
	GetProject(ctx context.Context, p *schemas.Project) error               // Retrieves a project
	ProjectExists(ctx context.Context, pk schemas.ProjectKey) (bool, error) // Checks the existence of a project
	Projects(ctx context.Context) (schemas.Projects, error)                 // Retrieves all projects
	ProjectsCount(ctx context.Context) (int64, error)                       // Counts the number of projects

	// Methods for manipulating environments
	SetEnvironment(ctx context.Context, e schemas.Environment) error                // Stores an environment
	DelEnvironment(ctx context.Context, ek schemas.EnvironmentKey) error            // Deletes an environment
	GetEnvironment(ctx context.Context, e *schemas.Environment) error               // Retrieves an environment
	EnvironmentExists(ctx context.Context, ek schemas.EnvironmentKey) (bool, error) // Checks the existence of an environment
	Environments(ctx context.Context) (schemas.Environments, error)                 // Retrieves all environments
	EnvironmentsCount(ctx context.Context) (int64, error)                           // Counts the number of environments

	// Methods for manipulating references
	SetRef(ctx context.Context, r schemas.Ref) error                // Stores a reference
	DelRef(ctx context.Context, rk schemas.RefKey) error            // Deletes a reference
	GetRef(ctx context.Context, r *schemas.Ref) error               // Retrieves a reference
	RefExists(ctx context.Context, rk schemas.RefKey) (bool, error) // Checks the existence of a reference
	Refs(ctx context.Context) (schemas.Refs, error)                 // Retrieves all references
	RefsCount(ctx context.Context) (int64, error)                   // Counts the number of references

	// Methods for manipulating metrics
	SetMetric(ctx context.Context, m schemas.Metric) error                // Stores a metric
	DelMetric(ctx context.Context, mk schemas.MetricKey) error            // Deletes a metric
	GetMetric(ctx context.Context, m *schemas.Metric) error               // Retrieves a metric
	MetricExists(ctx context.Context, mk schemas.MetricKey) (bool, error) // Checks the existence of a metric
	Metrics(ctx context.Context) (schemas.Metrics, error)                 // Retrieves all metrics
	MetricsCount(ctx context.Context) (int64, error)                      // Counts the number of metrics

	// Methods for managing queued tasks
	QueueTask(ctx context.Context, tt schemas.TaskType, taskUUID, processUUID string) (bool, error) // Adds a task to the queue
	UnqueueTask(ctx context.Context, tt schemas.TaskType, taskUUID string) error                    // Removes a task from the queue
	CurrentlyQueuedTasksCount(ctx context.Context) (uint64, error)                                  // Counts the number of currently queued tasks
	ExecutedTasksCount(ctx context.Context) (uint64, error)                                         // Counts the number of executed tasks
}

// NewLocalStore creates a new instance of local storage.
func NewLocalStore() Store {
	return &Local{
		projects:     make(schemas.Projects),     // Initializes a collection of projects
		environments: make(schemas.Environments), // Initializes a collection of environments
		refs:         make(schemas.Refs),         // Initializes a collection of references
		metrics:      make(schemas.Metrics),      // Initializes a collection of metrics
	}
}

// NewRedisStore creates a new instance of storage using Redis.
func NewRedisStore(client *redis.Client) Store {
	return &Redis{
		Client: client, // Redis client to interact with the Redis server
	}
}

// New creates a new store and populates it with the provided projects.
func New(
	ctx context.Context,
	r *redis.Client,
	projects config.Projects,
) (s Store) {
	// Initializes an OpenTelemetry span for tracing
	ctx, span := otel.Tracer("gitlab-ci-pipelines-exporter").Start(ctx, "store:New")
	defer span.End()

	// Chooses the type of storage based on the presence of a Redis client
	if r != nil {
		s = NewRedisStore(r) // Uses Redis if a client is provided
	} else {
		s = NewLocalStore() // Uses local storage otherwise
	}

	// Loads all the configured projects into the store
	for _, p := range projects {
		sp := schemas.Project{Project: p}

		exists, err := s.ProjectExists(ctx, sp.Key())
		if err != nil {
			// Logs an error if reading the project fails
			log.WithContext(ctx).
				WithFields(log.Fields{
					"project-name": p.Name,
				}).
				WithError(err).
				Error("reading project from the store")
		}

		if !exists {
			// Stores the project if it does not already exist
			if err = s.SetProject(ctx, sp); err != nil {
				// Logs an error if writing the project fails
				log.WithContext(ctx).
					WithFields(log.Fields{
						"project-name": p.Name,
					}).
					WithError(err).
					Error("writing project in the store")
			}
		}
	}

	return s // Returns the initialized store
}
