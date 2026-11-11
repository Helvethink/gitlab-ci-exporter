package client

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
)

type testMonitorServer struct {
	pb.UnimplementedMonitorServer
}

func (testMonitorServer) GetConfig(ctx context.Context, _ *pb.Empty) (*pb.Config, error) {
	return &pb.Config{Content: "test-config"}, nil
}

func TestNewClient(t *testing.T) {
	dir, err := os.MkdirTemp("", "gcpe")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "m.sock")

	lis, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = lis.Close()
	})

	grpcServer := grpc.NewServer()
	pb.RegisterMonitorServer(grpcServer, testMonitorServer{})
	t.Cleanup(grpcServer.Stop)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	endpoint, err := url.Parse("unix://" + socketPath)
	require.NoError(t, err)

	c, err := NewClient(context.Background(), endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NotNil(t, c)
	require.NotNil(t, c.MonitorClient)

	cfg, err := c.GetConfig(context.Background(), &pb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, "test-config", cfg.GetContent())
}

func TestNewClientRequiresEndpoint(t *testing.T) {
	_, err := NewClient(context.Background(), nil)
	require.Error(t, err)
}

func TestNewClientRejectsNonUnixEndpoints(t *testing.T) {
	for _, raw := range []string{"tcp://127.0.0.1:8082", "dns:///localhost:8082", "http://127.0.0.1:8082"} {
		endpoint, err := url.Parse(raw)
		require.NoError(t, err)
		_, err = NewClient(context.Background(), endpoint)
		require.ErrorIs(t, err, monitor.ErrUnsupportedListenerAddress, raw)
	}
}
