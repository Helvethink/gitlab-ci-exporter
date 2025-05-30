package controller

import (
	"context"
	"fmt"
	"reflect"

	log "github.com/sirupsen/logrus"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/exp/slices"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// PullRefMetrics fetches and updates metrics related to a specific GitLab ref (branch, tag, or merge request).
// It retrieves the latest pipeline information, updates stored metrics, and optionally pulls job and test report metrics.
func (c *Controller) PullRefMetrics(ctx context.Context, ref schemas.Ref) error {
	// List of pipeline statuses considered "finished" for fetching test reports
	finishedStatusesList := []string{
		"success",
		"failed",
		"skipped",
		"cancelled",
	}

	// Refresh the ref from the store to avoid stale data in case of concurrent updates
	if err := c.Store.GetRef(ctx, &ref); err != nil {
		return err
	}

	// Prepare log fields for consistent logging related to this ref
	logFields := log.Fields{
		"project-name": ref.Project.Name,
		"ref":          ref.Name,
		"ref-kind":     ref.Kind,
	}

	// Adjust ref name if the ref is a merge request to use the correct Git ref syntax
	var refName string
	if ref.Kind == schemas.RefKindMergeRequest {
		refName = fmt.Sprintf("refs/merge-requests/%s/head", ref.Name)
	} else {
		refName = ref.Name
	}

	// Fetch the most recent pipeline for the ref from GitLab API (only one pipeline needed)
	pipelines, _, err := c.Gitlab.GetProjectPipelines(ctx, ref.Project.Name, &goGitlab.ListProjectPipelinesOptions{
		ListOptions: goGitlab.ListOptions{
			PerPage: 1,
			Page:    1,
		},
		Ref: &refName,
	})
	if err != nil {
		return fmt.Errorf("error fetching project pipelines for %s: %v", ref.Project.Name, err)
	}

	// If no pipelines found, log and exit early
	if len(pipelines) == 0 {
		log.WithFields(logFields).Debug("could not find any pipeline for the ref")
		return nil
	}

	// Fetch detailed pipeline information for the latest pipeline
	pipeline, err := c.Gitlab.GetRefPipeline(ctx, ref, pipelines[0].ID)
	if err != nil {
		return err
	}

	// If the stored latest pipeline is different or does not exist, update metrics
	if ref.LatestPipeline.ID == 0 || !reflect.DeepEqual(pipeline, ref.LatestPipeline) {
		formerPipeline := ref.LatestPipeline
		ref.LatestPipeline = pipeline

		// Optionally fetch pipeline variables if enabled in project config
		if ref.Project.Pull.Pipeline.Variables.Enabled {
			ref.LatestPipeline.Variables, err = c.Gitlab.GetRefPipelineVariablesAsConcatenatedString(ctx, ref)
			if err != nil {
				return err
			}
		}

		// Update the ref in the store with the new pipeline data
		if err = c.Store.SetRef(ctx, ref); err != nil {
			return err
		}

		// Prepare default labels for metrics based on the ref info
		labels := ref.DefaultLabelsValues()

		// Initialize run count metric with the current value from the store (if any)
		runCount := schemas.Metric{
			Kind:   schemas.MetricKindRunCount,
			Labels: labels,
		}
		storeGetMetric(ctx, c.Store, &runCount)

		// Increment run count only if this is a new pipeline different from the previous one
		if formerPipeline.ID != 0 && formerPipeline.ID != ref.LatestPipeline.ID {
			runCount.Value++
		}
		// Store the updated run count metric
		storeSetMetric(ctx, c.Store, runCount)

		// Store other key metrics for the pipeline: coverage, ID, status, duration, queue time, and timestamp
		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind:   schemas.MetricKindCoverage,
			Labels: labels,
			Value:  pipeline.Coverage,
		})

		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind:   schemas.MetricKindID,
			Labels: labels,
			Value:  float64(pipeline.ID),
		})

		emitStatusMetric(
			ctx,
			c.Store,
			schemas.MetricKindStatus,
			labels,
			statusesList[:], // List of valid statuses for metrics emission
			pipeline.Status,
			ref.Project.OutputSparseStatusMetrics,
		)

		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind:   schemas.MetricKindDurationSeconds,
			Labels: labels,
			Value:  pipeline.DurationSeconds,
		})

		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind:   schemas.MetricKindQueuedDurationSeconds,
			Labels: labels,
			Value:  pipeline.QueuedDurationSeconds,
		})

		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind:   schemas.MetricKindTimestamp,
			Labels: labels,
			Value:  pipeline.Timestamp,
		})

		// If job metrics collection is enabled, pull metrics for all jobs in the pipeline
		if ref.Project.Pull.Pipeline.Jobs.Enabled {
			if err := c.PullRefPipelineJobsMetrics(ctx, ref); err != nil {
				return err
			}
		}
	} else {
		// If the latest pipeline hasn't changed, still update the most recent jobs metrics
		if err := c.PullRefMostRecentJobsMetrics(ctx, ref); err != nil {
			return err
		}
	}

	// If test report metrics are enabled and pipeline status is finished,
	// fetch and process test reports and test cases metrics
	if ref.Project.Pull.Pipeline.TestReports.Enabled && slices.Contains(finishedStatusesList, ref.LatestPipeline.Status) {
		ref.LatestPipeline.TestReport, err = c.Gitlab.GetRefPipelineTestReport(ctx, ref)
		if err != nil {
			return err
		}

		c.ProcessTestReportMetrics(ctx, ref, ref.LatestPipeline.TestReport)

		for _, ts := range ref.LatestPipeline.TestReport.TestSuites {
			c.ProcessTestSuiteMetrics(ctx, ref, ts)
			// If test cases metrics are enabled, process each test case within the suite
			if ref.Project.Pull.Pipeline.TestReports.TestCases.Enabled {
				for _, tc := range ts.TestCases {
					c.ProcessTestCaseMetrics(ctx, ref, ts, tc)
				}
			}
		}
	}

	return nil
}

