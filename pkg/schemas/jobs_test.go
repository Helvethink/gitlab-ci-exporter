package schemas

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
)

func mustJobFromJSON(t *testing.T, raw string) goGitlab.Job {
	t.Helper()

	var job goGitlab.Job
	err := json.Unmarshal([]byte(raw), &job)
	require.NoError(t, err)

	return job
}

func mustParseTime(t *testing.T, raw string) time.Time {
	t.Helper()

	ts, err := time.Parse(time.RFC3339, raw)
	require.NoError(t, err)

	return ts
}

func TestNewJob(t *testing.T) {
	gj := mustJobFromJSON(t, `{
		"id": 101,
		"name": "unit-tests",
		"stage": "test",
		"created_at": "2024-03-09T16:00:00Z",
		"duration": 42.5,
		"queued_duration": 3.2,
		"status": "success",
		"tag_list": ["docker", "linux"],
		"failure_reason": "",
		"pipeline": {
			"id": 999
		},
		"artifacts": [
			{"size": 100},
			{"size": 250},
			{"size": 50}
		],
		"runner": {
			"description": "shared-runner-1"
		}
	}`)

	expectedCreatedAt := mustParseTime(t, "2024-03-09T16:00:00Z")

	got := NewJob(gj)

	assert.Equal(t, int64(101), got.ID)
	assert.Equal(t, "unit-tests", got.Name)
	assert.Equal(t, "test", got.Stage)
	assert.Equal(t, float64(expectedCreatedAt.Unix()), got.Timestamp)
	assert.Equal(t, 42.5, got.DurationSeconds)
	assert.Equal(t, 3.2, got.QueuedDurationSeconds)
	assert.Equal(t, "success", got.Status)
	assert.Equal(t, int64(999), got.PipelineID)
	assert.Equal(t, "docker,linux", got.TagList)
	assert.Equal(t, 400.0, got.ArtifactSize)
	assert.Equal(t, "", got.FailureReason)
	assert.Equal(t, "shared-runner-1", got.Runner.Description)
}

func TestNewJob_NoCreatedAt(t *testing.T) {
	gj := mustJobFromJSON(t, `{
		"id": 102,
		"name": "build",
		"stage": "build",
		"duration": 10,
		"queued_duration": 1,
		"status": "running",
		"tag_list": ["kubernetes"],
		"pipeline": {
			"id": 1000
		},
		"artifacts": [
			{"size": 500}
		],
		"runner": {
			"description": "runner-k8s"
		}
	}`)

	got := NewJob(gj)

	assert.Equal(t, 0.0, got.Timestamp)
	assert.Equal(t, 500.0, got.ArtifactSize)
	assert.Equal(t, "kubernetes", got.TagList)
	assert.Equal(t, "runner-k8s", got.Runner.Description)
}

func TestNewJob_EmptyArtifactsAndTags(t *testing.T) {
	gj := mustJobFromJSON(t, `{
		"id": 103,
		"name": "deploy",
		"stage": "deploy",
		"created_at": "2024-03-09T16:01:40Z",
		"duration": 5,
		"queued_duration": 0.5,
		"status": "failed",
		"tag_list": [],
		"failure_reason": "script_failure",
		"pipeline": {
			"id": 1001
		},
		"artifacts": [],
		"runner": {
			"description": ""
		}
	}`)

	expectedCreatedAt := mustParseTime(t, "2024-03-09T16:01:40Z")

	got := NewJob(gj)

	assert.Equal(t, int64(103), got.ID)
	assert.Equal(t, "deploy", got.Name)
	assert.Equal(t, "deploy", got.Stage)
	assert.Equal(t, float64(expectedCreatedAt.Unix()), got.Timestamp)
	assert.Equal(t, 5.0, got.DurationSeconds)
	assert.Equal(t, 0.5, got.QueuedDurationSeconds)
	assert.Equal(t, "failed", got.Status)
	assert.Equal(t, int64(1001), got.PipelineID)
	assert.Equal(t, "", got.TagList)
	assert.Equal(t, 0.0, got.ArtifactSize)
	assert.Equal(t, "script_failure", got.FailureReason)
	assert.Equal(t, "", got.Runner.Description)
}

func TestNewJob_ArtifactSizeSum(t *testing.T) {
	gj := mustJobFromJSON(t, `{
		"pipeline": {
			"id": 1
		},
		"artifacts": [
			{"size": 1},
			{"size": 2},
			{"size": 3},
			{"size": 4}
		]
	}`)

	got := NewJob(gj)

	assert.Equal(t, 10.0, got.ArtifactSize)
}
