package gitlab

import (
	"context"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	log "github.com/sirupsen/logrus"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"regexp"
)

/**
func (c *Client) ListRunners(ctx context.Context) (runners schemas.Runners, err error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListRunners")
	defer span.End()

	// TODO: Add tracing attributes for Runners
	//span.SetAttributes(attribute.String("runners"))

	return
}
*/

func (c *Client) GetProjectRunners(ctx context.Context, p schemas.Project) (runners schemas.Runners, err error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetProjectRunners")
	defer span.End()

	// TODO: Add tracing attributes for Runners
	span.SetAttributes(attribute.String("project_name", p.Name))

	runners = make(schemas.Runners)

	options := &goGitlab.ListProjectRunnersOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	// If configured to exclude stopped runners
	/**
	if p.Pull.Runners.ExcludeStopped {
		options.Paused = goGitlab.BoolValue(true)
	}
	*/

	re, err := regexp.Compile(p.Pull.Runners.Regexp)
	if err != nil {
		return nil, err
	}

	for {
		c.rateLimit(ctx)

		var (
			glrunners []*goGitlab.Runner // Runners returned by Gitlab API
			resp      *goGitlab.Response // API response metadata (pagination info, headers, etc...)
		)

		glrunners, resp, err = c.Runners.ListProjectRunners(p.Name, options, goGitlab.WithContext(ctx))
		if err != nil {
			return
		}

		// Track remaining API request quota based on response headers
		c.requestsRemaining(resp)

		// Iterate over runners returned by this API call
		for _, glrunner := range glrunners {
			if re.MatchString(glrunner.Name) {
				// Create a new Runner struct to hold relevant info
				runner := schemas.Runner{
					ProjectName:               p.Name,
					ID:                        glrunner.ID,
					Name:                      glrunner.Name,
					Description:               glrunner.Description,
					OutputSparseStatusMetrics: p.OutputSparseStatusMetrics,
				}

				// Mark runner as paused if its state is "Paused"
				/*
					if glrunner.Paused {
						runner.Paused = true
					}
				*/

				// Store runner in the result map with a unique key
				runners[runner.Key()] = runner
			}
		}

		// Check if all pages have been feteched; if yes, exit loop
		if resp.CurrentPage >= resp.NextPage {
			break
		}

		// Otherwise, update the page number for the next iteration to fetch more runners
		options.Page = resp.NextPage
	}

	// Return the collected runners and no error
	return
}

// GetRunner retrieves detailed information about a specific runner
// in a GitLab project, including its latest deployment data if available.
func (c *Client) GetRunner(ctx context.Context, project string, runnerID int) (runner schemas.Runner, err error) {
	// Start an OpenTelemetry span for tracing this method call
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetRunner")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", project))
	span.SetAttributes(attribute.Int("runner_id", runnerID))

	// Initialize the return Runner struct with project name and runner ID
	runner = schemas.Runner{
		ProjectName: project,
		ID:          runnerID,
	}

	// Respect API rate limits before making the request
	c.rateLimit(ctx)

	var (
		r    *goGitlab.RunnerDetails // pointer to the GitLab runner object returned by the API
		resp *goGitlab.Response      // response metadata including pagination and headers
	)

	// Call GitLab API to get environment details
	r, resp, err = c.Runners.GetRunnerDetails(runnerID, goGitlab.WithContext(ctx))
	if err != nil || r == nil {
		// Return immediately if error occurs or no environment was found
		return
	}

	// Update internal tracking of remaining API request quota
	c.requestsRemaining(resp)

	// Fill runner details from API response
	runner.Name = r.Name
	runner.ID = r.ID

	// Mark environment as available if its state is "available"
	/**
	if e.State == "available" {
			runner.Available = true
	}
	*/

	// If the environment has no recorded last deployment, log and return as is
	if r.Groups == nil {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"project-name": project,
				"runner-name":  r.Name,
				"runner-group": r.Groups,
			}).
			Debug("no Group found for this runner")
		return
	}

	// Fill with the rest of the last runner details
	runner.Paused = r.Paused
	runner.Description = r.Description
	runner.IsShared = r.IsShared
	runner.RunnerType = r.RunnerType
	runner.ContactedAt = r.ContactedAt
	runner.MaintenanceNote = r.MaintenanceNote
	runner.Name = r.Name
	runner.Online = r.Online
	runner.Status = r.Status
	runner.Projects = r.Projects
	runner.Token = r.Token
	runner.TagList = r.TagList
	runner.RunUntagged = r.RunUntagged
	runner.Locked = r.Locked
	runner.AccessLevel = r.AccessLevel
	runner.MaximumTimeout = r.MaximumTimeout
	runner.Groups = r.Groups

	// Return the populated Environment struct and nil error
	return
}
