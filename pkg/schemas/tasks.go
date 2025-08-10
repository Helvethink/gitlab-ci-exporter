package schemas

// TaskType represents the type of task as a string.
type TaskType string

const (
	// TaskTypePullProject represents a task type for pulling a single project.
	TaskTypePullProject TaskType = "PullProject"

	// TaskTypePullProjectsFromWildcard represents a task type for pulling projects from a wildcard pattern.
	TaskTypePullProjectsFromWildcard TaskType = "PullProjectsFromWildcard"

	// TaskTypePullProjectsFromWildcards represents a task type for pulling projects from multiple wildcard patterns.
	TaskTypePullProjectsFromWildcards TaskType = "PullProjectsFromWildcards"

	// TaskTypePullEnvironmentsFromProject represents a task type for pulling environments from a single project.
	TaskTypePullEnvironmentsFromProject TaskType = "PullEnvironmentsFromProject"

	// TaskTypePullEnvironmentsFromProjects represents a task type for pulling environments from multiple projects.
	TaskTypePullEnvironmentsFromProjects TaskType = "PullEnvironmentsFromProjects"

	// TaskTypePullEnvironmentMetrics represents a task type for pulling metrics from environments.
	TaskTypePullEnvironmentMetrics TaskType = "PullEnvironmentMetrics"

	// TaskTypePullMetrics represents a task type for pulling metrics.
	TaskTypePullMetrics TaskType = "PullMetrics"

	// TaskTypePullRefsFromProject represents a task type for pulling references from a single project.
	TaskTypePullRefsFromProject TaskType = "PullRefsFromProject"

	// TaskTypePullRefsFromProjects represents a task type for pulling references from multiple projects.
	TaskTypePullRefsFromProjects TaskType = "PullRefsFromProjects"

	// TaskTypePullRefMetrics represents a task type for pulling metrics from references.
	TaskTypePullRefMetrics TaskType = "PullRefMetrics"

	// TaskTypeGarbageCollectProjects represents a task type for garbage collecting projects.
	TaskTypeGarbageCollectProjects TaskType = "GarbageCollectProjects"

	// TaskTypeGarbageCollectEnvironments represents a task type for garbage collecting environments.
	TaskTypeGarbageCollectEnvironments TaskType = "GarbageCollectEnvironments"

	// TaskTypeGarbageCollectRefs represents a task type for garbage collecting references.
	TaskTypeGarbageCollectRefs TaskType = "GarbageCollectRefs"

	// TaskTypeGarbageCollectMetrics represents a task type for garbage collecting metrics.
	TaskTypeGarbageCollectMetrics TaskType = "GarbageCollectMetrics"

	TaskTypePullRunnersMetrics      TaskType = "PullRunnersMetrics"
	TaskTypePullRunnersFromProjects TaskType = "PullRunnersFromProjects"
	TaskTypePullRunnersFromProject  TaskType = "PullRunnerFromProject"

	TaskTypeGarbageCollectRunners TaskType = "GarbageCollectRunners"
)

// Tasks is a map structure used to keep track of tasks.
// It maps a TaskType to another map, which associates task identifiers with empty interfaces.
type Tasks map[TaskType]map[string]interface{}
