package gitlab

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// GetRefPipeline retrieves a specific pipeline by ID for a given ref (branch, tag, or MR)
func (c *Client) GetRefPipeline(ctx context.Context, ref schemas.Ref, pipelineID int) (p schemas.Pipeline, err error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetRefPipeline")
	defer span.End()

	// Add attributes for observability
	span.SetAttributes(attribute.String("project_name", ref.Project.Name))
	span.SetAttributes(attribute.String("ref_name", ref.Name))
	span.SetAttributes(attribute.Int("pipeline_id", pipelineID))

	c.rateLimit(ctx)

	// Fetch pipeline information from GitLab
	gp, resp, err := c.Pipelines.GetPipeline(ref.Project.Name, pipelineID, goGitlab.WithContext(ctx))
	if err != nil || gp == nil {
		return schemas.Pipeline{}, fmt.Errorf("could not read content of pipeline %s - %s | %s", ref.Project.Name, ref.Name, err.Error())
	}

	c.requestsRemaining(resp)

	// Convert and return pipeline to internal schema
	return schemas.NewPipeline(ctx, *gp), nil
}

// GetProjectPipelines lists pipelines for a given project, supporting pagination and filters
func (c *Client) GetProjectPipelines(
	ctx context.Context,
	projectName string,
	options *goGitlab.ListProjectPipelinesOptions,
) (
	[]*goGitlab.PipelineInfo,
	*goGitlab.Response,
	error,
) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetProjectPipelines")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", projectName))

	// Prepare structured logging
	fields := log.Fields{
		"project-name": projectName,
	}

	// Set default pagination values if missing
	if options.Page == 0 {
		options.Page = 1
	}
	if options.PerPage == 0 {
		options.PerPage = 100
	}

	// Include additional optional filters for logging
	if options.Ref != nil {
		fields["ref"] = *options.Ref
	}
	if options.Scope != nil {
		fields["scope"] = *options.Scope
	}
	fields["page"] = options.Page

	log.WithFields(fields).Trace("listing project pipelines")
	c.rateLimit(ctx)

	// Fetch pipeline list from GitLab API
	pipelines, resp, err := c.Pipelines.ListProjectPipelines(projectName, options, goGitlab.WithContext(ctx))
	if err != nil {
		return nil, resp, fmt.Errorf("error listing project pipelines for project %s: %s", projectName, err.Error())
	}

	c.requestsRemaining(resp)
	return pipelines, resp, nil
}

// GetRefPipelineVariablesAsConcatenatedString returns filtered pipeline variables as a concatenated string
func (c *Client) GetRefPipelineVariablesAsConcatenatedString(ctx context.Context, ref schemas.Ref) (string, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetRefPipelineVariablesAsConcatenatedString")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", ref.Project.Name))
	span.SetAttributes(attribute.String("ref_name", ref.Name))

	// If latest pipeline isn't defined, skip
	if reflect.DeepEqual(ref.LatestPipeline, (schemas.Pipeline{})) {
		log.WithFields(
			log.Fields{
				"project-name": ref.Project.Name,
				"ref":          ref.Name,
			},
		).Debug("most recent pipeline not defined, exiting..")

		return "", nil
	}

	log.WithFields(
		log.Fields{
			"project-name": ref.Project.Name,
			"ref":          ref.Name,
			"pipeline-id":  ref.LatestPipeline.ID,
		},
	).Debug("fetching pipeline variables")

	// Compile regex filter for variables
	variablesFilter, err := regexp.Compile(ref.Project.Pull.Pipeline.Variables.Regexp)
	if err != nil {
		return "", fmt.Errorf(
			"the provided filter regex for pipeline variables is invalid '(%s)': %v",
			ref.Project.Pull.Pipeline.Variables.Regexp,
			err,
		)
	}

	c.rateLimit(ctx)

	// Fetch variables from GitLab
	variables, resp, err := c.Pipelines.GetPipelineVariables(ref.Project.Name, ref.LatestPipeline.ID, goGitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("could not fetch pipeline variables for %d: %s", ref.LatestPipeline.ID, err.Error())
	}

	c.requestsRemaining(resp)

	// Filter variables using regex
	var keptVariables []string
	if len(variables) > 0 {
		for _, v := range variables {
			if variablesFilter.MatchString(v.Key) {
				keptVariables = append(keptVariables, strings.Join([]string{v.Key, v.Value}, ":"))
			}
		}
	}

	// Concatenate filtered variables as a comma-separated string
	return strings.Join(keptVariables, ","), nil
}

