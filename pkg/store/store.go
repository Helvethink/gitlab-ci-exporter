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
	SetProject(ctx context.Context, p schemas.Project) error                // SetProject Stores a project
	DelProject(ctx context.Context, pk schemas.ProjectKey) error            // DelProject Deletes a project
	GetProject(ctx context.Context, p *schemas.Project) error               // GetProject Retrieves a project
	ProjectExists(ctx context.Context, pk schemas.ProjectKey) (bool, error) // ProjectExists Checks the existence of a project
	Projects(ctx context.Context) (schemas.Projects, error)                 // Projects Retrieves all projects
	ProjectsCount(ctx context.Context) (int64, error)                       // ProjectsCount Counts the number of projects

	// Methods for manipulating environments
	SetEnvironment(ctx context.Context, e schemas.Environment) error                // SetEnvironment Stores an environment
	DelEnvironment(ctx context.Context, ek schemas.EnvironmentKey) error            // DelEnvironment Deletes an environment
	GetEnvironment(ctx context.Context, e *schemas.Environment) error               // GetEnvironment Retrieves an environment
	EnvironmentExists(ctx context.Context, ek schemas.EnvironmentKey) (bool, error) // EnvironmentExists Checks the existence of an environment
	Environments(ctx context.Context) (schemas.Environments, error)                 // Environments Retrieves all environments
	EnvironmentsCount(ctx context.Context) (int64, error)                           // EnvironmentsCount Counts the number of environments

	// Methods for manipulating runners
	SetRunner(ctx context.Context, r schemas.Runner) error                // SetRunner Stores an environment
	DelRunner(ctx context.Context, rk schemas.RunnerKey) error            // DelRunner Deletes an environment
	GetRunner(ctx context.Context, r *schemas.Runner) error               // GetRunner Retrieves an environment
	RunnerExists(ctx context.Context, rk schemas.RunnerKey) (bool, error) // RunnerExists Checks the existence of an environment
	Runners(ctx context.Context) (schemas.Runners, error)                 // Environments Retrieves all environments
	RunnersCount(ctx context.Context) (int64, error)                      // RunnersCount Counts the number of environments

	// Methods for manipulating references
	SetRef(ctx context.Context, r schemas.Ref) error                // SetRef Stores a reference
	DelRef(ctx context.Context, rk schemas.RefKey) error            // DelRef Deletes a reference
	GetRef(ctx context.Context, r *schemas.Ref) error               // GetRef Retrieves a reference
	RefExists(ctx context.Context, rk schemas.RefKey) (bool, error) // RefExists Checks the existence of a reference
	Refs(ctx context.Context) (schemas.Refs, error)                 // Refs Retrieves all references
	RefsCount(ctx context.Context) (int64, error)                   // RefsCount Counts the number of references

	// Methods for manipulating metrics
	SetMetric(ctx context.Context, m schemas.Metric) error                // SetMetric Stores a metric
	DelMetric(ctx context.Context, mk schemas.MetricKey) error            // DelMetric Deletes a metric
	GetMetric(ctx context.Context, m *schemas.Metric) error               // GetMetric Retrieves a metric
	MetricExists(ctx context.Context, mk schemas.MetricKey) (bool, error) // MetricExists Checks the existence of a metric
	Metrics(ctx context.Context) (schemas.Metrics, error)                 // Metrics Retrieves all metrics
	MetricsCount(ctx context.Context) (int64, error)                      // MetricsCount Counts the number of metrics

	// Methods for manipulating Pipelines
	SetPipeline(ctx context.Context, pipeline schemas.Pipeline) error                            // SetPipeline sets a pipeline in the storage.
	GetPipeline(ctx context.Context, pipeline *schemas.Pipeline) error                           // GetPipeline retrieves a pipeline from the storage.
	PipelineExists(ctx context.Context, key schemas.PipelineKey) (bool, error)                   // SetPipelineVariables sets the variables for a pipeline in the storage.
	SetPipelineVariables(ctx context.Context, pipeline schemas.Pipeline, variables string) error // PipelineExists checks if a pipeline exists in the storage.
	GetPipelineVariables(ctx context.Context, pipeline schemas.Pipeline) (string, error)         // GetPipelineVariables retrieves the variables for a pipeline from the storage.
	PipelineVariablesExists(ctx context.Context, pipeline schemas.Pipeline) (bool, error)        // PipelineVariablesExists checks if variables for a pipeline exist in the storage.

	// Helpers to keep track of currently queued tasks and avoid scheduling them
	// twice at the risk of ending up with loads of dangling goroutines being locked
	QueueTask(ctx context.Context, tt schemas.TaskType, taskUUID, processUUID string) (bool, error) // QueueTask Adds a task to the queue
	UnqueueTask(ctx context.Context, tt schemas.TaskType, taskUUID string) error                    // UnqueueTask Removes a task from the queue
	CurrentlyQueuedTasksCount(ctx context.Context) (uint64, error)                                  // CurrentlyQueuedTasksCount Counts the number of currently queued tasks
	ExecutedTasksCount(ctx context.Context) (uint64, error)                                         // ExecutedTasksCount Counts the number of executed tasks
}

// NewLocalStore creates a new instance of local storage.
func NewLocalStore() Store {
	return &Local{
		projects:          make(schemas.Projects),               // Initializes a collection of projects
		environments:      make(schemas.Environments),           // Initializes a collection of environments
		runners:           make(schemas.Runners),                // Initializes a collection of Runners
		refs:              make(schemas.Refs),                   // Initializes a collection of references
		metrics:           make(schemas.Metrics),                // initalizes a collection of metrics
		pipelines:         make(schemas.Pipelines),              // Initialise a collection of pipleines
		pipelineVariables: make(map[schemas.PipelineKey]string), // Initializes a collection of pipelines variables
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
