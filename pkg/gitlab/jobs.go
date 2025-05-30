package gitlab

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// ListRefPipelineJobs retrieves the list of jobs for the latest pipeline associated with a given ref.
// If configured, it also includes jobs from child pipelines.
func (c *Client) ListRefPipelineJobs(ctx context.Context, ref schemas.Ref) (jobs []schemas.Job, err error) {
	// Start OpenTelemetry tracing span for observability
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListRefPipelineJobs")
	defer span.End()

	// Add tracing attributes for project and ref
	span.SetAttributes(attribute.String("project_name", ref.Project.Name))
	span.SetAttributes(attribute.String("ref_name", ref.Name))

	// Check if the latest pipeline is defined. Exit early if not.
	if reflect.DeepEqual(ref.LatestPipeline, (schemas.Pipeline{})) {
		log.WithFields(
			log.Fields{
				"project-name": ref.Project.Name,
				"ref":          ref.Name,
			},
		).Debug("most recent pipeline not defined, exiting..")
		return
	}

	// Fetch jobs for the main pipeline
	jobs, err = c.ListPipelineJobs(ctx, ref.Project.Name, ref.LatestPipeline.ID)
	if err != nil {
		return
	}

	// Optionally include jobs from child pipelines if configured
	if ref.Project.Pull.Pipeline.Jobs.FromChildPipelines.Enabled {
		var childJobs []schemas.Job

		// Fetch jobs from child pipelines
		childJobs, err = c.ListPipelineChildJobs(ctx, ref.Project.Name, ref.LatestPipeline.ID)
		if err != nil {
			return
		}

		// Append child jobs to the main jobs list
		jobs = append(jobs, childJobs...)
	}

	return
}

// ListPipelineJobs retrieves all jobs associated with a given pipeline ID for a specified project.
// It handles pagination and rate limiting automatically.
func (c *Client) ListPipelineJobs(ctx context.Context, projectNameOrID string, pipelineID int) (jobs []schemas.Job, err error) {
	// Start a tracing span for observability (OpenTelemetry)
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListPipelineJobs")
	defer span.End()

	// Attach useful metadata to the span
	span.SetAttributes(attribute.String("project_name_or_id", projectNameOrID))
	span.SetAttributes(attribute.Int("pipeline_id", pipelineID))

	var (
		foundJobs []*goGitlab.Job    // Jobs returned by GitLab API
		resp      *goGitlab.Response // API response metadata (pagination info, headers, etc.)
	)

	// Define pagination and filtering options for the API call
	options := &goGitlab.ListJobsOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,
			PerPage: 100, // Retrieve up to 100 jobs per API call
		},
	}

	// Paginate through all pages of jobs
	for {
		// Respect rate limiting before making the API call
		c.rateLimit(ctx)

		// Fetch jobs for the current page
		foundJobs, resp, err = c.Jobs.ListPipelineJobs(projectNameOrID, pipelineID, options, goGitlab.WithContext(ctx))
		if err != nil {
			return // Return on API error
		}

		// Update rate limit tracking from response headers
		c.requestsRemaining(resp)

		// Convert raw GitLab jobs into internal schema and collect them
		for _, job := range foundJobs {
			jobs = append(jobs, schemas.NewJob(*job))
		}

		// Exit the loop if there are no more pages to fetch
		if resp.CurrentPage >= resp.NextPage {
			// Log the successful retrieval for debug/traceability
			log.WithFields(
				log.Fields{
					"project-name-or-id": projectNameOrID,
					"pipeline-id":        pipelineID,
					"jobs-count":         resp.TotalItems,
				},
			).Debug("found pipeline jobs")
			break
		}

		// Move to the next page
		options.Page = resp.NextPage
	}

	// Return the accumulated list of jobs
	return
}

