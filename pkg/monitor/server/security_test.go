package server

import (
	"context"
	"io"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

// startUnixServer starts the monitoring server on a short Unix socket path and
// returns the path. The server is stopped when the test ends.
func startUnixServer(t *testing.T, cfg config.Config) string {
	t.Helper()

	path := shortSocketPath(t)
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

	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	return path
}

func TestServeRejectsNonUnixAddresses(t *testing.T) {
	for _, address := range []*url.URL{
		{Scheme: "tcp", Host: "127.0.0.1:0"},
		{Scheme: "tcp", Host: "0.0.0.0:0"},
		{Scheme: "http", Host: "127.0.0.1:0"},
	} {
		cfg := config.New()
		cfg.Global.InternalMonitoringListenerAddress = address
		s := NewServer(&gitlab.Client{}, cfg, store.NewLocalStore(), nil)
		require.ErrorIs(t, s.Serve(context.Background()), monitor.ErrUnsupportedListenerAddress, address.String())
	}
}

func TestServeRestrictsSocketPermissions(t *testing.T) {
	path := startUnixServer(t, config.New())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestServeRefusesToReplaceNonSocketFile(t *testing.T) {
	path := shortSocketPath(t)
	require.NoError(t, os.WriteFile(path, []byte("keep me"), 0o600))

	cfg := config.New()
	cfg.Global.InternalMonitoringListenerAddress = &url.URL{Scheme: "unix", Path: path}
	s := NewServer(&gitlab.Client{}, cfg, store.NewLocalStore(), nil)
	require.ErrorContains(t, s.Serve(context.Background()), "is not a socket")

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(content))
}

func TestGetConfigNeverExposesCredentials(t *testing.T) {
	cfg := config.New()
	cfg.Gitlab.Token = "SECRET-gitlab-token"
	cfg.Gitlab.URL = "https://user:SECRET-basic@gitlab.example.com/?private_token=SECRET-query"
	cfg.Server.Webhook.SecretToken = "SECRET-webhook"
	cfg.Redis.URL = "redis://default:SECRET-redis@redis.example.com:6379/0"
	cfg.OpenTelemetry.GRPCEndpoint = "https://otel:SECRET-otel@collector.example.com:4317"
	cfg.Projects = []config.Project{config.NewProject("SECRET-project-name")}

	path := startUnixServer(t, cfg)
	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	reply, err := pb.NewMonitorClient(conn).GetConfig(context.Background(), &pb.Empty{})
	require.NoError(t, err)

	content := reply.GetContent()
	assert.NotContains(t, content, "SECRET")
	assert.NotContains(t, content, "redis.example.com")
	assert.Contains(t, content, "gitlab.example.com")
	assert.Contains(t, content, "redis:\n    enabled: true")
	assert.Contains(t, content, "count: 1")
}

func TestPeerUIDReturnsCurrentUser(t *testing.T) {
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

	uid, err := peerUID(conn)
	if err == errPeerCredentialsUnsupported {
		t.Skip("peer credentials are not supported on this platform")
	}
	require.NoError(t, err)
	assert.Equal(t, uint32(os.Getuid()), uid)
}

func TestPeerAuthenticatingListenerRejectsOtherUsers(t *testing.T) {
	path := shortSocketPath(t)
	inner, err := net.Listen("unix", path)
	require.NoError(t, err)

	const ownUID, otherUID = 1000, 4242
	uids := make(chan uint32, 2)
	uids <- otherUID
	uids <- ownUID
	l := &peerAuthenticatingListener{
		Listener:   inner,
		allowedUID: ownUID,
		peerUID:    func(*net.UnixConn) (uint32, error) { return <-uids, nil },
	}
	t.Cleanup(func() { _ = l.Close() })

	intruder, err := net.Dial("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = intruder.Close() })
	owner, err := net.Dial("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })

	accepted, err := l.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = accepted.Close() })

	// The intruder's connection was closed by the server without exchanging data.
	require.NoError(t, intruder.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = intruder.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)

	// The accepted connection is the owner's.
	_, err = owner.Write([]byte("x"))
	require.NoError(t, err)
	buf := make([]byte, 1)
	require.NoError(t, accepted.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = accepted.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "x", string(buf))
}

func TestMonitoringListenerLimitsConnections(t *testing.T) {
	path := shortSocketPath(t)
	inner, err := net.Listen("unix", path)
	require.NoError(t, err)
	l := newMonitoringListener(inner)
	t.Cleanup(func() { _ = l.Close() })

	accepted := make(chan net.Conn, maxMonitoringConnections+1)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			accepted <- conn
		}
	}()

	for range maxMonitoringConnections + 1 {
		c, err := net.Dial("unix", path)
		require.NoError(t, err)
		t.Cleanup(func() { _ = c.Close() })
	}

	var conns []net.Conn
	for range maxMonitoringConnections {
		select {
		case c := <-accepted:
			conns = append(conns, c)
		case <-time.After(2 * time.Second):
			t.Fatal("expected connection to be accepted")
		}
	}

	select {
	case <-accepted:
		t.Fatal("accepted more connections than the limit")
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, conns[0].Close())
	select {
	case c := <-accepted:
		_ = c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("pending connection was not accepted after a slot was released")
	}
	for _, c := range conns[1:] {
		_ = c.Close()
	}
}

func TestServeLimitsConcurrentStreamsPerConnection(t *testing.T) {
	path := startUnixServer(t, config.New())
	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := pb.NewMonitorClient(conn)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for range maxStreamsPerConnection {
		stream, err := client.GetTelemetry(ctx, &pb.Empty{})
		require.NoError(t, err)
		_, err = stream.Recv()
		require.NoError(t, err)
	}

	// The next stream is queued by the transport until a slot frees up.
	extraCtx, extraCancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer extraCancel()
	extra, err := client.GetTelemetry(extraCtx, &pb.Empty{})
	if err == nil {
		_, err = extra.Recv()
	}
	require.Error(t, err)
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
}
