package httpServer

import (
	"html/template"
	"net/http"

	"github.com/helvethink/gitlab-ci-exporter/internal/collectors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	rootTemplate string = `
	<!DOCTYPE html>
	<head><title>Gitlab CI Exporter</title></head>
	<body>
		<h1>Gitlab CI Exporter</h1>
		<p>Metrics at: <a href='{{ .MetricsPath }}'>{{ .MetricsPath }}</a></p>
		<p>Source: <a href='https://github.com/Helvethink/gitlab-ci-exporter'>github.com/Helvethink/gitlab-ci-exporter</a></p>
	</body>
	</html>`
)

// NewServer Serves root page with html template on root page
// Serves metrics on settings.MetricPath.
func NewServer(e *collectors.Exporter) *http.Server {
	s := e.Settings
	t := template.Must(template.New("root").Parse(rootTemplate))

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		e,
		// versioncollector.NewCollector("exporter"),
		// promCollectors.NewBuildCollector(),
		// promCollectors.NewGoCollector(),
	)

	promHandlerOpts := promhttp.HandlerOpts{
		Registry: reg,
	}

	// Metrics Handler
	http.Handle("/"+s.MetricsPath, promhttp.HandlerFor(reg, promHandlerOpts))

	// Root Page Handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := t.Execute(w, e.Settings)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	return &http.Server{Addr: ":" + s.ListenPort}
}
