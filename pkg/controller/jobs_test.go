package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func TestProcessJobMetricsStoresJobMetrics(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	project := schemas.NewProject("group/project")
	project.Pull.Pipeline.Jobs.Enabled = true
	project.Pull.Pipeline.Jobs.RunnerDescription.Enabled = true
	project.Pull.Pipeline.Jobs.RunnerDescription.AggregationRegexp = `shared-runner-\d+`

	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")
	require.NoError(t, c.Store.SetRef(ctx, ref))

	job := schemas.Job{
		ID:                    101,
		Name:                  "build",
		Stage:                 "build",
		Timestamp:             1710000000,
		DurationSeconds:       10,
		QueuedDurationSeconds: 2,
		Status:                "success",
		PipelineID:            123,
		TagList:               "docker",
		ArtifactSize:          2048,
		FailureReason:         "",
		Runner: schemas.RunnerDesc{
			Description: "shared-runner-42",
		},
	}

	c.ProcessJobMetrics(ctx, ref, job)

	storedRef := schemas.NewRef(project, schemas.RefKindBranch, "main")
	require.NoError(t, c.Store.GetRef(ctx, &storedRef))
	require.Contains(t, storedRef.LatestJobs, "build")
	assert.Equal(t, job, storedRef.LatestJobs["build"])

	labels := map[string]string{
		"project":            "group/project",
		"topics":             "",
		"kind":               "branch",
		"ref":                "main",
		"source":             "",
		"variables":          "",
		"stage":              "build",
		"job_name":           "build",
		"runner_description": `shared-runner-\d+`,
		"tag_list":           "docker",
		"status":             "success",
		"job_id":             "101",
		"pipeline_id":        "123",
		"failure_reason":     "",
	}

	jobIDMetric := schemas.Metric{
		Kind:   schemas.MetricKindJobID,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &jobIDMetric))
	assert.Equal(t, float64(101), jobIDMetric.Value)

	jobRunCountMetric := schemas.Metric{
		Kind:   schemas.MetricKindJobRunCount,
		Labels: labels,
	}
	require.NoError(t, c.Store.GetMetric(ctx, &jobRunCountMetric))
	assert.Equal(t, float64(0), jobRunCountMetric.Value)

	jobStatusMetric := schemas.Metric{
		Kind: schemas.MetricKindJobStatus,
		Labels: map[string]string{
			"project":            "group/project",
			"topics":             "",
			"kind":               "branch",
			"ref":                "main",
			"source":             "",
			"variables":          "",
			"stage":              "build",
			"job_name":           "build",
			"runner_description": `shared-runner-\d+`,
			"tag_list":           "docker",
			"status":             "success",
			"job_id":             "101",
			"pipeline_id":        "123",
			"failure_reason":     "",
		},
	}
	require.NoError(t, c.Store.GetMetric(ctx, &jobStatusMetric))
	assert.Equal(t, float64(1), jobStatusMetric.Value)
}

func TestProcessJobMetricsDoesNotRewriteIdenticalJob(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	project := schemas.NewProject("group/project")
	project.Pull.Pipeline.Jobs.Enabled = true

	job := schemas.Job{
		ID:                    101,
		Name:                  "build",
		Stage:                 "build",
		Timestamp:             1710000000,
		DurationSeconds:       10,
		QueuedDurationSeconds: 2,
		Status:                "success",
		PipelineID:            123,
		TagList:               "docker",
		Runner: schemas.RunnerDesc{
			Description: "runner-1",
		},
	}

	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")
	ref.LatestJobs["build"] = job
	require.NoError(t, c.Store.SetRef(ctx, ref))

	c.ProcessJobMetrics(ctx, ref, job)

	metrics, err := c.Store.Metrics(ctx)
	require.NoError(t, err)
	assert.Len(t, metrics, 0)
}

func TestPullRefMostRecentJobsMetricsReturnsEarlyWhenDisabled(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s", r.URL.Path)
	})

	ref := schemas.NewRef(schemas.NewProject("group/project"), schemas.RefKindBranch, "main")
	ref.Project.Pull.Pipeline.Jobs.Enabled = false

	err := c.PullRefMostRecentJobsMetrics(ctx, ref)
	require.NoError(t, err)
}

func TestPullRefPipelineJobsMetrics(t *testing.T) {
	ctx := context.Background()
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")
		w.Header().Set("X-Total", "1")

		_, _ = w.Write([]byte(`[
			{
				"id": 101,
				"name": "build",
				"stage": "build",
				"created_at": "2024-03-09T16:00:00Z",
				"duration": 10,
				"queued_duration": 1,
				"status": "success",
				"ref": "main",
				"tag_list": ["docker"],
				"failure_reason": "",
				"pipeline": {"id": 123},
				"artifacts": [{"size": 512}],
				"runner": {"description": "runner-1"}
			}
		]`))
	})

	project := schemas.NewProject("group/project")
	project.Pull.Pipeline.Jobs.Enabled = true

	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")
	ref.LatestPipeline.ID = 123
	require.NoError(t, c.Store.SetRef(ctx, ref))

	err := c.PullRefPipelineJobsMetrics(ctx, ref)
	require.NoError(t, err)

	jobIDMetric := schemas.Metric{
		Kind: schemas.MetricKindJobID,
		Labels: map[string]string{
			"project":            "group/project",
			"topics":             "",
			"kind":               "branch",
			"ref":                "main",
			"source":             "",
			"variables":          "",
			"stage":              "build",
			"job_name":           "build",
			"runner_description": "runner-1",
			"tag_list":           "docker",
			"status":             "success",
			"job_id":             "101",
			"pipeline_id":        "123",
			"failure_reason":     "",
		},
	}
	require.NoError(t, c.Store.GetMetric(ctx, &jobIDMetric))
	assert.Equal(t, float64(101), jobIDMetric.Value)
}
