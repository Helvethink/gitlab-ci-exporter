package store

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func newStoreGoTestRedis(t *testing.T) (*miniredis.Miniredis, *Redis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	return mr, NewRedisStore(client)
}

func testStoreProjects() config.Projects {
	return config.Projects{
		config.NewProject("group/project1"),
		config.NewProject("group/project2"),
	}
}

func TestNewLocalStore(t *testing.T) {
	s := NewLocalStore()

	l, ok := s.(*Local)
	require.True(t, ok)

	assert.NotNil(t, l.projects)
	assert.NotNil(t, l.environments)
	assert.NotNil(t, l.runners)
	assert.NotNil(t, l.refs)
	assert.NotNil(t, l.metrics)
	assert.NotNil(t, l.pipelines)
	assert.NotNil(t, l.pipelineVariables)

	assert.Len(t, l.projects, 0)
	assert.Len(t, l.environments, 0)
	assert.Len(t, l.runners, 0)
	assert.Len(t, l.refs, 0)
	assert.Len(t, l.metrics, 0)
	assert.Len(t, l.pipelines, 0)
	assert.Len(t, l.pipelineVariables, 0)
}

func TestNewRedisStore(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() {
		_ = client.Close()
	})

	r := NewRedisStore(client)

	require.NotNil(t, r)
	assert.Same(t, client, r.Client)
	require.NotNil(t, r.StoreConfig)
	assert.Nil(t, r.StoreConfig.TTLConfig)
}

func TestNewRedisStore_WithTTLConfig(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() {
		_ = client.Close()
	})

	ttl := &RedisTTLConfig{
		Project:     time.Minute,
		Environment: 2 * time.Minute,
		Runner:      3 * time.Minute,
		Refs:        4 * time.Minute,
		Metrics:     5 * time.Minute,
	}

	r := NewRedisStore(client, WithTTLConfig(ttl))

	require.NotNil(t, r)
	require.NotNil(t, r.StoreConfig)
	require.NotNil(t, r.StoreConfig.TTLConfig)

	assert.Equal(t, time.Minute, r.StoreConfig.TTLConfig.Project)
	assert.Equal(t, 2*time.Minute, r.StoreConfig.TTLConfig.Environment)
	assert.Equal(t, 3*time.Minute, r.StoreConfig.TTLConfig.Runner)
	assert.Equal(t, 4*time.Minute, r.StoreConfig.TTLConfig.Refs)
	assert.Equal(t, 5*time.Minute, r.StoreConfig.TTLConfig.Metrics)
}

func TestNew_WithNilRedis_UsesLocalStoreAndLoadsProjects(t *testing.T) {
	ctx := context.Background()
	projects := testStoreProjects()

	s := New(ctx, nil, projects)

	_, ok := s.(*Local)
	require.True(t, ok)

	count, err := s.ProjectsCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	for _, p := range projects {
		project := schemas.Project{Project: p}

		exists, err := s.ProjectExists(ctx, project.Key())
		require.NoError(t, err)
		assert.True(t, exists)
	}
}

func TestNew_WithRedis_UsesRedisStoreAndLoadsProjects(t *testing.T) {
	ctx := context.Background()
	_, r := newStoreGoTestRedis(t)
	projects := testStoreProjects()

	s := New(ctx, r, projects)

	rr, ok := s.(*Redis)
	require.True(t, ok)
	assert.Same(t, r, rr)

	count, err := s.ProjectsCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	for _, p := range projects {
		project := schemas.Project{Project: p}

		exists, err := s.ProjectExists(ctx, project.Key())
		require.NoError(t, err)
		assert.True(t, exists)
	}
}

func TestNew_DoesNotOverwriteExistingProjectInRedis(t *testing.T) {
	ctx := context.Background()
	_, r := newStoreGoTestRedis(t)

	existing := schemas.NewProject("group/project1")
	existing.Topics = "keep-me"
	require.NoError(t, r.SetProject(ctx, existing))

	projects := config.Projects{
		config.NewProject("group/project1"),
	}

	s := New(ctx, r, projects)

	got := schemas.NewProject("group/project1")
	require.NoError(t, s.GetProject(ctx, &got))

	assert.Equal(t, "keep-me", got.Topics)

	count, err := s.ProjectsCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}
