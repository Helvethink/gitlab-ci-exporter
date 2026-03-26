package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

type metricStoreStub struct {
	store.Store
	getMetricErr error
	setMetricErr error
	delMetricErr error
}

func (s metricStoreStub) GetMetric(ctx context.Context, m *schemas.Metric) error {
	if s.getMetricErr != nil {
		return s.getMetricErr
	}

	return s.Store.GetMetric(ctx, m)
}

func (s metricStoreStub) SetMetric(ctx context.Context, m schemas.Metric) error {
	if s.setMetricErr != nil {
		return s.setMetricErr
	}

	return s.Store.SetMetric(ctx, m)
}

func (s metricStoreStub) DelMetric(ctx context.Context, k schemas.MetricKey) error {
	if s.delMetricErr != nil {
		return s.delMetricErr
	}

	return s.Store.DelMetric(ctx, k)
}

func TestMetricLogFields(t *testing.T) {
	m := schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: map[string]string{
			"project": "group/project",
			"ref":     "main",
		},
	}

	fields := metricLogFields(m)

	assert.Equal(t, m.Kind, fields["metric-kind"])
	assert.Equal(t, m.Labels, fields["metric-labels"])
}

func TestStoreSetMetricStoresMetric(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	m := schemas.Metric{
		Kind: schemas.MetricKindCoverage,
		Labels: map[string]string{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 98.5,
	}

	storeSetMetric(ctx, s, m)

	var got schemas.Metric
	got.Kind = m.Kind
	got.Labels = m.Labels

	require.NoError(t, s.GetMetric(ctx, &got))
	assert.Equal(t, m.Value, got.Value)
}

func TestStoreGetMetricLoadsMetric(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	want := schemas.Metric{
		Kind: schemas.MetricKindRunner,
		Labels: map[string]string{
			"runner_id": "42",
		},
		Value: 1,
	}
	require.NoError(t, s.SetMetric(ctx, want))

	got := schemas.Metric{
		Kind: want.Kind,
		Labels: map[string]string{
			"runner_id": "42",
		},
	}

	storeGetMetric(ctx, s, &got)

	assert.Equal(t, want, got)
}

func TestStoreDelMetricDeletesMetric(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	m := schemas.Metric{
		Kind: schemas.MetricKindRunnerTagInfo,
		Labels: map[string]string{
			"runner_id": "42",
			"tag":       "docker",
		},
		Value: 1,
	}
	require.NoError(t, s.SetMetric(ctx, m))

	storeDelMetric(ctx, s, m)

	exists, err := s.MetricExists(ctx, m.Key())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestStoreMetricHelpersIgnoreStoreErrors(t *testing.T) {
	ctx := context.Background()
	s := metricStoreStub{
		Store:        store.NewLocalStore(),
		getMetricErr: errors.New("get failed"),
		setMetricErr: errors.New("set failed"),
		delMetricErr: errors.New("del failed"),
	}
	m := schemas.Metric{
		Kind: schemas.MetricKindRunner,
		Labels: map[string]string{
			"runner_id": "42",
		},
	}

	assert.NotPanics(t, func() {
		storeSetMetric(ctx, s, m)
		storeGetMetric(ctx, s, &m)
		storeDelMetric(ctx, s, m)
	})
}
