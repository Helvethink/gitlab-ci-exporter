package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/taskq/memqueue/v4"
	"github.com/vmihailenco/taskq/redisq/v4"
	"github.com/vmihailenco/taskq/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

// TaskController holds the components needed to manage task queues and scheduling.
type TaskController struct {
	Factory                  taskq.Factory
	Queue                    taskq.Queue
	TaskMap                  *taskq.TaskMap
	TaskSchedulingMonitoring *monitor.SchedulingStatus
	admission                *taskAdmission
	capacity                 int
}

// NewTaskController initializes and returns a new TaskController.
// It sets up the task queue backed either by Redis (if provided) or an in-memory queue.
// maximumJobsQueueSize controls the queue buffer size.
// Redis consumers are started after handlers and the store are initialized.
func NewTaskController(ctx context.Context, r *redis.Client, maximumJobsQueueSize int) (t TaskController) {
	// Start an OpenTelemetry tracing span for monitoring initialization time
	_, span := otel.Tracer(tracerName).Start(ctx, "controller:NewTaskController")
	defer span.End()

	// Initialize the TaskMap that will register task handlers
	t.TaskMap = &taskq.TaskMap{}

	// Configure queue options including name, error handling, buffer size, and handler map
	queueOptions := &taskq.QueueConfig{
		Name:                 "default",            // Name of the queue
		PauseErrorsThreshold: 3,                    // Number of consecutive errors to pause queue processing
		Handler:              t.TaskMap,            // Task handler map for processing enqueued tasks
		BufferSize:           maximumJobsQueueSize, // Buffer size for queued jobs
	}

	// Use Redis-backed queue if redis client is provided, else use in-memory queue
	if r != nil {
		t.Factory = redisq.NewFactory() // Redis-backed task queue factory
		queueOptions.Handler = ownedTaskHandler{tasks: t.TaskMap, redis: r}
		queueOptions.Redis = r // Set Redis client in queue options
	} else {
		t.Factory = memqueue.NewFactory() // In-memory task queue factory
	}

	// Register the queue using the factory with the configured options
	t.Queue = t.Factory.RegisterQueue(queueOptions)

	// Initialize the monitoring map to track scheduling status per task type
	t.TaskSchedulingMonitoring = &monitor.SchedulingStatus{}
	if r == nil {
		t.admission = newTaskAdmission(maximumJobsQueueSize)
	}
	if maximumJobsQueueSize < 1 {
		maximumJobsQueueSize = 1
	}
	t.capacity = maximumJobsQueueSize

	return
}

// TaskHandlerPullProject handles a task to pull metrics or data for a specific project.
// It ensures that the task is unqueued after processing regardless of success or failure.
func (c *Controller) TaskHandlerPullProject(ctx context.Context, name string, pull config.ProjectPull) error {
	// Ensure the task is removed from the queue once this handler finishes
	defer c.dequeueTask(ctx, schemas.TaskTypePullProject, name)

	// Call the actual project pulling logic
	return c.PullProject(ctx, name, pull)
}

// TaskHandlerPullProjectsFromWildcard handles a task to pull multiple projects
// based on a wildcard configuration. It unqueues the task when done.
func (c *Controller) TaskHandlerPullProjectsFromWildcard(ctx context.Context, id string, w config.Wildcard) error {
	// Ensure the task is removed from the queue after processing
	defer c.dequeueTask(ctx, schemas.TaskTypePullProjectsFromWildcard, id)

	// Execute pulling projects matching the wildcard configuration
	return c.PullProjectsFromWildcard(ctx, w)
}

// TaskHandlerPullEnvironmentsFromProject handles the task of pulling environments
// from a given project. It unqueues the task once done and logs any errors without retrying.
func (c *Controller) TaskHandlerPullEnvironmentsFromProject(ctx context.Context, p schemas.Project) {
	// Ensure the task is removed from the queue after processing
	defer c.dequeueTask(ctx, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()))

	// Only proceed if environment pulling is enabled for the project
	if p.Pull.Environments.Enabled {
		// Attempt to pull environments from the project
		if err := c.PullEnvironmentsFromProject(ctx, p); err != nil {
			log.WithContext(ctx).
				WithFields(log.Fields{
					"project-name": p.Name,
				}).
				WithError(err).
				Warn("pulling environments from project")
		}
	}
}

