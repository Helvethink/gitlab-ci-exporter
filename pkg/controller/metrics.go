package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

// Registry wraps a pointer to prometheus.Registry and manages metric collectors.
type Registry struct {
	*prometheus.Registry // The main Prometheus registry.

	// InternalCollectors holds custom internal application metrics (not user-facing).
	InternalCollectors struct {
		CurrentlyQueuedTasksCount  prometheus.Collector // Number of tasks currently queued.
		EnvironmentsCount          prometheus.Collector // Total number of environments tracked.
		ExecutedTasksCount         prometheus.Collector // Total number of tasks that have been executed.
		GitLabAPIRequestsCount     prometheus.Collector // Total number of GitLab API requests made.
		GitlabAPIRequestsRemaining prometheus.Collector // Number of remaining GitLab API requests (rate limit).
		GitlabAPIRequestsLimit     prometheus.Collector // GitLab API request limit.
		MetricsCount               prometheus.Collector // Number of exported metrics.
		ProjectsCount              prometheus.Collector // Total number of GitLab projects tracked.
		RefsCount                  prometheus.Collector // Total number of Git refs (branches, tags, MRs).
		RunnersCount               prometheus.Collector // Total number of Runners exported.
	}

	// Collectors maps each MetricKind to its Prometheus collector.
	Collectors RegistryCollectors
}

// RegistryCollectors defines a mapping between metric kinds and their Prometheus collectors.
type RegistryCollectors map[schemas.MetricKind]prometheus.Collector

// NewRegistry initializes and returns a new Registry instance with all the necessary collectors registered.
func NewRegistry(ctx context.Context) *Registry {
	r := &Registry{
		Registry: prometheus.NewRegistry(), // Create a new Prometheus registry instance.

		// Initialize the collectors for each supported metric kind.
		Collectors: RegistryCollectors{
			schemas.MetricKindCoverage:                             NewCollectorCoverage(),
			schemas.MetricKindDurationSeconds:                      NewCollectorDurationSeconds(),
			schemas.MetricKindEnvironmentBehindCommitsCount:        NewCollectorEnvironmentBehindCommitsCount(),
			schemas.MetricKindEnvironmentBehindDurationSeconds:     NewCollectorEnvironmentBehindDurationSeconds(),
			schemas.MetricKindEnvironmentDeploymentCount:           NewCollectorEnvironmentDeploymentCount(),
			schemas.MetricKindEnvironmentDeploymentDurationSeconds: NewCollectorEnvironmentDeploymentDurationSeconds(),
			schemas.MetricKindEnvironmentDeploymentJobID:           NewCollectorEnvironmentDeploymentJobID(),
			schemas.MetricKindEnvironmentDeploymentStatus:          NewCollectorEnvironmentDeploymentStatus(),
			schemas.MetricKindEnvironmentDeploymentTimestamp:       NewCollectorEnvironmentDeploymentTimestamp(),
			schemas.MetricKindEnvironmentInformation:               NewCollectorEnvironmentInformation(),
			schemas.MetricKindID:                                   NewCollectorID(),
			schemas.MetricKindJobArtifactSizeBytes:                 NewCollectorJobArtifactSizeBytes(),
			schemas.MetricKindJobDurationSeconds:                   NewCollectorJobDurationSeconds(),
			schemas.MetricKindJobID:                                NewCollectorJobID(),
			schemas.MetricKindJobQueuedDurationSeconds:             NewCollectorJobQueuedDurationSeconds(),
			schemas.MetricKindJobRunCount:                          NewCollectorJobRunCount(),
			schemas.MetricKindJobStatus:                            NewCollectorJobStatus(),
			schemas.MetricKindJobTimestamp:                         NewCollectorJobTimestamp(),
			schemas.MetricKindQueuedDurationSeconds:                NewCollectorQueuedDurationSeconds(),
			schemas.MetricKindRunCount:                             NewCollectorRunCount(),
			schemas.MetricKindStatus:                               NewCollectorStatus(),
			schemas.MetricKindRunner:                               NewCollectorRunners(),
			schemas.MetricKindTimestamp:                            NewCollectorTimestamp(),
			schemas.MetricKindTestReportTotalTime:                  NewCollectorTestReportTotalTime(),
			schemas.MetricKindTestReportTotalCount:                 NewCollectorTestReportTotalCount(),
			schemas.MetricKindTestReportSuccessCount:               NewCollectorTestReportSuccessCount(),
			schemas.MetricKindTestReportFailedCount:                NewCollectorTestReportFailedCount(),
			schemas.MetricKindTestReportSkippedCount:               NewCollectorTestReportSkippedCount(),
			schemas.MetricKindTestReportErrorCount:                 NewCollectorTestReportErrorCount(),
			schemas.MetricKindTestSuiteTotalTime:                   NewCollectorTestSuiteTotalTime(),
			schemas.MetricKindTestSuiteTotalCount:                  NewCollectorTestSuiteTotalCount(),
			schemas.MetricKindTestSuiteSuccessCount:                NewCollectorTestSuiteSuccessCount(),
			schemas.MetricKindTestSuiteFailedCount:                 NewCollectorTestSuiteFailedCount(),
			schemas.MetricKindTestSuiteSkippedCount:                NewCollectorTestSuiteSkippedCount(),
			schemas.MetricKindTestSuiteErrorCount:                  NewCollectorTestSuiteErrorCount(),
			schemas.MetricKindTestCaseExecutionTime:                NewCollectorTestCaseExecutionTime(),
			schemas.MetricKindTestCaseStatus:                       NewCollectorTestCaseStatus(),
		},
	}

	// Register internal metrics collectors (e.g., for internal health and stats).
	r.RegisterInternalCollectors()

	// Register all custom collectors into the Prometheus registry.
	if err := r.RegisterCollectors(); err != nil {
		// Fatal error: the application cannot proceed without successful metric registration.
		log.WithContext(ctx).
			Fatal(err)
	}

	return r
}

