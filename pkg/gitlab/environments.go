package gitlab

import (
	"context"
	"regexp"

	log "github.com/sirupsen/logrus"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// GetProjectEnvironments fetches the environments of a given GitLab project.
// It filters environments based on a regular expression defined in the project configuration.
func (c *Client) GetProjectEnvironments(ctx context.Context, p schemas.Project) (envs schemas.Environments, err error) {
	// Start an OpenTelemetry span for tracing this operation
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetProjectEnvironments")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", p.Name))

	// Initialize the map that will hold the resulting environments
	envs = make(schemas.Environments)

	// Set up options for listing environments from the GitLab API with pagination
	options := &goGitlab.ListEnvironmentsOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,   // start from first page
			PerPage: 100, // fetch up to 100 environments per page
		},
	}

	// If configured to exclude stopped environments,
	// limit the query to only environments that are currently "available"
	if p.Pull.Environments.ExcludeStopped {
		options.States = goGitlab.String("available")
	}

	// Compile the regular expression that will be used to filter environment names
	re, err := regexp.Compile(p.Pull.Environments.Regexp)
	if err != nil {
		// If the regexp is invalid, return the error
		return nil, err
	}

	// Loop to handle paginated API responses
	for {
		// Respect API rate limits before making a request
		c.rateLimit(ctx)

		var (
			glenvs []*goGitlab.Environment // environments returned by GitLab API
			resp   *goGitlab.Response      // API response metadata (pagination info, headers, etc.)
		)

		// Call GitLab API to list environments for the project with current options
		glenvs, resp, err = c.Environments.ListEnvironments(p.Name, options, goGitlab.WithContext(ctx))
		if err != nil {
			// Return on API error
			return
		}

		// Track remaining API request quota based on response headers
		c.requestsRemaining(resp)

		// Iterate over environments returned by this API call
		for _, glenv := range glenvs {
			// Filter environment by matching its name against the compiled regexp
			if re.MatchString(glenv.Name) {
				// Create a new Environment struct to hold relevant info
				env := schemas.Environment{
					ProjectName:               p.Name,
					ID:                        glenv.ID,
					Name:                      glenv.Name,
					OutputSparseStatusMetrics: p.OutputSparseStatusMetrics, // project config flag
				}

				// Mark environment as available if its state is "available"
				if glenv.State == "available" {
					env.Available = true
				}

				// Store environment in the result map with a unique key
				envs[env.Key()] = env
			}
		}

		// Check if all pages have been fetched; if yes, exit loop
		if resp.CurrentPage >= resp.NextPage {
			break
		}

		// Otherwise, update the page number for the next iteration to fetch more environments
		options.Page = resp.NextPage
	}

	// Return the collected environments and no error
	return
}

// GetEnvironment retrieves detailed information about a specific environment
// in a GitLab project, including its latest deployment data if available.
func (c *Client) GetEnvironment(ctx context.Context, project string, environmentID int) (environment schemas.Environment, err error) {
	// Start an OpenTelemetry span for tracing this method call
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetEnvironment")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", project))
	span.SetAttributes(attribute.Int("environment_id", environmentID))

	// Initialize the return Environment struct with project name and environment ID
	environment = schemas.Environment{
		ProjectName: project,
		ID:          environmentID,
	}

	// Respect API rate limits before making the request
	c.rateLimit(ctx)

	var (
		e    *goGitlab.Environment // pointer to the GitLab Environment object returned by the API
		resp *goGitlab.Response    // response metadata including pagination and headers
	)

	// Call GitLab API to get environment details
	e, resp, err = c.Environments.GetEnvironment(project, environmentID, goGitlab.WithContext(ctx))
	if err != nil || e == nil {
		// Return immediately if error occurs or no environment was found
		return
	}

	// Update internal tracking of remaining API request quota
	c.requestsRemaining(resp)

	// Fill environment details from API response
	environment.Name = e.Name
	environment.ExternalURL = e.ExternalURL

	// Mark environment as available if its state is "available"
	if e.State == "available" {
		environment.Available = true
	}

	// If the environment has no recorded last deployment, log and return as is
	if e.LastDeployment == nil {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"project-name":     project,
				"environment-name": e.Name,
			}).
			Debug("no deployments found for the environment")

		return
	}

	// Determine if the last deployment was from a tag or a branch and set RefKind accordingly
	if e.LastDeployment.Deployable.Tag {
		environment.LatestDeployment.RefKind = schemas.RefKindTag
	} else {
		environment.LatestDeployment.RefKind = schemas.RefKindBranch
	}

	// Fill the rest of the last deployment details
	environment.LatestDeployment.RefName = e.LastDeployment.Ref
	environment.LatestDeployment.JobID = e.LastDeployment.Deployable.ID
	environment.LatestDeployment.DurationSeconds = e.LastDeployment.Deployable.Duration
	environment.LatestDeployment.Status = e.LastDeployment.Deployable.Status

	// Optionally include the username of the user who triggered the deployment if available
	if e.LastDeployment.Deployable.User != nil {
		environment.LatestDeployment.Username = e.LastDeployment.Deployable.User.Username
	}

	// Optionally include the commit short ID of the deployed commit if available
	if e.LastDeployment.Deployable.Commit != nil {
		environment.LatestDeployment.CommitShortID = e.LastDeployment.Deployable.Commit.ShortID
	}

	// Record the timestamp of when the deployment was created, in Unix seconds
	if e.LastDeployment.CreatedAt != nil {
		environment.LatestDeployment.Timestamp = float64(e.LastDeployment.CreatedAt.Unix())
	}

	// Return the populated Environment struct and nil error
	return
}
