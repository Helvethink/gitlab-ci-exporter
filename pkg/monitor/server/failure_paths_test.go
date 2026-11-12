package server

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

var errStoreUnavailable = errors.New("store unavailable")

// failingTelemetryStore fails the telemetry query named by failOn.
type failingTelemetryStore struct {
	store.Store
	failOn string
}

func (s failingTelemetryStore) fail(name string) error {
	if s.failOn == name {
		return errStoreUnavailable
	}
	return nil
}

func (s failingTelemetryStore) CurrentlyQueuedTasksCount(ctx context.Context) (uint64, error) {
	if err := s.fail("queued"); err != nil {
		return 0, err
	}
	return s.Store.CurrentlyQueuedTasksCount(ctx)
}

func (s failingTelemetryStore) ExecutedTasksCount(ctx context.Context) (uint64, error) {
	if err := s.fail("executed"); err != nil {
		return 0, err
	}
	return s.Store.ExecutedTasksCount(ctx)
}

func (s failingTelemetryStore) ProjectsCount(ctx context.Context) (int64, error) {
	if err := s.fail("projects"); err != nil {
		return 0, err
	}
	return s.Store.ProjectsCount(ctx)
}

func (s failingTelemetryStore) EnvironmentsCount(ctx context.Context) (int64, error) {
	if err := s.fail("environments"); err != nil {
		return 0, err
	}
	return s.Store.EnvironmentsCount(ctx)
}

func (s failingTelemetryStore) RunnersCount(ctx context.Context) (int64, error) {
	if err := s.fail("runners"); err != nil {
		return 0, err
	}
	return s.Store.RunnersCount(ctx)
}

func (s failingTelemetryStore) RefsCount(ctx context.Context) (int64, error) {
	if err := s.fail("refs"); err != nil {
		return 0, err
	}
	return s.Store.RefsCount(ctx)
}

func (s failingTelemetryStore) MetricsCount(ctx context.Context) (int64, error) {
	if err := s.fail("metrics"); err != nil {
		return 0, err
	}
	return s.Store.MetricsCount(ctx)
}

// TestGetTelemetryReturnsStoreErrors checks that a store failure ends only the
// affected stream, with the error, instead of sending partial telemetry.
func TestGetTelemetryReturnsStoreErrors(t *testing.T) {
	for _, failOn := range []string{"queued", "executed", "projects", "environments", "runners", "refs", "metrics"} {
		t.Run(failOn, func(t *testing.T) {
			g := &gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}
			s := NewServer(g, config.New(), failingTelemetryStore{Store: store.NewLocalStore(), failOn: failOn}, nil)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := &telemetryStreamStub{ctx: ctx, cancel: cancel}

			require.ErrorIs(t, s.GetTelemetry(&pb.Empty{}, stream), errStoreUnavailable)
			assert.Empty(t, stream.sent, "no partial telemetry may be sent")
		})
	}
}

func TestGetTelemetryClampsAPIUsage(t *testing.T) {
	counter := ratecounter.NewRateCounter(time.Second)
	counter.Incr(50)
	g := &gitlab.Client{RateCounter: counter}
	cfg := config.New()
	cfg.Gitlab.MaximumRequestsPerSecond = 5
	s := NewServer(g, cfg, store.NewLocalStore(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := &telemetryStreamStub{ctx: ctx, cancel: cancel}
	require.NoError(t, s.GetTelemetry(&pb.Empty{}, stream))

	require.Len(t, stream.sent, 1)
	assert.InDelta(t, 1.0, stream.sent[0].GetGitlabApiUsage(), 0)
}

func TestPeerAuthenticatingListenerRejectsUnverifiableConnections(t *testing.T) {
	l := &peerAuthenticatingListener{allowedUID: 1000}

	// A connection that is not a Unix socket cannot be authenticated.
	server, client := net.Pipe()
	defer client.Close()
	defer server.Close()
	l.peerUID = func(*net.UnixConn) (uint32, error) { return 1000, nil }
	assert.False(t, l.authorized(server))

	unixConn := shortUnixConn(t)

	l.peerUID = func(*net.UnixConn) (uint32, error) { return 0, errors.New("getsockopt failed") }
	assert.False(t, l.authorized(unixConn), "unreadable peer credentials must be rejected")

	l.peerUID = func(*net.UnixConn) (uint32, error) { return 0, errPeerCredentialsUnsupported }
	assert.True(t, l.authorized(unixConn), "platforms without peer credentials rely on file permissions")

	l.peerUID = func(*net.UnixConn) (uint32, error) { return 0, nil }
	assert.True(t, l.authorized(unixConn), "root may connect")
}

// shortUnixConn returns the server side of a connected Unix socket pair.
func shortUnixConn(t *testing.T) *net.UnixConn {
	t.Helper()
	path := shortSocketPath(t)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	client, err := net.Dial("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	conn, err := l.AcceptUnix()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestServeReplacesStaleSocket(t *testing.T) {
	path := shortSocketPath(t)

	// A socket file left behind by a previous, crashed run.
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	stale.SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	_, err = os.Stat(path)
	require.NoError(t, err, "stale socket file must exist before Serve")

	cfg := config.New()
	cfg.Global.InternalMonitoringListenerAddress = &url.URL{Scheme: "unix", Path: path}
	s := NewServer(&gitlab.Client{RateCounter: ratecounter.NewRateCounter(time.Second)}, cfg, store.NewLocalStore(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()

	require.Eventually(t, func() bool {
		conn, err := net.Dial("unix", path)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 10*time.Millisecond, "Serve must replace the stale socket and accept connections")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(6 * time.Second):
		t.Fatal("monitoring server did not stop")
	}
}
