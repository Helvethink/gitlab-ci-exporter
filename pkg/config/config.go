package config

import (
	"fmt"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// validate is a global validator instance used to validate struct fields based on tags.
var validate *validator.Validate

// Config holds all the configuration parameters necessary for properly configuring the application.
type Config struct {
	Global          Global            `yaml:",omitempty"`       // Global contains global/shared exporter configuration settings.
	Log             Log               `yaml:"log"`              // Log holds configuration related to logging for the exporter.
	OpenTelemetry   OpenTelemetry     `yaml:"opentelemetry"`    // OpenTelemetry contains configuration settings for OpenTelemetry integration.
	Server          Server            `yaml:"server"`           // Server holds configuration related to the server settings.
	Gitlab          Gitlab            `yaml:"gitlab"`           // Gitlab contains GitLab-specific configuration settings.
	Redis           Redis             `yaml:"redis"`            // Redis holds configuration parameters for connecting to Redis.
	Pull            Pull              `yaml:"pull"`             // Pull contains configuration related to data pulling behavior.
	GarbageCollect  GarbageCollect    `yaml:"garbage_collect"`  // GarbageCollect contains configuration for garbage collection.
	ProjectDefaults ProjectParameters `yaml:"project_defaults"` // ProjectDefaults defines default project parameters which can be overridden at individual Project or Wildcard levels.

	// Projects is a list of specific projects to pull metrics from.
	// Validation: Must be unique, at least one project or wildcard must be provided, and each element is validated.
	Projects []Project `validate:"unique,at-least-1-project-or-wildcard,dive" yaml:"projects"`

	// Wildcards is a list of wildcard project definitions used to dynamically discover projects.
	// Validation: Must be unique, at least one project or wildcard must be provided, and each element is validated.
	Wildcards []Wildcard `validate:"unique,at-least-1-project-or-wildcard,dive" yaml:"wildcards"`
}

// Log holds configuration settings related to runtime logging.
type Log struct {
	// Level sets the logging verbosity level.
	// Valid values: trace, debug, info, warning, error, fatal, panic.
	// Defaults to "info".
	Level string `default:"info" validate:"required,oneof=trace debug info warning error fatal panic"`

	// Format sets the output format of the logs.
	// Valid values: "text" or "json".
	// Defaults to "text".
	Format string `default:"text" validate:"oneof=text json"`
}

// OpenTelemetry holds configuration related to OpenTelemetry integration.
type OpenTelemetry struct {
	// GRPCEndpoint is the gRPC address of the OpenTelemetry collector to send traces/metrics to.
	GRPCEndpoint string `yaml:"grpc_endpoint"`
}

// Server holds the configuration for the HTTP server.
type Server struct {
	// ListenAddress specifies the address and port the server will bind to and listen on.
	// Default is ":8080" (all interfaces on port 8080).
	ListenAddress string        `default:":8080" yaml:"listen_address"`
	EnablePprof   bool          `default:"false" yaml:"enable_pprof"` // EnablePprof enables profiling endpoints for debugging performance issues.
	Metrics       ServerMetrics `yaml:"metrics"`                      // Metrics contains configuration related to exposing Prometheus metrics.
	Webhook       ServerWebhook `yaml:"webhook"`                      // Webhook holds configuration for webhook-related HTTP endpoints.
}

// ServerMetrics holds configuration for the metrics HTTP endpoint.
type ServerMetrics struct {
	// EnableOpenmetricsEncoding enables OpenMetrics content encoding in the Prometheus HTTP handler.
	// This can be useful for compatibility with Prometheus 2.0+.
	EnableOpenmetricsEncoding bool `default:"false" yaml:"enable_openmetrics_encoding"`
	Enabled                   bool `default:"true" yaml:"enabled"` // Enabled controls whether the /metrics endpoint is exposed.
}

// ServerWebhook holds configuration for the webhook HTTP endpoint.
type ServerWebhook struct {
	// Enabled enables the /webhook endpoint to receive GitLab webhook requests.
	Enabled bool `default:"false" yaml:"enabled"`

	// SecretToken is used to authenticate incoming webhook requests to ensure they come from a legitimate GitLab server.
	// This token is required if the webhook endpoint is enabled.
	SecretToken string `validate:"required_if=Enabled true" yaml:"secret_token"`
}

// Gitlab holds the configuration needed to connect to a GitLab instance.
type Gitlab struct {
	// URL of the GitLab server or API endpoint.
	// Defaults to https://gitlab.com (the public GitLab instance).
	URL string `default:"https://gitlab.com" validate:"required,url" yaml:"url"`

	// HealthURL is the URL used to check if the GitLab server is reachable.
	// Defaults to a publicly accessible endpoint on gitlab.com.
	HealthURL string `default:"https://gitlab.com/explore" validate:"required,url" yaml:"health_url"`

	Token                      string `validate:"required" yaml:"token"`                                  // Token is the authentication token used to access the GitLab API.
	EnableHealthCheck          bool   `default:"true" yaml:"enable_health_check"`                         // EnableHealthCheck toggles periodic health checks by requesting the HealthURL.
	EnableTLSVerify            bool   `default:"true" yaml:"enable_tls_verify"`                           // EnableTLSVerify toggles TLS certificate verification for HTTPS connections to the HealthURL.
	MaximumRequestsPerSecond   int    `default:"5" validate:"gte=1" yaml:"maximum_requests_per_second"`   // MaximumRequestsPerSecond limits the maximum number of GitLab API requests per second.
	BurstableRequestsPerSecond int    `default:"5" validate:"gte=1" yaml:"burstable_requests_per_second"` // BurstableRequestsPerSecond allows short bursts above the normal max request rate.

	// MaximumJobsQueueSize limits the number of jobs queued internally before dropping new ones.
	// Recommended not to change unless you understand the implications.
	// Alternatives to hitting this limit include:
	// - Increasing polling intervals
	// - Increasing API rate limits
	// - Reducing number of monitored projects, refs, environments, or metrics
	// - Using webhooks instead of polling
	MaximumJobsQueueSize int `default:"1000" validate:"gte=10" yaml:"maximum_jobs_queue_size"`
}

// Redis holds the configuration for connecting to a Redis instance.
type Redis struct {
	// URL is the connection string used to connect to the Redis server.
	// Format example: redis[s]://[:password@]host[:port][/db-number][?option=value]
	URL string `yaml:"url"`
}

// Pull holds configuration related to how and when data is pulled from GitLab.
type Pull struct {
	// ProjectsFromWildcards configures the fetching of projects discovered through wildcard searches.
	ProjectsFromWildcards struct {
		OnInit          bool `default:"true" yaml:"on_init"`                           // OnInit determines whether projects should be fetched once at startup.
		Scheduled       bool `default:"true" yaml:"scheduled"`                         // Scheduled enables periodic fetching of projects.
		IntervalSeconds int  `default:"1800" validate:"gte=1" yaml:"interval_seconds"` // IntervalSeconds defines the interval in seconds between scheduled fetches.
	} `yaml:"projects_from_wildcards"`

	// EnvironmentsFromProjects configures the fetching of environments associated with projects.
	EnvironmentsFromProjects struct {
		OnInit          bool `default:"true" yaml:"on_init"`                           // OnInit determines whether environments should be fetched once at startup.
		Scheduled       bool `default:"true" yaml:"scheduled"`                         // Scheduled enables periodic fetching of environments.
		IntervalSeconds int  `default:"1800" validate:"gte=1" yaml:"interval_seconds"` // IntervalSeconds defines the interval in seconds between scheduled fetches.
	} `yaml:"environments_from_projects"`

	// RunnersFromProjects configures the fetching of runners associated with projects.
	RunnersFromProjects struct {
		OnInit          bool `default:"true" yaml:"on_init"`
		Scheduled       bool `default:"true" yaml:"scheduled"`
		IntervalSeconds int  `default:"1800" validate:"gte=1" yaml:"interval_seconds"`
	} `yaml:"runners_from_projects"`

	// RefsFromProjects configures the fetching of refs (branches, tags, MRs) for projects.
	RefsFromProjects struct {
		OnInit          bool `default:"true" yaml:"on_init"`                          // OnInit determines whether refs should be fetched once at startup.
		Scheduled       bool `default:"true" yaml:"scheduled"`                        // Scheduled enables periodic fetching of refs.
		IntervalSeconds int  `default:"300" validate:"gte=1" yaml:"interval_seconds"` // IntervalSeconds defines the interval in seconds between scheduled fetches.
	} `yaml:"refs_from_projects"`

	// Metrics configures how metrics data is fetched.
	Metrics struct {
		OnInit          bool `default:"true" yaml:"on_init"`                         // OnInit determines whether metrics should be fetched once at startup.
		Scheduled       bool `default:"true" yaml:"scheduled"`                       // Scheduled enables periodic fetching of metrics.
		IntervalSeconds int  `default:"30" validate:"gte=1" yaml:"interval_seconds"` // IntervalSeconds defines the interval in seconds between scheduled fetches.
	} `yaml:"metrics"`
}

// GarbageCollect holds configuration for periodic cleanup tasks.
type GarbageCollect struct {
	// Projects configures cleanup behavior related to projects.
	Projects struct {
		OnInit          bool `default:"false" yaml:"on_init"`                           // OnInit indicates if cleanup should run once at startup.
		Scheduled       bool `default:"true" yaml:"scheduled"`                          // Scheduled indicates if cleanup should run periodically.
		IntervalSeconds int  `default:"14400" validate:"gte=1" yaml:"interval_seconds"` // IntervalSeconds sets the interval in seconds between cleanup runs. 4 hours
	} `yaml:"projects"`

	// Environments configures cleanup behavior related to environments.
	Environments struct {
		OnInit          bool `default:"false" yaml:"on_init"`
		Scheduled       bool `default:"true" yaml:"scheduled"`
		IntervalSeconds int  `default:"14400" validate:"gte=1" yaml:"interval_seconds"` // 4 hours
	} `yaml:"environments"`

	// Runners configures cleanup behavior related to runners.
	Runners struct {
		OnInit          bool `default:"false" yaml:"on_init"`
		Scheduled       bool `default:"true" yaml:"scheduled"`
		IntervalSeconds int  `default:"14400" validate:"gte=1" yaml:"interval_seconds"`
	} `yaml:"runners"`

	// Refs configures cleanup behavior related to Git references (branches, tags, etc).
	Refs struct {
		OnInit          bool `default:"false" yaml:"on_init"`
		Scheduled       bool `default:"true" yaml:"scheduled"`
		IntervalSeconds int  `default:"1800" validate:"gte=1" yaml:"interval_seconds"` // 30 minutes
	} `yaml:"refs"`

	// Metrics configures cleanup behavior related to metrics data.
	Metrics struct {
		OnInit          bool `default:"false" yaml:"on_init"`
		Scheduled       bool `default:"true" yaml:"scheduled"`
		IntervalSeconds int  `default:"600" validate:"gte=1" yaml:"interval_seconds"` // 10 minutes
	} `yaml:"metrics"`
}

// UnmarshalYAML implements custom YAML unmarshaling logic for the Config struct.
// This allows more control over how the configuration is loaded from YAML files.
func (c *Config) UnmarshalYAML(v *yaml.Node) (err error) {
	// Define a local struct that mirrors Config but treats Projects and Wildcards as raw YAML nodes
	// so we can decode them individually with custom logic later.
	type localConfig struct {
		Log             Log               `yaml:"log"`
		OpenTelemetry   OpenTelemetry     `yaml:"opentelemetry"`
		Server          Server            `yaml:"server"`
		Gitlab          Gitlab            `yaml:"gitlab"`
		Redis           Redis             `yaml:"redis"`
		Pull            Pull              `yaml:"pull"`
		GarbageCollect  GarbageCollect    `yaml:"garbage_collect"`
		ProjectDefaults ProjectParameters `yaml:"project_defaults"`

		Projects  []yaml.Node `yaml:"projects"`  // hold projects as raw YAML nodes
		Wildcards []yaml.Node `yaml:"wildcards"` // hold wildcards as raw YAML nodes
	}

	// Initialize the local config with default values
	_cfg := localConfig{}
	defaults.MustSet(&_cfg)

	// Decode the input YAML into the local config struct
	if err = v.Decode(&_cfg); err != nil {
		return
	}

	// Copy the simple fields from local config to the actual Config struct
	c.Log = _cfg.Log
	c.OpenTelemetry = _cfg.OpenTelemetry
	c.Server = _cfg.Server
	c.Gitlab = _cfg.Gitlab
	c.Redis = _cfg.Redis
	c.Pull = _cfg.Pull
	c.GarbageCollect = _cfg.GarbageCollect
	c.ProjectDefaults = _cfg.ProjectDefaults

	// Decode each project YAML node into a Project object and append it
	for _, n := range _cfg.Projects {
		p := c.NewProject() // create a new Project with defaults
		if err = n.Decode(&p); err != nil {
			return
		}
		c.Projects = append(c.Projects, p)
	}

	// Decode each wildcard YAML node into a Wildcard object and append it
	for _, n := range _cfg.Wildcards {
		w := c.NewWildcard() // create a new Wildcard with defaults
		if err = n.Decode(&w); err != nil {
			return
		}
		c.Wildcards = append(c.Wildcards, w)
	}

	return
}

// ToYAML serializes the Config object into a YAML formatted string.
// Before serialization, it clears or masks sensitive data to avoid leaking secrets.
func (c Config) ToYAML() string {
	// Clear the Global config (not serialized)
	c.Global = Global{}

	// Mask sensitive tokens in the config to avoid exposing them in the output YAML
	c.Server.Webhook.SecretToken = "*******"
	c.Gitlab.Token = "*******"

	// Marshal the config struct into YAML bytes
	b, err := yaml.Marshal(c)
	if err != nil {
		// Panic on error because this function assumes marshaling should never fail
		panic(err)
	}

	// Return the YAML as a string
	return string(b)
}

// Validate checks if the Config struct's fields are valid according to
// the validation rules defined via struct tags and custom validators.
// It returns an error if any validation rule fails.
func (c Config) Validate() error {
	// Initialize the validator instance if not already done
	if validate == nil {
		validate = validator.New()
		// Register a custom validation rule to ensure at least
		// one project or wildcard is defined in the config
		_ = validate.RegisterValidation("at-least-1-project-or-wildcard", ValidateAtLeastOneProjectOrWildcard)
	}

	// Perform the validation on the Config struct and return the result
	return validate.Struct(c)
}

// SchedulerConfig defines common scheduling behavior for background tasks or jobs.
type SchedulerConfig struct {
	OnInit          bool // OnInit determines whether the task should run immediately at startup.
	Scheduled       bool // Scheduled determines whether the task should run on a recurring schedule.
	IntervalSeconds int  // IntervalSeconds specifies how often (in seconds) the task should run when scheduled.
}

// Log returns a structured representation of the scheduler configuration
// to help display it in logs for the end user.
func (sc SchedulerConfig) Log() log.Fields {
	onInit, scheduled := "no", "no"

	// Check if the job should run at startup
	if sc.OnInit {
		onInit = "yes"
	}

	// Check if the job is scheduled periodically and format the interval
	if sc.Scheduled {
		scheduled = fmt.Sprintf("every %vs", sc.IntervalSeconds)
	}

	// Return the log fields in a key-value format
	return log.Fields{
		"on-init":   onInit,
		"scheduled": scheduled,
	}
}

// ValidateAtLeastOneProjectOrWildcard is a custom validation function.
// It ensures that at least one project or one wildcard is configured in the Config.
// This is used by the validator to enforce that the configuration is not empty.
func ValidateAtLeastOneProjectOrWildcard(v validator.FieldLevel) bool {
	return v.Parent().FieldByName("Projects").Len() > 0 || v.Parent().FieldByName("Wildcards").Len() > 0
}

// New returns a new Config instance with default parameters set.
// It uses the `defaults` package to automatically populate the config struct
// with predefined default values where applicable.
func New() (c Config) {
	defaults.MustSet(&c) // Apply default values to the config fields
	return               // Return the initialized config
}

// NewProject returns a new Project instance initialized with the default project parameters
// defined in the Config (under ProjectDefaults).
func (c Config) NewProject() (p Project) {
	p.ProjectParameters = c.ProjectDefaults // Inherit default parameters from the Config
	return                                  // Return the initialized Project
}

// NewWildcard returns a new Wildcard instance initialized with the default project parameters
// defined in the Config (under ProjectDefaults).
func (c Config) NewWildcard() (w Wildcard) {
	w.ProjectParameters = c.ProjectDefaults // Inherit default parameters from the Config
	return                                  // Return the initialized Wildcard
}