// TaskHandlerPullRunnersFromProject TODO
func (c *Controller) TaskHandlerPullRunnersFromProject(ctx context.Context, p schemas.Project) {
	// Ensure the task is removed from the queue after processing
	defer c.dequeueTask(ctx, schemas.TaskTypePullRunnersFromProject, string(p.Key()))

	// Only proceed if runners pulling is enabled for the project
	if p.Pull.Runners.Enabled {
		// Attempt to pull runners from the project
		if err := c.PullRunnersFromProject(ctx, p); err != nil {
			log.WithContext(ctx).
				WithFields(log.Fields{
					"project-name": p.Name,
				}).
				WithError(err).
				Warn("pulling runners from project")
		}
	}
}

// TaskHandlerPullEnvironmentMetrics handles the task of pulling metrics
// for a specific environment. It ensures the task is unqueued after processing,
// and logs any errors encountered without retrying the task.
func (c *Controller) TaskHandlerPullEnvironmentMetrics(ctx context.Context, env schemas.Environment) {
	// Ensure the task is removed from the queue when this function exits
	defer c.dequeueTask(ctx, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()))

	// Attempt to pull environment metrics, log warning on failure but do not retry
	if err := c.PullEnvironmentMetrics(ctx, env); err != nil {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"project-name":     env.ProjectName,
				"environment-name": env.Name,
				"environment-id":   env.ID,
			}).
			WithError(err).
			Warn("pulling environment metrics")
	}
}

// TaskHandlerPullRunnerMetrics handles the task of pulling metrics
// for a specific runner. It ensures the task is unqueued after processing,
// and logs any errors encountered without retrying the task.
func (c *Controller) TaskHandlerPullRunnerMetrics(ctx context.Context, runner schemas.Runner) {
	// Ensure the task is removed from the queue when this function exits
	defer c.dequeueTask(ctx, schemas.TaskTypePullRunnersMetrics, string(runner.Key()))

	// Attempt to pull runner metrics, log warning on failure but do not retry
	if err := c.ProcessRunnerMetrics(ctx, runner); err != nil {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"project-name": runner.ProjectName,
				"runner-name":  runner.Name,
				"runner-id":    runner.ID,
			}).
			WithError(err).
			Warn("pulling runner metrics")
	}
}

// TaskHandlerPullRefsFromProject handles the task of pulling refs (branches, tags, etc.)
// from a given project. It ensures the task is removed from the queue after execution.
// Any errors encountered during the pull are logged as warnings, and the task is not retried.
func (c *Controller) TaskHandlerPullRefsFromProject(ctx context.Context, p schemas.Project) {
	// Ensure the task is removed from the queue when this function completes
	defer c.dequeueTask(ctx, schemas.TaskTypePullRefsFromProject, string(p.Key()))

	// Attempt to pull refs from the specified project, logging any errors without retrying
	if err := c.PullRefsFromProject(ctx, p); err != nil {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"project-name": p.Name,
			}).
			WithError(err).
			Warn("pulling refs from project")
	}
}

// TaskHandlerPullRefMetrics handles the task of pulling metrics for a specific ref
// (branch, tag, or merge request) within a project. It ensures the task is dequeued
// once completed. Errors during the pull are logged as warnings without retrying.
func (c *Controller) TaskHandlerPullRefMetrics(ctx context.Context, ref schemas.Ref) {
	// Ensure the task is removed from the queue when this function completes
	defer c.dequeueTask(ctx, schemas.TaskTypePullRefMetrics, string(ref.Key()))

	// Attempt to pull metrics for the specified ref, logging any errors without retrying
	if err := c.PullRefMetrics(ctx, ref); err != nil {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"project-name": ref.Project.Name,
				"ref":          ref.Name,
			}).
			WithError(err).
			Warn("pulling ref metrics")
	}
}

