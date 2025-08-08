package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"      // Redis client for Go
	"github.com/vmihailenco/msgpack/v5" // Library for MessagePack serialization

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas" // Data schemas
)

// Constants for Redis keys
const (
	redisProjectsKey           string = "projects"
	redisEnvironmentsKey       string = "environments"
	redisRunnerKey             string = "runner"
	redisRefsKey               string = "refs"
	redisMetricsKey            string = "metrics"
	redisPipelinesKey          string = "pipelines"
	redisPipelineVariablesKey  string = "pipelines_variable"
	redisTaskKey               string = "task"
	redisTasksExecutedCountKey string = "tasksExecutedCount"
	redisKeepaliveKey          string = "keepalive"
)

// Redis represents a Redis client wrapper.
type Redis struct {
	*redis.Client
}

// SetRunner stores an runner in Redis.
func (r *Redis) SetRunner(ctx context.Context, ru schemas.Runner) error {
	// Marshall the runner into binary format using MessagePack
	marshalledRunner, err := msgpack.Marshal(ru)
	if err != nil {
		return err
	}

	// Store the marshalled runner to Redis
	_, err = r.HSet(ctx, redisRunnerKey, string(ru.Key()), marshalledRunner).Result()
	return err
}

// DelRunner deletes a runner from Redis.
func (r *Redis) DelRunner(ctx context.Context, rk schemas.RunnerKey) error {
	// Delete the runner from redis
	_, err := r.HDel(ctx, redisRunnerKey, string(rk)).Result()
	return err
}

// GetRunner retrieves a runner from Redis.
func (r *Redis) GetRunner(ctx context.Context, ru *schemas.Runner) error {
	// Check if the runner exists
	exists, err := r.RunnerExists(ctx, ru.Key())
	if err != nil {
		return err
	}

	if exists {
		k := ru.Key()

		// Retrieve the marshalled runner from Redis
		marshalledEnvironment, err := r.HGet(ctx, redisRunnerKey, string(k)).Result()
		if err != nil {
			return err
		}

		// Unmarshal the environment data into the provided runner structure
		if err = msgpack.Unmarshal([]byte(marshalledEnvironment), ru); err != nil {
			return err
		}
	}

	return nil
}

// RunnerExists checks if an runner exists in Redis.
func (r *Redis) RunnerExists(ctx context.Context, rk schemas.RunnerKey) (bool, error) {
	// Check if the runner key exists in Redis
	return r.HExists(ctx, redisRunnerKey, string(rk)).Result()
}

// Runners retrieves all runners from Redis.
func (r *Redis) Runners(ctx context.Context) (schemas.Runners, error) {
	runners := schemas.Runners{}

	// Retrieve all marshalled runners from Redis
	marshalledRunners, err := r.HGetAll(ctx, redisRunnerKey).Result()
	if err != nil {
		return runners, err
	}

	// Unmarshal each runner and add it to the runners map
	for stringRunnerKey, marshalledRunner := range marshalledRunners {
		ru := schemas.Runner{}

		if err = msgpack.Unmarshal([]byte(marshalledRunner), &ru); err != nil {
			return runners, err
		}

		runners[schemas.RunnerKey(stringRunnerKey)] = ru
	}

	return runners, nil
}

// RunnersCount returns the count of runner in Redis.
func (r *Redis) RunnersCount(ctx context.Context) (int64, error) {
	// Get the number of runner stored in Redis
	return r.HLen(ctx, redisRunnerKey).Result()
}

// SetProject stores a project in Redis.
func (r *Redis) SetProject(ctx context.Context, p schemas.Project) error {
	// Marshal the project into a binary format using MessagePack
	marshalledProject, err := msgpack.Marshal(p)
	if err != nil {
		return err
	}

	// Store the marshalled project in Redis
	_, err = r.HSet(ctx, redisProjectsKey, string(p.Key()), marshalledProject).Result()
	return err
}