// ProcessTestReportMetrics processes and stores metrics extracted from a test report
// related to a specific GitLab ref (branch, tag, or merge request).
// It updates the metrics store with various counts and timing information from the test report.
func (c *Controller) ProcessTestReportMetrics(ctx context.Context, ref schemas.Ref, tr schemas.TestReport) {
	// Prepare consistent log fields identifying the project and ref for logging
	testReportLogFields := log.Fields{
		"project-name": ref.Project.Name,
		"ref":          ref.Name,
	}

	// Retrieve default labels from the ref for metrics (e.g., project name, ref name, kind, etc.)
	labels := ref.DefaultLabelsValues()

	// Refresh the ref's current state from the store to ensure up-to-date data
	if err := c.Store.GetRef(ctx, &ref); err != nil {
		// Log error if unable to retrieve ref from store and exit early
		log.WithContext(ctx).
			WithFields(testReportLogFields).
			WithError(err).
			Error("getting ref from the store")
		return
	}

	// Log a trace-level message indicating that test report metrics processing has started
	log.WithFields(testReportLogFields).Trace("processing test report metrics")

	// Store various test report metrics in the metrics store:
	// - Number of errors encountered during tests
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestReportErrorCount,
		Labels: labels,
		Value:  float64(tr.ErrorCount),
	})

	// - Number of failed tests
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestReportFailedCount,
		Labels: labels,
		Value:  float64(tr.FailedCount),
	})

	// - Number of skipped tests
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestReportSkippedCount,
		Labels: labels,
		Value:  float64(tr.SkippedCount),
	})

	// - Number of successful tests
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestReportSuccessCount,
		Labels: labels,
		Value:  float64(tr.SuccessCount),
	})

	// - Total number of tests executed (sum of success, failed, skipped, etc.)
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestReportTotalCount,
		Labels: labels,
		Value:  float64(tr.TotalCount),
	})

	// - Total time spent executing tests
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestReportTotalTime,
		Labels: labels,
		Value:  float64(tr.TotalTime),
	})
}