// TaskHandlerPullProjectsFromWildcards schedules tasks to pull projects matching configured wildcards.
// It dequeues the current task when done and updates monitoring for task scheduling.
// For each wildcard configured, it schedules a task to pull projects that match that wildcard.
func (c *Controller) TaskHandlerPullProjectsFromWildcards(ctx context.Context) {
	// Ensure this task is removed from the queue after execution
	defer c.dequeueTask(ctx, schemas.TaskTypePullProjectsFromWildcards, "_")

	// Update the monitoring system for the last scheduling of this task type
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypePullProjectsFromWildcards)

	// Log the total number of wildcards to process
	log.WithFields(
		log.Fields{
			"wildcards-count": len(c.Config.Wildcards),
		},
	).Info("scheduling projects from wildcards pull")

	// Iterate over each wildcard config and schedule a task to pull matching projects
	for id, w := range c.Config.Wildcards {
		c.ScheduleTask(ctx, schemas.TaskTypePullProjectsFromWildcard, strconv.Itoa(id), strconv.Itoa(id), w)
	}
}

// TaskHandlerPullEnvironmentsFromProjects schedules tasks to pull environments from all stored projects.
// It dequeues the current task after execution and updates task scheduling monitoring.
//
// For each project in the store, it schedules a task to pull that project's environments.
func (c *Controller) TaskHandlerPullEnvironmentsFromProjects(ctx context.Context) {
	// Remove this task from the queue after completion
	defer c.dequeueTask(ctx, schemas.TaskTypePullEnvironmentsFromProjects, "_")

	// Update the monitoring information about the last time this task type was scheduled
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypePullEnvironmentsFromProjects)

	// Retrieve the count of projects in the store
	projectsCount, err := c.Store.ProjectsCount(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error counting projects in the store")
	}

	// Log the number of projects found to schedule environment pulls
	log.WithFields(
		log.Fields{
			"projects-count": projectsCount,
		},
	).Info("scheduling environments from projects pull")

	// Retrieve the list of projects from the store
	projects, err := c.Store.Projects(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error retrieving projects from the store")
	}

	// For each project, schedule a task to pull its environments
	for _, p := range projects {
		c.ScheduleTask(ctx, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()), p)
	}
}

// TaskHandlerPullRunnersFromProjects TODO
func (c *Controller) TaskHandlerPullRunnersFromProjects(ctx context.Context) {
	// Remove this task from the queue after completion
	defer c.dequeueTask(ctx, schemas.TaskTypePullRunnersFromProjects, "_")

	// Update the monitoring information about the last time this task type was scheduled
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypePullRunnersFromProjects)

	// Retrieve the count of projects in the store
	projectsCount, err := c.Store.ProjectsCount(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error counting projects in the store")
	}

	// Log the number of projects found to schedule runners pulls
	log.WithFields(
		log.Fields{
			"projects-count": projectsCount,
		},
	).Info("scheduling runners from projects pull")

	// Retrieve the list of projects from the store
	projects, err := c.Store.Projects(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error retrieving projects from the store")
	}

	// For each project, schedule a task to pull its environments
	for _, p := range projects {
		c.ScheduleTask(ctx, schemas.TaskTypePullRunnersFromProject, string(p.Key()), p)
	}
}

// TaskHandlerPullRefsFromProjects schedules tasks to pull refs (branches, tags, MRs) from all stored projects.
// It dequeues the current task after execution and updates task scheduling monitoring.
//
// For each project in the store, it schedules a task to pull that project's refs.
func (c *Controller) TaskHandlerPullRefsFromProjects(ctx context.Context) {
	// Remove this task from the queue after completion
	defer c.dequeueTask(ctx, schemas.TaskTypePullRefsFromProjects, "_")

	// Update monitoring info about the last time this task type was scheduled
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypePullRefsFromProjects)

	// Retrieve the total count of projects in the store
	projectsCount, err := c.Store.ProjectsCount(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error counting projects in the store")
	}

	// Log the number of projects found to schedule refs pulling tasks
	log.WithFields(
		log.Fields{
			"projects-count": projectsCount,
		},
	).Info("scheduling refs from projects pull")

	// Retrieve the list of projects from the store
	projects, err := c.Store.Projects(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error retrieving projects from the store")
	}

	// For each project, schedule a task to pull refs
	for _, p := range projects {
		c.ScheduleTask(ctx, schemas.TaskTypePullRefsFromProject, string(p.Key()), p)
	}
}