// RegisterInternalCollectors declares and registers internal application metrics to the Prometheus registry.
func (r *Registry) RegisterInternalCollectors() {
	// Initialize each internal collector with its corresponding constructor.
	// These collectors track the internal state of the system (not user metrics).
	r.InternalCollectors.CurrentlyQueuedTasksCount = NewInternalCollectorCurrentlyQueuedTasksCount()   // Number of currently queued tasks
	r.InternalCollectors.EnvironmentsCount = NewInternalCollectorEnvironmentsCount()                   // Number of environments tracked
	r.InternalCollectors.ExecutedTasksCount = NewInternalCollectorExecutedTasksCount()                 // Number of tasks that have been executed
	r.InternalCollectors.GitLabAPIRequestsCount = NewInternalCollectorGitLabAPIRequestsCount()         // Total GitLab API requests
	r.InternalCollectors.GitlabAPIRequestsRemaining = NewInternalCollectorGitLabAPIRequestsRemaining() // Remaining GitLab API quota
	r.InternalCollectors.GitlabAPIRequestsLimit = NewInternalCollectorGitLabAPIRequestsLimit()         // GitLab API quota limit
	r.InternalCollectors.MetricsCount = NewInternalCollectorMetricsCount()                             // Number of metrics exported
	r.InternalCollectors.ProjectsCount = NewInternalCollectorProjectsCount()                           // Number of tracked projects
	r.InternalCollectors.RefsCount = NewInternalCollectorRefsCount()                                   // Number of Git refs (branches, tags, etc.)
	r.InternalCollectors.RunnersCount = NewInternalCollectorRunnersCount()                             // Number of Runners exported

	// Register all initialized internal collectors with the Prometheus registry.
	// The underscore `_` ignores any error returned by Register (e.g., if already registered).
	_ = r.Register(r.InternalCollectors.CurrentlyQueuedTasksCount)
	_ = r.Register(r.InternalCollectors.EnvironmentsCount)
	_ = r.Register(r.InternalCollectors.ExecutedTasksCount)
	_ = r.Register(r.InternalCollectors.GitLabAPIRequestsCount)
	_ = r.Register(r.InternalCollectors.GitlabAPIRequestsRemaining)
	_ = r.Register(r.InternalCollectors.GitlabAPIRequestsLimit)
	_ = r.Register(r.InternalCollectors.MetricsCount)
	_ = r.Register(r.InternalCollectors.ProjectsCount)
	_ = r.Register(r.InternalCollectors.RefsCount)
	_ = r.Register(r.InternalCollectors.RunnersCount)
}

