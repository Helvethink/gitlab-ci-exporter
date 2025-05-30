package gitlab

import (
	"context" // Package for managing context and cancellation
	"fmt"     // Package for formatted I/O operations

	log "github.com/sirupsen/logrus"               // Logging library
	goGitlab "gitlab.com/gitlab-org/api/client-go" // GitLab API client
	"go.openly.dev/pointy"                         // Package for creating pointers to values
	"go.opentelemetry.io/otel"                     // OpenTelemetry API for tracing
	"go.opentelemetry.io/otel/attribute"           // OpenTelemetry attribute package
)

// GetCommitCountBetweenRefs retrieves the number of commits between two references in a GitLab project.
func (c *Client) GetCommitCountBetweenRefs(ctx context.Context, project, from, to string) (int, error) {
	// Start a new OpenTelemetry span for tracing
	ctx, span := otel.Tracer(tracerName).Start(ctx, "gitlab:GetCommitCountBetweenRefs")
	defer span.End()

	// Set attributes for the span to provide context for tracing
	span.SetAttributes(attribute.String("project_name", project))
	span.SetAttributes(attribute.String("from_ref", from))
	span.SetAttributes(attribute.String("to_ref", to))

	// Log the comparison of references
	log.WithFields(log.Fields{
		"project-name": project,
		"from-ref":     from,
		"to-ref":       to,
	}).Debug("comparing refs")

	// Apply rate limiting to the request
	c.rateLimit(ctx)

	// Compare the two references using the GitLab API
	cmp, resp, err := c.Repositories.Compare(project, &goGitlab.CompareOptions{
		From:     &from,             // The source reference for comparison
		To:       &to,               // The target reference for comparison
		Straight: pointy.Bool(true), // Option to perform a straight comparison
	}, goGitlab.WithContext(ctx))

	// Handle any errors from the comparison
	if err != nil {
		return 0, err
	}

	// Update the remaining requests count based on the response
	c.requestsRemaining(resp)

	// Check if the comparison result is nil
	if cmp == nil {
		return 0, fmt.Errorf("could not compare refs successfully")
	}

	// Return the number of commits between the references
	return len(cmp.Commits), nil
}
