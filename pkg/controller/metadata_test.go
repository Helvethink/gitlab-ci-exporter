package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitlabclient "github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
)

func TestGetGitLabMetadataUpdatesVersion(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v4/metadata", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"16.11.2"}`))
	})

	err := c.GetGitLabMetadata(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v16.11.2", c.Gitlab.Version().Version)
}

func TestGetGitLabMetadataKeepsCurrentVersionWhenMetadataVersionIsEmpty(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v4/metadata", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":""}`))
	})
	c.Gitlab.UpdateVersion(gitlabclient.NewGitLabVersion("15.9.0"))

	err := c.GetGitLabMetadata(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v15.9.0", c.Gitlab.Version().Version)
}

func TestGetGitLabMetadataPropagatesGitLabError(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v4/metadata", r.URL.Path)
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	err := c.GetGitLabMetadata(ctx)
	require.Error(t, err)
}
