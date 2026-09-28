package server

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

type telemetryStreamStub struct {
	ctx     context.Context
	cancel  context.CancelFunc
	sent    []*pb.Telemetry
	sendErr error
}

func (s *telemetryStreamStub) Send(tel *pb.Telemetry) error {
	if s.sendErr != nil {
		return s.sendErr
	}
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

	require.NoError(t, s.Serve(context.Background()))
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
	require.NoError(t, st.DequeueTask(ctx, schemas.TaskTypePullMetrics, "task-1", ""))
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

func TestServeReturnsListenerError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	cfg := config.New()
	cfg.Global.InternalMonitoringListenerAddress = &url.URL{Scheme: "tcp", Host: listener.Addr().String()}
	s := NewServer(&gitlab.Client{}, cfg, store.NewLocalStore(), nil)
	require.Error(t, s.Serve(context.Background()))
}

func TestServeStopsAndRemovesSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.sock")
	cfg := config.New()
	cfg.Global.InternalMonitoringListenerAddress = &url.URL{Scheme: "unix", Path: path}
	s := NewServer(&gitlab.Client{}, cfg, store.NewLocalStore(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, time.Second, 10*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(6 * time.Second):
		t.Fatal("monitoring server did not stop")
	}
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestGetTelemetrySendFailureIsPerClient(t *testing.T) {
	g := &gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}
	s := NewServer(g, config.New(), store.NewLocalStore(), nil)
	sendErr := errors.New("client disconnected")
	stream := &telemetryStreamStub{ctx: context.Background(), sendErr: sendErr}
	require.ErrorIs(t, s.GetTelemetry(&pb.Empty{}, stream), sendErr)

	ctx, cancel := context.WithCancel(context.Background())
	healthyStream := &telemetryStreamStub{ctx: ctx, cancel: cancel}
	require.NoError(t, s.GetTelemetry(&pb.Empty{}, healthyStream))
	require.Len(t, healthyStream.sent, 1)
}

func TestDisconnectedTelemetryClientDoesNotStopServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.sock")
	cfg := config.New()
	cfg.Global.InternalMonitoringListenerAddress = &url.URL{Scheme: "unix", Path: path}
	g := &gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}
	s := NewServer(g, cfg, store.NewLocalStore(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(6 * time.Second):
			t.Error("monitoring server did not stop")
		}
	})

	conn, err := grpc.NewClient("unix://"+path,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	client := pb.NewMonitorClient(conn)
	require.Eventually(t, func() bool {
		callCtx, callCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer callCancel()
		_, err := client.GetConfig(callCtx, &pb.Empty{})
		return err == nil
	}, 3*time.Second, 10*time.Millisecond)
	streamCtx, disconnect := context.WithCancel(context.Background())
	first, err := client.GetTelemetry(streamCtx, &pb.Empty{})
	require.NoError(t, err)
	_, err = first.Recv()
	require.NoError(t, err)
	disconnect()

	second, err := client.GetTelemetry(context.Background(), &pb.Empty{})
	require.NoError(t, err)
	_, err = second.Recv()
	require.NoError(t, err)
	configReply, err := client.GetConfig(context.Background(), &pb.Empty{})
	require.NoError(t, err)
	require.NotNil(t, configReply)
}

func TestServeStopsActiveTelemetryStream(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.sock")
	cfg := config.New()
	cfg.Global.InternalMonitoringListenerAddress = &url.URL{Scheme: "unix", Path: path}
	g := &gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}
	server := NewServer(g, cfg, store.NewLocalStore(), nil)
	appCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(appCtx) }()
	defer cancel()

	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	client := pb.NewMonitorClient(conn)
	require.Eventually(t, func() bool {
		callCtx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer stop()
		_, err := client.GetConfig(callCtx, &pb.Empty{})
		return err == nil
	}, 3*time.Second, 10*time.Millisecond)

	streamCtx, stopStream := context.WithTimeout(context.Background(), 6*time.Second)
	defer stopStream()
	stream, err := client.GetTelemetry(streamCtx, &pb.Empty{})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.NoError(t, err)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("active telemetry stream prevented shutdown")
	}
	for {
		_, err = stream.Recv()
		if err != nil {
			break
		}
	}
	require.Error(t, err)
}
