package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type phaseRecorder struct {
	mu    sync.Mutex
	order []string
}

func (r *phaseRecorder) phase(name string, timeout time.Duration, run func(context.Context) error) shutdownPhase {
	return shutdownPhase{name: name, timeout: timeout, run: func(ctx context.Context) error {
		r.mu.Lock()
		r.order = append(r.order, name)
		r.mu.Unlock()
		return run(ctx)
	}}
}

func (r *phaseRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

func ok(context.Context) error { return nil }

func TestShutdownPhasesOrder(t *testing.T) {
	phases := shutdownPhases(nil, nil, func() {}, make(chan struct{}))

	names := make([]string, 0, len(phases))
	var budget time.Duration
	for _, p := range phases {
		names = append(names, p.name)
		budget += p.timeout
		assert.Positive(t, p.timeout, p.name)
	}
	assert.Equal(t, []string{phaseStopIntake, phaseDrainWork, phaseStopServers, phaseCloseClients}, names)
	assert.LessOrEqual(t, budget, applicationShutdownTimeout, "phase budgets must fit the overall shutdown timeout")
}

func TestRunShutdownRunsPhasesInOrder(t *testing.T) {
	r := &phaseRecorder{}
	err := runShutdown(context.Background(), []shutdownPhase{
		r.phase("a", time.Second, ok),
		r.phase("b", time.Second, ok),
		r.phase("c", time.Second, ok),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, r.recorded())
}

func TestRunShutdownContinuesAfterFailedOrHungPhase(t *testing.T) {
	r := &phaseRecorder{}
	boom := errors.New("boom")
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	start := time.Now()
	err := runShutdown(context.Background(), []shutdownPhase{
		r.phase("fails", time.Second, func(context.Context) error { return boom }),
		// Ignores its context entirely: the phase timeout must still bound it.
		r.phase("hangs", 100*time.Millisecond, func(context.Context) error { <-release; return nil }),
		r.phase("closes clients", time.Second, ok),
	})

	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, []string{"fails", "hangs", "closes clients"}, r.recorded())
	require.ErrorIs(t, err, boom)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorContains(t, err, "fails: boom")
	assert.ErrorContains(t, err, "hangs: context deadline exceeded")
}

func TestRunShutdownHonorsOverallDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	slow := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	r := &phaseRecorder{}
	start := time.Now()
	err := runShutdown(ctx, []shutdownPhase{
		r.phase("first", time.Hour, slow),
		r.phase("second", time.Hour, slow),
	})

	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, []string{"first", "second"}, r.recorded(), "later phases still run, with an expired budget")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestShutdownHTTPServersWaitsForActiveRequests(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(listener) }()

	response := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			response <- 0
			return
		}
		_ = resp.Body.Close()
		response <- resp.StatusCode
	}()
	<-started

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- shutdownHTTPServers(context.Background(), []*http.Server{server}) }()

	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned while a request was active")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-shutdownDone)
	assert.Equal(t, http.StatusNoContent, <-response)
}

func TestRunShutsDownWhenHTTPListenerFails(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()

	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer gitlabServer.Close()

	configPath := filepath.Join(t.TempDir(), "config.yml")
	configYAML := fmt.Sprintf("wildcards:\n  - {}\ngitlab:\n  url: %q\n  token: test-token\nserver:\n  listen_address: %q\n",
		gitlabServer.URL, occupied.Addr().String())
	require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o600))
	ctx, flags := NewTestContext()
	flags.String("config", configPath, "")
	flags.String("gitlab-token", "test-token", "")
	flags.String("internal-monitoring-listener-address", "", "")

	done := make(chan struct{})
	var (
		code   int
		runErr error
	)
	go func() {
		defer close(done)
		code, runErr = Run(ctx)
	}()

	select {
	case <-done:
	case <-time.After(applicationShutdownTimeout):
		t.Fatal("Run did not stop after the HTTP listener failed")
	}
	assert.Equal(t, 1, code)
	require.ErrorContains(t, runErr, "address already in use")
}
