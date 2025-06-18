package cmd

import (
	"fmt"
	stdlibLog "log"
	"net/url"
	"os"
	"time"

	"github.com/go-logr/stdr"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/opentelemetry-go-extra/otellogrus"
	"github.com/urfave/cli/v2"
	"github.com/vmihailenco/taskq/v4"

	"github.com/helvethink/gitlab-ci-exporter/internal/logging"
	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
)

var start time.Time

// configure loads and validates configuration from CLI context, sets up logging, and prints scheduler settings.
// It returns a populated config object or an error.
func configure(ctx *cli.Context) (cfg config.Config, err error) {
	// Retrieve and store application start time from CLI metadata
	start = ctx.App.Metadata["startTime"].(time.Time)

	// Ensure "config" CLI flag is defined
	assertStringVariableDefined(ctx, "config")

	// Parse the configuration file from the given path
	cfg, err = config.ParseFile(ctx.String("config"))
	if err != nil {
		return
	}

	// Parse global flags like internal monitoring listener address
	cfg.Global, err = parseGlobalFlags(ctx)
	if err != nil {
		return
	}

	// Override config parameters with any CLI-provided values
	configCliOverrides(ctx, &cfg)

	// Validate the final configuration structure
	if err = cfg.Validate(); err != nil {
		return
	}

	// Initialize logger with the config-defined level and format
	if err = logger.Configure(logger.Config{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
	}); err != nil {
		return
	}

	// Add OpenTelemetry logging hook to integrate tracing into logs
	log.AddHook(otellogrus.NewHook(otellogrus.WithLevels(
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
	)))

	// Redirect task queue logs to the main log system using standard library compatibility
	taskq.SetLogger(stdr.New(stdlibLog.New(log.StandardLogger().WriterLevel(log.WarnLevel), "taskq", 0)))

	// Log general GitLab configuration
	log.WithFields(
		log.Fields{
			"gitlab-endpoint":   cfg.Gitlab.URL,
			"gitlab-rate-limit": fmt.Sprintf("%drps", cfg.Gitlab.MaximumRequestsPerSecond),
		},
	).Info("configured")

	// Log pull scheduling settings
	log.WithFields(config.SchedulerConfig(cfg.Pull.ProjectsFromWildcards).Log()).Info("pull projects from wildcards")
	log.WithFields(config.SchedulerConfig(cfg.Pull.EnvironmentsFromProjects).Log()).Info("pull environments from projects")
	log.WithFields(config.SchedulerConfig(cfg.Pull.RefsFromProjects).Log()).Info("pull refs from projects")
	log.WithFields(config.SchedulerConfig(cfg.Pull.Metrics).Log()).Info("pull metrics")

	// Log garbage collection scheduling settings
	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Projects).Log()).Info("garbage collect projects")
	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Environments).Log()).Info("garbage collect environments")
	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Refs).Log()).Info("garbage collect refs")
	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Metrics).Log()).Info("garbage collect metrics")

	return
}

// parseGlobalFlags parses global CLI flags into the Global config struct.
func parseGlobalFlags(ctx *cli.Context) (cfg config.Global, err error) {
	// Parse internal monitoring address if provided
	if listenerAddr := ctx.String("internal-monitoring-listener-address"); listenerAddr != "" {
		cfg.InternalMonitoringListenerAddress, err = url.Parse(listenerAddr)
	}
	return
}

// exit logs the execution time and error (if any), then returns a CLI exit code.
func exit(exitCode int, err error) cli.ExitCoder {
	defer log.WithFields(
		log.Fields{
			"execution-time": time.Since(start), // nolint: govet
		},
	).Debug("exited..") // Log execution time when exiting

	if err != nil {
		log.WithError(err).Error() // Log error if present
	}

	return cli.Exit("", exitCode)
}

// ExecWrapper gracefully logs and exits our `run` functions.
// It wraps a function returning (int, error) into a `cli.ActionFunc` compatible with urfave/cli.
func ExecWrapper(f func(ctx *cli.Context) (int, error)) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		return exit(f(ctx)) // Handles logging and clean exit based on result
	}
}

// configCliOverrides overrides configuration fields with command-line flags if present.
func configCliOverrides(ctx *cli.Context, cfg *config.Config) {
	// Override GitLab token if provided via CLI
	if ctx.String("gitlab-token") != "" {
		cfg.Gitlab.Token = ctx.String("gitlab-token")
	}

	// Override webhook secret token if webhook is enabled and CLI flag is present
	if cfg.Server.Webhook.Enabled {
		if ctx.String("webhook-secret-token") != "" {
			cfg.Server.Webhook.SecretToken = ctx.String("webhook-secret-token")
		}
	}

	// Override Redis URL if provided
	if ctx.String("redis-url") != "" {
		cfg.Redis.URL = ctx.String("redis-url")
	}

	// Override health URL and enable health check if provided
	if healthURL := ctx.String("gitlab-health-url"); healthURL != "" {
		cfg.Gitlab.HealthURL = healthURL
		cfg.Gitlab.EnableHealthCheck = true
	}
}

// assertStringVariableDefined ensures a required string flag is set.
// If not, it prints help and exits the program.
func assertStringVariableDefined(ctx *cli.Context, k string) {
	if len(ctx.String(k)) == 0 {
		_ = cli.ShowAppHelp(ctx) // Show CLI help to guide the user

		log.Errorf("'--%s' must be set!", k)
		os.Exit(2) // Exit with code 2 (convention for incorrect usage)
	}
}
