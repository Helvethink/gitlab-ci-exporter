package client

import (
	"context" // Package for managing context and cancellation
	"errors"
	"fmt"
	"net/url" // Package for URL parsing and manipulation

	log "github.com/sirupsen/logrus"              // Logging library
	"google.golang.org/grpc"                      // gRPC library for remote procedure calls
	"google.golang.org/grpc/credentials/insecure" // Package for insecure gRPC credentials

	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf" // Protobuf definitions for the monitor
)

// Client represents a gRPC client for interacting with the monitoring server.
type Client struct {
	pb.MonitorClient // Embedded gRPC client for the Monitor service
	conn             *grpc.ClientConn
}

// NewClient creates a new gRPC client for the monitoring server.
func NewClient(ctx context.Context, endpoint *url.URL) (*Client, error) {
	if endpoint == nil {
		return nil, errors.New("monitoring endpoint is required")
	}
	// Log the attempt to establish a gRPC connection
	log.WithField("endpoint", endpoint.String()).Debug("establishing gRPC connection to the server..")

	// Establish a gRPC connection to the server using insecure credentials
	// nolint: staticcheck
	conn, err := grpc.DialContext(
		ctx,
		endpoint.String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()), // Use insecure transport credentials
	)
	if err != nil {
		// Return the connection error to the caller
		return nil, fmt.Errorf("connect to monitoring server: %w", err)
	}

	// Log successful establishment of the gRPC connection
	log.Debug("gRPC connection established")

	// Create and return a new Client instance with the established connection
	return &Client{
		MonitorClient: pb.NewMonitorClient(conn), // Create a new MonitorClient using the established connection
		conn:          conn,
	}, nil
}

// Close closes the monitoring connection.
func (c *Client) Close() error {
	return c.conn.Close()
}
