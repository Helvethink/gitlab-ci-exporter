package gitlab

import (
	"context" // Package for managing context and cancellation
	"regexp"  // Package for regular expression operations

	goGitlab "gitlab.com/gitlab-org/api/client-go" // GitLab API client
	"go.opentelemetry.io/otel"                     // OpenTelemetry API for tracing
	"go.opentelemetry.io/otel/attribute"           // OpenTelemetry attribute package

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas" // Package for schemas
)

// GetProjectTags retrieves tags for a given project that match a specified regular expression.
func (c *Client) GetProjectTags(ctx context.Context, p schemas.Project) (refs schemas.Refs, err error) {
	// Start a new OpenTelemetry span for tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetProjectTags")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", p.Name)) // Set project name as an attribute

	refs = make(schemas.Refs) // Initialize a map to store the references

	// Set up options for listing tags
	options := &goGitlab.ListTagsOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,   // Start from the first page
			PerPage: 100, // Number of tags per page
		},
	}

	var re *regexp.Regexp

	// Compile the regular expression for tag filtering
	if re, err = regexp.Compile(p.Pull.Refs.Tags.Regexp); err != nil {
		return // Return if there's an error compiling the regex
	}

	// Loop through pages of tags
	for {
		c.rateLimit(ctx) // Apply rate limiting

		var (
			tags []*goGitlab.Tag    // Slice to store tags
			resp *goGitlab.Response // Response from the GitLab API
		)

		// Retrieve tags from GitLab
		tags, resp, err = c.Tags.ListTags(p.Name, options, goGitlab.WithContext(ctx))
		if err != nil {
			return // Return if there's an error retrieving tags
		}

		c.requestsRemaining(resp) // Update the remaining requests count

		// Iterate through the tags and filter them using the regular expression
		for _, tag := range tags {
			if re.MatchString(tag.Name) {
				ref := schemas.NewRef(p, schemas.RefKindTag, tag.Name) // Create a new reference
				refs[ref.Key()] = ref                                  // Add the reference to the map
			}
		}

		// Break the loop if there are no more pages
		if resp.CurrentPage >= resp.NextPage {
			break
		}

		options.Page = resp.NextPage // Move to the next page
	}

	return // Return the filtered references
}

// GetProjectMostRecentTagCommit retrieves the most recent tag commit for a project that matches a specified regular expression.
func (c *Client) GetProjectMostRecentTagCommit(ctx context.Context, projectName, filterRegexp string) (string, float64, error) {
	// Start a new OpenTelemetry span for tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetProjectTags")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", projectName)) // Set project name as an attribute
	span.SetAttributes(attribute.String("regexp", filterRegexp))      // Set regex as an attribute

	// Set up options for listing tags
	options := &goGitlab.ListTagsOptions{
		ListOptions: goGitlab.ListOptions{
			Page:    1,   // Start from the first page
			PerPage: 100, // Number of tags per page
		},
	}

	// Compile the regular expression for tag filtering
	re, err := regexp.Compile(filterRegexp)
	if err != nil {
		return "", 0, err // Return if there's an error compiling the regex
	}

	// Loop through pages of tags
	for {
		c.rateLimit(ctx) // Apply rate limiting

		// Retrieve tags from GitLab
		tags, resp, err := c.Tags.ListTags(projectName, options, goGitlab.WithContext(ctx))
		if err != nil {
			return "", 0, err // Return if there's an error retrieving tags
		}

		c.requestsRemaining(resp) // Update the remaining requests count

		// Iterate through the tags and find the most recent matching tag
		for _, tag := range tags {
			if re.MatchString(tag.Name) {
				return tag.Commit.ShortID, float64(tag.Commit.CommittedDate.Unix()), nil // Return the commit short ID and timestamp
			}
		}

		// Break the loop if there are no more pages
		if resp.CurrentPage >= resp.NextPage {
			break
		}

		options.Page = resp.NextPage // Move to the next page
	}

	return "", 0, nil // Return empty values if no matching tag is found
}
