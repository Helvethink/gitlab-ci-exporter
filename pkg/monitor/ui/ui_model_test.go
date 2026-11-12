package ui

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
)

const (
	uiWait = 3 * time.Second
	uiTick = 10 * time.Millisecond
)

// uiServer is a stub monitoring server. GetTelemetry forwards every value sent
// on telemetry, then returns streamErr once telemetry is closed.
type uiServer struct {
	pb.UnimplementedMonitorServer
	config    string
	configErr error
	telemetry chan *pb.Telemetry
	streamErr error
}

func (s *uiServer) GetConfig(context.Context, *pb.Empty) (*pb.Config, error) {
	if s.configErr != nil {
		return nil, s.configErr
	}
	return &pb.Config{Content: s.config}, nil
}

func (s *uiServer) GetTelemetry(_ *pb.Empty, stream grpc.ServerStreamingServer[pb.Telemetry]) error {
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case tm, ok := <-s.telemetry:
			if !ok {
				return s.streamErr
			}
			if err := stream.Send(tm); err != nil {
				return err
			}
		}
	}
}

func uiNewServer() *uiServer {
	return &uiServer{config: "gitlab:\n  url: https://gitlab.example.invalid\n", telemetry: make(chan *pb.Telemetry)}
}

// uiStartServer serves srv on a short Unix socket path and returns its endpoint
// and the grpc.Server so tests can stop it early.
func uiStartServer(t *testing.T, srv pb.MonitorServer) (*url.URL, *grpc.Server) {
	t.Helper()
	dir, err := os.MkdirTemp("", "gce")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "m.sock")

	lis, err := net.Listen("unix", socket)
	require.NoError(t, err)

	gs := grpc.NewServer()
	pb.RegisterMonitorServer(gs, srv)
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = gs.Serve(lis)
	}()
	t.Cleanup(func() {
		gs.Stop()
		<-served
	})

	endpoint, err := url.Parse("unix://" + socket)
	require.NoError(t, err)
	return endpoint, gs
}

// uiNewModel builds a model connected to srv and registers its teardown.
func uiNewModel(t *testing.T, srv pb.MonitorServer) (*model, *grpc.Server) {
	t.Helper()
	endpoint, gs := uiStartServer(t, srv)
	m, err := newModel("v1.2.3", endpoint)
	require.NoError(t, err)
	t.Cleanup(func() {
		m.cancelStream()
		_ = m.client.Close()
		// Keep goroutine counts per test isolated.
		assert.Eventually(t, func() bool { return uiStreamGoroutines() == 0 }, uiWait, uiTick, "stream goroutine leaked")
	})
	return m, gs
}

// uiRun executes cmd with a deadline so a blocked command fails the test
// instead of hanging it.
func uiRun(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg
	case <-time.After(uiWait):
		t.Fatal("command did not return in time")
		return nil
	}
}

func uiTelemetry(executed uint64) *pb.Telemetry {
	now := timestamppb.New(time.Unix(1710000000, 0))
	e := func(n int64) *pb.Entity {
		return &pb.Entity{Count: n, LastPull: now, LastGc: now, NextPull: now, NextGc: now}
	}
	return &pb.Telemetry{
		GitlabApiUsage:          0.5,
		GitlabApiRequestsCount:  11,
		GitlabApiRateLimit:      0.2,
		GitlabApiLimitRemaining: 3,
		TasksBufferUsage:        0.1,
		TasksExecutedCount:      executed,
		Projects:                e(1),
		Envs:                    e(2),
		Refs:                    e(3),
		Metrics:                 e(4),
	}
}

// uiStreamGoroutines counts live streamTelemetry reader goroutines.
func uiStreamGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "ui.(*model).streamTelemetry.func")
}

func uiIsQuit(t *testing.T, cmd tea.Cmd) bool {
	t.Helper()
	if cmd == nil {
		return false
	}
	_, ok := uiRun(t, cmd).(tea.QuitMsg)
	return ok
}

func TestUINewModelRejectsInvalidEndpoint(t *testing.T) {
	_, err := newModel("v", nil)
	require.Error(t, err)

	endpoint, err := url.Parse("tcp://127.0.0.1:8082")
	require.NoError(t, err)
	_, err = newModel("v", endpoint)
	require.ErrorIs(t, err, monitor.ErrUnsupportedListenerAddress)
}

