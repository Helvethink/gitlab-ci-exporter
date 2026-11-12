package cmd

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor"
)

func TestMonitor(t *testing.T) {
	ctx, flags := NewTestContext()
	ctx.App.Version = "1.2.3"
	flags.String("internal-monitoring-listener-address", "", "")
	require.NoError(t, flags.Set("internal-monitoring-listener-address", "unix:///tmp/gcpe-monitor.sock"))

	var (
		called         bool
		gotVersion     string
		gotListenerURL *url.URL
	)

	previousStart := startMonitorUI
	startMonitorUI = func(version string, listenerAddress *url.URL) error {
		called = true
		gotVersion = version
		gotListenerURL = listenerAddress
		return nil
	}
	t.Cleanup(func() {
		startMonitorUI = previousStart
	})

	exitCode, err := Monitor(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.True(t, called)
	assert.Equal(t, "1.2.3", gotVersion)
	require.NotNil(t, gotListenerURL)
	assert.Equal(t, "unix:///tmp/gcpe-monitor.sock", gotListenerURL.String())
}

func TestMonitorReturnsErrorWhenInternalMonitoringAddressIsInvalid(t *testing.T) {
	ctx, flags := NewTestContext()
	flags.String("internal-monitoring-listener-address", "", "")
	require.NoError(t, flags.Set("internal-monitoring-listener-address", "://bad-url"))

	called := false
	previousStart := startMonitorUI
	startMonitorUI = func(version string, listenerAddress *url.URL) error {
		called = true
		return nil
	}
	t.Cleanup(func() {
		startMonitorUI = previousStart
	})

	exitCode, err := Monitor(ctx)
	require.Error(t, err)
	assert.Equal(t, 1, exitCode)
	assert.False(t, called)
}

func TestMonitorWithoutInternalMonitoringAddress(t *testing.T) {
	ctx, flags := NewTestContext()
	ctx.App.Version = "dev"
	flags.String("internal-monitoring-listener-address", "", "")

	called := false
	var gotListenerURL *url.URL
	previousStart := startMonitorUI
	startMonitorUI = func(version string, listenerAddress *url.URL) error {
		called = true
		gotListenerURL = listenerAddress
		return nil
	}
	t.Cleanup(func() {
		startMonitorUI = previousStart
	})

	exitCode, err := Monitor(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.True(t, called)
	assert.Nil(t, gotListenerURL)
}

func TestMonitorReturnsUIError(t *testing.T) {
	ctx, flags := NewTestContext()
	flags.String("internal-monitoring-listener-address", "", "")
	require.NoError(t, flags.Set("internal-monitoring-listener-address", "unix:///tmp/gcpe-monitor.sock"))
	uiErr := errors.New("monitoring connection failed")
	previousStart := startMonitorUI
	startMonitorUI = func(string, *url.URL) error { return uiErr }
	t.Cleanup(func() { startMonitorUI = previousStart })
	code, err := Monitor(ctx)
	require.ErrorIs(t, err, uiErr)
	assert.Equal(t, 1, code)
}

func TestMonitorRejectsNonUnixAddresses(t *testing.T) {
	for _, address := range []string{"tcp://127.0.0.1:8081", "http://127.0.0.1:8081", "tcp://0.0.0.0:8081"} {
		t.Run(address, func(t *testing.T) {
			ctx, flags := NewTestContext()
			flags.String("internal-monitoring-listener-address", "", "")
			require.NoError(t, flags.Set("internal-monitoring-listener-address", address))

			called := false
			previousStart := startMonitorUI
			startMonitorUI = func(string, *url.URL) error {
				called = true
				return nil
			}
			t.Cleanup(func() { startMonitorUI = previousStart })

			code, err := Monitor(ctx)
			require.ErrorIs(t, err, monitor.ErrUnsupportedListenerAddress)
			assert.Equal(t, 1, code)
			assert.False(t, called)
		})
	}
}