// TaskHandlerPullMetrics schedules metric pull tasks for all environments and refs stored in the system.
// After running, it removes itself from the queue and updates task scheduling monitoring.
//
// It fetches all environments and refs from the store, then schedules separate tasks
// to pull metrics for each environment and each ref.
func (c *Controller) TaskHandlerPullMetrics(ctx context.Context) {
	// Remove this task from the queue when done
	defer c.dequeueTask(ctx, schemas.TaskTypePullMetrics, "_")

	// Update monitoring about when this task type was last scheduled
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypePullMetrics)

	// Count refs for logging
	refsCount, err := c.Store.RefsCount(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error counting refs in the store")
	}

	// Count environments for logging
	envsCount, err := c.Store.EnvironmentsCount(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error counting environments in the store")
	}

	// Count runners for logging
	runnersCount, err := c.Store.RunnersCount(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error counting runners in the store")
	}

	// Log counts before scheduling tasks
	log.WithFields(
		log.Fields{
			"environments-count": envsCount,
			"refs-count":         refsCount,
			"runners-count":      runnersCount,
		},
	).Info("scheduling metrics pull")

	// Fetch all environments from the store
	envs, err := c.Store.Environments(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error retrieving environments from the store")
	}

	// Schedule a metrics pull task for each environment
	for _, env := range envs {
		c.ScheduleTask(ctx, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()), env)
	}

	// Fetch all runners from the store
	runners, err := c.Store.Runners(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error retrieving runners from the store")
	}

	// Schedule a metrics pull task for each runner
	for _, r := range runners {
		c.ScheduleTask(ctx, schemas.TaskTypePullRunnersMetrics, string(r.Key()), r)
	}

	// Fetch all refs from the store
	refs, err := c.Store.Refs(ctx)
	if err != nil {
		log.WithContext(ctx).
			WithError(err).
			Error("error retrieving refs from the store")
	}

	// Schedule a metrics pull task for each ref
	for _, ref := range refs {
		c.ScheduleTask(ctx, schemas.TaskTypePullRefMetrics, string(ref.Key()), ref)
	}
}

// TaskHandlerGarbageCollectProjects handles the task of garbage collecting unused or obsolete projects.
// It ensures the task is properly unqueued and updates task scheduling monitoring.
// Returns an error if the garbage collection fails.
func (c *Controller) TaskHandlerGarbageCollectProjects(ctx context.Context) error {
	defer c.dequeueTask(ctx, schemas.TaskTypeGarbageCollectProjects, "_")
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypeGarbageCollectProjects)

	return c.GarbageCollectProjects(ctx)
}

// TaskHandlerGarbageCollectEnvironments handles the task of garbage collecting unused or obsolete environments.
// It ensures the task is properly unqueued and updates task scheduling monitoring.
// Returns an error if the garbage collection fails.
func (c *Controller) TaskHandlerGarbageCollectEnvironments(ctx context.Context) error {
	defer c.dequeueTask(ctx, schemas.TaskTypeGarbageCollectEnvironments, "_")
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypeGarbageCollectEnvironments)

	return c.GarbageCollectEnvironments(ctx)
}

// TaskHandlerGarbageCollectRefs handles the task of garbage collecting unused or obsolete refs.
// It ensures the task is properly unqueued and updates task scheduling monitoring.
// Returns an error if the garbage collection fails.
func (c *Controller) TaskHandlerGarbageCollectRefs(ctx context.Context) error {
	defer c.dequeueTask(ctx, schemas.TaskTypeGarbageCollectRefs, "_")
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypeGarbageCollectRefs)

	return c.GarbageCollectRefs(ctx)
}