// DelProject deletes a project from Redis.
func (r *Redis) DelProject(ctx context.Context, k schemas.ProjectKey) error {
	// Delete the project from Redis
	_, err := r.HDel(ctx, redisProjectsKey, string(k)).Result()
	return err
}

// GetProject retrieves a project from Redis.
func (r *Redis) GetProject(ctx context.Context, p *schemas.Project) error {
	// Check if the project exists
	exists, err := r.ProjectExists(ctx, p.Key())
	if err != nil {
		return err
	}

	if exists {
		k := p.Key()

		// Retrieve the marshalled project from Redis
		marshalledProject, err := r.HGet(ctx, redisProjectsKey, string(k)).Result()
		if err != nil {
			return err
		}

		// Unmarshal the project data into the provided project structure
		if err = msgpack.Unmarshal([]byte(marshalledProject), p); err != nil {
			return err
		}
	}

	return nil
}

// ProjectExists checks if a project exists in Redis.
func (r *Redis) ProjectExists(ctx context.Context, k schemas.ProjectKey) (bool, error) {
	// Check if the project key exists in Redis
	return r.HExists(ctx, redisProjectsKey, string(k)).Result()
}

// Projects retrieves all projects from Redis.
func (r *Redis) Projects(ctx context.Context) (schemas.Projects, error) {
	projects := schemas.Projects{}

	// Retrieve all marshalled projects from Redis
	marshalledProjects, err := r.HGetAll(ctx, redisProjectsKey).Result()
	if err != nil {
		return projects, err
	}

	// Unmarshal each project and add it to the projects map
	for stringProjectKey, marshalledProject := range marshalledProjects {
		p := schemas.Project{}

		if err = msgpack.Unmarshal([]byte(marshalledProject), &p); err != nil {
			return projects, err
		}

		projects[schemas.ProjectKey(stringProjectKey)] = p
	}

	return projects, nil
}

// ProjectsCount returns the count of projects in Redis.
func (r *Redis) ProjectsCount(ctx context.Context) (int64, error) {
	// Get the number of projects stored in Redis
	return r.HLen(ctx, redisProjectsKey).Result()
}

// SetEnvironment stores an environment in Redis.
func (r *Redis) SetEnvironment(ctx context.Context, e schemas.Environment) error {
	// Marshal the environment into a binary format using MessagePack
	marshalledEnvironment, err := msgpack.Marshal(e)
	if err != nil {
		return err
	}

	// Store the marshalled environment in Redis
	_, err = r.HSet(ctx, redisEnvironmentsKey, string(e.Key()), marshalledEnvironment).Result()
	return err
}

// DelEnvironment deletes an environment from Redis.
func (r *Redis) DelEnvironment(ctx context.Context, k schemas.EnvironmentKey) error {
	// Delete the environment from Redis
	_, err := r.HDel(ctx, redisEnvironmentsKey, string(k)).Result()
	return err
}

// GetEnvironment retrieves an environment from Redis.
func (r *Redis) GetEnvironment(ctx context.Context, e *schemas.Environment) error {
	// Check if the environment exists
	exists, err := r.EnvironmentExists(ctx, e.Key())
	if err != nil {
		return err
	}

	if exists {
		k := e.Key()

		// Retrieve the marshalled environment from Redis
		marshalledEnvironment, err := r.HGet(ctx, redisEnvironmentsKey, string(k)).Result()
		if err != nil {
			return err
		}

		// Unmarshal the environment data into the provided environment structure
		if err = msgpack.Unmarshal([]byte(marshalledEnvironment), e); err != nil {
			return err
		}
	}

	return nil
}

// EnvironmentExists checks if an environment exists in Redis.
func (r *Redis) EnvironmentExists(ctx context.Context, k schemas.EnvironmentKey) (bool, error) {
	// Check if the environment key exists in Redis
	return r.HExists(ctx, redisEnvironmentsKey, string(k)).Result()
}