// GetRefsFromPipelines retrieves references (branches, tags, or merge requests) based on GitLab pipelines.
// It filters them using the project's configuration, including optional regex, max age, and deletion status.
func (c *Client) GetRefsFromPipelines(ctx context.Context, p schemas.Project, refKind schemas.RefKind) (refs schemas.Refs, err error) {
	// Start OpenTelemetry span for tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetRefsFromPipelines")
	defer span.End()

	// Add attributes for observability
	span.SetAttributes(attribute.String("project_name", p.Name))
	span.SetAttributes(attribute.String("ref_kind", string(refKind)))

	// Initialize empty map to store discovered references
	refs = make(schemas.Refs)

	// Set initial options for pipeline listing (pagination, sort order)
	options := &goGitlab.ListProjectPipelinesOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
		OrderBy: goGitlab.Ptr("updated_at"),
	}

	// Compile the regex used to filter refs (based on config)
	var re *regexp.Regexp
	if re, err = schemas.GetRefRegexp(p.Pull.Refs, refKind); err != nil {
		return
	}

	// Config variables depending on the ref type
	var (
		mostRecent, maxAgeSeconds         uint
		limitToMostRecent, excludeDeleted bool
		existingRefs                      schemas.Refs
	)

	// Set behavior depending on the reference kind (MR, branch, or tag)
	switch refKind {
	case schemas.RefKindMergeRequest:
		maxAgeSeconds = p.Pull.Refs.MergeRequests.MaxAgeSeconds
		mostRecent = p.Pull.Refs.MergeRequests.MostRecent

	case schemas.RefKindBranch:
		options.Scope = goGitlab.Ptr("branches")
		maxAgeSeconds = p.Pull.Refs.Branches.MaxAgeSeconds
		mostRecent = p.Pull.Refs.Branches.MostRecent

		if p.Pull.Refs.Branches.ExcludeDeleted {
			excludeDeleted = true
			// Get list of current branches to later detect deleted ones
			if existingRefs, err = c.GetProjectBranches(ctx, p); err != nil {
				return
			}
		}

	case schemas.RefKindTag:
		options.Scope = goGitlab.Ptr("tags")
		maxAgeSeconds = p.Pull.Refs.Tags.MaxAgeSeconds
		mostRecent = p.Pull.Refs.Tags.MostRecent

		if p.Pull.Refs.Tags.ExcludeDeleted {
			excludeDeleted = true
			// Get list of current tags to later detect deleted ones
			if existingRefs, err = c.GetProjectTags(ctx, p); err != nil {
				return
			}
		}

	default:
		return refs, fmt.Errorf("unsupported ref kind %v", refKind)
	}

	// Activate mostRecent limit if set
	if mostRecent > 0 {
		limitToMostRecent = true
	}

	// Set max age filter if defined
	if maxAgeSeconds > 0 {
		t := time.Now().Add(-time.Second * time.Duration(maxAgeSeconds))
		options.UpdatedAfter = &t
	}

	// Paginated loop to retrieve all matching pipelines
	for {
		var (
			pipelines []*goGitlab.PipelineInfo
			resp      *goGitlab.Response
		)

		// Fetch pipeline list
		pipelines, resp, err = c.GetProjectPipelines(ctx, p.Name, options)
		if err != nil {
			return
		}

		// Iterate over pipelines to extract and filter refs
		for _, pipeline := range pipelines {
			refName := pipeline.Ref

			// Skip if the ref doesn't match the configured regex
			if !re.MatchString(refName) {
				if refKind != schemas.RefKindMergeRequest {
					log.WithField("ref", refName).Debug("discovered pipeline ref not matching regexp")
				}
				continue
			}

			// For merge requests, extract the IID from the ref name
			if refKind == schemas.RefKindMergeRequest {
				if refName, err = schemas.GetMergeRequestIIDFromRefName(refName); err != nil {
					log.WithContext(ctx).
						WithField("ref", refName).
						WithError(err).
						Warn()
					continue
				}
			}

			// Create internal ref object
			ref := schemas.NewRef(p, refKind, refName)

			// Optionally skip deleted refs
			if excludeDeleted {
				if _, refExists := existingRefs[ref.Key()]; !refExists {
					log.WithFields(log.Fields{
						"project-name": ref.Project.Name,
						"ref":          ref.Name,
						"ref-kind":     ref.Kind,
					}).Debug("found deleted ref, ignoring..")
					continue
				}
			}

			// Add new unique refs to the result
			if _, ok := refs[ref.Key()]; !ok {
				log.WithFields(log.Fields{
					"project-name": ref.Project.Name,
					"ref":          ref.Name,
					"ref-kind":     ref.Kind,
				}).Trace("found ref")

				refs[ref.Key()] = ref

				// Stop if we've reached the limit
				if limitToMostRecent {
					mostRecent--
					if mostRecent <= 0 {
						return
					}
				}
			}
		}

		// Break if no more pages
		if resp.CurrentPage >= resp.NextPage {
			break
		}

		// Move to next page
		options.Page = resp.NextPage
	}

	return
}

