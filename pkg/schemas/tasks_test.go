package schemas

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskTypeValues(t *testing.T) {
	assert.Equal(t, TaskType("PullProject"), TaskTypePullProject)
	assert.Equal(t, TaskType("PullProjectsFromWildcard"), TaskTypePullProjectsFromWildcard)
	assert.Equal(t, TaskType("PullProjectsFromWildcards"), TaskTypePullProjectsFromWildcards)
	assert.Equal(t, TaskType("PullEnvironmentsFromProject"), TaskTypePullEnvironmentsFromProject)
	assert.Equal(t, TaskType("PullEnvironmentsFromProjects"), TaskTypePullEnvironmentsFromProjects)
	assert.Equal(t, TaskType("PullEnvironmentMetrics"), TaskTypePullEnvironmentMetrics)
	assert.Equal(t, TaskType("PullMetrics"), TaskTypePullMetrics)
	assert.Equal(t, TaskType("PullRefsFromProject"), TaskTypePullRefsFromProject)
	assert.Equal(t, TaskType("PullRefsFromProjects"), TaskTypePullRefsFromProjects)
	assert.Equal(t, TaskType("PullRefMetrics"), TaskTypePullRefMetrics)
	assert.Equal(t, TaskType("PullRunnerFromProject"), TaskTypePullRunnersFromProject)
	assert.Equal(t, TaskType("PullRunnersFromProjects"), TaskTypePullRunnersFromProjects)
	assert.Equal(t, TaskType("PullRunnersMetrics"), TaskTypePullRunnersMetrics)
	assert.Equal(t, TaskType("GarbageCollectProjects"), TaskTypeGarbageCollectProjects)
	assert.Equal(t, TaskType("GarbageCollectEnvironments"), TaskTypeGarbageCollectEnvironments)
	assert.Equal(t, TaskType("GarbageCollectRefs"), TaskTypeGarbageCollectRefs)
	assert.Equal(t, TaskType("GarbageCollectMetrics"), TaskTypeGarbageCollectMetrics)
	assert.Equal(t, TaskType("GarbageCollectRunners"), TaskTypeGarbageCollectRunners)
}

func TestTasksMapUsage(t *testing.T) {
	tasks := Tasks{
		TaskTypePullMetrics: {
			"task-1": nil,
			"task-2": nil,
		},
		TaskTypeGarbageCollectEnvironments: {
			"task-3": nil,
		},
	}

	assert.Len(t, tasks, 2)
	assert.Contains(t, tasks, TaskTypePullMetrics)
	assert.Contains(t, tasks, TaskTypeGarbageCollectEnvironments)

	assert.Len(t, tasks[TaskTypePullMetrics], 2)
	assert.Len(t, tasks[TaskTypeGarbageCollectEnvironments], 1)

	_, ok := tasks[TaskTypePullMetrics]["task-1"]
	assert.True(t, ok)

	_, ok = tasks[TaskTypeGarbageCollectEnvironments]["task-3"]
	assert.True(t, ok)
}

func TestTasksMapInitialization(t *testing.T) {
	var tasks Tasks
	assert.Nil(t, tasks)

	tasks = make(Tasks)
	assert.NotNil(t, tasks)
	assert.Len(t, tasks, 0)

	tasks[TaskTypePullProject] = make(map[string]interface{})
	tasks[TaskTypePullProject]["project-1"] = nil

	assert.Len(t, tasks, 1)
	assert.Len(t, tasks[TaskTypePullProject], 1)

	_, ok := tasks[TaskTypePullProject]["project-1"]
	assert.True(t, ok)
}