// ListPipelineBridges retrieves all bridge jobs (i.e., jobs that trigger downstream pipelines)
// associated with a given pipeline ID in a specified project.
// It paginates through results and respects API rate limits.
func (c *Client) ListPipelineBridges(ctx context.Context, projectNameOrID string, pipelineID int) (bridges []*goGitlab.Bridge, err error) {
	// Start a tracing span for this operation
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListPipelineBridges")
	defer span.End()

	// Add useful attributes to the span for better traceability
	span.SetAttributes(attribute.String("project_name_or_id", projectNameOrID))
	span.SetAttributes(attribute.Int("pipeline_id", pipelineID))

	var (
		foundBridges []*goGitlab.Bridge // Temporary storage for API-returned bridge jobs
		resp         *goGitlab.Response // API response metadata (used for pagination and rate limits)
	)

	// Configure pagination options
	options := &goGitlab.ListJobsOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,   // Start from the first page
			PerPage: 100, // Retrieve up to 100 items per page
		},
	}

	// Loop to paginate through all available bridge jobs
	for {
		// Respect rate limits before performing API calls
		c.rateLimit(ctx)

		// Fetch the bridge jobs from the GitLab API
		foundBridges, resp, err = c.Jobs.ListPipelineBridges(projectNameOrID, pipelineID, options, goGitlab.WithContext(ctx))
		if err != nil {
			return // Return early if an error occurs
		}

		// Track rate limit status from the response headers
		c.requestsRemaining(resp)

		// Append the found bridge jobs to the final list
		bridges = append(bridges, foundBridges...)

		// Check if we've reached the last page
		if resp.CurrentPage >= resp.NextPage {
			// Log debug information about the retrieved bridge jobs
			log.WithFields(
				log.Fields{
					"project-name-or-id": projectNameOrID,
					"pipeline-id":        pipelineID,
					"bridges-count":      resp.TotalItems,
				},
			).Debug("found pipeline bridges")

			break // Exit the loop if there are no more pages
		}

		// Move to the next page for the next iteration
		options.Page = resp.NextPage
	}

	// Return the full list of collected bridge jobs
	return
}

// ListPipelineChildJobs retrieves all jobs from child pipelines triggered by a given parent pipeline.
// It traverses through bridge jobs recursively to explore all levels of downstream pipelines.
func (c *Client) ListPipelineChildJobs(ctx context.Context, projectNameOrID string, parentPipelineID int) (jobs []schemas.Job, err error) {
	// Start a tracing span for observability (via OpenTelemetry)
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListPipelineChildJobs")
	defer span.End()

	// Add metadata to the tracing span
	span.SetAttributes(attribute.String("project_name_or_id", projectNameOrID))
	span.SetAttributes(attribute.Int("parent_pipeline_id", parentPipelineID))

	// Internal type to represent a pipeline's project and ID
	type pipelineDef struct {
		projectNameOrID string
		pipelineID      int
	}

	// Seed the traversal with the initial (parent) pipeline
	pipelines := []pipelineDef{{projectNameOrID, parentPipelineID}}

	// Process pipelines in a depth-first manner using a stack
	for {
		if len(pipelines) == 0 {
			// No more pipelines to process, return accumulated jobs
			return
		}

		var (
			foundBridges []*goGitlab.Bridge            // Bridges in the current pipeline
			pipeline     = pipelines[len(pipelines)-1] // Get the last pipeline in the stack (LIFO)
		)

		// Remove the current pipeline from the stack
		pipelines = pipelines[:len(pipelines)-1]

		// Retrieve all bridge jobs from the current pipeline
		foundBridges, err = c.ListPipelineBridges(ctx, pipeline.projectNameOrID, pipeline.pipelineID)
		if err != nil {
			return // Exit on error
		}

		// Iterate over each bridge job to explore downstream pipelines
		for _, foundBridge := range foundBridges {
			// If no downstream pipeline is available (job not yet executed), skip it
			if foundBridge.DownstreamPipeline == nil {
				continue
			}

			// Add the downstream pipeline to the stack for further traversal
			pipelines = append(pipelines, pipelineDef{
				projectNameOrID: strconv.Itoa(foundBridge.DownstreamPipeline.ProjectID),
				pipelineID:      foundBridge.DownstreamPipeline.ID,
			})

			// Fetch jobs from the downstream pipeline
			var foundJobs []schemas.Job
			foundJobs, err = c.ListPipelineJobs(ctx, strconv.Itoa(foundBridge.DownstreamPipeline.ProjectID), foundBridge.DownstreamPipeline.ID)
			if err != nil {
				return // Exit on error
			}

			// Accumulate the jobs
			jobs = append(jobs, foundJobs...)
		}
	}
}

