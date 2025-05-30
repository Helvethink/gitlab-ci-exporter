package cmd

import (
	"context"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/helvethink/gitlab-ci-exporter/pkg/controller"
	monitoringServer "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/server"
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

	// Start the monitoring RPC server asynchronously in a separate goroutine
	go func(c *controller.Controller) {
		s := monitoringServer.NewServer(
			c.Gitlab,
			c.Config,
			c.Store,
			c.TaskController.TaskSchedulingMonitoring,
		)
		s.Serve()
	}(&c)

	// Setup channel to listen for OS termination signals for graceful shutdown
	onShutdown := make(chan os.Signal, 1)
	signal.Notify(onShutdown, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT)

	// Create an HTTP server multiplexer
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    cfg.Server.ListenAddress,
		Handler: mux,
	}

	// Register health check endpoints
	health := c.HealthCheckHandler(ctx)
	mux.HandleFunc("/health/live", health.LiveEndpoint)
	mux.HandleFunc("/health/ready", health.ReadyEndpoint)

	// Register metrics endpoint if enabled in config
	if cfg.Server.Metrics.Enabled {
		mux.HandleFunc("/metrics", c.MetricsHandler)
	}

	// Register pprof debug endpoints if enabled in config
	if cfg.Server.EnablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	// Register webhook endpoint if enabled in config
	if cfg.Server.Webhook.Enabled {
		mux.HandleFunc("/webhook", c.WebhookHandler)
	}

	// Start the HTTP server asynchronously
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Fatal log if server unexpectedly stops
			log.WithContext(ctx).
				WithError(err).
				Fatal()
		}
	}()

	// Log server startup details
	log.WithFields(
		log.Fields{
			"listen-address":               cfg.Server.ListenAddress,
			"pprof-endpoint-enabled":       cfg.Server.EnablePprof,
			"metrics-endpoint-enabled":     cfg.Server.Metrics.Enabled,
			"webhook-endpoint-enabled":     cfg.Server.Webhook.Enabled,
			"openmetrics-encoding-enabled": cfg.Server.Metrics.EnableOpenmetricsEncoding,
			"controller-uuid":              c.UUID,
		},
	).Info("http server started")

	// Wait here until a termination signal is received
	<-onShutdown

	// Received termination signal - begin graceful shutdown
	log.Info("received signal, attempting to gracefully exit..")
	ctxCancel()

	// Create a context with timeout to force HTTP server shutdown after 5 seconds
	httpServerContext, forceHTTPServerShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer forceHTTPServerShutdown()

	// Attempt graceful shutdown of HTTP server
	if err := srv.Shutdown(httpServerContext); err != nil {
		return 1, err
	}

	log.Info("stopped!")

	// Return success exit code
	return 0, nil
}