// Environments retrieves all environments from Redis.
func (r *Redis) Environments(ctx context.Context) (schemas.Environments, error) {
	environments := schemas.Environments{}

	// Retrieve all marshalled environments from Redis
	marshalledEnvironments, err := r.HGetAll(ctx, redisEnvironmentsKey).Result()
	if err != nil {
		return environments, err
	}

	// Unmarshal each environment and add it to the environments map
	for stringEnvironmentKey, marshalledEnvironment := range marshalledEnvironments {
		e := schemas.Environment{}

		if err = msgpack.Unmarshal([]byte(marshalledEnvironment), &e); err != nil {
			return environments, err
		}

		environments[schemas.EnvironmentKey(stringEnvironmentKey)] = e
	}

	return environments, nil
}

// EnvironmentsCount returns the count of environments in Redis.
func (r *Redis) EnvironmentsCount(ctx context.Context) (int64, error) {
	// Get the number of environments stored in Redis
	return r.HLen(ctx, redisEnvironmentsKey).Result()
}

// SetRef stores a reference in Redis.
func (r *Redis) SetRef(ctx context.Context, ref schemas.Ref) error {
	// Marshal the reference into a binary format using MessagePack
	marshalledRef, err := msgpack.Marshal(ref)
	if err != nil {
		return err
	}

	// Store the marshalled reference in Redis
	_, err = r.HSet(ctx, redisRefsKey, string(ref.Key()), marshalledRef).Result()
	return err
}

// DelRef deletes a reference from Redis.
func (r *Redis) DelRef(ctx context.Context, k schemas.RefKey) error {
	// Delete the reference from Redis
	_, err := r.HDel(ctx, redisRefsKey, string(k)).Result()
	return err
}

// GetRef retrieves a reference from Redis.
func (r *Redis) GetRef(ctx context.Context, ref *schemas.Ref) error {
	// Check if the reference exists
	exists, err := r.RefExists(ctx, ref.Key())
	if err != nil {
		return err
	}

	if exists {
		k := ref.Key()

		// Retrieve the marshalled reference from Redis
		marshalledRef, err := r.HGet(ctx, redisRefsKey, string(k)).Result()
		if err != nil {
			return err
		}

		// Unmarshal the reference data into the provided reference structure
		if err = msgpack.Unmarshal([]byte(marshalledRef), ref); err != nil {
			return err
		}
	}

	return nil
}

// RefExists checks if a reference exists in Redis.
func (r *Redis) RefExists(ctx context.Context, k schemas.RefKey) (bool, error) {
	// Check if the reference key exists in Redis
	return r.HExists(ctx, redisRefsKey, string(k)).Result()
}

// Refs retrieves all references from Redis.
func (r *Redis) Refs(ctx context.Context) (schemas.Refs, error) {
	refs := schemas.Refs{}

	// Retrieve all marshalled references from Redis
	marshalledRefs, err := r.HGetAll(ctx, redisRefsKey).Result()
	if err != nil {
		return refs, err
	}

	// Unmarshal each reference and add it to the refs map
	for stringRefKey, marshalledRef := range marshalledRefs {
		ref := schemas.Ref{}

		if err = msgpack.Unmarshal([]byte(marshalledRef), &ref); err != nil {
			return refs, err
		}

		refs[schemas.RefKey(stringRefKey)] = ref
	}

	return refs, nil
}

// RefsCount returns the count of references in Redis.
func (r *Redis) RefsCount(ctx context.Context) (int64, error) {
	// Get the number of references stored in Redis
	return r.HLen(ctx, redisRefsKey).Result()
}

// SetMetric stores a metric in Redis.
func (r *Redis) SetMetric(ctx context.Context, m schemas.Metric) error {
	// Marshal the metric into a binary format using MessagePack
	marshalledMetric, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}

	// Store the marshalled metric in Redis
	_, err = r.HSet(ctx, redisMetricsKey, string(m.Key()), marshalledMetric).Result()
	return err
}

// DelMetric deletes a metric from Redis.
func (r *Redis) DelMetric(ctx context.Context, k schemas.MetricKey) error {
	// Delete the metric from Redis
	_, err := r.HDel(ctx, redisMetricsKey, string(k)).Result()
	return err
}

