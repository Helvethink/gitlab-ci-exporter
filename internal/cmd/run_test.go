package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/controller"
)

// func TestRunWrongLogLevel(t *testing.T) {
// 	ctx, flags := NewTestContext()
// 	flags.String("log-format", "foo", "")
// 	exitCode, err := Run(ctx)
// 	assert.Equal(t, 1, exitCode)
// 	assert.Error(t, err)
// }

func TestRunInvalidConfigFile(t *testing.T) {
	ctx, flags := NewTestContext()

	flags.String("config", "path_does_not_exist", "")

	exitCode, err := Run(ctx)
	assert.Equal(t, 1, exitCode)
	assert.Error(t, err)
}

func TestNewPublicHTTPServerIsBoundedAndExcludesPprof(t *testing.T) {
	cfg := config.New()
	cfg.Server.EnablePprof = true
	cfg.Server.Metrics.Enabled = true
	cfg.Server.Webhook.Enabled = true
	cfg.Server.Webhook.SecretToken = "secret"
	c := &controller.Controller{Config: cfg}

	server := newPublicHTTPServer(context.Background(), c)

	assert.Equal(t, cfg.Server.ListenAddress, server.Addr)
	assert.Equal(t, cfg.Server.ReadHeaderTimeout, server.ReadHeaderTimeout)
	assert.Equal(t, cfg.Server.ReadTimeout, server.ReadTimeout)
	assert.Equal(t, cfg.Server.WriteTimeout, server.WriteTimeout)
	assert.Equal(t, cfg.Server.IdleTimeout, server.IdleTimeout)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "health endpoint rejects POST",
			method:     http.MethodPost,
			path:       "/health/live",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "webhook endpoint rejects GET",
			method:     http.MethodGet,
			path:       "/webhook",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "metrics endpoint rejects POST",
			method:     http.MethodPost,
			path:       "/metrics",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "pprof is absent from public server",
			method:     http.MethodGet,
			path:       "/debug/pprof/",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()

			server.Handler.ServeHTTP(res, req)

			assert.Equal(t, tt.wantStatus, res.Code)
		})
	}
}

func TestNewPprofHTTPServerUsesDedicatedListener(t *testing.T) {
	cfg := config.New().Server
	cfg.PprofListenAddress = "127.0.0.1:6061"

	server := newPprofHTTPServer(cfg)

	assert.Equal(t, "127.0.0.1:6061", server.Addr)
	assert.Zero(t, server.WriteTimeout)
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	res := httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)

	req = httptest.NewRequest(http.MethodPost, "/debug/pprof/", nil)
	res = httptest.NewRecorder()
	server.Handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusMethodNotAllowed, res.Code)
}

func TestNewHTTPServerAppliesConfiguredTimeouts(t *testing.T) {
	cfg := config.Server{
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       5 * time.Second,
	}

	server := newHTTPServer(":9090", http.NotFoundHandler(), cfg)

	assert.Equal(t, 2*time.Second, server.ReadHeaderTimeout)
	assert.Equal(t, 3*time.Second, server.ReadTimeout)
	assert.Equal(t, 4*time.Second, server.WriteTimeout)
	assert.Equal(t, 5*time.Second, server.IdleTimeout)
}