// TaskHandlerGarbageCollectMetrics handles the garbage collection of metrics data.
// It ensures that the task is properly unqueued once done, and updates monitoring info about the last
// time this type of task was scheduled. Any error from the actual garbage collection is returned.
func (c *Controller) TaskHandlerGarbageCollectMetrics(ctx context.Context) error {
	defer c.dequeueTask(ctx, schemas.TaskTypeGarbageCollectMetrics, "_")
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypeGarbageCollectMetrics)

	return c.GarbageCollectMetrics(ctx)
}

// TaskHandlerGarbageCollectRunners handles the task of garbage collecting unused or obsolete environments.
// It ensures the task is properly unqueued and updates task scheduling monitoring.
// Returns an error if the garbage collection fails.
func (c *Controller) TaskHandlerGarbageCollectRunners(ctx context.Context) error {
	defer c.dequeueTask(ctx, schemas.TaskTypeGarbageCollectRunners, "_")
	defer c.TaskController.monitorLastTaskScheduling(schemas.TaskTypeGarbageCollectRunners)

	return c.GarbageCollectRunners(ctx)
}

// Schedule initializes and schedules various periodic tasks based on configuration.
// It starts an OpenTelemetry span to trace scheduling operations.
//
// It launches a background goroutine to refresh GitLab metadata asynchronously.
//
// Then, it iterates over a map of task types and their scheduler configurations
// derived from the Pull and GarbageCollect configs.
//
// For each task type:
// - If OnInit is true, the task is scheduled immediately once.
// - If Scheduled is true, the task is scheduled repeatedly at the configured interval.
//
// If a Redis client is configured, it also schedules a keepalive task for Redis.
func (c *Controller) Schedule(ctx context.Context, pull config.Pull, gc config.GarbageCollect) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "controller:Schedule")
	defer span.End()

	go func() {
		err := c.GetGitLabMetadata(ctx)
		if err != nil {
			log.WithContext(ctx).
				WithError(err).
				Error("error retrieving Gitlab Metadata from the store")
		}
	}()

	for tt, cfg := range map[schemas.TaskType]config.SchedulerConfig{
		schemas.TaskTypePullProjectsFromWildcards:    config.SchedulerConfig(pull.ProjectsFromWildcards),
		schemas.TaskTypePullEnvironmentsFromProjects: config.SchedulerConfig(pull.EnvironmentsFromProjects),
		schemas.TaskTypePullRunnersFromProjects:      config.SchedulerConfig(pull.RunnersFromProjects),
		schemas.TaskTypePullRefsFromProjects:         config.SchedulerConfig(pull.RefsFromProjects),
		schemas.TaskTypePullMetrics:                  config.SchedulerConfig(pull.Metrics),
		schemas.TaskTypeGarbageCollectProjects:       config.SchedulerConfig(gc.Projects),
		schemas.TaskTypeGarbageCollectEnvironments:   config.SchedulerConfig(gc.Environments),
		schemas.TaskTypeGarbageCollectRunners:        config.SchedulerConfig(gc.Runners),
		schemas.TaskTypeGarbageCollectRefs:           config.SchedulerConfig(gc.Refs),
		schemas.TaskTypeGarbageCollectMetrics:        config.SchedulerConfig(gc.Metrics),
	} {
		if cfg.OnInit {
			c.ScheduleTask(ctx, tt, "_")
		}

		if cfg.Scheduled {
			c.ScheduleTaskWithTicker(ctx, tt, cfg.IntervalSeconds)
		}
	}

	// Start the Redis keepalive loop only once.
	if c.Redis != nil {
		c.ScheduleRedisSetKeepalive(ctx)
	}
}

// ScheduleRedisSetKeepalive refreshes this replica's liveness marker.
// Failed updates degrade readiness and are retried on the next tick.
func (c *Controller) ScheduleRedisSetKeepalive(ctx context.Context) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "controller:ScheduleRedisSetKeepalive")
	defer span.End()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := c.setKeepaliveWithRetry(ctx)
				if ctx.Err() != nil {
					return
				}
				c.redisReady.Store(err == nil)
				if err != nil {
					log.WithContext(ctx).WithError(err).Warn("setting redis keepalive")
				}
			}
		}
	}()
}

