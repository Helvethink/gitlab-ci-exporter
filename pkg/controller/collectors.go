package controller

import "github.com/prometheus/client_golang/prometheus"

// Commonly used label sets for Prometheus metrics in this application.
// These labels help categorize and filter metrics for better observability.

var (
	// defaultLabels contains generic labels applicable to most metrics related to projects and refs.
	defaultLabels = []string{"project", "topics", "kind", "ref", "source", "variables"}

	// jobLabels contains labels specific to CI job runs, describing stages, job names, runner info, etc.
	jobLabels = []string{"stage", "job_name", "runner_description", "tag_list", "failure_reason"}

	// statusLabels contains labels related to job or environment statuses.
	statusLabels = []string{"status"}

	// environmentLabels defines labels for metrics related to deployment environments.
	environmentLabels = []string{"project", "environment"}

	// environmentInformationLabels contains more detailed labels providing metadata about environments.
	environmentInformationLabels = []string{
		"environment_id", "external_url", "kind", "ref",
		"latest_commit_short_id", "current_commit_short_id",
		"available", "username",
	}

	// testSuiteLabels holds labels for test suite metrics.
	testSuiteLabels = []string{"test_suite_name"}

	// testCaseLabels holds labels for individual test case metrics.
	testCaseLabels = []string{"test_case_name", "test_case_classname"}

	// statusesList defines all possible status values used throughout the application,
	// for example, job or environment deployment statuses.
	statusesList = [...]string{
		"created", "waiting_for_resource", "preparing", "pending", "running",
		"success", "failed", "canceled", "skipped", "manual", "scheduled", "error",
	}
)

// NewInternalCollectorCurrentlyQueuedTasksCount creates and returns a new Prometheus GaugeVec metric collector
// for tracking the number of currently queued tasks in the system.
//
// This metric has no labels (empty label slice), representing a simple gauge value.
// It can be used internally to monitor the task queue size.
func NewInternalCollectorCurrentlyQueuedTasksCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_currently_queued_tasks_count",
			Help: "Number of tasks in the queue",
		},
		[]string{}, // no labels for this metric
	)
}

// NewInternalCollectorEnvironmentsCount returns a new Prometheus gauge collector
// for the metric "gcpe_environments_count" which tracks the total number of GitLab environments
// currently being exported by the application.
//
// This metric has no labels, representing a simple numeric gauge.
func NewInternalCollectorEnvironmentsCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_environments_count",
			Help: "Number of GitLab environments being exported",
		},
		[]string{}, // no labels for this metric
	)
}

// NewInternalCollectorExecutedTasksCount returns a new Prometheus gauge collector
// for the metric "gcpe_executed_tasks_count" which tracks the total number of tasks
// that have been executed by the system.
//
// This metric is a gauge without labels.
func NewInternalCollectorExecutedTasksCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_executed_tasks_count",
			Help: "Number of tasks executed",
		},
		[]string{}, // no labels
	)
}

// NewInternalCollectorGitLabAPIRequestsCount returns a new Prometheus gauge collector
// for the metric "gcpe_gitlab_api_requests_count" which monitors the total number of
// GitLab API requests made by the application.
//
// This metric is useful to observe API usage and detect potential throttling issues.
//
// This gauge has no labels.
func NewInternalCollectorGitLabAPIRequestsCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_gitlab_api_requests_count",
			Help: "GitLab API requests count",
		},
		[]string{}, // no labels
	)
}

// NewInternalCollectorGitLabAPIRequestsRemaining returns a new Prometheus gauge collector
// for the metric "gcpe_gitlab_api_requests_remaining" which tracks the number of remaining
// GitLab API requests allowed within the current rate limit window.
//
// This metric helps monitor how close the application is to hitting the GitLab API rate limit.
//
// The metric has no labels.
func NewInternalCollectorGitLabAPIRequestsRemaining() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_gitlab_api_requests_remaining",
			Help: "GitLab API requests remaining in the api limit",
		},
		[]string{}, // no labels
	)
}

// NewInternalCollectorGitLabAPIRequestsLimit returns a new Prometheus gauge collector
// for the metric "gcpe_gitlab_api_requests_limit" which tracks the total number of
// GitLab API requests allowed per rate limit window.
//
// This metric provides context to the remaining requests metric and helps understand the API quota.
//
// The metric has no labels.
func NewInternalCollectorGitLabAPIRequestsLimit() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_gitlab_api_requests_limit",
			Help: "GitLab API requests available in the api limit",
		},
		[]string{}, // no labels
	)
}