// ExportInternalMetrics gathers internal statistics from the store and GitLab client,
// then sets the values for the corresponding Prometheus internal collectors.
func (r *Registry) ExportInternalMetrics(ctx context.Context, g *gitlab.Client, s store.Store) (err error) {
	var (
		currentlyQueuedTasks uint64 // Number of tasks currently in the queue
		environmentsCount    int64  // Number of environments
		executedTasksCount   uint64 // Number of tasks that have been executed
		metricsCount         int64  // Number of stored/exported metrics
		projectsCount        int64  // Number of projects tracked
		refsCount            int64  // Number of Git references (branches/tags)
		runnersCount         int64  // Number of Runners exported
	)

	// Retrieve the number of currently queued tasks from the store
	currentlyQueuedTasks, err = s.CurrentlyQueuedTasksCount(ctx)
	if err != nil {
		return
	}

	// Retrieve the number of executed tasks
	executedTasksCount, err = s.ExecutedTasksCount(ctx)
	if err != nil {
		return
	}

	// Retrieve the number of projects
	projectsCount, err = s.ProjectsCount(ctx)
	if err != nil {
		return
	}

	// Retrieve the number of environments
	environmentsCount, err = s.EnvironmentsCount(ctx)
	if err != nil {
		return
	}

	// Retrieve the number of Git references (branches, tags, etc.)
	refsCount, err = s.RefsCount(ctx)
	if err != nil {
		return
	}

	// Retrieve the number of stored metrics
	metricsCount, err = s.MetricsCount(ctx)
	if err != nil {
		return
	}

	runnersCount, err = s.RunnersCount(ctx)
	if err != nil {
		return
	}

	// Set Prometheus gauge values for each internal metric.
	// All collectors are asserted as GaugeVec and updated with empty labels.
	r.InternalCollectors.CurrentlyQueuedTasksCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(currentlyQueuedTasks))
	r.InternalCollectors.EnvironmentsCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(environmentsCount))
	r.InternalCollectors.ExecutedTasksCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(executedTasksCount))
	r.InternalCollectors.GitLabAPIRequestsCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(g.RequestsCounter.Load()))
	r.InternalCollectors.GitlabAPIRequestsRemaining.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(g.RequestsRemaining))
	r.InternalCollectors.GitlabAPIRequestsLimit.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(g.RequestsLimit))
	r.InternalCollectors.MetricsCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(metricsCount))
	r.InternalCollectors.ProjectsCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(projectsCount))
	r.InternalCollectors.RefsCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(refsCount))
	r.InternalCollectors.RunnersCount.(*prometheus.GaugeVec).With(prometheus.Labels{}).Set(float64(runnersCount))

	return
}

// RegisterCollectors adds all defined custom metric collectors to the Prometheus registry.
// It iterates over the Registry.Collectors map and registers each collector.
// If a registration fails, it returns a formatted error.
func (r *Registry) RegisterCollectors() error {
	for _, c := range r.Collectors {
		// Attempt to register the collector to the Prometheus registry
		if err := r.Register(c); err != nil {
			// If registration fails, return a descriptive error
			return fmt.Errorf("could not add provided collector '%v' to the Prometheus registry: %v", c, err)
		}
	}

	// Return nil if all collectors were successfully registered
	return nil
}

// GetCollector returns the Prometheus collector associated with the given metric kind.
// It retrieves the collector from the Registry.Collectors map using the provided metric kind as the key.
func (r *Registry) GetCollector(kind schemas.MetricKind) prometheus.Collector {
	return r.Collectors[kind]
}

// ExportMetrics updates the corresponding Prometheus collectors with the provided metric data.
// It iterates over all metrics and dispatches their values to the appropriate registered collectors.
func (r *Registry) ExportMetrics(metrics schemas.Metrics) {
	for _, m := range metrics {
		log.Tracef("P2L metric: %v", m.Kind)

		// Get the collector associated with the metric kind
		switch c := r.GetCollector(m.Kind).(type) {
		// If it's a GaugeVec, set the value directly
		case *prometheus.GaugeVec:
			c.With(m.Labels).Set(m.Value)

		// If it's a CounterVec, increment the counter by the value
		case *prometheus.CounterVec:
			c.With(m.Labels).Add(m.Value)

		// If the collector type is not supported, log an error
		default:
			log.Errorf("unsupported collector type : %v", reflect.TypeOf(c))
		}
	}
}

// emitStatusMetric records a set of status metrics for a given entity (e.g., job, test case).
// It writes a Prometheus metric for each possible status, setting the value to 1 for the current status,
// and either 0 or deletes the metric for other statuses depending on the 'sparseMetrics' flag.
func emitStatusMetric(ctx context.Context, s store.Store, metricKind schemas.MetricKind,
	labelValues map[string]string, // Base labels (e.g., project, test suite, etc.)
	statuses []string, // List of all possible statuses
	status string, // Current status of the entity
	sparseMetrics bool, // If true, only emit metrics for the current status
) {
	// Loop through all possible statuses
	for _, currentStatus := range statuses {
		var (
			value        float64
			statusLabels = make(map[string]string) // Copy base labels for current status
		)

		// Copy original label values to the status-specific map
		for k, v := range labelValues {
			statusLabels[k] = v
		}

		// Add the current status as a label
		statusLabels["status"] = currentStatus

		// Initialize the metric with default value (0)
		statusMetric := schemas.Metric{
			Kind:   metricKind,
			Labels: statusLabels,
			Value:  value,
		}

		// If this status matches the current status, set the metric value to 1
		if currentStatus == status {
			statusMetric.Value = 1
		} else {
			// If sparseMetrics is enabled, we delete metrics for statuses that don't apply
			if sparseMetrics {
				storeDelMetric(ctx, s, statusMetric)
				continue
			}

			// Otherwise, explicitly record a metric with value 0 for this status
			statusMetric.Value = 0
		}

		// Save the metric to the store (either value 1 or 0)
		storeSetMetric(ctx, s, statusMetric)
	}
}
