package controller

import (
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectorDescString(t *testing.T, collector prometheus.Collector) string {
	t.Helper()

	ch := make(chan *prometheus.Desc, 8)
	collector.Describe(ch)
	close(ch)

	descs := make([]string, 0, len(ch))
	for desc := range ch {
		descs = append(descs, desc.String())
	}

	require.NotEmpty(t, descs)

	return strings.Join(descs, "\n")
}

func metricFamilyByName(t *testing.T, families []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()

	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}

	t.Fatalf("metric family %q not found", name)
	return nil
}

func labelNames(metric *dto.Metric) []string {
	names := make([]string, 0, len(metric.GetLabel()))
	for _, label := range metric.GetLabel() {
		names = append(names, label.GetName())
	}

	return names
}

func sampleLabelValues(size int) []string {
	values := make([]string, 0, size)
	for i := 0; i < size; i++ {
		values = append(values, "value")
	}

	return values
}

func registerAndCollectMetricFamily(t *testing.T, name string, collector prometheus.Collector, labelCount int) *dto.MetricFamily {
	t.Helper()

	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(collector))

	switch c := collector.(type) {
	case *prometheus.GaugeVec:
		c.WithLabelValues(sampleLabelValues(labelCount)...).Set(1)
	case *prometheus.CounterVec:
		c.WithLabelValues(sampleLabelValues(labelCount)...).Add(1)
	default:
		t.Fatalf("unsupported collector type %T", collector)
	}

	families, err := registry.Gather()
	require.NoError(t, err)

	return metricFamilyByName(t, families, name)
}

func TestCollectorConstructorsReturnExpectedCollectorTypesAndNames(t *testing.T) {
	tests := []struct {
		name        string
		newCollector func() prometheus.Collector
		metricName  string
		counter     bool
	}{
		{"internal queued", NewInternalCollectorCurrentlyQueuedTasksCount, "gcpe_currently_queued_tasks_count", false},
		{"internal envs", NewInternalCollectorEnvironmentsCount, "gcpe_environments_count", false},
		{"internal runners", NewInternalCollectorRunnersCount, "gcpe_runners_count", false},
		{"internal executed", NewInternalCollectorExecutedTasksCount, "gcpe_executed_tasks_count", false},
		{"internal requests count", NewInternalCollectorGitLabAPIRequestsCount, "gcpe_gitlab_api_requests_count", false},
		{"internal requests remaining", NewInternalCollectorGitLabAPIRequestsRemaining, "gcpe_gitlab_api_requests_remaining", false},
		{"internal requests limit", NewInternalCollectorGitLabAPIRequestsLimit, "gcpe_gitlab_api_requests_limit", false},
		{"internal metrics", NewInternalCollectorMetricsCount, "gcpe_metrics_count", false},
		{"internal projects", NewInternalCollectorProjectsCount, "gcpe_projects_count", false},
		{"internal refs", NewInternalCollectorRefsCount, "gcpe_refs_count", false},
		{"coverage", NewCollectorCoverage, "gitlab_ci_pipeline_coverage", false},
		{"duration", NewCollectorDurationSeconds, "gitlab_ci_pipeline_duration_seconds", false},
		{"queued duration", NewCollectorQueuedDurationSeconds, "gitlab_ci_pipeline_queued_duration_seconds", false},
		{"env behind commits", NewCollectorEnvironmentBehindCommitsCount, "gitlab_ci_environment_behind_commits_count", false},
		{"env behind duration", NewCollectorEnvironmentBehindDurationSeconds, "gitlab_ci_environment_behind_duration_seconds", false},
		{"env deployment count", NewCollectorEnvironmentDeploymentCount, "gitlab_ci_environment_deployment_count", true},
		{"env deployment duration", NewCollectorEnvironmentDeploymentDurationSeconds, "gitlab_ci_environment_deployment_duration_seconds", false},
		{"env deployment job id", NewCollectorEnvironmentDeploymentJobID, "gitlab_ci_environment_deployment_job_id", false},
		{"env deployment status", NewCollectorEnvironmentDeploymentStatus, "gitlab_ci_environment_deployment_status", false},
		{"env deployment timestamp", NewCollectorEnvironmentDeploymentTimestamp, "gitlab_ci_environment_deployment_timestamp", false},
		{"env information", NewCollectorEnvironmentInformation, "gitlab_ci_environment_information", false},
		{"pipeline id", NewCollectorID, "gitlab_ci_pipeline_id", false},
		{"job artifact size", NewCollectorJobArtifactSizeBytes, "gitlab_ci_pipeline_job_artifact_size_bytes", false},
		{"job duration", NewCollectorJobDurationSeconds, "gitlab_ci_pipeline_job_duration_seconds", false},
		{"job id", NewCollectorJobID, "gitlab_ci_pipeline_job_id", false},
		{"job queued duration", NewCollectorJobQueuedDurationSeconds, "gitlab_ci_pipeline_job_queued_duration_seconds", false},
		{"job run count", NewCollectorJobRunCount, "gitlab_ci_pipeline_job_run_count", true},
		{"job status", NewCollectorJobStatus, "gitlab_ci_pipeline_job_status", false},
		{"job timestamp", NewCollectorJobTimestamp, "gitlab_ci_pipeline_job_timestamp", false},
		{"pipeline status", NewCollectorStatus, "gitlab_ci_pipeline_status", false},
		{"pipeline timestamp", NewCollectorTimestamp, "gitlab_ci_pipeline_timestamp", false},
		{"pipeline run count", NewCollectorRunCount, "gitlab_ci_pipeline_run_count", true},
		{"runners info", NewCollectorRunners, "gitlab_ci_runners_info", false},
		{"runner contacted at", NewCollectorRunnerContactedAtSeconds, "gitlab_ci_runner_contacted_at_seconds", false},
		{"runner project", NewCollectorRunnerProjectInfo, "gitlab_ci_runner_project_info", false},
		{"runner tag", NewCollectorRunnerTagInfo, "gitlab_ci_runner_tag_info", false},
		{"runner group", NewCollectorRunnerGroupInfo, "gitlab_ci_runner_group_info", false},
		{"test report total time", NewCollectorTestReportTotalTime, "gitlab_ci_pipeline_test_report_total_time", false},
		{"test report total count", NewCollectorTestReportTotalCount, "gitlab_ci_pipeline_test_report_total_count", false},
		{"test report success count", NewCollectorTestReportSuccessCount, "gitlab_ci_pipeline_test_report_success_count", false},
		{"test report failed count", NewCollectorTestReportFailedCount, "gitlab_ci_pipeline_test_report_failed_count", false},
		{"test report skipped count", NewCollectorTestReportSkippedCount, "gitlab_ci_pipeline_test_report_skipped_count", false},
		{"test report error count", NewCollectorTestReportErrorCount, "gitlab_ci_pipeline_test_report_error_count", false},
		{"test suite total time", NewCollectorTestSuiteTotalTime, "gitlab_ci_pipeline_test_suite_total_time", false},
		{"test suite total count", NewCollectorTestSuiteTotalCount, "gitlab_ci_pipeline_test_suite_total_count", false},
		{"test suite success count", NewCollectorTestSuiteSuccessCount, "gitlab_ci_pipeline_test_suite_success_count", false},
		{"test suite failed count", NewCollectorTestSuiteFailedCount, "gitlab_ci_pipeline_test_suite_failed_count", false},
		{"test suite skipped count", NewCollectorTestSuiteSkippedCount, "gitlab_ci_pipeline_test_suite_skipped_count", false},
		{"test suite error count", NewCollectorTestSuiteErrorCount, "gitlab_ci_pipeline_test_suite_error_count", false},
		{"test case execution time", NewCollectorTestCaseExecutionTime, "gitlab_ci_pipeline_test_case_execution_time", false},
		{"test case status", NewCollectorTestCaseStatus, "gitlab_ci_pipeline_test_case_status", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := tt.newCollector()
			desc := collectorDescString(t, collector)

			assert.Contains(t, desc, `fqName: "`+tt.metricName+`"`)

			if tt.counter {
				_, ok := collector.(*prometheus.CounterVec)
				assert.True(t, ok, "expected CounterVec, got %T", collector)
			} else {
				_, ok := collector.(*prometheus.GaugeVec)
				assert.True(t, ok, "expected GaugeVec, got %T", collector)
			}
		})
	}
}