// NewInternalCollectorMetricsCount returns a new Prometheus gauge collector
// for the metric "gcpe_metrics_count" which counts the number of GitLab pipeline metrics
// currently being exported by the application.
//
// This metric can be used to monitor the volume of pipeline metrics managed and exposed.
//
// The metric has no labels.
func NewInternalCollectorMetricsCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_metrics_count",
			Help: "Number of GitLab pipelines metrics being exported",
		},
		[]string{}, // no labels
	)
}

// NewInternalCollectorProjectsCount returns a new Prometheus gauge collector
// for the metric "gcpe_projects_count" which tracks the number of GitLab projects
// currently being exported by the application.
//
// This metric helps monitor how many projects the exporter is handling.
//
// The metric has no labels.
func NewInternalCollectorProjectsCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_projects_count",
			Help: "Number of GitLab projects being exported",
		},
		[]string{}, // no labels
	)
}

// NewInternalCollectorRefsCount returns a new Prometheus gauge collector
// for the metric "gcpe_refs_count" which tracks the number of GitLab refs
// (branches, tags, etc.) currently being exported.
//
// This metric is useful to understand the scope of ref data being monitored.
//
// The metric has no labels.
func NewInternalCollectorRefsCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gcpe_refs_count",
			Help: "Number of GitLab refs being exported",
		},
		[]string{}, // no labels
	)
}

// NewCollectorCoverage returns a new Prometheus gauge collector for the metric
// "gitlab_ci_pipeline_coverage" which represents the test coverage percentage
// of the most recent GitLab CI pipeline.
//
// This metric uses a set of default labels (such as project, ref, kind, etc.)
// to provide detailed context about the pipeline.
//
// It helps monitor code quality via pipeline coverage over time.
func NewCollectorCoverage() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_coverage",
			Help: "Coverage of the most recent pipeline",
		},
		defaultLabels, // use predefined default labels for context
	)
}

// NewCollectorDurationSeconds returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_duration_seconds". This metric measures the duration
// (in seconds) of the most recent GitLab CI pipeline run.
//
// The metric includes default labels to provide contextual information about the pipeline,
// such as project, ref, kind, etc.
func NewCollectorDurationSeconds() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_duration_seconds",
			Help: "Duration in seconds of the most recent pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorQueuedDurationSeconds returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_queued_duration_seconds". This metric tracks how long
// (in seconds) the most recent pipeline was queued before starting execution.
//
// It uses the same default labels as other pipeline metrics for consistent context.
func NewCollectorQueuedDurationSeconds() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_queued_duration_seconds",
			Help: "Duration in seconds the most recent pipeline has been queued before starting",
		},
		defaultLabels,
	)
}

// NewCollectorEnvironmentBehindCommitsCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_environment_behind_commits_count". This metric measures the number of commits
// that the environment is behind based on its last deployment.
//
// This metric uses environment-specific labels (such as project and environment)
// to properly attribute the measurement.
func NewCollectorEnvironmentBehindCommitsCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_environment_behind_commits_count",
			Help: "Number of commits the environment is behind given its last deployment",
		},
		environmentLabels,
	)
}

// NewCollectorEnvironmentBehindDurationSeconds returns a new Prometheus gauge collector for the
// metric "gitlab_ci_environment_behind_duration_seconds". This metric measures the duration in seconds
// that an environment is behind the most recent commit, based on its last deployment.
//
// The collector uses environment-related labels such as project and environment to provide context.
func NewCollectorEnvironmentBehindDurationSeconds() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_environment_behind_duration_seconds",
			Help: "Duration in seconds the environment is behind the most recent commit given its last deployment",
		},
		environmentLabels,
	)
}

// NewCollectorEnvironmentDeploymentCount returns a new Prometheus counter collector for the
// metric "gitlab_ci_environment_deployment_count". This metric counts the total number of
// deployments for a given environment.
//
// It uses environment labels to associate deployment counts with specific projects/environments.
func NewCollectorEnvironmentDeploymentCount() prometheus.Collector {
	return prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gitlab_ci_environment_deployment_count",
			Help: "Number of deployments for an environment",
		},
		environmentLabels,
	)
}

