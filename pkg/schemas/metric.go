package schemas

import (
	"fmt"
	"hash/crc32" // For calculating CRC32 checksums
	"strconv"    // For string conversion operations

	"github.com/prometheus/client_golang/prometheus" // Prometheus client library for metrics
)

// MetricKind represents different kinds of metrics that can be collected.
type MetricKind int32

const (
	// MetricKindCoverage refers to the coverage metric of a job or pipeline.
	MetricKindCoverage MetricKind = iota

	// MetricKindDurationSeconds refers to the duration of a job or pipeline in seconds.
	MetricKindDurationSeconds

	// MetricKindEnvironmentBehindCommitsCount refers to the number of commits an environment is behind.
	MetricKindEnvironmentBehindCommitsCount

	// MetricKindEnvironmentBehindDurationSeconds refers to the duration in seconds an environment is behind.
	MetricKindEnvironmentBehindDurationSeconds

	// MetricKindEnvironmentDeploymentCount refers to the count of deployments in an environment.
	MetricKindEnvironmentDeploymentCount

	// MetricKindEnvironmentDeploymentDurationSeconds refers to the duration of a deployment in an environment in seconds.
	MetricKindEnvironmentDeploymentDurationSeconds

	// MetricKindEnvironmentDeploymentJobID refers to the job ID of a deployment in an environment.
	MetricKindEnvironmentDeploymentJobID

	// MetricKindEnvironmentDeploymentStatus refers to the status of a deployment in an environment.
	MetricKindEnvironmentDeploymentStatus

	// MetricKindEnvironmentDeploymentTimestamp refers to the timestamp of a deployment in an environment.
	MetricKindEnvironmentDeploymentTimestamp

	// MetricKindEnvironmentInformation refers to general information about an environment.
	MetricKindEnvironmentInformation

	// MetricKindID refers to the ID of a job or pipeline.
	MetricKindID

	// MetricKindJobArtifactSizeBytes refers to the size of job artifacts in bytes.
	MetricKindJobArtifactSizeBytes

	// MetricKindJobDurationSeconds refers to the duration of a job in seconds.
	MetricKindJobDurationSeconds

	// MetricKindJobID refers to the ID of a job.
	MetricKindJobID

	// MetricKindJobQueuedDurationSeconds refers to the queued duration of a job in seconds.
	MetricKindJobQueuedDurationSeconds

	// MetricKindJobRunCount refers to the run count of a job.
	MetricKindJobRunCount

	// MetricKindJobStatus refers to the status of a job.
	MetricKindJobStatus

	// MetricKindJobTimestamp refers to the timestamp of a job.
	MetricKindJobTimestamp

	// MetricKindQueuedDurationSeconds refers to the queued duration of a pipeline in seconds.
	MetricKindQueuedDurationSeconds

	// MetricKindRunCount refers to the run count of a pipeline.
	MetricKindRunCount

	// MetricKindStatus refers to the status of a pipeline.
	MetricKindStatus

	// MetricKindTimestamp refers to the timestamp of a pipeline.
	MetricKindTimestamp

	// MetricKindTestReportTotalTime refers to the total time of a test report in seconds.
	MetricKindTestReportTotalTime

	// MetricKindTestReportTotalCount refers to the total count of tests in a test report.
	MetricKindTestReportTotalCount

	// MetricKindTestReportSuccessCount refers to the count of successful tests in a test report.
	MetricKindTestReportSuccessCount

	// MetricKindTestReportFailedCount refers to the count of failed tests in a test report.
	MetricKindTestReportFailedCount

	// MetricKindTestReportSkippedCount refers to the count of skipped tests in a test report.
	MetricKindTestReportSkippedCount

	// MetricKindTestReportErrorCount refers to the count of tests with errors in a test report.
	MetricKindTestReportErrorCount

	// MetricKindTestSuiteTotalTime refers to the total time of a test suite in seconds.
	MetricKindTestSuiteTotalTime

	// MetricKindTestSuiteTotalCount refers to the total count of tests in a test suite.
	MetricKindTestSuiteTotalCount

	// MetricKindTestSuiteSuccessCount refers to the count of successful tests in a test suite.
	MetricKindTestSuiteSuccessCount

	// MetricKindTestSuiteFailedCount refers to the count of failed tests in a test suite.
	MetricKindTestSuiteFailedCount

	// MetricKindTestSuiteSkippedCount refers to the count of skipped tests in a test suite.
	MetricKindTestSuiteSkippedCount

	// MetricKindTestSuiteErrorCount refers to the count of tests with errors in a test suite.
	MetricKindTestSuiteErrorCount

	// MetricKindTestCaseExecutionTime refers to the execution time of a test case in seconds.
	MetricKindTestCaseExecutionTime

	// MetricKindTestCaseStatus refers to the status of a test case.
	MetricKindTestCaseStatus
)

