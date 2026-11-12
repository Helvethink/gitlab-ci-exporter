package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/controller"
	monitoringServer "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/server"
)

// Shutdown phase budgets. Their sum stays within applicationShutdownTimeout, which
// stays below the default Kubernetes termination grace period (30s).
const (
	stopIntakeTimeout          = 5 * time.Second
	drainWorkTimeout           = 8 * time.Second
	stopServersTimeout         = 5 * time.Second
	closeClientsTimeout        = 5 * time.Second
	applicationShutdownTimeout = 25 * time.Second
)

// Shutdown phase names, in execution order.
const (
	phaseStopIntake   = "stop intake"
	phaseDrainWork    = "stop producers and drain work"
	phaseStopServers  = "stop servers"
	phaseCloseClients = "close clients and flush tracing"
)

// shutdownPhase is one bounded step of the shutdown sequence.
type shutdownPhase struct {
	name    string
	timeout time.Duration
	run     func(context.Context) error
}

// runShutdown runs phases in order. Each phase is bounded by its own timeout and
// by ctx. A phase that fails or times out is reported, and the next phases still
// run, so clients are always closed and traces flushed.
func runShutdown(ctx context.Context, phases []shutdownPhase) error {
	var shutdownErr error
	for _, phase := range phases {
		phaseCtx, cancel := context.WithTimeout(ctx, phase.timeout)
		started := time.Now()
		done := make(chan error, 1)
		go func() { done <- phase.run(phaseCtx) }()

		var err error
		select {
		case err = <-done:
		case <-phaseCtx.Done():
			err = phaseCtx.Err()
		}
		cancel()

		fields := log.Fields{"phase": phase.name, "duration": time.Since(started)}
		if err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("%s: %w", phase.name, err))
			log.WithFields(fields).WithError(err).Warn("shutdown phase failed")
			continue
		}
		log.WithFields(fields).Debug("shutdown phase completed")
	}
	return shutdownErr
}

// Run launches the exporter and owns the lifecycle of every long-lived component:
// the HTTP servers, the monitoring gRPC server, and the controller (schedulers,
// webhook workers, task queue and consumers, Redis, GitLab, and tracing clients).
//
// Components run under one errgroup. A signal or the failure of any component
// starts a bounded shutdown in this order: stop intake (HTTP servers), stop
// producers and drain work (schedulers, webhook workers, task queue), stop
// servers (monitoring gRPC), and close clients and flush tracing.
func Run(cliCtx *cli.Context) (int, error) {
	// Load and validate configuration from CLI context
	cfg, err := configure(cliCtx)
	if err != nil {
		return 1, err
	}

	// signalCtx is cancelled by the first termination signal. After shutdown has
	// started, signals are no longer intercepted: a second one kills the process.
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT)
	defer stopSignals()

	// appCtx bounds the controller and HTTP handlers. It is not cancelled by the
	// signal directly: the shutdown phases stop each component in order.
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	c, err := controller.New(appCtx, cfg, cliCtx.App.Version)
	if err != nil {
		return 1, err
	}

	group, groupCtx := errgroup.WithContext(signalCtx)

	servers := []*http.Server{newPublicHTTPServer(appCtx, c)}
	if cfg.Server.EnablePprof {
		servers = append(servers, newPprofHTTPServer(cfg.Server))
	}
	for _, server := range servers {
		group.Go(func() error { return serveHTTP(server) })
	}

	monitoringCtx, stopMonitoring := context.WithCancel(context.Background())
	defer stopMonitoring()
	monitoringDone := make(chan struct{})
	if cfg.Global.InternalMonitoringListenerAddress != nil {
		monitoring := monitoringServer.NewServer(
			c.Gitlab,
			c.Config,
			c.Store,
			c.TaskController.TaskSchedulingMonitoring,
		)
		group.Go(func() error {
			defer close(monitoringDone)
			return monitoring.Serve(monitoringCtx)
		})
	} else {
		close(monitoringDone)
	}

	// Log server startup details
	log.WithFields(
		log.Fields{
			"listen-address":               cfg.Server.ListenAddress,
			"pprof-endpoint-enabled":       cfg.Server.EnablePprof,
			"pprof-listen-address":         cfg.Server.PprofListenAddress,
			"metrics-endpoint-enabled":     cfg.Server.Metrics.Enabled,
			"webhook-endpoint-enabled":     cfg.Server.Webhook.Enabled,
			"openmetrics-encoding-enabled": cfg.Server.Metrics.EnableOpenmetricsEncoding,
			"controller-uuid":              c.UUID,
		},
	).Info("http server started")

	<-groupCtx.Done()
	stopSignals()
	if signalCtx.Err() != nil {
		log.Info("received signal, attempting to gracefully exit..")
	} else {
		log.Error("a component stopped unexpectedly, shutting down..")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), applicationShutdownTimeout)
	defer cancelShutdown()

	shutdownErr := runShutdown(shutdownCtx, shutdownPhases(servers, c, stopMonitoring, monitoringDone))
	cancelApp()

	// Collect component errors. Every component has been stopped by now.
	componentErr := waitGroup(shutdownCtx, group)
	if runErr := errors.Join(componentErr, shutdownErr); runErr != nil {
		return 1, runErr
	}

	log.Info("stopped!")

	// Return success exit code
	return 0, nil
}

