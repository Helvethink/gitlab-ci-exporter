package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

func TestRegisterTasksRegistersAllHandlers(t *testing.T) {
	ctx := context.Background()
	c := &Controller{
		TaskController: NewTaskController(ctx, nil, 10),
	}

	c.registerTasks()

	for _, tt := range []schemas.TaskType{
		schemas.TaskTypeGarbageCollectEnvironments,
		schemas.TaskTypeGarbageCollectMetrics,
		schemas.TaskTypeGarbageCollectProjects,
		schemas.TaskTypeGarbageCollectRefs,
		schemas.TaskTypeGarbageCollectRunners,
		schemas.TaskTypePullEnvironmentMetrics,
		schemas.TaskTypePullEnvironmentsFromProject,
		schemas.TaskTypePullEnvironmentsFromProjects,
		schemas.TaskTypePullMetrics,
		schemas.TaskTypePullProject,
		schemas.TaskTypePullProjectsFromWildcard,
		schemas.TaskTypePullProjectsFromWildcards,
		schemas.TaskTypePullRefMetrics,
		schemas.TaskTypePullRefsFromProject,
		schemas.TaskTypePullRefsFromProjects,
		schemas.TaskTypePullRunnersMetrics,
		schemas.TaskTypePullRunnersFromProject,
		schemas.TaskTypePullRunnersFromProjects,
	} {
		assert.NotNil(t, c.TaskController.TaskMap.Get(string(tt)), string(tt))
	}
}

func TestDequeueTaskRemovesTaskFromStore(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	c := &Controller{Store: s}

	ok, err := s.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-1", "")
	require.NoError(t, err)
	require.True(t, ok)

	c.dequeueTask(ctx, schemas.TaskTypePullMetrics, "task-1")

	queued, err := s.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), queued)

	executed, err := s.ExecutedTasksCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), executed)
}

func TestConfigureTracingWithoutEndpointReturnsNil(t *testing.T) {
	provider, err := configureTracing(context.Background(), "")
	assert.NoError(t, err)
	assert.Nil(t, provider)
}

func TestConfigureGitlabInitializesClient(t *testing.T) {
	c := &Controller{}

	err := c.configureGitlab(config.Gitlab{
		URL:                        "https://gitlab.example.com",
		HealthURL:                  "https://gitlab.example.com/-/health",
		Token:                      "test-token",
		EnableTLSVerify:            true,
		MaximumRequestsPerSecond:   5,
		BurstableRequestsPerSecond: 10,
		MaximumJobsQueueSize:       10,
		EnableHealthCheck:          true,
	}, "1.2.3")
	require.NoError(t, err)

	require.NotNil(t, c.Gitlab)
	assert.Equal(t, "https://gitlab.example.com/-/health", c.Gitlab.Readiness.URL)
	assert.Contains(t, c.Gitlab.UserAgent, "1.2.3")
	assert.NotNil(t, c.Gitlab.RateLimiter)
}

func TestConfigureRedisWithoutURLSkipsConfiguration(t *testing.T) {
	c := &Controller{}

	err := c.configureRedis(context.Background(), &config.Redis{})
	require.NoError(t, err)
	assert.Nil(t, c.Redis)
}

func TestConfigureRedisWithMiniredisConnectsSuccessfully(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)

	c := &Controller{}
	err := c.configureRedis(ctx, &config.Redis{
		URL: "redis://" + mr.Addr(),
	})
	require.NoError(t, err)
	require.NotNil(t, c.Redis)

	pong, err := c.Redis.Ping(ctx).Result()
	require.NoError(t, err)
	assert.Equal(t, "PONG", pong)
}

func TestConfigureRedisWithInvalidURLReturnsError(t *testing.T) {
	c := &Controller{}

	err := c.configureRedis(context.Background(), &config.Redis{
		URL: "://bad-url",
	})
	require.Error(t, err)
}

func TestNewControllerClosesStartedResources(t *testing.T) {
	mr := miniredis.RunT(t)
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer gitlabServer.Close()
	cfg := config.New()
	cfg.Redis.URL = "redis://" + mr.Addr()
	cfg.Gitlab.URL = gitlabServer.URL + "/api/v4"
	cfg.Gitlab.Token = "test-token"
	cfg.Pull = config.Pull{}
	cfg.GarbageCollect = config.GarbageCollect{}
	cfg.Pull.Metrics.Scheduled = true
	cfg.Pull.Metrics.IntervalSeconds = 1

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := New(appCtx, cfg, "test")
	require.NoError(t, err)
	require.NotNil(t, c)
	shutdownCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	require.NoError(t, c.Close(shutdownCtx))
	assert.False(t, c.startBackground(func() {}))
	assert.Error(t, c.Redis.Ping(context.Background()).Err())
}