func (c *Controller) setKeepaliveWithRetry(ctx context.Context) error {
	const attempts = 3
	const backoff = 100 * time.Millisecond
	var lastErr error
	for attempt := range attempts {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, lastErr = c.Store.(*store.Redis).SetKeepalive(checkCtx, c.UUID.String(), 10*time.Second)
		cancel()
		if lastErr == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt < attempts-1 {
			timer := time.NewTimer(time.Duration(attempt+1) * backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("refresh redis keepalive: %w", lastErr)
}

// ScheduleTask schedules a new task of type `tt` with a unique identifier `uniqueID` and optional arguments.
//
// It performs the following steps:
//  1. Starts an OpenTelemetry span for tracing the scheduling operation, annotating it with the task type and unique ID.
//  2. Retrieves the task constructor from the TaskMap and creates a new job instance with the provided arguments.
//  3. Acquires a capacity slot atomically; jobs over capacity are skipped.
//  4. Attempts to declare the task in the persistent store queue to ensure idempotency and track the task state.
//     If the task is already queued, it skips scheduling to avoid duplicates.
//  5. Publishes the job synchronously, rolling back its reservation on failure.
//  6. Logs warnings or debug messages at each failure or skip point to aid in diagnostics.
//
// This function helps ensure tasks are only scheduled when the queue has capacity and the task is not already enqueued,
// preventing duplicate work and managing system load effectively.
func (c *Controller) ScheduleTask(ctx context.Context, tt schemas.TaskType, uniqueID string, args ...interface{}) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "controller:ScheduleTask")
	defer span.End()

	span.SetAttributes(attribute.String("task_type", string(tt)))
	span.SetAttributes(attribute.String("task_unique_id", uniqueID))

	logFields := log.Fields{
		"task_type":      tt,
		"task_unique_id": uniqueID,
	}
	task := c.TaskController.TaskMap.Get(string(tt))
	if task == nil {
		log.WithContext(ctx).WithFields(logFields).Error("unknown task")
		return
	}
	admission := c.TaskController.admission
	if admission != nil && !admission.acquire() {
		log.WithContext(ctx).WithFields(logFields).Warn("queue capacity exhausted, skipping scheduling of task")
		return
	}
	if err := ctx.Err(); err != nil {
		if admission != nil {
			admission.discard()
		}
		return
	}
	reservationID := c.UUID.String() + "|" + uuid.NewString()
	var queued bool
	var err error
	if redisStore, ok := c.Store.(*store.Redis); ok {
		queued, err = redisStore.QueueTaskLimited(ctx, tt, uniqueID, reservationID, c.TaskController.capacity)
	} else {
		queued, err = c.Store.QueueTask(ctx, tt, uniqueID, reservationID)
	}
	if err != nil || !queued {
		if admission != nil {
			admission.discard()
		}
		if err != nil {
			log.WithContext(ctx).WithFields(logFields).WithError(err).Warn("reserving task")
		}
		return
	}
	if admission != nil {
		admission.track(taskKey{typeName: tt, id: uniqueID})
	}
	if c.Redis != nil {
		args = append(args, reservationMarker+reservationID+":"+uniqueID)
	}
	msg := task.NewJob(args...)
	if err := ctx.Err(); err != nil {
		c.rollbackTaskReservation(ctx, tt, uniqueID, reservationID)
		return
	}
	if err := c.TaskController.Queue.AddJob(ctx, msg); err != nil {
		log.WithContext(ctx).WithFields(logFields).WithError(err).Warn("scheduling task")
		c.rollbackTaskReservation(ctx, tt, uniqueID, reservationID)
	}
}

func (c *Controller) rollbackTaskReservation(ctx context.Context, tt schemas.TaskType, uniqueID, reservationID string) {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := c.Store.CancelTask(rollbackCtx, tt, uniqueID, reservationID); err != nil {
		log.WithContext(ctx).WithError(err).Warn("rolling back task reservation")
	}
	if c.TaskController.admission != nil {
		c.TaskController.admission.release(taskKey{typeName: tt, id: uniqueID})
	}
}

// ScheduleTaskWithTicker repeatedly schedules a task of the specified type `tt` at fixed intervals defined by `intervalSeconds`.
//
// It performs the following:
// 1. Starts an OpenTelemetry span for tracing, recording the task type and interval.
// 2. Validates the interval; if it is zero or negative, logs a warning and disables scheduling for the task.
// 3. Logs a debug message confirming the task has been scheduled with the given interval.
// 4. Updates monitoring metadata to indicate when the next scheduling is expected.
// 5. Launches a goroutine that ticks every `intervalSeconds` seconds:
//   - On each tick, it schedules the task using `ScheduleTask` with a fixed unique ID "_".
//   - Updates monitoring to track the next scheduled time.
//   - Listens for context cancellation to cleanly stop the ticker and log shutdown.
//
// This function ensures that periodic tasks are automatically scheduled at consistent intervals,
// allowing the system to regularly perform recurring work without manual intervention.
func (c *Controller) ScheduleTaskWithTicker(ctx context.Context, tt schemas.TaskType, intervalSeconds int) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "controller:ScheduleTaskWithTicker")
	defer span.End()
	span.SetAttributes(attribute.String("task_type", string(tt)))
	span.SetAttributes(attribute.Int("interval_seconds", intervalSeconds))

	if intervalSeconds <= 0 {
		log.WithContext(ctx).
			WithField("task", tt).
			Warn("task scheduling misconfigured, currently disabled")

		return
	}

	log.WithFields(log.Fields{
		"task":             tt,
		"interval_seconds": intervalSeconds,
	}).Debug("task scheduled")

	c.TaskController.monitorNextTaskScheduling(tt, intervalSeconds)

	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)

		for {
			select {
			case <-ctx.Done():
				log.WithField("task", tt).Info("scheduling of task stopped")

				return
			case <-ticker.C:
				c.ScheduleTask(ctx, tt, "_")
				c.TaskController.monitorNextTaskScheduling(tt, intervalSeconds)
			}
		}
	}(ctx)
}

