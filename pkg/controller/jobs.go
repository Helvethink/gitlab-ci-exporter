package controller

import (
	"context"
	"reflect"
	"regexp"
	"strconv"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	log "github.com/sirupsen/logrus"
)

// PullRefPipelineJobsMetrics fetches the pipeline jobs for the given reference
// and processes their metrics.
func (c *Controller) PullRefPipelineJobsMetrics(ctx context.Context, ref schemas.Ref) error {
	// Retrieve the list of pipeline jobs associated with the reference
	jobs, err := c.Gitlab.ListRefPipelineJobs(ctx, ref)
	if err != nil {
		// Return the error if fetching jobs failed
		return err
	}

	// Iterate over each job and process its metrics
	for _, job := range jobs {
		c.ProcessJobMetrics(ctx, ref, job)
	}

	// Return nil indicating successful processing
	return nil
}

// PullRefMostRecentJobsMetrics collects and processes metrics for the most recent jobs
// associated with a specific Git reference (e.g., a branch or tag).
func (c *Controller) PullRefMostRecentJobsMetrics(ctx context.Context, ref schemas.Ref) error {
	// Check if job metrics collection is enabled for this project reference.
	if !ref.Project.Pull.Pipeline.Jobs.Enabled {
		// If disabled, exit early without doing anything.
		return nil
	}

	// Retrieve the most recent jobs for the given Git reference from GitLab.
	jobs, err := c.Gitlab.ListRefMostRecentJobs(ctx, ref)
	if err != nil {
		// If an error occurred while fetching jobs, return the error.
		return err
	}

	// Iterate over each job and process its metrics.
	for _, job := range jobs {
		c.ProcessJobMetrics(ctx, ref, job)
	}

	// Return nil to indicate success.
	return nil
}

// ProcessJobMetrics processes metrics for a given pipeline job and updates the store accordingly.
func (c *Controller) ProcessJobMetrics(ctx context.Context, ref schemas.Ref, job schemas.Job) {
	// Prepare logging fields with project, job name, and job ID for contextual logging
	projectRefLogFields := log.Fields{
		"project-name": ref.Project.Name,
		"job-name":     job.Name,
		"job-id":       job.ID,
	}

	// Initialize labels from the reference default labels and add job-specific labels
	labels := ref.DefaultLabelsValues()
	labels["stage"] = job.Stage
	labels["job_name"] = job.Name
	labels["tag_list"] = job.TagList
	labels["status"] = job.Status
	labels["job_id"] = strconv.Itoa(job.ID)
	labels["pipeline_id"] = strconv.Itoa(job.PipelineID)
	labels["failure_reason"] = job.FailureReason

	// If runner description aggregation is enabled in the config, use regexp to aggregate runner descriptions
	if ref.Project.Pull.Pipeline.Jobs.RunnerDescription.Enabled {
		re, err := regexp.Compile(ref.Project.Pull.Pipeline.Jobs.RunnerDescription.AggregationRegexp)
		if err != nil {
			// Log an error if the regexp is invalid
			log.WithContext(ctx).
				WithFields(projectRefLogFields).
				WithError(err).
				Error("invalid job runner description aggregation regexp")
		}

		// If runner description matches the aggregation regexp, set it to the regexp pattern,
		// otherwise use the actual runner description string
		if re.MatchString(job.Runner.Description) {
			labels["runner_description"] = ref.Project.Pull.Pipeline.Jobs.RunnerDescription.AggregationRegexp
		} else {
			labels["runner_description"] = job.Runner.Description
		}
	} else {
		// Runner description aggregation is disabled: set runner_description label as empty
		// TODO: consider how to remove this label entirely from the metrics exporter if needed
		labels["runner_description"] = ""
	}

	// Refresh the reference state from the store to get the latest data
	if err := c.Store.GetRef(ctx, &ref); err != nil {
		log.WithContext(ctx).
			WithFields(projectRefLogFields).
			WithError(err).
			Error("getting ref from the store")
		return
	}

	// Retrieve the last recorded job with the same name
	lastJob, lastJobExists := ref.LatestJobs[job.Name]

	// If the last job exists and is deeply equal to the current job, no update needed, skip processing
	// This prevents reprocessing identical job data (e.g., when the job has not changed)
	if lastJobExists && reflect.DeepEqual(lastJob, job) {
		return
	}

	// Ensure LatestJobs map is initialized to avoid nil map assignment
	if ref.LatestJobs == nil {
		ref.LatestJobs = make(schemas.Jobs)
	}

	// Update the latest job in the ref's map to the current job
	ref.LatestJobs[job.Name] = job

	// Persist the updated reference state back into the store
	if err := c.Store.SetRef(ctx, ref); err != nil {
		log.WithContext(ctx).
			WithFields(projectRefLogFields).
			WithError(err).
			Error("writing ref in the store")
		return
	}

	// Log trace info indicating that job metrics are being processed
	log.WithFields(projectRefLogFields).Trace("processing job metrics")

	// Store various metrics related to the job:
	// - Job ID
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindJobID,
		Labels: labels,
		Value:  float64(job.ID),
	})

	// - Job timestamp (e.g. when it started or finished)
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindJobTimestamp,
		Labels: labels,
		Value:  job.Timestamp,
	})

	// - Job duration in seconds
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindJobDurationSeconds,
		Labels: labels,
		Value:  job.DurationSeconds,
	})

	// - Job queued duration in seconds (time spent waiting before running)
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindJobQueuedDurationSeconds,
		Labels: labels,
		Value:  job.QueuedDurationSeconds,
	})

	// Initialize a metric counter for the number of times the job has run
	jobRunCount := schemas.Metric{
		Kind:   schemas.MetricKindJobRunCount,
		Labels: labels,
	}

	// Check if this run count metric already exists in the store
	jobRunCountExists, err := c.Store.MetricExists(ctx, jobRunCount.Key())
	if err != nil {
		log.WithContext(ctx).
			WithFields(projectRefLogFields).
			WithError(err).
			Error("checking if metric exists in the store")
		return
	}

	// Define a regexp to identify job statuses that mean the job was NOT triggered (skipped, manual, scheduled)
	jobTriggeredRegexp := regexp.MustCompile("^(skipped|manual|scheduled)$")

	// Determine whether the last job and the current job have triggered statuses
	lastJobTriggered := !jobTriggeredRegexp.MatchString(lastJob.Status)
	jobTriggered := !jobTriggeredRegexp.MatchString(job.Status)

	// Increment job run count only once per job ID, if:
	// - This is a new job ID and it has been triggered
	// - OR the same job ID but previously not triggered and now triggered
	if jobRunCountExists && ((lastJob.ID != job.ID && jobTriggered) || (lastJob.ID == job.ID && jobTriggered && !lastJobTriggered)) {
		// Fetch current run count value
		storeGetMetric(ctx, c.Store, &jobRunCount)

		// Increment the run count
		jobRunCount.Value++
	}

	// Store the updated job run count metric
	storeSetMetric(ctx, c.Store, jobRunCount)

	// Store the size of job artifacts in bytes
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindJobArtifactSizeBytes,
		Labels: labels,
		Value:  job.ArtifactSize,
	})

	// Emit a metric representing the job status with labels
	emitStatusMetric(
		ctx,
		c.Store,
		schemas.MetricKindJobStatus,
		labels,
		statusesList[:],                       // list of possible statuses
		job.Status,                            // current job status
		ref.Project.OutputSparseStatusMetrics, // config flag for sparse output
	)
}