// NewCollectorEnvironmentDeploymentDurationSeconds returns a new Prometheus gauge collector for the
// metric "gitlab_ci_environment_deployment_duration_seconds". This metric tracks the duration (in seconds)
// of the most recent deployment of a specific environment.
//
// This collector also uses environment labels to provide detailed context.
func NewCollectorEnvironmentDeploymentDurationSeconds() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_environment_deployment_duration_seconds",
			Help: "Duration in seconds of the most recent deployment of the environment",
		},
		environmentLabels,
	)
}

// NewCollectorEnvironmentDeploymentJobID returns a new Prometheus gauge collector for the
// metric "gitlab_ci_environment_deployment_job_id". This metric stores the ID of the most recent
// deployment job associated with an environment.
//
// It uses environment labels to contextualize the job ID per environment.
func NewCollectorEnvironmentDeploymentJobID() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_environment_deployment_job_id",
			Help: "ID of the most recent deployment job of the environment",
		},
		environmentLabels,
	)
}

// NewCollectorEnvironmentDeploymentStatus returns a new Prometheus gauge collector for the
// metric "gitlab_ci_environment_deployment_status". This metric represents the status of the most
// recent deployment for a given environment.
//
// The collector adds an extra label "status" to the standard environment labels, enabling
// tracking the deployment status alongside environment details.
func NewCollectorEnvironmentDeploymentStatus() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_environment_deployment_status",
			Help: "Status of the most recent deployment of the environment",
		},
		append(environmentLabels, "status"),
	)
}

// NewCollectorEnvironmentDeploymentTimestamp returns a new Prometheus gauge collector for the
// metric "gitlab_ci_environment_deployment_timestamp". This metric stores the creation timestamp
// of the most recent deployment of the environment.
//
// It uses environment labels to provide the context of when each environment was last deployed.
func NewCollectorEnvironmentDeploymentTimestamp() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_environment_deployment_timestamp",
			Help: "Creation date of the most recent deployment of the environment",
		},
		environmentLabels,
	)
}

// NewCollectorEnvironmentInformation returns a new Prometheus gauge collector for the
// metric "gitlab_ci_environment_information". This metric provides detailed static
// information about the environment.
//
// The collector uses a combination of basic environment labels and additional
// environment information labels to give context such as environment ID, external URL,
// kind, ref, commit IDs, availability, and username.
func NewCollectorEnvironmentInformation() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_environment_information",
			Help: "Information about the environment",
		},
		append(environmentLabels, environmentInformationLabels...),
	)
}

// NewCollectorID returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_id". This metric tracks the ID of the most recent
// GitLab pipeline.
//
// It uses default labels to provide contextual metadata such as project, topics, kind, ref, source, and variables.
func NewCollectorID() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_id",
			Help: "ID of the most recent pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorJobArtifactSizeBytes returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_job_artifact_size_bytes". This metric measures the
// total artifact size in bytes for the most recent job.
//
// It uses both the default labels and job-specific labels to provide detailed context
// such as stage, job name, runner description, tags, and failure reasons.
func NewCollectorJobArtifactSizeBytes() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_job_artifact_size_bytes",
			Help: "Artifact size in bytes (sum of all of them) of the most recent job",
		},
		append(defaultLabels, jobLabels...),
	)
}

// NewCollectorJobDurationSeconds returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_job_duration_seconds". This metric tracks the duration
// in seconds of the most recent GitLab CI job.
//
// The collector uses a combination of default labels (project, ref, etc.) and job-specific
// labels (stage, job name, runner, etc.) to provide detailed context for each job's duration.
func NewCollectorJobDurationSeconds() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_job_duration_seconds",
			Help: "Duration in seconds of the most recent job",
		},
		append(defaultLabels, jobLabels...),
	)
}

// NewCollectorJobID returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_job_id". This metric tracks the ID of the most recent
// job executed in GitLab CI pipelines.
//
// It uses both the default and job-specific labels to provide detailed metadata for
// each job, helping uniquely identify jobs within projects and stages.
func NewCollectorJobID() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_job_id",
			Help: "ID of the most recent job",
		},
		append(defaultLabels, jobLabels...),
	)
}

