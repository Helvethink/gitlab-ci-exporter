package collectors

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func descString(t *testing.T, desc *prometheus.Desc) string {
	t.Helper()
	require.NotNil(t, desc)

	return desc.String()
}

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	require.NotNil(t, m)
	assert.Contains(t, descString(t, m.sampleMetric1), `fqName: "gitlab-ci-exporter_sampleMetric1"`)
	assert.Contains(t, descString(t, m.sampleMetric1), `variableLabels: {label1}`)
	assert.Contains(t, descString(t, m.sampleMetric2), `fqName: "gitlab-ci-exporter_sampleMetric2"`)
	assert.Contains(t, descString(t, m.sampleMetric2), `variableLabels: {label2}`)
}

func TestNewExporter(t *testing.T) {
	settings := &Settings{
		LogLevel:    "info",
		LogFormat:   "text",
		MetricsPath: "/metrics",
		ListenPort:  "8080",
		Address:     "0.0.0.0",
	}
	logger := newTestLogger()

	exporter, err := NewExporter(settings, logger)
	require.NoError(t, err)
	require.NotNil(t, exporter)
	require.NotNil(t, exporter.metrics)
	assert.Same(t, settings, exporter.Settings)
	assert.Same(t, logger, exporter.Logger)
}

func TestExporterDescribe(t *testing.T) {
	exporter, err := NewExporter(&Settings{}, newTestLogger())
	require.NoError(t, err)

	ch := make(chan *prometheus.Desc, 2)
	exporter.Describe(ch)
	close(ch)

	var descs []string
	for desc := range ch {
		descs = append(descs, desc.String())
	}

	require.Len(t, descs, 2)
	assert.True(t, strings.Contains(descs[0], "sampleMetric1") || strings.Contains(descs[1], "sampleMetric1"))
	assert.True(t, strings.Contains(descs[0], "sampleMetric2") || strings.Contains(descs[1], "sampleMetric2"))
}

func TestExporterCollect(t *testing.T) {
	exporter, err := NewExporter(&Settings{}, newTestLogger())
	require.NoError(t, err)

	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(exporter))

	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 2)

	familyByName := make(map[string]*dto.MetricFamily, len(families))
	for _, family := range families {
		familyByName[family.GetName()] = family
	}

	sample1 := familyByName["gitlab-ci-exporter_sampleMetric1"]
	require.NotNil(t, sample1)
	require.Len(t, sample1.GetMetric(), 1)
	assert.Equal(t, dto.MetricType_GAUGE, sample1.GetType())
	require.Len(t, sample1.GetMetric()[0].GetLabel(), 1)
	assert.Equal(t, "label1", sample1.GetMetric()[0].GetLabel()[0].GetName())
	assert.Equal(t, "labelValue", sample1.GetMetric()[0].GetLabel()[0].GetValue())
	assert.GreaterOrEqual(t, sample1.GetMetric()[0].GetGauge().GetValue(), float64(0))
	assert.Less(t, sample1.GetMetric()[0].GetGauge().GetValue(), float64(1))

	sample2 := familyByName["gitlab-ci-exporter_sampleMetric2"]
	require.NotNil(t, sample2)
	require.Len(t, sample2.GetMetric(), 1)
	assert.Equal(t, dto.MetricType_GAUGE, sample2.GetType())
	require.Len(t, sample2.GetMetric()[0].GetLabel(), 1)
	assert.Equal(t, "label2", sample2.GetMetric()[0].GetLabel()[0].GetName())
	assert.Equal(t, "labelValue", sample2.GetMetric()[0].GetLabel()[0].GetValue())
	assert.GreaterOrEqual(t, sample2.GetMetric()[0].GetGauge().GetValue(), float64(0))
	assert.Less(t, sample2.GetMetric()[0].GetGauge().GetValue(), float64(1))
}

func TestSampleMetricsReturnValueBetweenZeroAndOne(t *testing.T) {
	exporter, err := NewExporter(&Settings{}, newTestLogger())
	require.NoError(t, err)

	sample1 := exporter.sampleMetric1()
	sample2 := exporter.sampleMetric2()

	assert.GreaterOrEqual(t, sample1, float64(0))
	assert.Less(t, sample1, float64(1))
	assert.GreaterOrEqual(t, sample2, float64(0))
	assert.Less(t, sample2, float64(1))
}