// shutdownPhases returns the ordered shutdown sequence.
func shutdownPhases(servers []*http.Server, c *controller.Controller, stopMonitoring context.CancelFunc, monitoringDone <-chan struct{}) []shutdownPhase {
	return []shutdownPhase{
		{
			name:    phaseStopIntake,
			timeout: stopIntakeTimeout,
			run: func(ctx context.Context) error {
				return shutdownHTTPServers(ctx, servers)
			},
		},
		{
			name:    phaseDrainWork,
			timeout: drainWorkTimeout,
			run:     c.Drain,
		},
		{
			name:    phaseStopServers,
			timeout: stopServersTimeout,
			run: func(ctx context.Context) error {
				stopMonitoring()
				select {
				case <-monitoringDone:
					return nil
				case <-ctx.Done():
					return fmt.Errorf("wait for monitoring server: %w", ctx.Err())
				}
			},
		},
		{
			name:    phaseCloseClients,
			timeout: closeClientsTimeout,
			run:     c.Close,
		},
	}
}

// shutdownHTTPServers gracefully stops all servers concurrently, waiting for
// active requests to complete.
func shutdownHTTPServers(ctx context.Context, servers []*http.Server) error {
	var group errgroup.Group
	for _, server := range servers {
		group.Go(func() error { return server.Shutdown(ctx) })
	}
	return group.Wait()
}

// waitGroup waits for the errgroup, bounded by ctx.
func waitGroup(ctx context.Context, group *errgroup.Group) error {
	done := make(chan error, 1)
	go func() { done <- group.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("wait for components to stop: %w", ctx.Err())
	}
}

func newPublicHTTPServer(ctx context.Context, c *controller.Controller) *http.Server {
	mux := http.NewServeMux()
	health := c.HealthCheckHandler(ctx)
	mux.Handle("GET /health/live", http.HandlerFunc(health.LiveEndpoint))
	mux.Handle("GET /health/ready", http.HandlerFunc(health.ReadyEndpoint))

	if c.Config.Server.Metrics.Enabled {
		mux.Handle("GET /metrics", http.HandlerFunc(c.MetricsHandler))
	}
	if c.Config.Server.Webhook.Enabled {
		mux.Handle("POST /webhook", c.NewWebhookHandler(ctx))
	}

	return newHTTPServer(c.Config.Server.ListenAddress, mux, c.Config.Server)
}

func newPprofHTTPServer(cfg config.Server) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	server := newHTTPServer(cfg.PprofListenAddress, mux, cfg)
	// CPU profiles may legitimately run longer than the public server's write
	// timeout. Access is bounded by the dedicated private listener instead.
	server.WriteTimeout = 0
	return server
}

func newHTTPServer(address string, handler http.Handler, cfg config.Server) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

// serveHTTP serves until the server is shut down. It returns nil after a
// graceful Shutdown and the listener error otherwise.
func serveHTTP(server *http.Server) error {
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("http server %s: %w", server.Addr, err)
}