func TestUIStartRejectsInvalidEndpoint(t *testing.T) {
	endpoint, err := url.Parse("http://127.0.0.1:8082")
	require.NoError(t, err)
	require.ErrorIs(t, Start("v", endpoint), monitor.ErrUnsupportedListenerAddress)
}

func TestUINewModelInitialState(t *testing.T) {
	m, _ := uiNewModel(t, uiNewServer())
	assert.Equal(t, "v1.2.3", m.version)
	assert.NotNil(t, m.client)
	assert.NotNil(t, m.progress)
	assert.NotNil(t, m.telemetryStream)
	assert.Zero(t, m.tabID)
	assert.Nil(t, m.telemetry)
	require.NoError(t, m.streamContext.Err())
}

func TestUIWindowSizeRendersLayout(t *testing.T) {
	m, _ := uiNewModel(t, uiNewServer())

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	assert.Same(t, m, next)
	assert.Nil(t, cmd)
	assert.Equal(t, 100, m.vp.Width)
	assert.Equal(t, 26, m.vp.Height)
	assert.Equal(t, 73, m.progress.Width)

	view := m.View()
	assert.Contains(t, view, "telemetry")
	assert.Contains(t, view, "config")
	assert.Contains(t, view, "loading data..")
	assert.Contains(t, view, "github.com/helvethink/gitlab-ci-exporter")
	assert.Contains(t, view, "v1.2.3")
}

func TestUITabSwitchingChangesPane(t *testing.T) {
	srv := uiNewServer()
	m, _ := uiNewModel(t, srv)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Nil(t, cmd)
	assert.Zero(t, m.tabID, "left on first tab stays put")

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Nil(t, cmd)
	assert.Equal(t, 1, m.tabID)
	view := m.View()
	assert.Contains(t, view, "gitlab.example.invalid")
	assert.NotContains(t, view, "loading data..")

	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 1, m.tabID, "right on last tab stays put")

	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Zero(t, m.tabID)
	view = m.View()
	assert.Contains(t, view, "loading data..")
	assert.NotContains(t, view, "gitlab.example.invalid")
}

func TestUIConfigFailureRendersMessage(t *testing.T) {
	srv := uiNewServer()
	srv.configErr = status.Error(codes.Internal, "config exploded")
	m, _ := uiNewModel(t, srv)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	view := m.View()
	assert.Contains(t, view, "Unable to load configuration")
	assert.Contains(t, view, "config exploded")
}

func TestUIConfigServerDownRendersMessage(t *testing.T) {
	m, gs := uiNewModel(t, uiNewServer())
	gs.Stop()
	m.tabID = 1
	assert.Contains(t, m.renderConfigViewport(), "Unable to load configuration")
}

func TestUIQuitKeysCancelStream(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyCtrlC, tea.KeyEsc} {
		t.Run(key.String(), func(t *testing.T) {
			m, _ := uiNewModel(t, uiNewServer())
			_, cmd := m.Update(tea.KeyMsg{Type: key})
			assert.True(t, uiIsQuit(t, cmd))
			assert.ErrorIs(t, m.streamContext.Err(), context.Canceled)
			assert.NoError(t, m.terminalErr)
		})
	}
}

func TestUIUnhandledKeysAreIgnored(t *testing.T) {
	m, _ := uiNewModel(t, uiNewServer())
	for _, msg := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")},
		tea.KeyMsg{Type: tea.KeyEnter},
		struct{}{},
	} {
		next, cmd := m.Update(msg)
		assert.Same(t, m, next)
		assert.Nil(t, cmd)
	}
	assert.NoError(t, m.streamContext.Err())
}

func TestUIScrollKeysMoveViewport(t *testing.T) {
	m, _ := uiNewModel(t, uiNewServer())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m.vp.SetContent(strings.Repeat("line\n", 100))

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m.vp.YOffset)
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	assert.Greater(t, m.vp.YOffset, 1)
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	assert.Equal(t, 1, m.vp.YOffset)
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Zero(t, m.vp.YOffset)
}

func TestUITelemetryMessageUpdatesView(t *testing.T) {
	m, _ := uiNewModel(t, uiNewServer())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	require.Contains(t, m.View(), "loading data..")

	_, cmd := m.Update(uiTelemetry(424242))
	view := m.View()
	assert.NotContains(t, view, "loading data..")
	assert.Contains(t, view, "Tasks executed")
	assert.Contains(t, view, "424242")

	// The returned command waits for the next stream message.
	next := uiTelemetry(7)
	go func() { m.telemetryStream <- next }()
	assert.Same(t, next, uiRun(t, cmd))
}