// ListRefMostRecentJobs fetches the most recent state of jobs already held in memory (ref.LatestJobs)
// for a given GitLab reference (typically a branch or merge request ref).
// It supports both page-based and keyset pagination depending on the GitLab version.
func (c *Client) ListRefMostRecentJobs(ctx context.Context, ref schemas.Ref) (jobs []schemas.Job, err error) {
	// Start OpenTelemetry tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListRefMostRecentJobs")
	defer span.End()

	// Add metadata to the trace
	span.SetAttributes(attribute.String("project_name", ref.Project.Name))
	span.SetAttributes(attribute.String("ref_name", ref.Name))

	// If there are no cached jobs for the ref, exit early
	if len(ref.LatestJobs) == 0 {
		log.WithFields(log.Fields{
			"project-name": ref.Project.Name,
			"ref":          ref.Name,
		}).Debug("no jobs are currently held in memory, exiting..")
		return
	}

	// Deep copy of the LatestJobs map to track jobs we need to refresh
	jobsToRefresh := make(schemas.Jobs)
	for k, v := range ref.LatestJobs {
		jobsToRefresh[k] = v
	}

	// Prepare variables for job listing
	var (
		foundJobs []*goGitlab.Job
		resp      *goGitlab.Response
		opt       *goGitlab.ListJobsOptions
	)

	// Determine if the GitLab version supports keyset pagination
	keysetPagination := c.Version().PipelineJobsKeysetPaginationSupported()
	if keysetPagination {
		opt = &goGitlab.ListJobsOptions{
			ListOptions: goGitlab.ListOptions{
				Pagination: "keyset",
				PerPage:    100,
			},
		}
	} else {
		opt = &goGitlab.ListJobsOptions{
			ListOptions: goGitlab.ListOptions{
				Page:    1,
				PerPage: 100,
			},
		}
	}

	// Initial request options
	options := []goGitlab.RequestOptionFunc{goGitlab.WithContext(ctx)}

	// Begin job listing loop
	for {
		c.rateLimit(ctx)

		// Fetch jobs for the project
		foundJobs, resp, err = c.Jobs.ListProjectJobs(ref.Project.Name, opt, options...)
		if err != nil {
			return
		}

		// Update internal rate-limit tracker
		c.requestsRemaining(resp)

		// Filter jobs that match the names and reference
		for _, job := range foundJobs {
			if _, ok := jobsToRefresh[job.Name]; ok {
				jobRefName, _ := schemas.GetMergeRequestIIDFromRefName(job.Ref)
				if ref.Name == jobRefName {
					jobs = append(jobs, schemas.NewJob(*job))
					delete(jobsToRefresh, job.Name)
				}
			}

			// All jobs were found, we can return early
			if len(jobsToRefresh) == 0 {
				log.WithFields(log.Fields{
					"project-name": ref.Project.Name,
					"ref":          ref.Name,
					"jobs-count":   len(ref.LatestJobs),
				}).Debug("found all jobs to refresh")
				return
			}
		}

		// Check if pagination is complete
		if (keysetPagination && resp.NextLink == "") ||
			(!keysetPagination && resp.CurrentPage >= resp.NextPage) {

			// Some jobs were not refreshed; log a warning
			var notFoundJobs []string
			for k := range jobsToRefresh {
				notFoundJobs = append(notFoundJobs, k)
			}

			log.WithContext(ctx).WithFields(log.Fields{
				"project-name":   ref.Project.Name,
				"ref":            ref.Name,
				"jobs-count":     resp.TotalItems,
				"not-found-jobs": strings.Join(notFoundJobs, ","),
			}).Warn("found some ref jobs but did not manage to refresh all jobs which were in memory")

			break
		}

		// Prepare next page for keyset pagination
		if keysetPagination {
			options = []goGitlab.RequestOptionFunc{
				goGitlab.WithContext(ctx),
				goGitlab.WithKeysetPaginationParameters(resp.NextLink),
			}
		}
	}

	// Return whatever jobs were found
	return
}
