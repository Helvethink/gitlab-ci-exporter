package client

import (
	"context"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
)

type testMonitorServer struct {
	pb.UnimplementedMonitorServer
}

func (testMonitorServer) GetConfig(ctx context.Context, _ *pb.Empty) (*pb.Config, error) {
	return &pb.Config{Content: "test-config"}, nil
}

func TestNewClient(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
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

	endpoint, err := url.Parse("dns:///" + lis.Addr().String())
	require.NoError(t, err)

	c := NewClient(context.Background(), endpoint)
	require.NotNil(t, c)
	require.NotNil(t, c.MonitorClient)

	cfg, err := c.GetConfig(context.Background(), &pb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, "test-config", cfg.GetContent())
}