func TestCollectorConstructorsExposeExpectedLabels(t *testing.T) {
	tests := []struct {
		name         string
		newCollector func() prometheus.Collector
		metricName   string
		expected     []string
	}{
		{
			name:         "coverage labels",
			newCollector: NewCollectorCoverage,
			metricName:   "gitlab_ci_pipeline_coverage",
			expected:     append(append([]string{}, defaultLabels...), pipelineLabels...),
		},
		{
			name:         "environment information labels",
			newCollector: NewCollectorEnvironmentInformation,
			metricName:   "gitlab_ci_environment_information",
			expected:     append(append([]string{}, environmentLabels...), environmentInformationLabels...),
		},
		{
			name:         "job status labels",
			newCollector: NewCollectorJobStatus,
			metricName:   "gitlab_ci_pipeline_job_status",
			expected:     append(append([]string{}, defaultLabels...), jobLabels...),
		},
		{
			name:         "runner group labels",
			newCollector: NewCollectorRunnerGroupInfo,
			metricName:   "gitlab_ci_runner_group_info",
			expected:     append([]string{}, runnerGroupLabels...),
		},
		{
			name:         "test case status labels",
			newCollector: NewCollectorTestCaseStatus,
			metricName:   "gitlab_ci_pipeline_test_case_status",
			expected:     append(append([]string{}, defaultLabels...), append(testSuiteLabels, append(testCaseLabels, statusLabels...)...)...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family := registerAndCollectMetricFamily(t, tt.metricName, tt.newCollector(), len(tt.expected))
			require.Len(t, family.GetMetric(), 1)
			assert.ElementsMatch(t, tt.expected, labelNames(family.GetMetric()[0]))
		})
	}
}
