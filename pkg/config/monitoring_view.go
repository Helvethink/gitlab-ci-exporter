package config

import (
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MonitoringView is the allowlisted subset of the configuration exposed by the
// internal monitoring service.
//
// Every field is copied explicitly. Fields added to Config later are therefore not
// exposed until they are reviewed and added here. It never contains tokens,
// secrets, connection strings, or per-project settings.
type MonitoringView struct {
	Log            MonitoringLogView          `yaml:"log"`
	Server         MonitoringServerView       `yaml:"server"`
	Gitlab         MonitoringGitlabView       `yaml:"gitlab"`
	Redis          MonitoringToggleView       `yaml:"redis"`
	OpenTelemetry  MonitoringToggleView       `yaml:"opentelemetry"`
	Pull           map[string]SchedulerConfig `yaml:"pull"`
	GarbageCollect map[string]SchedulerConfig `yaml:"garbage_collect"`
	Projects       MonitoringProjectsView     `yaml:"projects"`
}

// MonitoringLogView exposes logging settings.
type MonitoringLogView struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// MonitoringServerView exposes HTTP server settings, without the webhook secret.
type MonitoringServerView struct {
	ListenAddress                      string        `yaml:"listen_address"`
	ReadHeaderTimeout                  time.Duration `yaml:"read_header_timeout"`
	ReadTimeout                        time.Duration `yaml:"read_timeout"`
	WriteTimeout                       time.Duration `yaml:"write_timeout"`
	IdleTimeout                        time.Duration `yaml:"idle_timeout"`
	EnablePprof                        bool          `yaml:"enable_pprof"`
	MetricsEnabled                     bool          `yaml:"metrics_enabled"`
	MetricsEnableOpenmetricsEncoding   bool          `yaml:"metrics_enable_openmetrics_encoding"`
	WebhookEnabled                     bool          `yaml:"webhook_enabled"`
	WebhookMaximumBodyBytes            int64         `yaml:"webhook_maximum_body_bytes"`
	WebhookMaximumConcurrentProcessing int           `yaml:"webhook_maximum_concurrent_processing"`
}

// MonitoringGitlabView exposes GitLab client settings, without the token.
type MonitoringGitlabView struct {
	URL                        string `yaml:"url"`
	HealthURL                  string `yaml:"health_url"`
	EnableHealthCheck          bool   `yaml:"enable_health_check"`
	EnableTLSVerify            bool   `yaml:"enable_tls_verify"`
	MaximumRequestsPerSecond   int    `yaml:"maximum_requests_per_second"`
	BurstableRequestsPerSecond int    `yaml:"burstable_requests_per_second"`
	MaximumJobsQueueSize       int    `yaml:"maximum_jobs_queue_size"`
}

// MonitoringToggleView reports only whether an integration is configured.
type MonitoringToggleView struct {
	Enabled bool `yaml:"enabled"`
}

// MonitoringProjectsView reports how many projects and wildcards are configured.
type MonitoringProjectsView struct {
	Count          int `yaml:"count"`
	WildcardsCount int `yaml:"wildcards_count"`
}

// MonitoringView returns the allowlisted configuration exposed by the monitoring service.
func (c Config) MonitoringView() MonitoringView {
	sched := func(onInit, scheduled bool, interval int) SchedulerConfig {
		return SchedulerConfig{OnInit: onInit, Scheduled: scheduled, IntervalSeconds: interval}
	}

	return MonitoringView{
		Log: MonitoringLogView{Level: c.Log.Level, Format: c.Log.Format},
		Server: MonitoringServerView{
			ListenAddress:                      c.Server.ListenAddress,
			ReadHeaderTimeout:                  c.Server.ReadHeaderTimeout,
			ReadTimeout:                        c.Server.ReadTimeout,
			WriteTimeout:                       c.Server.WriteTimeout,
			IdleTimeout:                        c.Server.IdleTimeout,
			EnablePprof:                        c.Server.EnablePprof,
			MetricsEnabled:                     c.Server.Metrics.Enabled,
			MetricsEnableOpenmetricsEncoding:   c.Server.Metrics.EnableOpenmetricsEncoding,
			WebhookEnabled:                     c.Server.Webhook.Enabled,
			WebhookMaximumBodyBytes:            c.Server.Webhook.MaximumBodyBytes,
			WebhookMaximumConcurrentProcessing: c.Server.Webhook.MaximumConcurrentProcessing,
		},
		Gitlab: MonitoringGitlabView{
			URL:                        RedactURL(c.Gitlab.URL),
			HealthURL:                  RedactURL(c.Gitlab.HealthURL),
			EnableHealthCheck:          c.Gitlab.EnableHealthCheck,
			EnableTLSVerify:            c.Gitlab.EnableTLSVerify,
			MaximumRequestsPerSecond:   c.Gitlab.MaximumRequestsPerSecond,
			BurstableRequestsPerSecond: c.Gitlab.BurstableRequestsPerSecond,
			MaximumJobsQueueSize:       c.Gitlab.MaximumJobsQueueSize,
		},
		Redis:         MonitoringToggleView{Enabled: c.Redis.URL != ""},
		OpenTelemetry: MonitoringToggleView{Enabled: c.OpenTelemetry.GRPCEndpoint != ""},
		Pull: map[string]SchedulerConfig{
			"projects_from_wildcards":    sched(c.Pull.ProjectsFromWildcards.OnInit, c.Pull.ProjectsFromWildcards.Scheduled, c.Pull.ProjectsFromWildcards.IntervalSeconds),
			"environments_from_projects": sched(c.Pull.EnvironmentsFromProjects.OnInit, c.Pull.EnvironmentsFromProjects.Scheduled, c.Pull.EnvironmentsFromProjects.IntervalSeconds),
			"runners_from_projects":      sched(c.Pull.RunnersFromProjects.OnInit, c.Pull.RunnersFromProjects.Scheduled, c.Pull.RunnersFromProjects.IntervalSeconds),
			"refs_from_projects":         sched(c.Pull.RefsFromProjects.OnInit, c.Pull.RefsFromProjects.Scheduled, c.Pull.RefsFromProjects.IntervalSeconds),
			"metrics":                    sched(c.Pull.Metrics.OnInit, c.Pull.Metrics.Scheduled, c.Pull.Metrics.IntervalSeconds),
		},
		GarbageCollect: map[string]SchedulerConfig{
			"projects":     sched(c.GarbageCollect.Projects.OnInit, c.GarbageCollect.Projects.Scheduled, c.GarbageCollect.Projects.IntervalSeconds),
			"environments": sched(c.GarbageCollect.Environments.OnInit, c.GarbageCollect.Environments.Scheduled, c.GarbageCollect.Environments.IntervalSeconds),
			"runners":      sched(c.GarbageCollect.Runners.OnInit, c.GarbageCollect.Runners.Scheduled, c.GarbageCollect.Runners.IntervalSeconds),
			"refs":         sched(c.GarbageCollect.Refs.OnInit, c.GarbageCollect.Refs.Scheduled, c.GarbageCollect.Refs.IntervalSeconds),
			"metrics":      sched(c.GarbageCollect.Metrics.OnInit, c.GarbageCollect.Metrics.Scheduled, c.GarbageCollect.Metrics.IntervalSeconds),
		},
		Projects: MonitoringProjectsView{Count: len(c.Projects), WildcardsCount: len(c.Wildcards)},
	}
}

// YAML renders the view as YAML.
func (v MonitoringView) YAML() (string, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RedactURL removes credentials (user info), the query string, and the fragment from a URL.
// A value that cannot be parsed is fully redacted, because it may still hold a secret.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "*******"
	}
	if u.User != nil {
		u.User = url.User("redacted")
	}
	// Without "//", "user:password@host" parses as scheme "user" with the opaque
	// part "password@host". Redact everything before the last "@".
	if at := strings.LastIndex(u.Opaque, "@"); at >= 0 {
		u.Opaque = "redacted" + u.Opaque[at:]
	}
	if u.RawQuery != "" || u.ForceQuery {
		u.RawQuery = "redacted"
	}
	if u.Fragment != "" {
		u.Fragment, u.RawFragment = "redacted", ""
	}
	return u.String()
}