func TestUIErrorMessageQuits(t *testing.T) {
	m, _ := uiNewModel(t, uiNewServer())
	boom := errors.New("boom")
	_, cmd := m.Update(boom)
	assert.True(t, uiIsQuit(t, cmd))
	assert.ErrorIs(t, m.terminalErr, boom)
	assert.ErrorIs(t, m.streamContext.Err(), context.Canceled)
}

// TestUIInitStreamsTelemetry drives Init against the stub server and feeds the
// received message back through Update.
func TestUIInitStreamsTelemetry(t *testing.T) {
	srv := uiNewServer()
	m, _ := uiNewModel(t, srv)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})

	cmd := m.Init()
	require.NotNil(t, cmd)

	go func() { srv.telemetry <- uiTelemetry(31337) }()
	msg := uiRun(t, cmd)
	tm, ok := msg.(*pb.Telemetry)
	require.True(t, ok, "got %T", msg)
	assert.Equal(t, uint64(31337), tm.GetTasksExecutedCount())

	_, cmd = m.Update(msg)
	assert.Contains(t, m.View(), "31337")

	go func() { srv.telemetry <- uiTelemetry(5150) }()
	_, _ = m.Update(uiRun(t, cmd))
	assert.Contains(t, m.View(), "5150")
}

// TestUIStreamErrorSurfacesAsTerminalErr asserts a server-side stream error
// reaches the model as terminalErr and quits the program.
func TestUIStreamErrorSurfacesAsTerminalErr(t *testing.T) {
	srv := uiNewServer()
	srv.streamErr = status.Error(codes.PermissionDenied, "peer rejected")
	m, _ := uiNewModel(t, srv)

	require.Nil(t, m.streamTelemetry(m.streamContext))
	close(srv.telemetry)

	msg := uiRun(t, waitForTelemetryUpdate(m.telemetryStream))
	err, ok := msg.(error)
	require.True(t, ok, "got %T", msg)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	_, cmd := m.Update(msg)
	assert.True(t, uiIsQuit(t, cmd))
	assert.Equal(t, codes.PermissionDenied, status.Code(m.terminalErr))
	assert.Error(t, m.streamContext.Err())
	require.Eventually(t, func() bool { return uiStreamGoroutines() == 0 }, uiWait, uiTick)
}

// TestUIServerDisconnectEndsStream asserts that stopping the server delivers an
// error and the reader goroutine exits.
func TestUIServerDisconnectEndsStream(t *testing.T) {
	srv := uiNewServer()
	m, gs := uiNewModel(t, srv)

	require.Nil(t, m.streamTelemetry(m.streamContext))
	go func() { srv.telemetry <- uiTelemetry(1) }()
	_, ok := uiRun(t, waitForTelemetryUpdate(m.telemetryStream)).(*pb.Telemetry)
	require.True(t, ok)
	require.Equal(t, 1, uiStreamGoroutines())

	gs.Stop()

	msg := uiRun(t, waitForTelemetryUpdate(m.telemetryStream))
	_, isErr := msg.(error)
	require.True(t, isErr, "got %T", msg)
	require.Eventually(t, func() bool { return uiStreamGoroutines() == 0 }, uiWait, uiTick)
}

// TestUICancelEndsUnreadStream asserts the reader goroutine exits on context
// cancellation even when nobody drains telemetryStream.
func TestUICancelEndsUnreadStream(t *testing.T) {
	srv := uiNewServer()
	m, _ := uiNewModel(t, srv)

	require.Nil(t, m.streamTelemetry(m.streamContext))
	require.Eventually(t, func() bool { return uiStreamGoroutines() == 1 }, uiWait, uiTick)

	m.cancelStream()
	require.Eventually(t, func() bool { return uiStreamGoroutines() == 0 }, uiWait, uiTick)
}

// TestUIStreamTelemetryOpenFailure asserts that a failure to open the stream is
// returned as a command producing the error, with no goroutine started.
func TestUIStreamTelemetryOpenFailure(t *testing.T) {
	m, _ := uiNewModel(t, uiNewServer())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := m.streamTelemetry(ctx)
	require.NotNil(t, cmd)
	err, ok := uiRun(t, cmd).(error)
	require.True(t, ok)
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Zero(t, uiStreamGoroutines())
}