// monitorNextTaskScheduling updates the monitoring status of the next expected execution time for the given task type `tt`.
// If no monitoring record exists, it creates one and sets the Next scheduled time to now + duration.
func (tc *TaskController) monitorNextTaskScheduling(tt schemas.TaskType, duration int) {
	tc.TaskSchedulingMonitoring.SetNext(tt, time.Now().Add(time.Duration(duration)*time.Second))
}

// monitorLastTaskScheduling updates the monitoring status to record the last execution time of the given task type `tt`.
// If no monitoring record exists, it creates one and sets the Last scheduled time to now.
func (tc *TaskController) monitorLastTaskScheduling(tt schemas.TaskType) {
	tc.TaskSchedulingMonitoring.SetLast(tt, time.Now())
}

const reservationMarker = "reservation-owner:v1:"

type reservationOwnerKey struct{}

type ownedTaskHandler struct {
	tasks *taskq.TaskMap
	redis *redis.Client
}

func (h ownedTaskHandler) HandleJob(ctx context.Context, job *taskq.Job) error {
	encoded, err := job.MarshalArgs()
	if err != nil {
		return err
	}
	var args []msgpack.RawMessage
	if err := msgpack.Unmarshal(encoded, &args); err != nil {
		return err
	}
	if len(args) > 0 {
		var marker string
		if msgpack.Unmarshal(args[len(args)-1], &marker) == nil && strings.HasPrefix(marker, reservationMarker) {
			parts := strings.SplitN(strings.TrimPrefix(marker, reservationMarker), ":", 2)
			if len(parts) != 2 {
				return nil
			}
			owner, err := h.redis.Get(ctx, "task:"+job.TaskName+":"+parts[1]).Result()
			if err == redis.Nil {
				return nil
			}
			if err != nil {
				return err
			}
			if owner != parts[0] {
				return nil
			}
			ctx = context.WithValue(ctx, reservationOwnerKey{}, owner)
			originalArgs, originalBin := job.Args, job.ArgsBin
			defer func() {
				job.Args, job.ArgsBin = originalArgs, originalBin
			}()
			job.ArgsBin, err = msgpack.Marshal(args[:len(args)-1])
			if err != nil {
				return err
			}
			job.Args = nil
		}
	}
	return h.tasks.HandleJob(ctx, job)
}
