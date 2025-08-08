package controller

import (
	"context"
	log "github.com/sirupsen/logrus"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

// metricLogFields prepares structured log fields for a given metric.
// It includes the metric kind and its labels for better log context.
func metricLogFields(m schemas.Metric) log.Fields {
	return log.Fields{
		"metric-kind":   m.Kind,
		"metric-labels": m.Labels,
	}
}

// storeGetMetric retrieves a metric from the store.
// If an error occurs, it logs the error with context including metric details.
func storeGetMetric(ctx context.Context, s store.Store, m *schemas.Metric) {
	if err := s.GetMetric(ctx, m); err != nil {
		log.WithContext(ctx).
			WithFields(metricLogFields(*m)).
			WithError(err).
			Errorf("reading metric from the store")
	}
}

// storeSetMetric saves or updates a metric in the store.
// Logs any error encountered during the operation with metric context.
func storeSetMetric(ctx context.Context, s store.Store, m schemas.Metric) {
	if err := s.SetMetric(ctx, m); err != nil {
		log.WithContext(ctx).
			WithFields(metricLogFields(m)).
			WithError(err).
			Errorf("writing metric from the store")
	}
}

// storeDelMetric deletes a metric from the store by its key.
// Logs errors if the deletion fails, including metric context in logs.
func storeDelMetric(ctx context.Context, s store.Store, m schemas.Metric) {
	if err := s.DelMetric(ctx, m.Key()); err != nil {
		log.WithContext(ctx).
			WithFields(metricLogFields(m)).
			WithError(err).
			Errorf("deleting metric from the store")
	}
}
