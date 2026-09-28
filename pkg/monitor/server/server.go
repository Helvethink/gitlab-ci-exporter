package server

import (
	"context" // Package for managing context and cancellation
	"errors"
	"fmt"
	"net"  // Package for network I/O
	"os"   // Package for OS operations
	"time" // Package for time-related operations

	log "github.com/sirupsen/logrus"                     // Logging library
	"google.golang.org/grpc"                             // gRPC library for remote procedure calls
	"google.golang.org/protobuf/types/known/timestamppb" // Protobuf timestamp utilities

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"              // Configuration package
	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"              // GitLab client package
	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"             // Monitoring package
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf" // Protobuf definitions
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"             // Schemas package
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"               // Storage package
)

// Server represents a gRPC server for monitoring GitLab CI exporter.
type Server struct {
	pb.UnimplementedMonitorServer // Embedded unimplemented server for protobuf

	gitlabClient             *gitlab.Client // GitLab client for API interactions
	cfg                      config.Config  // Configuration for the server
	store                    store.Store    // Storage interface for data persistence
	taskSchedulingMonitoring *monitor.SchedulingStatus
}

// NewServer creates a new Server instance.
func NewServer(
	gitlabClient *gitlab.Client, // GitLab client instance
	c config.Config, // Configuration instance
	st store.Store, // Storage instance
	tsm *monitor.SchedulingStatus,
) (s *Server) {
	// Initialize and return a new Server instance
	s = &Server{
		gitlabClient:             gitlabClient,
		cfg:                      c,
		store:                    st,
		taskSchedulingMonitoring: tsm,
	}

	return
}

// Serve listens for monitoring requests until ctx is canceled or serving fails.
func (s *Server) Serve(ctx context.Context) (serveErr error) {
	address := s.cfg.Global.InternalMonitoringListenerAddress
	if address == nil {
		return nil
	}

	log.WithFields(log.Fields{
		"scheme": address.Scheme,
		"host":   address.Host,
		"path":   address.Path,
	}).Info("internal monitoring listener set")

	var listener net.Listener
	if address.Scheme == "unix" {
		unixAddress, err := net.ResolveUnixAddr("unix", address.Path)
		if err != nil {
			return fmt.Errorf("resolve monitoring socket: %w", err)
		}
		if _, err := os.Stat(address.Path); err == nil {
			if err := os.Remove(address.Path); err != nil {
				return fmt.Errorf("remove existing monitoring socket: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat monitoring socket: %w", err)
		}
		listener, err = net.ListenUnix("unix", unixAddress)
		if err != nil {
			return fmt.Errorf("listen on monitoring socket: %w", err)
		}
	} else {
		var err error
		listener, err = net.Listen(address.Scheme, address.Host)
		if err != nil {
			return fmt.Errorf("listen for monitoring: %w", err)
		}
	}
	defer func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			serveErr = errors.Join(serveErr, fmt.Errorf("close monitoring listener: %w", err))
		}
		if address.Scheme == "unix" {
			if err := os.Remove(address.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				serveErr = errors.Join(serveErr, fmt.Errorf("remove monitoring socket: %w", err))
			}
		}
	}()

	grpcServer := grpc.NewServer()
	pb.RegisterMonitorServer(grpcServer, s)
	watcherDone := make(chan struct{})
	watcherFinished := make(chan struct{})
	go func() {
		defer close(watcherFinished)
		select {
		case <-watcherDone:
			return
		case <-ctx.Done():
			stopped := make(chan struct{})
			go func() {
				grpcServer.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
			case <-time.After(4 * time.Second):
				grpcServer.Stop()
				<-stopped
			}
		}
	}()

	err := grpcServer.Serve(listener)
	close(watcherDone)
	<-watcherFinished
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("serve monitoring: %w", err)
	}
	return errors.New("monitoring server stopped unexpectedly")
}

// GetConfig retrieves the server configuration.
func (s *Server) GetConfig(ctx context.Context, _ *pb.Empty) (*pb.Config, error) {
	// Return the configuration as a protobuf Config message
	return &pb.Config{
		Content: s.cfg.ToYAML(),
	}, nil
}

