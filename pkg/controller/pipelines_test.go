package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func TestProcessPipelinesMetricsStoresPipelineMetrics(t *testing.T) {
	ctx := context.Background()

	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines/123"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 123,
			"coverage": "85.5",
			"updated_at": "2024-03-09T16:00:00Z",
			"duration": 12,
			"queued_duration": 3,
			"source": "push",
			"status": "success"
		}`))
	})

	ref := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{ID: 122}

	err := c.ProcessPipelinesMetrics(ctx, ref, &goGitlab.PipelineInfo{ID: 123})
	require.NoError(t, err)

	storedPipeline := schemas.Pipeline{ID: 123}
	require.NoError(t, c.Store.GetPipeline(ctx, &storedPipeline))
	assert.Equal(t, 85.5, storedPipeline.Coverage)
	assert.Equal(t, "success", storedPipeline.Status)
	assert.Equal(t, "push", storedPipeline.Source)

	storedRef := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	require.NoError(t, c.Store.GetRef(ctx, &storedRef))
	assert.Equal(t, int64(123), storedRef.LatestPipeline.ID)

	labels := map[string]string{
		"project":     "group/project",
		"topics":      "",
		"kind":        "branch",
		"ref":         "main",
		"source":      "push",
		"variables":   "",
		"pipeline_id": "123",
		"status":      "success",
	}

	runCountMetric := schemas.Metric{
		Kind:   schemas.MetricKindRunCount,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &runCountMetric))
	assert.Equal(t, float64(1), runCountMetric.Value)

	coverageMetric := schemas.Metric{
		Kind:   schemas.MetricKindCoverage,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &coverageMetric))
	assert.Equal(t, 85.5, coverageMetric.Value)

	durationMetric := schemas.Metric{
		Kind:   schemas.MetricKindDurationSeconds,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &durationMetric))
	assert.Equal(t, float64(12), durationMetric.Value)

	statusMetric := schemas.Metric{
		Kind:   schemas.MetricKindStatus,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &statusMetric))
	assert.Equal(t, float64(1), statusMetric.Value)
}

func TestProcessTestReportMetricsStoresMetrics(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	ref := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{
		ID:     123,
		Source: "push",
	}
	require.NoError(t, c.Store.SetRef(ctx, ref))

	report := schemas.TestReport{
		TotalTime:    12.5,
		TotalCount:   10,
		SuccessCount: 8,
		FailedCount:  1,
		SkippedCount: 1,
		ErrorCount:   0,
	}

	c.ProcessTestReportMetrics(ctx, ref, report)

	labels := map[string]string{
		"project":   "group/project",
		"topics":    "",
		"kind":      "branch",
		"ref":       "main",
		"source":    "push",
		"variables": "",
	}

	totalMetric := schemas.Metric{
		Kind:   schemas.MetricKindTestReportTotalCount,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &totalMetric))
	assert.Equal(t, float64(10), totalMetric.Value)

	timeMetric := schemas.Metric{
		Kind:   schemas.MetricKindTestReportTotalTime,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &timeMetric))
	assert.Equal(t, 12.5, timeMetric.Value)
}

func TestProcessTestSuiteMetricsStoresMetrics(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	ref := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{Source: "push"}
	require.NoError(t, c.Store.SetRef(ctx, ref))

	suite := schemas.TestSuite{
		Name:         "unit",
		TotalTime:    4.2,
		TotalCount:   5,
		SuccessCount: 4,
		FailedCount:  1,
		SkippedCount: 0,
		ErrorCount:   0,
	}

	c.ProcessTestSuiteMetrics(ctx, ref, suite)

	labels := map[string]string{
		"project":         "group/project",
		"topics":          "",
		"kind":            "branch",
		"ref":             "main",
		"source":          "push",
		"variables":       "",
		"test_suite_name": "unit",
	}

	totalMetric := schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteTotalCount,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &totalMetric))
	assert.Equal(t, float64(5), totalMetric.Value)

	timeMetric := schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteTotalTime,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &timeMetric))
	assert.Equal(t, 4.2, timeMetric.Value)
}

func TestProcessTestCaseMetricsStoresMetrics(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	ref := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{Source: "push"}
	require.NoError(t, c.Store.SetRef(ctx, ref))

	suite := schemas.TestSuite{Name: "unit"}
	testCase := schemas.TestCase{
		Name:          "TestCreateUser",
		Classname:     "service.user",
		ExecutionTime: 0.42,
		Status:        "success",
	}

	c.ProcessTestCaseMetrics(ctx, ref, suite, testCase)

	baseLabels := map[string]string{
		"project":             "group/project",
		"topics":              "",
		"kind":                "branch",
		"ref":                 "main",
		"source":              "push",
		"variables":           "",
		"test_suite_name":     "unit",
		"test_case_name":      "TestCreateUser",
		"test_case_classname": "service.user",
	}

	timeMetric := schemas.Metric{
		Kind:   schemas.MetricKindTestCaseExecutionTime,
		Labels: baseLabels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &timeMetric))
	assert.Equal(t, 0.42, timeMetric.Value)

	statusLabels := map[string]string{
		"project":             "group/project",
		"topics":              "",
		"kind":                "branch",
		"ref":                 "main",
		"source":              "push",
		"variables":           "",
		"test_suite_name":     "unit",
		"test_case_name":      "TestCreateUser",
		"test_case_classname": "service.user",
		"status":              "success",
	}
	statusMetric := schemas.Metric{
		Kind:   schemas.MetricKindTestCaseStatus,
		Labels: statusLabels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &statusMetric))
	assert.Equal(t, float64(1), statusMetric.Value)
}