// ProcessTestSuiteMetrics processes and stores metrics for a single test suite
// within a GitLab ref (branch, tag, or merge request).
// It updates the metrics store with counts of errors, failures, skipped tests,
// successes, total tests, and total execution time for the test suite.
func (c *Controller) ProcessTestSuiteMetrics(ctx context.Context, ref schemas.Ref, ts schemas.TestSuite) {
	// Prepare log fields with project, ref, and test suite name for consistent logging
	testSuiteLogFields := log.Fields{
		"project-name":    ref.Project.Name,
		"ref":             ref.Name,
		"test-suite-name": ts.Name,
	}

	// Retrieve the default labels from the ref and add the test suite name label
	labels := ref.DefaultLabelsValues()
	labels["test_suite_name"] = ts.Name

	// Refresh the current ref state from the store to ensure the latest data
	if err := c.Store.GetRef(ctx, &ref); err != nil {
		// Log an error if the ref cannot be retrieved and stop processing this suite
		log.WithContext(ctx).
			WithFields(testSuiteLogFields).
			WithError(err).
			Error("getting ref from the store")
		return
	}

	// Log a trace-level message indicating the start of processing metrics for the test suite
	log.WithFields(testSuiteLogFields).Trace("processing test suite metrics")

	// Store metrics for the test suite:

	// Number of test errors
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteErrorCount,
		Labels: labels,
		Value:  float64(ts.ErrorCount),
	})

	// Number of test failures
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteFailedCount,
		Labels: labels,
		Value:  float64(ts.FailedCount),
	})

	// Number of skipped tests
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteSkippedCount,
		Labels: labels,
		Value:  float64(ts.SkippedCount),
	})

	// Number of successful tests
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteSuccessCount,
		Labels: labels,
		Value:  float64(ts.SuccessCount),
	})

	// Total number of tests run (all statuses)
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteTotalCount,
		Labels: labels,
		Value:  float64(ts.TotalCount),
	})

	// Total time spent running the test suite (in seconds)
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestSuiteTotalTime,
		Labels: labels,
		Value:  ts.TotalTime,
	})
}

// ProcessTestCaseMetrics processes and stores metrics for a single test case
// within a given test suite and GitLab ref (branch, tag, or merge request).
// It updates the metrics store with execution time and the test case status.
func (c *Controller) ProcessTestCaseMetrics(ctx context.Context, ref schemas.Ref, ts schemas.TestSuite, tc schemas.TestCase) {
	// Prepare log fields with project, ref, test suite, test case name and status for detailed logging
	testCaseLogFields := log.Fields{
		"project-name":     ref.Project.Name,
		"ref":              ref.Name,
		"test-suite-name":  ts.Name,
		"test-case-name":   tc.Name,
		"test-case-status": tc.Status,
	}

	// Retrieve the default labels from the ref and add labels specific to the test suite and test case
	labels := ref.DefaultLabelsValues()
	labels["test_suite_name"] = ts.Name          // Label for the test suite name
	labels["test_case_name"] = tc.Name           // Label for the test case name
	labels["test_case_classname"] = tc.Classname // Label for the test case class (optional grouping)

	// Refresh the current ref state from the store to ensure we work with the latest data
	if err := c.Store.GetRef(ctx, &ref); err != nil {
		// Log an error if unable to fetch the ref and abort metric processing for this test case
		log.WithContext(ctx).
			WithFields(testCaseLogFields).
			WithError(err).
			Error("getting ref from the store")

		return
	}

	// Log a trace-level message indicating the start of processing metrics for this test case
	log.WithFields(testCaseLogFields).Trace("processing test case metrics")

	// Store the execution time of the test case in the metrics store
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindTestCaseExecutionTime,
		Labels: labels,
		Value:  tc.ExecutionTime, // Execution time in seconds (or appropriate time unit)
	})

	// Emit a metric for the test case status (e.g., passed, failed, skipped)
	// This uses a helper to emit sparse status metrics, respecting project settings
	emitStatusMetric(
		ctx,
		c.Store,
		schemas.MetricKindTestCaseStatus,
		labels,
		statusesList[:],                       // List of possible statuses to report
		tc.Status,                             // Current status of this test case
		ref.Project.OutputSparseStatusMetrics, // Whether to output sparse status metrics for efficiency
	)
}
