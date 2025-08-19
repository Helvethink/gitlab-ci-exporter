package store

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

var testCtx = context.Background()

func TestNewLocalStore(t *testing.T) {
	expectedValue := &Local{
		projects:          make(schemas.Projects),
		environments:      make(schemas.Environments),
		refs:              make(schemas.Refs),
		runners:           make(schemas.Runners),
		metrics:           make(schemas.Metrics),
		pipelines:         make(schemas.Pipelines),
		pipelineVariables: make(map[schemas.PipelineKey]string),
	}
	assert.Equal(t, expectedValue, NewLocalStore())
}

func TestNewRedisStore(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{})
	redisStore := NewRedisStore(redisClient)

	assert.IsType(t, &Redis{}, redisStore)
	assert.Equal(t, redisClient, redisStore.Client)
	assert.NotNil(t, redisStore.StoreConfig) // since constructor sets it
}

func TestNew(t *testing.T) {
	localStore := New(testCtx, nil, config.Projects{})
	assert.IsType(t, &Local{}, localStore)

	redisClient := redis.NewClient(&redis.Options{})
	redisStore := NewRedisStore(redisClient)
	store := New(testCtx, redisStore, config.Projects{})
	assert.IsType(t, &Redis{}, store)

	localStore = New(testCtx, nil, config.Projects{
		{
			Name: "foo",
		},
		{
			Name: "foo",
		},
		{
			Name: "bar",
		},
	})
	count, _ := localStore.ProjectsCount(testCtx)
	assert.Equal(t, int64(2), count)
}
