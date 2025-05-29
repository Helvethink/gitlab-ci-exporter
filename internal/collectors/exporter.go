package collectors

import (
	"log/slog"
	"math/rand/v2"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "gitlab-ci-exporter"
)

type Settings struct {
	LogLevel    string
	LogFormat   string
	MetricsPath string
	ListenPort  string
	Address     string
}

type metrics struct {
	sampleMetric1 *prometheus.Desc
	sampleMetric2 *prometheus.Desc
}

type Exporter struct {
	metrics  *metrics
	Settings *Settings
	Logger   *slog.Logger
}

// Describe Metrics function.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.metrics.sampleMetric1
	ch <- e.metrics.sampleMetric2
}

// Collect metrics configured and returns them as prometheus metrics
// Implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	// Init collector Sample 1.
	ch <- prometheus.MustNewConstMetric(
		e.metrics.sampleMetric1,
		prometheus.GaugeValue,
		e.sampleMetric1(),
		"labelValue",
	)
	// Init collector Sample 2.
	ch <- prometheus.MustNewConstMetric(
		e.metrics.sampleMetric2,
		prometheus.GaugeValue,
		e.sampleMetric2(),
		"labelValue",
	)
}

func (e *Exporter) sampleMetric1() float64 {
	return rand.Float64()
}

func (e *Exporter) sampleMetric2() float64 {
	return rand.Float64()
}

// NewMetrics Initializes the metrics.
func NewMetrics() *metrics {
	return &metrics{
		sampleMetric1: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "sampleMetric1"),
			"Sample metric 1 Description",
			[]string{"label1"}, nil,
		),

		sampleMetric2: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "sampleMetric2"),
			"Sample metric 2 Description",
			[]string{"label2"}, nil,
		),
	}
}

// NewExporter Initialize the exporter.
func NewExporter(settings *Settings, logger *slog.Logger) (*Exporter, error) {
	metrics := NewMetrics()
	exporter := &Exporter{
		metrics:  metrics,
		Settings: settings,
		Logger:   logger,
	}

	return exporter, nil
}