// GetTelemetry streams telemetry data to the client.
func (s *Server) GetTelemetry(_ *pb.Empty, ts pb.Monitor_GetTelemetryServer) (err error) {
	ctx := ts.Context()
	ticker := time.NewTicker(time.Second) // Create a ticker to send telemetry data every second
	defer ticker.Stop()

	for {
		// Initialize a telemetry message
		telemetry := &pb.Telemetry{
			Projects: &pb.Entity{},
			Envs:     &pb.Entity{},
			Refs:     &pb.Entity{},
			Metrics:  &pb.Entity{},
			Runners:  &pb.Entity{},
		}

		// Calculate GitLab API usage
		telemetry.GitlabApiUsage = float64(s.gitlabClient.RateCounter.Rate()) / float64(s.cfg.Gitlab.MaximumRequestsPerSecond)
		if telemetry.GitlabApiUsage > 1 {
			telemetry.GitlabApiUsage = 1
		}

		// Set GitLab API requests count
		telemetry.GitlabApiRequestsCount = s.gitlabClient.RequestsCounter.Load()

		// Calculate GitLab API rate limit usage
		rateLimit := s.gitlabClient.RateLimit()
		if rateLimit.Limit > 0 {
			telemetry.GitlabApiRateLimit = float64(rateLimit.Remaining) / float64(rateLimit.Limit)
		}

		// Set GitLab API limit remaining
		telemetry.GitlabApiLimitRemaining = uint64(rateLimit.Remaining)

		// Get the count of currently queued tasks
		var queuedTasks uint64
		queuedTasks, err = s.store.CurrentlyQueuedTasksCount(ctx)
		if err != nil {
			return
		}

		// Calculate tasks buffer usage
		telemetry.TasksBufferUsage = float64(queuedTasks) / 1000

		// Get the count of executed tasks
		telemetry.TasksExecutedCount, err = s.store.ExecutedTasksCount(ctx)
		if err != nil {
			return
		}

		// Get the count of projects
		telemetry.Projects.Count, err = s.store.ProjectsCount(ctx)
		if err != nil {
			return
		}

		// Get the count of environments
		telemetry.Envs.Count, err = s.store.EnvironmentsCount(ctx)
		if err != nil {
			return
		}

		// Get the count of runners
		telemetry.Runners.Count, err = s.store.RunnersCount(ctx)
		if err != nil {
			return
		}

		// Get the count of refs
		telemetry.Refs.Count, err = s.store.RefsCount(ctx)
		if err != nil {
			return
		}

		// Get the count of metrics
		telemetry.Metrics.Count, err = s.store.MetricsCount(ctx)
		if err != nil {
			return
		}

		// Use one detached snapshot for the entire telemetry message.
		var schedule map[schemas.TaskType]monitor.TaskSchedulingStatus
		if s.taskSchedulingMonitoring != nil {
			schedule = s.taskSchedulingMonitoring.Snapshot()
		}

		// Set last and next pull times for projects
		if status, ok := schedule[schemas.TaskTypePullProjectsFromWildcards]; ok {
			telemetry.Projects.LastPull = timestamppb.New(status.Last)
			telemetry.Projects.NextPull = timestamppb.New(status.Next)
		}

		// Set the last and next garbage collection times for projects
		if status, ok := schedule[schemas.TaskTypeGarbageCollectProjects]; ok {
			telemetry.Projects.LastGc = timestamppb.New(status.Last)
			telemetry.Projects.NextGc = timestamppb.New(status.Next)
		}

		// Set the last and next pull times for environments
		if status, ok := schedule[schemas.TaskTypePullEnvironmentsFromProjects]; ok {
			telemetry.Envs.LastPull = timestamppb.New(status.Last)
			telemetry.Envs.NextPull = timestamppb.New(status.Next)
		}

		// Set the last and next garbage collection times for environments
		if status, ok := schedule[schemas.TaskTypeGarbageCollectEnvironments]; ok {
			telemetry.Envs.LastGc = timestamppb.New(status.Last)
			telemetry.Envs.NextGc = timestamppb.New(status.Next)
		}

		// Set last and next pull times for Runners
		if status, ok := schedule[schemas.TaskTypePullRunnersFromProjects]; ok {
			telemetry.Runners.LastPull = timestamppb.New(status.Last)
			telemetry.Runners.NextPull = timestamppb.New(status.Next)
		}

		// Set the last and next garbage collection times for Runners
		if status, ok := schedule[schemas.TaskTypeGarbageCollectRunners]; ok {
			telemetry.Runners.LastGc = timestamppb.New(status.Last)
			telemetry.Runners.NextGc = timestamppb.New(status.Next)
		}

		// Set the last and next pull times for refs
		if status, ok := schedule[schemas.TaskTypePullRefsFromProjects]; ok {
			telemetry.Refs.LastPull = timestamppb.New(status.Last)
			telemetry.Refs.NextPull = timestamppb.New(status.Next)
		}

		// Set the last and next garbage collection times for refs
		if status, ok := schedule[schemas.TaskTypeGarbageCollectRefs]; ok {
			telemetry.Refs.LastGc = timestamppb.New(status.Last)
			telemetry.Refs.NextGc = timestamppb.New(status.Next)
		}

		// Set the last and next pull times for metrics
		if status, ok := schedule[schemas.TaskTypePullMetrics]; ok {
			telemetry.Metrics.LastPull = timestamppb.New(status.Last)
			telemetry.Metrics.NextPull = timestamppb.New(status.Next)
		}

		// Set the last and next garbage collection times for metrics
		if status, ok := schedule[schemas.TaskTypeGarbageCollectMetrics]; ok {
			telemetry.Metrics.LastGc = timestamppb.New(status.Last)
			telemetry.Metrics.NextGc = timestamppb.New(status.Next)
		}

		// Send the telemetry data to the client
		if err := ts.Send(telemetry); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		// Wait for either the context to be done or the ticker to tick
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			time.Sleep(1 * time.Nanosecond)
		}
	}
}
