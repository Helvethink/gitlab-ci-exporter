package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

type telemetryStreamStub struct {
	ctx    context.Context
	cancel context.CancelFunc
	sent   []*pb.Telemetry
}

func (s *telemetryStreamStub) Send(tel *pb.Telemetry) error {
	s.sent = append(s.sent, tel)
	s.cancel()
	return nil
}

func (s *telemetryStreamStub) SetHeader(metadata.MD) error  { return nil }
func (s *telemetryStreamStub) SendHeader(metadata.MD) error { return nil }
func (s *telemetryStreamStub) SetTrailer(metadata.MD)       {}
func (s *telemetryStreamStub) Context() context.Context     { return s.ctx }
func (s *telemetryStreamStub) SendMsg(interface{}) error    { return nil }
func (s *telemetryStreamStub) RecvMsg(interface{}) error    { return nil }

func TestNewServer(t *testing.T) {
	cfg := config.New()
	st := store.NewLocalStore()
	tsm := &monitor.SchedulingStatus{}
	g := &gitlab.Client{}

	s := NewServer(g, cfg, st, tsm)

	require.NotNil(t, s)
	assert.Same(t, g, s.gitlabClient)
	assert.Equal(t, cfg, s.cfg)
	assert.Same(t, st, s.store)
	assert.Equal(t, tsm, s.taskSchedulingMonitoring)
}

func TestServeWithoutInternalMonitoringAddressReturns(t *testing.T) {
	s := NewServer(&gitlab.Client{}, config.New(), store.NewLocalStore(), nil)
	s.cfg.Global.InternalMonitoringListenerAddress = nil

	assert.NotPanics(t, func() {
		s.Serve()
	})
}

func TestGetConfig(t *testing.T) {
	cfg := config.New()
	cfg.Gitlab.Token = "secret-token"
	cfg.Server.Webhook.SecretToken = "webhook-secret"

	s := NewServer(&gitlab.Client{}, cfg, store.NewLocalStore(), nil)

	got, err := s.GetConfig(context.Background(), &pb.Empty{})
	require.NoError(t, err)
	assert.Contains(t, got.GetContent(), "*******")
	assert.NotContains(t, got.GetContent(), "secret-token")
	assert.NotContains(t, got.GetContent(), "webhook-secret")
}

func TestGetTelemetry(t *testing.T) {
	ctx := context.Background()
	st := store.NewLocalStore()
	require.NoError(t, st.SetProject(ctx, schemas.NewProject("group/project")))
	require.NoError(t, st.SetEnvironment(ctx, schemas.Environment{ProjectName: "group/project", Name: "production"}))
	require.NoError(t, st.SetRunner(ctx, schemas.Runner{ID: 42, ProjectName: "group/project"}))
	require.NoError(t, st.SetRef(ctx, schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")))
	require.NoError(t, st.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: map[string]string{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 1,
	}))
	ok, err := st.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-1", "")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, st.DequeueTask(ctx, schemas.TaskTypePullMetrics, "task-1"))
	ok, err = st.QueueTask(ctx, schemas.TaskTypePullMetrics, "task-2", "")
	require.NoError(t, err)
	require.True(t, ok)

	now := time.Unix(1710000000, 0)
	tsm := &monitor.SchedulingStatus{}
	statuses := []schemas.TaskType{
		schemas.TaskTypePullProjectsFromWildcards,
		schemas.TaskTypeGarbageCollectProjects,
		schemas.TaskTypePullEnvironmentsFromProjects,
		schemas.TaskTypeGarbageCollectEnvironments,
		schemas.TaskTypePullRunnersFromProjects,
		schemas.TaskTypeGarbageCollectRunners,
		schemas.TaskTypePullRefsFromProjects,
		schemas.TaskTypeGarbageCollectRefs,
		schemas.TaskTypePullMetrics,
		schemas.TaskTypeGarbageCollectMetrics,
	}
	for i, task := range statuses {
		tsm.SetLast(task, now.Add(time.Duration(i*2)*time.Minute))
		tsm.SetNext(task, now.Add(time.Duration(i*2+1)*time.Minute))
	}

	g := &gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}
	g.UpdateRateLimit(5, 10)
	g.RequestsCounter.Add(7)
	g.RateCounter.Incr(2)

	cfg := config.New()
	cfg.Gitlab.MaximumRequestsPerSecond = 4

	s := NewServer(g, cfg, st, tsm)
	streamCtx, cancel := context.WithCancel(context.Background())
	stream := &telemetryStreamStub{ctx: streamCtx, cancel: cancel}

	err = s.GetTelemetry(&pb.Empty{}, stream)
	require.NoError(t, err)
	require.Len(t, stream.sent, 1)

	tel := stream.sent[0]
	assert.Equal(t, uint64(7), tel.GetGitlabApiRequestsCount())
	assert.Equal(t, uint64(5), tel.GetGitlabApiLimitRemaining())
	assert.Equal(t, uint64(1), tel.GetTasksExecutedCount())
	assert.InDelta(t, 0.5, tel.GetGitlabApiUsage(), 0.001)
	assert.InDelta(t, 0.5, tel.GetGitlabApiRateLimit(), 0.001)
	assert.InDelta(t, 0.001, tel.GetTasksBufferUsage(), 0.0001)
	assert.Equal(t, int64(1), tel.GetProjects().GetCount())
	assert.Equal(t, int64(1), tel.GetEnvs().GetCount())
	assert.Equal(t, int64(1), tel.GetRefs().GetCount())
	assert.Equal(t, int64(1), tel.GetMetrics().GetCount())
	require.NotNil(t, tel.Runners)
	assert.Equal(t, int64(1), tel.Runners.GetCount())
	assert.Equal(t, now.Unix(), tel.GetProjects().GetLastPull().AsTime().Unix())
	assert.Equal(t, now.Add(19*time.Minute).Unix(), tel.GetMetrics().GetNextGc().AsTime().Unix())
}

func TestGetTelemetryWithRunnerPullOnly(t *testing.T) {
	status := &monitor.SchedulingStatus{}
	status.SetNext(schemas.TaskTypePullRunnersFromProjects, time.Now())
	g := &gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}
	s := NewServer(g, config.New(), store.NewLocalStore(), status)
	ctx, cancel := context.WithCancel(context.Background())
	stream := &telemetryStreamStub{ctx: ctx, cancel: cancel}

	require.NoError(t, s.GetTelemetry(&pb.Empty{}, stream))
	require.Len(t, stream.sent, 1)
	assert.Nil(t, stream.sent[0].GetRunners().GetLastGc())
	assert.Zero(t, stream.sent[0].GetGitlabApiRateLimit())
}

func TestGetTelemetryDuringConcurrentScheduleUpdates(t *testing.T) {
	status := &monitor.SchedulingStatus{}
	g := &gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}
	s := NewServer(g, config.New(), store.NewLocalStore(), status)
	ctx, cancel := context.WithCancel(context.Background())
	stream := &telemetryStreamStub{ctx: ctx, cancel: cancel}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				status.SetNext(schemas.TaskTypePullRunnersFromProjects, time.Now())
				status.SetLast(schemas.TaskTypeGarbageCollectRunners, time.Now())
			}
		}()
	}
	require.NoError(t, s.GetTelemetry(&pb.Empty{}, stream))
	wg.Wait()
	require.Len(t, stream.sent, 1)
}
