package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

const (
	// redisLegacyPipelineVariablesKey is the hash used by earlier versions to persist
	// pipeline variables as "KEY:VALUE" pairs, including raw values.
	redisLegacyPipelineVariablesKey = "pipelines_variable"

	// redisMigrationPipelineVariableKeysOnly marks that legacy variable values were purged.
	redisMigrationPipelineVariableKeysOnly = "migrations:pipeline_variable_keys_only"
)

// PurgeLegacyPipelineVariables removes pipeline-variable values persisted by earlier
// versions of the exporter. It is idempotent and runs its cleanup only once per Redis
// database, recorded by a marker key.
//
// It removes:
//   - the legacy "pipelines_variable" hash;
//   - the "pipelines" cache, whose entries may embed variable values (it is rebuilt on the next pull);
//   - variable data embedded in the latest pipeline of each stored ref;
//   - every stored metric carrying a non-empty "variables" label (they are recreated on the next pull).
func (r *Redis) PurgeLegacyPipelineVariables(ctx context.Context) error {
	done, err := r.Exists(ctx, redisMigrationPipelineVariableKeysOnly).Result()
	if err != nil {
		return fmt.Errorf("check pipeline variables migration marker: %w", err)
	}
	if done > 0 {
		return nil
	}

	if err := r.Del(ctx, redisLegacyPipelineVariablesKey, redisPipelinesKey).Err(); err != nil {
		return fmt.Errorf("delete legacy pipeline variables: %w", err)
	}

	if err := r.scrubRefVariables(ctx); err != nil {
		return err
	}

	if err := r.scrubMetricVariables(ctx); err != nil {
		return err
	}

	if err := r.Set(ctx, redisMigrationPipelineVariableKeysOnly, 1, 0).Err(); err != nil {
		return fmt.Errorf("set pipeline variables migration marker: %w", err)
	}

	return nil
}

func (r *Redis) scrubRefVariables(ctx context.Context) error {
	marshalledRefs, err := r.HGetAll(ctx, redisRefsKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("read refs for pipeline variables migration: %w", err)
	}

	for key, marshalledRef := range marshalledRefs {
		var ref schemas.Ref
		if err := msgpack.Unmarshal([]byte(marshalledRef), &ref); err != nil {
			// An undecodable entry cannot be scrubbed safely; drop it, it is rebuilt on the next pull.
			if err := r.HDel(ctx, redisRefsKey, key).Err(); err != nil {
				return fmt.Errorf("delete undecodable ref during pipeline variables migration: %w", err)
			}
			continue
		}
		if ref.LatestPipeline.Variables == "" {
			continue
		}

		ref.LatestPipeline.Variables = ""
		updated, err := msgpack.Marshal(ref)
		if err != nil {
			return fmt.Errorf("encode ref during pipeline variables migration: %w", err)
		}
		if err := r.HSet(ctx, redisRefsKey, key, updated).Err(); err != nil {
			return fmt.Errorf("write ref during pipeline variables migration: %w", err)
		}
	}

	return nil
}

func (r *Redis) scrubMetricVariables(ctx context.Context) error {
	marshalledMetrics, err := r.HGetAll(ctx, redisMetricsKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("read metrics for pipeline variables migration: %w", err)
	}

	var stale []string
	for key, marshalledMetric := range marshalledMetrics {
		var m schemas.Metric
		if err := msgpack.Unmarshal([]byte(marshalledMetric), &m); err != nil || m.Labels["variables"] != "" {
			stale = append(stale, key)
		}
	}

	if len(stale) == 0 {
		return nil
	}
	if err := r.HDel(ctx, redisMetricsKey, stale...).Err(); err != nil {
		return fmt.Errorf("delete metrics during pipeline variables migration: %w", err)
	}

	return nil
}
