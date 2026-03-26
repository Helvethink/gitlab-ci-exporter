package logger

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveLogrusState(t *testing.T) {
	t.Helper()

	prevLevel := log.GetLevel()
	prevFormatter := log.StandardLogger().Formatter
	prevReportCaller := log.StandardLogger().ReportCaller
	prevOutput := log.StandardLogger().Out

	t.Cleanup(func() {
		log.SetLevel(prevLevel)
		log.SetFormatter(prevFormatter)
		log.SetReportCaller(prevReportCaller)
		log.SetOutput(prevOutput)
	})
}

func TestConfigureTextFormat(t *testing.T) {
	preserveLogrusState(t)

	err := Configure(Config{
		Level:        "debug",
		Format:       "text",
		ReportCaller: true,
	})
	require.NoError(t, err)

	assert.Equal(t, log.DebugLevel, log.GetLevel())
	_, ok := log.StandardLogger().Formatter.(*log.TextFormatter)
	assert.True(t, ok)
	assert.True(t, log.StandardLogger().ReportCaller)
	assert.Same(t, os.Stdout, log.StandardLogger().Out)
}

func TestConfigureJSONFormat(t *testing.T) {
	preserveLogrusState(t)

	err := Configure(Config{
		Level:        "info",
		Format:       "json",
		ReportCaller: false,
	})
	require.NoError(t, err)

	assert.Equal(t, log.InfoLevel, log.GetLevel())
	_, ok := log.StandardLogger().Formatter.(*log.JSONFormatter)
	assert.True(t, ok)
	assert.False(t, log.StandardLogger().ReportCaller)
	assert.Same(t, os.Stdout, log.StandardLogger().Out)
}

func TestConfigureInvalidLevelReturnsError(t *testing.T) {
	preserveLogrusState(t)

	err := Configure(Config{
		Level:  "not-a-level",
		Format: "text",
	})
	require.Error(t, err)
}

func TestConfigureInvalidFormatReturnsError(t *testing.T) {
	preserveLogrusState(t)

	err := Configure(Config{
		Level:  "info",
		Format: "xml",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log format")
}