// NewCollectorJobQueuedDurationSeconds returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_job_queued_duration_seconds". This metric measures the
// time in seconds that the most recent job was queued before it started running.
//
// This collector also uses default and job-specific labels to add context on project,
// job stage, runner, etc., allowing fine-grained monitoring of job queue delays.
func NewCollectorJobQueuedDurationSeconds() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_job_queued_duration_seconds",
			Help: "Duration in seconds the most recent job has been queued before starting",
		},
		append(defaultLabels, jobLabels...),
	)
}

// NewCollectorJobRunCount returns a new Prometheus counter collector for the
// metric "gitlab_ci_pipeline_job_run_count". This metric counts the total number of
// times a specific job has been executed.
//
// It uses a combination of default labels (e.g., project, ref) and job-specific labels
// (e.g., stage, job_name, runner_description) to provide detailed context for counting job runs.
func NewCollectorJobRunCount() prometheus.Collector {
	return prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gitlab_ci_pipeline_job_run_count",
			Help: "Number of executions of a job",
		},
		append(defaultLabels, jobLabels...),
	)
}

// NewCollectorJobStatus returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_job_status". This metric reports the status of the
// most recent job execution.
//
// It uses default labels combined with job-specific labels and also includes status labels
// (such as success, failed, running) to reflect the current state of each job.
func NewCollectorJobStatus() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_job_status",
			Help: "Status of the most recent job",
		},
		append(defaultLabels, append(jobLabels, statusLabels...)...),
	)
}

// NewCollectorJobTimestamp returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_job_timestamp". This metric records the creation timestamp
// of the most recent job.
//
// Like others, it uses both default and job-specific labels to provide detailed
// metadata for each job's timestamp.
func NewCollectorJobTimestamp() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_job_timestamp",
			Help: "Creation date timestamp of the most recent job",
		},
		append(defaultLabels, jobLabels...),
	)
}

// NewCollectorStatus returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_status". This metric reports the status of the
// most recent pipeline.
//
// The labels include the default set of labels plus an additional "status" label
// that describes the pipeline status (e.g., success, failed, running).
func NewCollectorStatus() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_status",
			Help: "Status of the most recent pipeline",
		},
		append(defaultLabels, "status"),
	)
}

// NewCollectorTimestamp returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_timestamp". This metric stores the timestamp
// of the last update (e.g., last run or modification) of the most recent pipeline.
//
// It uses the default labels to provide context on which pipeline this timestamp belongs to.
func NewCollectorTimestamp() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_timestamp",
			Help: "Timestamp of the last update of the most recent pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorRunCount returns a new Prometheus counter collector for the
// metric "gitlab_ci_pipeline_run_count". This metric counts how many times a
// particular pipeline has been executed.
//
// It uses the default labels to differentiate counts per pipeline.
func NewCollectorRunCount() prometheus.Collector {
	return prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gitlab_ci_pipeline_run_count",
			Help: "Number of executions of a pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorTestReportTotalTime returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_report_total_time". This metric tracks the total
// duration, in seconds, of all tests executed in the most recently finished pipeline.
//
// It uses the defaultLabels to associate the metric with pipeline-specific metadata,
// such as project, ref, and job information.
func NewCollectorTestReportTotalTime() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_report_total_time",
			Help: "Duration in seconds of all the tests in the most recently finished pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorTestReportTotalCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_report_total_count". This metric records the total
// number of tests run in the most recently finished pipeline.
//
// It also uses the defaultLabels to provide detailed context for the metric.
func NewCollectorTestReportTotalCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_report_total_count",
			Help: "Number of total tests in the most recently finished pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorTestReportSuccessCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_report_success_count". This metric tracks the number
// of successful tests in the most recently finished pipeline.
//
// The defaultLabels provide pipeline-related metadata to label the metric.
func NewCollectorTestReportSuccessCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_report_success_count",
			Help: "Number of successful tests in the most recently finished pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorTestReportFailedCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_report_failed_count". This metric counts the number
// of failed tests in the most recently finished pipeline.
//
// It uses the defaultLabels to associate the metric with relevant pipeline context,
// such as project, ref, and job details.
func NewCollectorTestReportFailedCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_report_failed_count",
			Help: "Number of failed tests in the most recently finished pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorTestReportSkippedCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_report_skipped_count". This metric records the number
// of tests that were skipped in the most recently finished pipeline.
//
// The defaultLabels provide pipeline metadata for labeling this metric.
func NewCollectorTestReportSkippedCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_report_skipped_count",
			Help: "Number of skipped tests in the most recently finished pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorTestReportErrorCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_report_error_count". This metric tracks the number
// of tests that errored (i.e., had runtime errors) in the most recently finished pipeline.
//
// It uses the defaultLabels to provide detailed context about the pipeline.
func NewCollectorTestReportErrorCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_report_error_count",
			Help: "Number of errored tests in the most recently finished pipeline",
		},
		defaultLabels,
	)
}