// MetricExists checks if a metric exists in Redis.
func (r *Redis) MetricExists(ctx context.Context, k schemas.MetricKey) (bool, error) {
	// Check if the metric key exists in Redis
	return r.HExists(ctx, redisMetricsKey, string(k)).Result()
}

// GetMetric retrieves a metric from Redis.
func (r *Redis) GetMetric(ctx context.Context, m *schemas.Metric) error {
	// Check if the metric exists
	exists, err := r.MetricExists(ctx, m.Key())
	if err != nil {
		return err
	}

	if exists {
		k := m.Key()

		// Retrieve the marshalled metric from Redis
		marshalledMetric, err := r.HGet(ctx, redisMetricsKey, string(k)).Result()
		if err != nil {
			return err
		}

		// Unmarshal the metric data into the provided metric structure
		if err = msgpack.Unmarshal([]byte(marshalledMetric), m); err != nil {
			return err
		}
	}

	return nil
}

// Metrics retrieves all metrics from Redis.
func (r *Redis) Metrics(ctx context.Context) (schemas.Metrics, error) {
	metrics := schemas.Metrics{}

	// Retrieve all marshalled metrics from Redis
	marshalledMetrics, err := r.HGetAll(ctx, redisMetricsKey).Result()
	if err != nil {
		return metrics, err
	}

	// Unmarshal each metric and add it to the metrics map
	for stringMetricKey, marshalledMetric := range marshalledMetrics {
		m := schemas.Metric{}

		if err := msgpack.Unmarshal([]byte(marshalledMetric), &m); err != nil {
			return metrics, err
		}

		metrics[schemas.MetricKey(stringMetricKey)] = m
	}

	return metrics, nil
}

// MetricsCount returns the count of metrics in Redis.
func (r *Redis) MetricsCount(ctx context.Context) (int64, error) {
	// Get the number of metrics stored in Redis
	return r.HLen(ctx, redisMetricsKey).Result()
}

func (r *Redis) SetPipeline(ctx context.Context, pipeline schemas.Pipeline) error {
	marshalledPipeline, err := msgpack.Marshal(pipeline)
	if err != nil {
		return err
	}

	_, err = r.HSet(ctx, redisPipelinesKey, fmt.Sprintf("%d", pipeline.Key()), marshalledPipeline).Result()

	return err
}

func (r *Redis) GetPipeline(ctx context.Context, pipeline *schemas.Pipeline) error {
	exists, err := r.PipelineExists(ctx, pipeline.Key())
	if err != nil {
		return err
	}

	if exists {
		k := pipeline.Key()

		marshalledPipeline, err := r.HGet(ctx, redisPipelinesKey, fmt.Sprintf("%d", k)).Result()
		if err != nil {
			return err
		}

		if err = msgpack.Unmarshal([]byte(marshalledPipeline), pipeline); err != nil {
			return err
		}
	}

	return nil
}

func (r *Redis) PipelineExists(ctx context.Context, key schemas.PipelineKey) (bool, error) {
	return r.HExists(ctx, redisPipelinesKey, fmt.Sprintf("%d", key)).Result()
}

func (r *Redis) SetPipelineVariables(ctx context.Context, pipeline schemas.Pipeline, variables string) error {
	marshalledVariables, err := msgpack.Marshal(variables)
	if err != nil {
		return err
	}

	_, err = r.HSet(ctx, redisPipelineVariablesKey, fmt.Sprintf("%d", pipeline.ID), marshalledVariables).Result()

	return err
}

func (r *Redis) GetPipelineVariables(ctx context.Context, pipeline schemas.Pipeline) (string, error) {
	exists, err := r.PipelineVariablesExists(ctx, pipeline)
	if err != nil {
		return "", err
	}

	if exists {
		k := fmt.Sprintf("%d", pipeline.ID)

		marshalledVariables, err := r.HGet(ctx, redisPipelineVariablesKey, string(k)).Result()
		if err != nil {
			return "", err
		}
		var variables string

		if err = msgpack.Unmarshal([]byte(marshalledVariables), variables); err != nil {
			return variables, err
		}
	}

	return "", err
}

