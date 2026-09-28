package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
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

func TestRunSupervisesMonitoringFailureAndShutsDown(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer gitlabServer.Close()

	configPath := filepath.Join(t.TempDir(), "config.yml")
	configYAML := fmt.Sprintf("wildcards:\n  - {}\ngitlab:\n  url: %q\n  token: test-token\nserver:\n  listen_address: '127.0.0.1:0'\n", gitlabServer.URL)
	require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o600))
	ctx, flags := NewTestContext()
	flags.String("config", configPath, "")
	flags.String("gitlab-token", "test-token", "")
	flags.String("internal-monitoring-listener-address", "tcp://"+occupied.Addr().String(), "")

	code, err := Run(ctx)
	require.Equal(t, 1, code)
	require.ErrorContains(t, err, "listen for monitoring")
}

func TestRunWaitsForActiveHTTPRequestOnSignal(t *testing.T) {
	healthStarted := make(chan struct{})
	releaseHealth := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseHealth:
		default:
			close(releaseHealth)
		}
	})
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/health" {
			close(healthStarted)
			<-releaseHealth
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer gitlabServer.Close()

	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := reserved.Addr().String()
	require.NoError(t, reserved.Close())
	configPath := filepath.Join(t.TempDir(), "config.yml")
	configYAML := fmt.Sprintf("wildcards:\n  - {}\ngitlab:\n  url: %q\n  health_url: %q\n  token: test-token\nserver:\n  listen_address: %q\n",
		gitlabServer.URL+"/api/v4", gitlabServer.URL+"/-/health", address)
	require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o600))
	ctx, flags := NewTestContext()
	flags.String("config", configPath, "")
	flags.String("gitlab-token", "test-token", "")
	flags.String("internal-monitoring-listener-address", "", "")

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, runErr := Run(ctx)
		done <- result{code: code, err: runErr}
	}()
	client := &http.Client{Timeout: 3 * time.Second}
	require.Eventually(t, func() bool {
		response, err := client.Get("http://" + address + "/health/live")
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, 3*time.Second, 10*time.Millisecond)

	readyDone := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + address + "/health/ready")
		if err == nil {
			_ = response.Body.Close()
		}
		readyDone <- err
	}()
	select {
	case <-healthStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("readiness request did not start")
	}
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	select {
	case got := <-done:
		t.Fatalf("Run stopped while HTTP request was active: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseHealth)
	require.NoError(t, <-readyDone)
	select {
	case got := <-done:
		require.NoError(t, got.err)
		assert.Equal(t, 0, got.code)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after request completed")
	}
}
