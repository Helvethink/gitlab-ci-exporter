package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func secretConfig() Config {
	c := New()
	c.Gitlab.Token = "SECRET-token"
	c.Gitlab.URL = "https://user:SECRET-basic@gitlab.example.com/?private_token=SECRET-query"
	c.Gitlab.HealthURL = "https://gitlab.example.com/-/health?token=SECRET-health"
	c.Server.Webhook.SecretToken = "SECRET-webhook"
	c.Redis.URL = "redis://default:SECRET-redis@redis.example.com:6379/0"
	c.OpenTelemetry.GRPCEndpoint = "https://otel:SECRET-otel@collector.example.com:4317"
	c.Projects = []Project{NewProject("group/project")}
	c.Wildcards = []Wildcard{NewWildcard()}
	return c
}

func TestMonitoringViewIsAllowlisted(t *testing.T) {
	out, err := secretConfig().MonitoringView().YAML()
	require.NoError(t, err)

	assert.NotContains(t, out, "SECRET")
	assert.NotContains(t, out, "redis.example.com")
	assert.NotContains(t, out, "collector.example.com")
	assert.Contains(t, out, "gitlab.example.com")
	assert.Contains(t, out, "redis:\n    enabled: true")
	assert.Contains(t, out, "opentelemetry:\n    enabled: true")
	assert.Contains(t, out, "count: 1")
	assert.Contains(t, out, "wildcards_count: 1")
	assert.Contains(t, out, "interval_seconds: 30")
}

func TestToYAMLRedactsCredentials(t *testing.T) {
	assert.NotContains(t, secretConfig().ToYAML(), "SECRET")
}

func TestRedactURL(t *testing.T) {
	tests := map[string]string{
		"":                                      "",
		"https://gitlab.com":                    "https://gitlab.com",
		"redis://default:pw@redis:6379/0":       "redis://redacted@redis:6379/0",
		"redis://:pw@redis:6379":                "redis://redacted@redis:6379",
		"https://gitlab.com/?private_token=abc": "https://gitlab.com/?redacted",
		"redis://user:p%zzw@redis:6379 (invalid)": "*******",
	}
	for in, want := range tests {
		assert.Equal(t, want, RedactURL(in), in)
	}
}