func (r *Redis) PipelineVariablesExists(ctx context.Context, pipeline schemas.Pipeline) (bool, error) {
	return r.HExists(ctx, redisPipelineVariablesKey, fmt.Sprintf("%d", pipeline.ID)).Result()
}

// SetKeepalive sets a key with a UUID corresponding to the currently running process.
func (r *Redis) SetKeepalive(ctx context.Context, uuid string, ttl time.Duration) (bool, error) {
	// Set a key with the UUID and a time-to-live (TTL) in Redis
	return r.SetNX(ctx, fmt.Sprintf("%s:%s", redisKeepaliveKey, uuid), nil, ttl).Result()
}

// KeepaliveExists returns whether a keepalive exists or not for a particular UUID.
func (r *Redis) KeepaliveExists(ctx context.Context, uuid string) (bool, error) {
	// Check if the keepalive key exists in Redis
	exists, err := r.Exists(ctx, fmt.Sprintf("%s:%s", redisKeepaliveKey, uuid)).Result()
	return exists == 1, err
}

// getRedisQueueKey generates a Redis key for a task.
func getRedisQueueKey(tt schemas.TaskType, taskUUID string) string {
	return fmt.Sprintf("%s:%v:%s", redisTaskKey, tt, taskUUID)
}

// QueueTask registers that we are queueing the task.
// It returns true if it managed to schedule it, false if it was already scheduled.
func (r *Redis) QueueTask(ctx context.Context, tt schemas.TaskType, taskUUID, processUUID string) (set bool, err error) {
	k := getRedisQueueKey(tt, taskUUID)

	// Attempt to set the key, if it already exists, do not overwrite it
	set, err = r.SetNX(ctx, k, processUUID, 0).Result()
	if err != nil || set {
		return
	}

	// If the key already exists, check if the associated process UUID is the same as the current one
	var tpuuid string
	if tpuuid, err = r.Get(ctx, k).Result(); err != nil {
		return
	}

	// If the process UUID is different, check if the associated process is still alive
	if tpuuid != processUUID {
		var uuidIsAlive bool
		if uuidIsAlive, err = r.KeepaliveExists(ctx, tpuuid); err != nil {
			return
		}

		// If the process is not alive, override the key and schedule the task
		if !uuidIsAlive {
			if _, err = r.Set(ctx, k, processUUID, 0).Result(); err != nil {
				return
			}
			return true, nil
		}
	}

	return
}

// UnqueueTask removes the task from the tracker.
func (r *Redis) UnqueueTask(ctx context.Context, tt schemas.TaskType, taskUUID string) (err error) {
	var matched int64

	// Delete the task key from Redis
	matched, err = r.Del(ctx, getRedisQueueKey(tt, taskUUID)).Result()
	if err != nil {
		return
	}

	// Increment the count of executed tasks
	if matched > 0 {
		_, err = r.Incr(ctx, redisTasksExecutedCountKey).Result()
	}

	return
}

// CurrentlyQueuedTasksCount returns the count of currently queued tasks.
func (r *Redis) CurrentlyQueuedTasksCount(ctx context.Context) (count uint64, err error) {
	// Scan for all task keys and count them
	iter := r.Scan(ctx, 0, fmt.Sprintf("%s:*", redisTaskKey), 0).Iterator()
	for iter.Next(ctx) {
		count++
	}

	err = iter.Err()
	return
}

// ExecutedTasksCount returns the count of executed tasks.
func (r *Redis) ExecutedTasksCount(ctx context.Context) (uint64, error) {
	// Retrieve the count of executed tasks from Redis
	countString, err := r.Get(ctx, redisTasksExecutedCountKey).Result()
	if err != nil {
		return 0, err
	}

	// Convert the count string to an integer
	c, err := strconv.Atoi(countString)
	return uint64(c), err
}