// NewCollectorTestSuiteTotalTime returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_suite_total_time". This metric records the total duration
// in seconds of the entire test suite execution.
//
// It uses defaultLabels combined with testSuiteLabels to provide detailed context
// about the pipeline and specific test suite.
func NewCollectorTestSuiteTotalTime() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_suite_total_time",
			Help: "Duration in seconds for the test suite",
		},
		append(defaultLabels, testSuiteLabels...),
	)
}

// NewCollectorTestSuiteTotalCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_suite_total_count". This metric tracks the total
// number of tests within a test suite.
//
// The metric is labeled using defaultLabels and testSuiteLabels to capture pipeline and
// test suite information.
func NewCollectorTestSuiteTotalCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_suite_total_count",
			Help: "Number of total tests for the test suite",
		},
		append(defaultLabels, testSuiteLabels...),
	)
}

// NewCollectorTestSuiteSuccessCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_suite_success_count". This metric counts the number
// of successful tests within the test suite.
//
// Labels include defaultLabels and testSuiteLabels for pipeline and test suite context.
func NewCollectorTestSuiteSuccessCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_suite_success_count",
			Help: "Number of successful tests for the test suite",
		},
		append(defaultLabels, testSuiteLabels...),
	)
}

// NewCollectorTestSuiteFailedCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_suite_failed_count". This metric tracks the number
// of failed tests within a test suite.
//
// The collector is labeled with defaultLabels and testSuiteLabels to provide
// pipeline and test suite context.
func NewCollectorTestSuiteFailedCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_suite_failed_count",
			Help: "Number of failed tests for the test suite",
		},
		append(defaultLabels, testSuiteLabels...),
	)
}

// NewCollectorTestSuiteSkippedCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_suite_skipped_count". This metric tracks the number
// of skipped tests within a test suite.
//
// Labels include defaultLabels and testSuiteLabels to capture pipeline and test suite details.
func NewCollectorTestSuiteSkippedCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_suite_skipped_count",
			Help: "Number of skipped tests for the test suite",
		},
		append(defaultLabels, testSuiteLabels...),
	)
}

// NewCollectorTestSuiteErrorCount returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_suite_error_count". This metric tracks the number
// of errored tests within a test suite.
//
// The collector uses defaultLabels and testSuiteLabels to provide necessary context.
func NewCollectorTestSuiteErrorCount() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_suite_error_count",
			Help: "Number of errors for the test suite",
		},
		append(defaultLabels, testSuiteLabels...),
	)
}

// NewCollectorTestCaseExecutionTime returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_case_execution_time". This metric measures the
// execution duration in seconds for each individual test case.
//
// The collector is labeled with defaultLabels, testSuiteLabels, and testCaseLabels
// to provide full context about the pipeline, test suite, and specific test case.
func NewCollectorTestCaseExecutionTime() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_case_execution_time",
			Help: "Duration in seconds for the test case",
		},
		append(defaultLabels, append(testSuiteLabels, testCaseLabels...)...),
	)
}

// NewCollectorTestCaseStatus returns a new Prometheus gauge collector for the
// metric "gitlab_ci_pipeline_test_case_status". This metric tracks the status of
// each test case in the most recent job.
//
// Labels include defaultLabels, testSuiteLabels, testCaseLabels, and statusLabels,
// to provide detailed information about pipeline, suite, test case, and test result status.
func NewCollectorTestCaseStatus() prometheus.Collector {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gitlab_ci_pipeline_test_case_status",
			Help: "Status of the test case in most recent job",
		},
		append(defaultLabels, append(testSuiteLabels, append(testCaseLabels, statusLabels...)...)...),
	)
}
