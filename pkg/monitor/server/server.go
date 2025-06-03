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

	gitlabClient             *gitlab.Client                                     // GitLab client for API interactions
	cfg                      config.Config                                      // Configuration for the server
	store                    store.Store                                        // Storage interface for data persistence
	taskSchedulingMonitoring map[schemas.TaskType]*monitor.TaskSchedulingStatus // Task scheduling statuses
}

// NewServer creates a new Server instance.
func NewServer(
	gitlabClient *gitlab.Client, // GitLab client instance
	c config.Config, // Configuration instance
	st store.Store, // Storage instance
	tsm map[schemas.TaskType]*monitor.TaskSchedulingStatus, // Task scheduling monitoring map
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

	for {
		// Initialize a telemetry message
		telemetry := &pb.Telemetry{
			Projects: &pb.Entity{},
			Envs:     &pb.Entity{},
			Refs:     &pb.Entity{},
			Metrics:  &pb.Entity{},
		}

		// Calculate GitLab API usage
		telemetry.GitlabApiUsage = float64(s.gitlabClient.RateCounter.Rate()) / float64(s.cfg.Gitlab.MaximumRequestsPerSecond)
		if telemetry.GitlabApiUsage > 1 {
			telemetry.GitlabApiUsage = 1
		}

		// Set GitLab API requests count
		telemetry.GitlabApiRequestsCount = s.gitlabClient.RequestsCounter.Load()

		// Calculate GitLab API rate limit usage
		telemetry.GitlabApiRateLimit = float64(s.gitlabClient.RequestsRemaining) / float64(s.gitlabClient.RequestsLimit)
		if telemetry.GitlabApiRateLimit > 1 {
			telemetry.GitlabApiRateLimit = 1
		}

		// Set GitLab API limit remaining
		telemetry.GitlabApiLimitRemaining = uint64(s.gitlabClient.RequestsRemaining)

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

		// Set last and next pull times for projects
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypePullProjectsFromWildcards]; ok {
			telemetry.Projects.LastPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullProjectsFromWildcards].Last)
			telemetry.Projects.NextPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullProjectsFromWildcards].Next)
		}

		// Set last and next garbage collection times for projects
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectProjects]; ok {
			telemetry.Projects.LastGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectProjects].Last)
			telemetry.Projects.NextGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectProjects].Next)
		}

		// Set last and next pull times for environments
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypePullEnvironmentsFromProjects]; ok {
			telemetry.Envs.LastPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullEnvironmentsFromProjects].Last)
			telemetry.Envs.NextPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullEnvironmentsFromProjects].Next)
		}

		// Set last and next garbage collection times for environments
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectEnvironments]; ok {
			telemetry.Envs.LastGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectEnvironments].Last)
			telemetry.Envs.NextGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectEnvironments].Next)
		}

		// Set last and next pull times for refs
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypePullRefsFromProjects]; ok {
			telemetry.Refs.LastPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullRefsFromProjects].Last)
			telemetry.Refs.NextPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullRefsFromProjects].Next)
		}

		// Set last and next garbage collection times for refs
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectRefs]; ok {
			telemetry.Refs.LastGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectRefs].Last)
			telemetry.Refs.NextGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectRefs].Next)
		}

		// Set last and next pull times for metrics
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypePullMetrics]; ok {
			telemetry.Metrics.LastPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullMetrics].Last)
			telemetry.Metrics.NextPull = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypePullMetrics].Next)
		}

		// Set last and next garbage collection times for metrics
		if _, ok := s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectMetrics]; ok {
			telemetry.Metrics.LastGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectMetrics].Last)
			telemetry.Metrics.NextGc = timestamppb.New(s.taskSchedulingMonitoring[schemas.TaskTypeGarbageCollectMetrics].Next)
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
