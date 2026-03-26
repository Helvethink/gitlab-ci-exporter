package gitlab

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGitLabVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected GitLabVersion
	}{
		{
			name:     "empty version",
			input:    "",
			expected: GitLabVersion{Version: ""},
		},
		{
			name:     "already prefixed",
			input:    "v15.9.0",
			expected: GitLabVersion{Version: "v15.9.0"},
		},
		{
			name:     "without prefix",
			input:    "15.9.0",
			expected: GitLabVersion{Version: "v15.9.0"},
		},
		{
			name:     "major minor only",
			input:    "15.9",
			expected: GitLabVersion{Version: "v15.9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewGitLabVersion(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestPipelineJobsKeysetPaginationSupported(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		{
			name:     "empty version",
			version:  "",
			expected: false,
		},
		{
			name:     "below minimum version",
			version:  "v15.8.9",
			expected: false,
		},
		{
			name:     "exact minimum version",
			version:  "v15.9.0",
			expected: true,
		},
		{
			name:     "greater patch version",
			version:  "v15.9.1",
			expected: true,
		},
		{
			name:     "greater minor version",
			version:  "v15.10.0",
			expected: true,
		},
		{
			name:     "greater major version",
			version:  "v16.0.0",
			expected: true,
		},
		{
			name:     "without v prefix but normalized first",
			version:  "15.9.0",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewGitLabVersion(tt.version)
			assert.Equal(t, tt.expected, v.PipelineJobsKeysetPaginationSupported())
		})
	}
}

func TestPipelineJobsKeysetPaginationSupported_DirectVersionValue(t *testing.T) {
	v := GitLabVersion{Version: "v15.8.0"}
	assert.False(t, v.PipelineJobsKeysetPaginationSupported())

	v = GitLabVersion{Version: "v15.9.0"}
	assert.True(t, v.PipelineJobsKeysetPaginationSupported())
}
