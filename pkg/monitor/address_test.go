package monitor

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateListenerAddress(t *testing.T) {
	valid := []string{"unix:///tmp/gcpe-monitor.sock", "unix:///var/run/gcpe/monitor.sock"}
	for _, raw := range valid {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		assert.NoError(t, ValidateListenerAddress(u), raw)
	}

	invalid := []string{
		"tcp://127.0.0.1:8082",
		"tcp://0.0.0.0:8082",
		"http://127.0.0.1:8082",
		"dns:///localhost:8082",
		"unix://relative/path.sock",
		"unix://",
		"/tmp/no-scheme.sock",
	}
	for _, raw := range invalid {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		assert.ErrorIs(t, ValidateListenerAddress(u), ErrUnsupportedListenerAddress, raw)
	}

	assert.Error(t, ValidateListenerAddress(nil))
}
