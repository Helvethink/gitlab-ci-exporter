package controller

import (
	"context"

	"github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
)

// GetGitLabMetadata retrieves metadata from the GitLab API and updates the local GitLab version if available.
func (c *Controller) GetGitLabMetadata(ctx context.Context) error {
	// Prepare request options with context for cancellation, deadlines, etc.
	options := []goGitlab.RequestOptionFunc{goGitlab.WithContext(ctx)}

	// Call GitLab's metadata API endpoint to get version and other metadata
	metadata, _, err := c.Gitlab.Metadata.GetMetadata(options...)
	if err != nil {
		// If an error occurs while fetching metadata, return the error
		return err
	}

	// If the GitLab version is present in the metadata, update the local version
	if metadata.Version != "" {
		c.Gitlab.UpdateVersion(gitlab.NewGitLabVersion(metadata.Version))
	}

	// No errors occurred; return nil
	return nil
}
