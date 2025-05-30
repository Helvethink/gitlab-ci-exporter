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

// GetProjectBranches retrieves all branches for a given project that match
// the regular expression pattern specified in the project's Pull.Refs.Branches.Regexp field.
// It paginates through all available branches, applies the regex filter, and returns the matching branches as refs.
func (c *Client) GetProjectBranches(ctx context.Context, p schemas.Project) (refs schemas.Refs, err error) {
	// Start a tracing span for observability
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetProjectBranches")
	defer span.End()

	// Add project name as a trace attribute
	span.SetAttributes(attribute.String("project_name", p.Name))

	// Initialize the map to store filtered refs
	refs = make(schemas.Refs)

	// Set initial pagination options: start at page 1 with 100 items per page
	options := &goGitlab.ListBranchesOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	// Compile the regular expression to filter branch names
	var re *regexp.Regexp
	if re, err = regexp.Compile(p.Pull.Refs.Branches.Regexp); err != nil {
		// Return early if regex compilation fails
		return
	}

	// Loop through paginated results until all pages are processed
	for {
		// Apply rate limiting before each API call
		c.rateLimit(ctx)

		var (
			branches []*goGitlab.Branch
			resp     *goGitlab.Response
		)

		// Call GitLab API to list branches for the project with current pagination options
		branches, resp, err = c.Branches.ListBranches(p.Name, options, goGitlab.WithContext(ctx))
		if err != nil {
			return
		}

		// Update internal counters for remaining requests from response headers
		c.requestsRemaining(resp)

		// Filter branches that match the regex pattern and add them to refs
		for _, branch := range branches {
			if re.MatchString(branch.Name) {
				ref := schemas.NewRef(p, schemas.RefKindBranch, branch.Name)
				refs[ref.Key()] = ref
			}
		}

		// Check if we've reached the last page, then break the loop
		if resp.CurrentPage >= resp.NextPage {
			break
		}

		// Otherwise, update the pagination to the next page
		options.Page = resp.NextPage
	}

	// Return the map of filtered branch refs and nil error
	return
}

// GetBranchLatestCommit fetches the latest commit ID and its timestamp for a specific branch in a project.
// It returns the commit short ID, the commit date as a Unix timestamp (float64), or an error if any occurs.
func (c *Client) GetBranchLatestCommit(ctx context.Context, project, branch string) (string, float64, error) {
	// Start a tracing span for monitoring and observability
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetBranchLatestCommit")
	defer span.End()

	// Add project and branch information to the span attributes
	span.SetAttributes(attribute.String("project_name", project))
	span.SetAttributes(attribute.String("branch_name", branch))

	// Log debug message with project and branch details
	log.WithFields(log.Fields{
		"project-name": project,
		"branch":       branch,
	}).Debug("reading project branch")

	// Apply rate limiting before making the API request
	c.rateLimit(ctx)

	// Call GitLab API to get branch details
	b, resp, err := c.Branches.GetBranch(project, branch, goGitlab.WithContext(ctx))
	if err != nil {
		// Return empty values and the error if the request failed
		return "", 0, err
	}

	// Update the internal request counters with info from the API response headers
	c.requestsRemaining(resp)

	// Return the short commit ID and the commit timestamp (converted to float64 Unix time)
	return b.Commit.ShortID, float64(b.Commit.CommittedDate.Unix()), nil
}