// GetRefPipelineTestReport retrieves and aggregates test reports for a given pipeline reference,
// including optionally its child pipelines if configured to do so.
func (c *Client) GetRefPipelineTestReport(ctx context.Context, ref schemas.Ref) (schemas.TestReport, error) {
	// Start OpenTelemetry span for tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetRefPipelineTestReport")
	defer span.End()

	// Add trace attributes for observability
	span.SetAttributes(attribute.String("project_name", ref.Project.Name))
	span.SetAttributes(attribute.String("ref_name", ref.Name))

	// Return early if no latest pipeline is available for this ref
	if reflect.DeepEqual(ref.LatestPipeline, (schemas.Pipeline{})) {
		log.WithFields(
			log.Fields{
				"project-name": ref.Project.Name,
				"ref":          ref.Name,
			},
		).Debug("most recent pipeline not defined, exiting...")

		return schemas.TestReport{}, nil
	}

	// Log the pipeline we're about to fetch the report for
	log.WithFields(
		log.Fields{
			"project-name": ref.Project.Name,
			"ref":          ref.Name,
			"pipeline-id":  ref.LatestPipeline.ID,
		},
	).Debug("fetching pipeline test report")

	// Rate limit the request to avoid hitting GitLab API limits
	c.rateLimit(ctx)

	// Internal type to keep track of pipelines to process
	type pipelineDef struct {
		projectNameOrID string
		pipelineID      int
	}

	var currentPipeline pipelineDef

	// Initialize an empty test report accumulator
	baseTestReport := schemas.TestReport{
		TotalTime:    0,
		TotalCount:   0,
		SuccessCount: 0,
		FailedCount:  0,
		SkippedCount: 0,
		ErrorCount:   0,
		TestSuites:   []schemas.TestSuite{},
	}

	// Start with the root pipeline (the one from the provided ref)
	childPipelines := []pipelineDef{{ref.Project.Name, ref.LatestPipeline.ID}}

	// Process each pipeline (including discovered child pipelines)
	for {
		// Exit if there are no more pipelines to process
		if len(childPipelines) == 0 {
			return baseTestReport, nil
		}

		// Dequeue the first pipeline to process
		currentPipeline, childPipelines = childPipelines[0], childPipelines[1:]

		// Fetch the test report for the current pipeline
		testReport, resp, err := c.Pipelines.GetPipelineTestReport(currentPipeline.projectNameOrID, currentPipeline.pipelineID, goGitlab.WithContext(ctx))
		if err != nil {
			return schemas.TestReport{}, fmt.Errorf("could not fetch test report for %d: %s", ref.LatestPipeline.ID, err.Error())
		}

		// Track rate limit from response headers
		c.requestsRemaining(resp)

		// Convert GitLab report to internal schema
		convertedTestReport := schemas.NewTestReport(*testReport)

		// Aggregate test report data into the base report
		baseTestReport = schemas.TestReport{
			TotalTime:    baseTestReport.TotalTime + convertedTestReport.TotalTime,
			TotalCount:   baseTestReport.TotalCount + convertedTestReport.TotalCount,
			SuccessCount: baseTestReport.SuccessCount + convertedTestReport.SuccessCount,
			FailedCount:  baseTestReport.FailedCount + convertedTestReport.FailedCount,
			SkippedCount: baseTestReport.SkippedCount + convertedTestReport.SkippedCount,
			ErrorCount:   baseTestReport.ErrorCount + convertedTestReport.ErrorCount,
			TestSuites:   append(baseTestReport.TestSuites, convertedTestReport.TestSuites...),
		}

		// If child pipeline reporting is enabled, try to find and queue them
		if ref.Project.Pull.Pipeline.TestReports.FromChildPipelines.Enabled {
			foundBridges, err := c.ListPipelineBridges(ctx, currentPipeline.projectNameOrID, currentPipeline.pipelineID)
			if err != nil {
				return baseTestReport, err
			}

			// Add all downstream child pipelines to the processing queue
			for _, foundBridge := range foundBridges {
				if foundBridge.DownstreamPipeline == nil {
					continue
				}

				childPipelines = append(childPipelines, pipelineDef{
					projectNameOrID: strconv.Itoa(foundBridge.DownstreamPipeline.ProjectID),
					pipelineID:      foundBridge.DownstreamPipeline.ID,
				})
			}
		}
	}
}
