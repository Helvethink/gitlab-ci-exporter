package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	gitlabclient "github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func TestHealthCheckHandlerWithGitLabReadinessEnabled(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/-/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	gl, err := gitlabclient.NewClient(gitlabclient.ClientConfig{
		URL:              server.URL + "/api/v4",
		Token:            "test-token",
		UserAgentVersion: "test",
		ReadinessURL:     server.URL + "/-/health",
		RateLimiter:      noopLimiterControllerRunnersTests{},
	})
	require.NoError(t, err)
	gl.RateCounter = ratecounter.NewRateCounter(time.Second)

	c := &Controller{
		Gitlab: gl,
		Config: config.Config{
			Gitlab: config.Gitlab{
				EnableHealthCheck: true,
			},
		},
	}

	handler := c.HealthCheckHandler(ctx)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	handler.ReadyEndpoint(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHealthCheckHandlerWithGitLabReadinessDisabled(t *testing.T) {
	ctx := context.Background()
	c := &Controller{
		Config: config.Config{
			Gitlab: config.Gitlab{
				EnableHealthCheck: false,
			},
		},
	}

	handler := c.HealthCheckHandler(ctx)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	handler.ReadyEndpoint(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestMetricsHandlerExportsStoredAndInternalMetrics(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	require.NoError(t, c.Store.SetMetric(ctx, schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: map[string]string{
			"project":     "group/project",
			"topics":      "",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 97.5,
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	c.MetricsHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "gitlab_ci_pipeline_coverage")
	assert.Contains(t, rr.Body.String(), `project="group/project"`)
	assert.Contains(t, rr.Body.String(), "97.5")
	assert.Contains(t, rr.Body.String(), "gcpe_metrics_count")
	assert.Contains(t, rr.Body.String(), "gcpe_projects_count")
}

func TestWebhookHandlerRejectsInvalidToken(t *testing.T) {
	c := &Controller{
		Config: config.Config{
			Server: config.Server{
				Webhook: config.ServerWebhook{
					SecretToken: "expected-secret",
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Gitlab-Token", "wrong-secret")
	rr := httptest.NewRecorder()

	c.WebhookHandler(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.JSONEq(t, `{"error":"invalid token"}`, rr.Body.String())
}

func TestWebhookHandlerReturnsBadRequestOnEmptyBody(t *testing.T) {
	c := &Controller{
		Config: config.Config{
			Server: config.Server{
				Webhook: config.ServerWebhook{
					SecretToken: "expected-secret",
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", http.NoBody)
	req.Header.Set("X-Gitlab-Token", "expected-secret")
	rr := httptest.NewRecorder()

	c.WebhookHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestWebhookHandlerReturnsBadRequestOnInvalidPayload(t *testing.T) {
	c := &Controller{
		Config: config.Config{
			Server: config.Server{
				Webhook: config.ServerWebhook{
					SecretToken: "expected-secret",
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"broken":`))
	req.Header.Set("X-Gitlab-Token", "expected-secret")
	req.Header.Set("X-Gitlab-Event", "Pipeline Hook")
	rr := httptest.NewRecorder()

	c.WebhookHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
