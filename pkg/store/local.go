package store

import (
	"context"
	"sync"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// Local represents an in-memory storage implementation for managing projects, environments, references, and metrics.
type Local struct {
	projects      schemas.Projects
	projectsMutex sync.RWMutex // Mutex for thread-safe access to projects

	environments      schemas.Environments
	environmentsMutex sync.RWMutex // Mutex for thread-safe access to environments

	refs      schemas.Refs
	refsMutex sync.RWMutex // Mutex for thread-safe access to references

	metrics      schemas.Metrics
	metricsMutex sync.RWMutex // Mutex for thread-safe access to metrics

	tasks              schemas.Tasks
	tasksMutex         sync.RWMutex // Mutex for thread-safe access to tasks
	executedTasksCount uint64       // Counter for the number of executed tasks
}

// SetProject stores a project in the local storage.
func (l *Local) SetProject(_ context.Context, p schemas.Project) error {
	l.projectsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.projectsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	l.projects[p.Key()] = p // Store the project

	return nil
}

// DelProject deletes a project from the local storage.
func (l *Local) DelProject(_ context.Context, k schemas.ProjectKey) error {
	l.projectsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.projectsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	delete(l.projects, k) // Delete the project

	return nil
}

// GetProject retrieves a project from the local storage.
func (l *Local) GetProject(ctx context.Context, p *schemas.Project) error {
	exists, _ := l.ProjectExists(ctx, p.Key())

	if exists {
		l.projectsMutex.RLock()   // Lock the mutex for read-only access
		*p = l.projects[p.Key()]  // Retrieve the project
		l.projectsMutex.RUnlock() // Unlock the mutex
	}

	return nil
}

// ProjectExists checks if a project exists in the local storage.
func (l *Local) ProjectExists(_ context.Context, k schemas.ProjectKey) (bool, error) {
	l.projectsMutex.RLock()         // Lock the mutex for read-only access
	defer l.projectsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	_, ok := l.projects[k] // Check if the project exists

	return ok, nil
}

// Projects retrieves all projects from the local storage.
func (l *Local) Projects(_ context.Context) (projects schemas.Projects, err error) {
	projects = make(schemas.Projects)

	l.projectsMutex.RLock()         // Lock the mutex for read-only access
	defer l.projectsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	for k, v := range l.projects {
		projects[k] = v // Copy all projects to the result
	}

	return
}

// ProjectsCount returns the count of projects in the local storage.
func (l *Local) ProjectsCount(_ context.Context) (int64, error) {
	l.projectsMutex.RLock()         // Lock the mutex for read-only access
	defer l.projectsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	return int64(len(l.projects)), nil // Return the number of projects
}

// SetEnvironment stores an environment in the local storage.
func (l *Local) SetEnvironment(_ context.Context, environment schemas.Environment) error {
	l.environmentsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.environmentsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	l.environments[environment.Key()] = environment // Store the environment

	return nil
}

// DelEnvironment deletes an environment from the local storage.
func (l *Local) DelEnvironment(_ context.Context, k schemas.EnvironmentKey) error {
	l.environmentsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.environmentsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	delete(l.environments, k) // Delete the environment

	return nil
}

// GetEnvironment retrieves an environment from the local storage.
func (l *Local) GetEnvironment(ctx context.Context, environment *schemas.Environment) error {
	exists, _ := l.EnvironmentExists(ctx, environment.Key())

	if exists {
		l.environmentsMutex.RLock()                      // Lock the mutex for read-only access
		*environment = l.environments[environment.Key()] // Retrieve the environment
		l.environmentsMutex.RUnlock()                    // Unlock the mutex
	}

	return nil
}

// EnvironmentExists checks if an environment exists in the local storage.
func (l *Local) EnvironmentExists(_ context.Context, k schemas.EnvironmentKey) (bool, error) {
	l.environmentsMutex.RLock()         // Lock the mutex for read-only access
	defer l.environmentsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	_, ok := l.environments[k] // Check if the environment exists

	return ok, nil
}

// Environments retrieves all environments from the local storage.
func (l *Local) Environments(_ context.Context) (environments schemas.Environments, err error) {
	environments = make(schemas.Environments)

	l.environmentsMutex.RLock()         // Lock the mutex for read-only access
	defer l.environmentsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	for k, v := range l.environments {
		environments[k] = v // Copy all environments to the result
	}

	return
}

// EnvironmentsCount returns the count of environments in the local storage.
func (l *Local) EnvironmentsCount(_ context.Context) (int64, error) {
	l.environmentsMutex.RLock()         // Lock the mutex for read-only access
	defer l.environmentsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	return int64(len(l.environments)), nil // Return the number of environments
}

// SetRef stores a reference in the local storage.
func (l *Local) SetRef(_ context.Context, ref schemas.Ref) error {
	l.refsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.refsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	l.refs[ref.Key()] = ref // Store the reference

	return nil
}

// DelRef deletes a reference from the local storage.
func (l *Local) DelRef(_ context.Context, k schemas.RefKey) error {
	l.refsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.refsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	delete(l.refs, k) // Delete the reference

	return nil
}

// GetRef retrieves a reference from the local storage.
func (l *Local) GetRef(ctx context.Context, ref *schemas.Ref) error {
	exists, _ := l.RefExists(ctx, ref.Key())

	if exists {
		l.refsMutex.RLock()      // Lock the mutex for read-only access
		*ref = l.refs[ref.Key()] // Retrieve the reference
		l.refsMutex.RUnlock()    // Unlock the mutex
	}

	return nil
}

// RefExists checks if a reference exists in the local storage.
func (l *Local) RefExists(_ context.Context, k schemas.RefKey) (bool, error) {
	l.refsMutex.RLock()         // Lock the mutex for read-only access
	defer l.refsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	_, ok := l.refs[k] // Check if the reference exists

	return ok, nil
}

// Refs retrieves all references from the local storage.
func (l *Local) Refs(_ context.Context) (refs schemas.Refs, err error) {
	refs = make(schemas.Refs)

	l.refsMutex.RLock()         // Lock the mutex for read-only access
	defer l.refsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	for k, v := range l.refs {
		refs[k] = v // Copy all references to the result
	}

	return
}

// RefsCount returns the count of references in the local storage.
func (l *Local) RefsCount(_ context.Context) (int64, error) {
	l.refsMutex.RLock()         // Lock the mutex for read-only access
	defer l.refsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	return int64(len(l.refs)), nil // Return the number of references
}

// SetMetric stores a metric in the local storage.
func (l *Local) SetMetric(_ context.Context, m schemas.Metric) error {
	l.metricsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.metricsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	l.metrics[m.Key()] = m // Store the metric

	return nil
}

// DelMetric deletes a metric from the local storage.
func (l *Local) DelMetric(_ context.Context, k schemas.MetricKey) error {
	l.metricsMutex.Lock()         // Lock the mutex for exclusive access
	defer l.metricsMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	delete(l.metrics, k) // Delete the metric

	return nil
}

// GetMetric retrieves a metric from the local storage.
func (l *Local) GetMetric(ctx context.Context, m *schemas.Metric) error {
	exists, _ := l.MetricExists(ctx, m.Key())

	if exists {
		l.metricsMutex.RLock()   // Lock the mutex for read-only access
		*m = l.metrics[m.Key()]  // Retrieve the metric
		l.metricsMutex.RUnlock() // Unlock the mutex
	}

	return nil
}

// MetricExists checks if a metric exists in the local storage.
func (l *Local) MetricExists(_ context.Context, k schemas.MetricKey) (bool, error) {
	l.metricsMutex.RLock()         // Lock the mutex for read-only access
	defer l.metricsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	_, ok := l.metrics[k] // Check if the metric exists

	return ok, nil
}

// Metrics retrieves all metrics from the local storage.
func (l *Local) Metrics(_ context.Context) (metrics schemas.Metrics, err error) {
	metrics = make(schemas.Metrics)

	l.metricsMutex.RLock()         // Lock the mutex for read-only access
	defer l.metricsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	for k, v := range l.metrics {
		metrics[k] = v // Copy all metrics to the result
	}

	return
}

// MetricsCount returns the count of metrics in the local storage.
func (l *Local) MetricsCount(_ context.Context) (int64, error) {
	l.metricsMutex.RLock()         // Lock the mutex for read-only access
	defer l.metricsMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	return int64(len(l.metrics)), nil // Return the number of metrics
}

// isTaskAlreadyQueued assesses if a task is already queued or not.
func (l *Local) isTaskAlreadyQueued(tt schemas.TaskType, uniqueID string) bool {
	l.tasksMutex.Lock()         // Lock the mutex for exclusive access
	defer l.tasksMutex.Unlock() // Ensure the mutex is unlocked when the function exits

	if l.tasks == nil {
		l.tasks = make(map[schemas.TaskType]map[string]interface{}) // Initialize the tasks map if it's nil
	}

	taskTypeQueue, ok := l.tasks[tt]
	if !ok {
		l.tasks[tt] = make(map[string]interface{}) // Initialize the task type queue if it doesn't exist

		return false
	}

	if _, alreadyQueued := taskTypeQueue[uniqueID]; alreadyQueued {
		return true // Return true if the task is already queued
	}

	return false
}

// QueueTask registers that we are queueing the task.
// It returns true if it managed to schedule it, false if it was already scheduled.
func (l *Local) QueueTask(_ context.Context, tt schemas.TaskType, uniqueID, _ string) (bool, error) {
	if !l.isTaskAlreadyQueued(tt, uniqueID) {
		l.tasksMutex.Lock()         // Lock the mutex for exclusive access
		defer l.tasksMutex.Unlock() // Ensure the mutex is unlocked when the function exits

		l.tasks[tt][uniqueID] = nil // Queue the task

		return true, nil
	}

	return false, nil
}

// UnqueueTask removes the task from the tracker.
func (l *Local) UnqueueTask(_ context.Context, tt schemas.TaskType, uniqueID string) error {
	if l.isTaskAlreadyQueued(tt, uniqueID) {
		l.tasksMutex.Lock()         // Lock the mutex for exclusive access
		defer l.tasksMutex.Unlock() // Ensure the mutex is unlocked when the function exits

		delete(l.tasks[tt], uniqueID) // Remove the task from the queue

		l.executedTasksCount++ // Increment the count of executed tasks
	}

	return nil
}

// CurrentlyQueuedTasksCount returns the count of currently queued tasks.
func (l *Local) CurrentlyQueuedTasksCount(_ context.Context) (count uint64, err error) {
	l.tasksMutex.RLock()         // Lock the mutex for read-only access
	defer l.tasksMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	for _, t := range l.tasks {
		count += uint64(len(t)) // Sum the number of tasks across all task types
	}

	return
}

// ExecutedTasksCount returns the count of executed tasks.
func (l *Local) ExecutedTasksCount(_ context.Context) (uint64, error) {
	l.tasksMutex.RLock()         // Lock the mutex for read-only access
	defer l.tasksMutex.RUnlock() // Ensure the mutex is unlocked when the function exits

	return l.executedTasksCount, nil // Return the count of executed tasks
}
