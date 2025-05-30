package gitlab

import (
	"strings" // Package for string manipulation

	"golang.org/x/mod/semver" // Package for semantic version comparison
)

// GitLabVersion represents a GitLab version with additional methods for version comparison.
type GitLabVersion struct {
	Version string // The version string of GitLab
}

// NewGitLabVersion creates a new GitLabVersion instance.
// It ensures the version string is prefixed with "v".
func NewGitLabVersion(version string) GitLabVersion {
	ver := ""
	if strings.HasPrefix(version, "v") {
		// If the version string already has a "v" prefix, use it as is
		ver = version
	} else if version != "" {
		// If the version string does not have a "v" prefix, add it
		ver = "v" + version
	}

	// Return a new GitLabVersion instance with the processed version string
	return GitLabVersion{Version: ver}
}

// PipelineJobsKeysetPaginationSupported checks if the GitLab instance version
// supports pipeline jobs keyset pagination, which is available in version 15.9 or later.
func (v GitLabVersion) PipelineJobsKeysetPaginationSupported() bool {
	// Return false if the version string is empty
	if v.Version == "" {
		return false
	}

	// Compare the version with "v15.9.0" and return true if it is greater or equal
	return semver.Compare(v.Version, "v15.9.0") >= 0
}
