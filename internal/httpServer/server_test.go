package httpServer

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/internal/collectors"
)

func newTestExporter(t *testing.T) *collectors.Exporter {
	t.Helper()

	exporter, err := collectors.NewExporter(&collectors.Settings{
		MetricsPath: "metrics",
		ListenPort:  "9191",
		Address:     "0.0.0.0",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	return exporter
}

func withFreshServeMux(t *testing.T) {
	t.Helper()

	previousMux := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
	t.Cleanup(func() {
		http.DefaultServeMux = previousMux
	})
}

func TestNewServerSetsExpectedAddr(t *testing.T) {
	withFreshServeMux(t)

	server := NewServer(newTestExporter(t))

	require.NotNil(t, server)
	assert.Equal(t, ":9191", server.Addr)
	assert.Nil(t, server.Handler)
}

func TestNewServerServesRootPage(t *testing.T) {
	withFreshServeMux(t)

	_ = NewServer(newTestExporter(t))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "Gitlab CI Exporter")
	assert.Contains(t, rr.Body.String(), "Metrics at:")
	assert.Contains(t, rr.Body.String(), "href='metrics'")
	assert.Contains(t, rr.Body.String(), "github.com/Helvethink/gitlab-ci-exporter")
}

func TestNewServerServesMetricsEndpoint(t *testing.T) {
	withFreshServeMux(t)

	_ = NewServer(newTestExporter(t))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "gitlab_ci_exporter_sampleMetric1")
	assert.Contains(t, rr.Body.String(), "gitlab_ci_exporter_sampleMetric2")
}
