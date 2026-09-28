package server

import (
	"context" // Package for managing context and cancellation
	"net"     // Package for network I/O
	"os"      // Package for OS operations
	"time"    // Package for time-related operations

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

// Serve starts the gRPC server to listen for incoming connections.
func (s *Server) Serve() {
	// Check if the internal monitoring listener address is set
	if s.cfg.Global.InternalMonitoringListenerAddress == nil {
		log.Info("internal monitoring listener address not set")
		return
	}

	// Log the internal monitoring listener address details
	log.WithFields(log.Fields{
		"scheme": s.cfg.Global.InternalMonitoringListenerAddress.Scheme,
		"host":   s.cfg.Global.InternalMonitoringListenerAddress.Host,
		"path":   s.cfg.Global.InternalMonitoringListenerAddress.Path,
	}).Info("internal monitoring listener set")

	// Create a new gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterMonitorServer(grpcServer, s)

	var (
		l   net.Listener
		err error
	)

	// Handle different listener schemes
	switch s.cfg.Global.InternalMonitoringListenerAddress.Scheme {
	case "unix":
		// Resolve the Unix address
		unixAddr, err := net.ResolveUnixAddr("unix", s.cfg.Global.InternalMonitoringListenerAddress.Path)
		if err != nil {
			log.WithError(err).Fatal()
		}

		// Remove the socket file if it already exists
		if _, err := os.Stat(s.cfg.Global.InternalMonitoringListenerAddress.Path); err == nil {
			if err := os.Remove(s.cfg.Global.InternalMonitoringListenerAddress.Path); err != nil {
				log.WithError(err).Fatal()
			}
		}

		// Ensure the socket file is removed when the server exits
		defer func(path string) {
			if err := os.Remove(path); err != nil {
				log.WithError(err).Fatal()
			}
		}(s.cfg.Global.InternalMonitoringListenerAddress.Path)

		// Listen on the Unix socket
		if l, err = net.ListenUnix("unix", unixAddr); err != nil {
			log.WithError(err).Fatal()
		}

	default:
		// Listen on the network address
		if l, err = net.Listen(s.cfg.Global.InternalMonitoringListenerAddress.Scheme, s.cfg.Global.InternalMonitoringListenerAddress.Host); err != nil {
			log.WithError(err).Fatal()
		}
	}

	// Ensure the listener is closed when the server exits
	defer l.Close() // nolint: errcheck

	// Start serving the gRPC server
	if err = grpcServer.Serve(l); err != nil {
		log.WithError(err).Fatal()
	}
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
		errTel := ts.Send(telemetry)
		if errTel != nil {
			log.WithError(errTel).Fatal()
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
