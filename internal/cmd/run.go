package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/controller"
	monitoringServer "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/server"
)

const (
	httpShutdownTimeout        = 5 * time.Second
	applicationShutdownTimeout = 15 * time.Second
)

// Run launches the exporter.
func Run(cliCtx *cli.Context) (int, error) {
	// Load and validate configuration from CLI context
	cfg, err := configure(cliCtx)
	if err != nil {
		return 1, err
	}

	// Create a cancellable context for the application's lifecycle
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	// Initialize the main controller with context, configuration, and app version
	c, err := controller.New(ctx, cfg, cliCtx.App.Version)
	if err != nil {
		return 1, err
	}

	var monitoringErrors chan error
	if cfg.Global.InternalMonitoringListenerAddress != nil {
		monitoringErrors = make(chan error, 1)
		monitoring := monitoringServer.NewServer(
			c.Gitlab,
			c.Config,
			c.Store,
			c.TaskController.TaskSchedulingMonitoring,
		)
		go func() {
			monitoringErrors <- monitoring.Serve(ctx)
		}()
	}

	// Setup channel to listen for OS termination signals for graceful shutdown
	onShutdown := make(chan os.Signal, 1)
	signal.Notify(onShutdown, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT)

	servers := []*http.Server{newPublicHTTPServer(ctx, c)}
	if cfg.Server.EnablePprof {
		servers = append(servers, newPprofHTTPServer(cfg.Server))
	}

	serverErrors := make(chan error, len(servers))
	for _, server := range servers {
		go serveHTTP(server, serverErrors)
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

	var runErr error
	monitoringStopped := false
	stoppedHTTPServers := 0
	select {
	case <-onShutdown:
		log.Info("received signal, attempting to gracefully exit..")
	case runErr = <-serverErrors:
		stoppedHTTPServers = 1
		if runErr == nil {
			runErr = errors.New("http server stopped unexpectedly")
		}
		log.WithContext(ctx).WithError(runErr).Error("http server stopped unexpectedly")
	case runErr = <-monitoringErrors:
		monitoringStopped = true
		if runErr == nil {
			runErr = errors.New("monitoring server stopped unexpectedly")
		}
	}
	signal.Stop(onShutdown)
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), applicationShutdownTimeout)
	defer cancelShutdown()
	httpCtx, cancelHTTP := context.WithTimeout(shutdownCtx, httpShutdownTimeout)
	shutdownResults := make(chan error, len(servers))
	for _, server := range servers {
		go func(server *http.Server) {
			shutdownResults <- server.Shutdown(httpCtx)
		}(server)
	}
	for range servers {
		select {
		case err := <-shutdownResults:
			runErr = errors.Join(runErr, err)
		case <-shutdownCtx.Done():
			runErr = errors.Join(runErr, fmt.Errorf("shut down HTTP servers: %w", shutdownCtx.Err()))
		}
	}
	cancelHTTP()
	ctxCancel()

	for stoppedHTTPServers < len(servers) {
		select {
		case err := <-serverErrors:
			stoppedHTTPServers++
			runErr = errors.Join(runErr, err)
		case <-shutdownCtx.Done():
			runErr = errors.Join(runErr, fmt.Errorf("wait for HTTP servers: %w", shutdownCtx.Err()))
			stoppedHTTPServers = len(servers)
		}
	}

	if monitoringErrors != nil && !monitoringStopped {
		select {
		case err := <-monitoringErrors:
			runErr = errors.Join(runErr, err)
		case <-shutdownCtx.Done():
			runErr = errors.Join(runErr, fmt.Errorf("wait for monitoring server: %w", shutdownCtx.Err()))
		}
	}
	runErr = errors.Join(runErr, c.Close(shutdownCtx))
	if runErr != nil {
		return 1, runErr
	}

	log.Info("stopped!")

	// Return success exit code
	return 0, nil
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

func serveHTTP(server *http.Server, errCh chan<- error) {
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	errCh <- err
}
