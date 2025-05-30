package config

import (
	"net/url"
)

// Global contains configuration settings that are shared across the entire exporter.
// It includes options that apply globally rather than to specific components.
type Global struct {
	// InternalMonitoringListenerAddress specifies the URL endpoint where internal
	// metrics and monitoring data of the exporter itself can be accessed.
	InternalMonitoringListenerAddress *url.URL
}
