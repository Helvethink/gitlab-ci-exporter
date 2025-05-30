package gitlab

import (
	"context" // Package for managing context and cancellation
	"fmt"     // Package for formatted I/O operations
	"regexp"  // Package for regular expression operations

	log "github.com/sirupsen/logrus"               // Logging library
	goGitlab "gitlab.com/gitlab-org/api/client-go" // GitLab API client
	"go.openly.dev/pointy"                         // Package for creating pointers to values
	"go.opentelemetry.io/otel"                     // OpenTelemetry API for tracing
	"go.opentelemetry.io/otel/attribute"           // OpenTelemetry attribute package

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"  // Configuration package
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas" // Schemas package
)

// GetProject retrieves a single project by its name from GitLab.
func (c *Client) GetProject(ctx context.Context, name string) (*goGitlab.Project, error) {
	// Start a new OpenTelemetry span for tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetProject")
	defer span.End()
	span.SetAttributes(attribute.String("project_name", name)) // Set project name as an attribute

	// Log the retrieval of the project
	log.WithFields(log.Fields{
		"project-name": name,
	}).Debug("reading project")

	// Apply rate limiting to the request
	c.rateLimit(ctx)

	// Retrieve the project from GitLab
	p, resp, err := c.Projects.GetProject(name, &goGitlab.GetProjectOptions{}, goGitlab.WithContext(ctx))
	c.requestsRemaining(resp) // Update the remaining requests count

	return p, err // Return the project and any error
}

// ListProjects lists projects based on a wildcard configuration.
func (c *Client) ListProjects(ctx context.Context, w config.Wildcard) ([]schemas.Project, error) {
	// Start a new OpenTelemetry span for tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:ListProjects")
	defer span.End()

	// Set attributes for the span to provide context for tracing
	span.SetAttributes(attribute.String("wildcard_search", w.Search))
	span.SetAttributes(attribute.String("wildcard_owner_kind", w.Owner.Kind))
	span.SetAttributes(attribute.String("wildcard_owner_name", w.Owner.Name))
	span.SetAttributes(attribute.Bool("wildcard_owner_include_subgroups", w.Owner.IncludeSubgroups))
	span.SetAttributes(attribute.Bool("wildcard_archived", w.Archived))

	// Log the listing of projects
	logFields := log.Fields{
		"wildcard-search":                  w.Search,
		"wildcard-owner-kind":              w.Owner.Kind,
		"wildcard-owner-name":              w.Owner.Name,
		"wildcard-owner-include-subgroups": w.Owner.IncludeSubgroups,
		"wildcard-archived":                w.Archived,
	}
	log.WithFields(logFields).Debug("listing all projects from wildcard")

	var projects []schemas.Project // Slice to store the resulting projects

	// Set up list options for pagination
	listOptions := goGitlab.ListOptions{
		Page:    1,   // Start from the first page
		PerPage: 100, // Number of projects per page
	}

	// Compile a regular expression to filter projects by owner
	var ownerRegexp *regexp.Regexp
	if len(w.Owner.Name) > 0 {
		ownerRegexp = regexp.MustCompile(fmt.Sprintf(`^%s\/`, w.Owner.Name))
	} else {
		ownerRegexp = regexp.MustCompile(`.*`)
	}

	// Loop through pages of projects
	for {
		var (
			gps  []*goGitlab.Project // Slice to store GitLab projects
			resp *goGitlab.Response  // Response from the GitLab API
			err  error               // Error variable
		)

		// Apply rate limiting to the request
		c.rateLimit(ctx)

		// Retrieve projects based on the owner kind
		switch w.Owner.Kind {
		case "user":
			// List projects for a user
			gps, resp, err = c.Projects.ListUserProjects(
				w.Owner.Name,
				&goGitlab.ListProjectsOptions{
					Archived:    &w.Archived,
					ListOptions: listOptions,
					Search:      &w.Search,
					Simple:      pointy.Bool(true),
				},
				goGitlab.WithContext(ctx),
			)
		case "group":
			// List projects for a group
			gps, resp, err = c.Groups.ListGroupProjects(
				w.Owner.Name,
				&goGitlab.ListGroupProjectsOptions{
					Archived:         &w.Archived,
					WithShared:       pointy.Bool(false),
					IncludeSubGroups: &w.Owner.IncludeSubgroups,
					ListOptions:      listOptions,
					Search:           &w.Search,
					Simple:           pointy.Bool(true),
				},
				goGitlab.WithContext(ctx),
			)
		default:
			// List all visible projects
			gps, resp, err = c.Projects.ListProjects(
				&goGitlab.ListProjectsOptions{
					ListOptions: listOptions,
					Archived:    &w.Archived,
					Search:      &w.Search,
					Simple:      pointy.Bool(true),
				},
				goGitlab.WithContext(ctx),
			)
		}

		// Handle any errors from retrieving projects
		if err != nil {
			return projects, fmt.Errorf("unable to list projects with search pattern '%s' from the GitLab API: %v", w.Search, err.Error())
		}

		c.requestsRemaining(resp) // Update the remaining requests count

		// Copy relevant settings from wildcard into created project
		for _, gp := range gps {
			if !ownerRegexp.MatchString(gp.PathWithNamespace) {
				// Skip projects that do not match the owner's name
				log.WithFields(logFields).WithFields(log.Fields{
					"project-id":   gp.ID,
					"project-name": gp.PathWithNamespace,
				}).Debug("project path not matching owner's name, skipping")
				continue
			}

			// Create a new project and append it to the projects slice
			p := schemas.NewProject(gp.PathWithNamespace)
			p.ProjectParameters = w.ProjectParameters
			projects = append(projects, p)
		}

		// Break the loop if there are no more pages
		if resp.CurrentPage >= resp.NextPage {
			break
		}

		// Move to the next page
		listOptions.Page = resp.NextPage
	}

	return projects, nil // Return the list of projects and any error
}