// Metric represents a metric with a kind, labels, and a value.
type Metric struct {
	Kind   MetricKind        // The kind of metric
	Labels prometheus.Labels // Labels associated with the metric
	Value  float64           // The value of the metric
}

// MetricKey is a custom type used as a key for identifying metrics.
type MetricKey string

// Metrics is a map used to keep track of multiple metrics, with MetricKey as the key.
type Metrics map[MetricKey]Metric

// Key generates a unique key for a Metric based on its kind and labels.
func (m Metric) Key() MetricKey {
	// Start with the metric kind as part of the key
	key := strconv.Itoa(int(m.Kind))

	// Append different label values to the key based on the metric kind
	switch m.Kind {
	case MetricKindCoverage, MetricKindDurationSeconds, MetricKindID, MetricKindQueuedDurationSeconds, MetricKindRunCount, MetricKindStatus, MetricKindTimestamp, MetricKindTestReportTotalCount, MetricKindTestReportErrorCount, MetricKindTestReportFailedCount, MetricKindTestReportSkippedCount, MetricKindTestReportSuccessCount, MetricKindTestReportTotalTime:
		// Append project, kind, ref, and source labels
		key += fmt.Sprintf("%v", []string{
			m.Labels["project"],
			m.Labels["kind"],
			m.Labels["ref"],
			m.Labels["source"],
		})

	case MetricKindJobArtifactSizeBytes, MetricKindJobDurationSeconds, MetricKindJobID, MetricKindJobQueuedDurationSeconds, MetricKindJobRunCount, MetricKindJobStatus, MetricKindJobTimestamp:
		// Append project, kind, ref, stage, tag_list, job_name, job_id, pipeline_id, status and failure_reason labels
		key += fmt.Sprintf("%v", []string{
			m.Labels["project"],
			m.Labels["kind"],
			m.Labels["ref"],
			m.Labels["stage"],
			m.Labels["tag_list"],
			m.Labels["job_name"],
			m.Labels["job_id"],
			m.Labels["pipeline_id"],
			m.Labels["status"],
			m.Labels["failure_reason"],
		})

	case MetricKindEnvironmentBehindCommitsCount, MetricKindEnvironmentBehindDurationSeconds, MetricKindEnvironmentDeploymentCount, MetricKindEnvironmentDeploymentDurationSeconds, MetricKindEnvironmentDeploymentJobID, MetricKindEnvironmentDeploymentStatus, MetricKindEnvironmentDeploymentTimestamp, MetricKindEnvironmentInformation:
		// Append project and environment labels
		key += fmt.Sprintf("%v", []string{
			m.Labels["project"],
			m.Labels["environment"],
		})

	case MetricKindTestSuiteErrorCount, MetricKindTestSuiteFailedCount, MetricKindTestSuiteSkippedCount, MetricKindTestSuiteSuccessCount, MetricKindTestSuiteTotalCount, MetricKindTestSuiteTotalTime:
		// Append project, kind, ref, and test_suite_name labels
		key += fmt.Sprintf("%v", []string{
			m.Labels["project"],
			m.Labels["kind"],
			m.Labels["ref"],
			m.Labels["test_suite_name"],
		})

	case MetricKindTestCaseExecutionTime, MetricKindTestCaseStatus:
		// Append project, kind, ref, test_suite_name, test_case_name, and test_case_classname labels
		key += fmt.Sprintf("%v", []string{
			m.Labels["project"],
			m.Labels["kind"],
			m.Labels["ref"],
			m.Labels["test_suite_name"],
			m.Labels["test_case_name"],
			m.Labels["test_case_classname"],
		})
	}

	// If the metric is a "status" one, add the status label
	switch m.Kind {
	case MetricKindJobStatus, MetricKindEnvironmentDeploymentStatus, MetricKindStatus, MetricKindTestCaseStatus:
		key += m.Labels["status"]
	}

	// Generate a unique key using the CRC32 checksum of the constructed key string
	return MetricKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(key)))))
}
