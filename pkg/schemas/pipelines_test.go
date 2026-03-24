package schemas

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
)

func TestPipelineKey(t *testing.T) {
	p := Pipeline{ID: 123}

	assert.Equal(t, PipelineKey(123), p.Key())
}

func TestNewPipeline_WithDetailedStatus(t *testing.T) {
	now := time.Unix(1710000000, 0)

	gp := goGitlab.Pipeline{
		ID:             42,
		Coverage:       "87.5",
		UpdatedAt:      &now,
		Duration:       120,
		QueuedDuration: 15,
		Source:         goGitlab.PipelineSource("push"),
		Status:         "success",
		DetailedStatus: &goGitlab.DetailedStatus{
			Group: "waiting-for-resource",
		},
	}

	got := NewPipeline(context.Background(), gp)

	assert.Equal(t, 42, got.ID)
	assert.Equal(t, 87.5, got.Coverage)
	assert.Equal(t, float64(now.Unix()), got.Timestamp)
	assert.Equal(t, 120.0, got.DurationSeconds)
	assert.Equal(t, 15.0, got.QueuedDurationSeconds)
	assert.Equal(t, "push", got.Source)
	assert.Equal(t, "waiting_for_resource", got.Status)
}

func TestNewPipeline_WithoutDetailedStatus_UsesStatus(t *testing.T) {
	now := time.Unix(1710000001, 0)

	gp := goGitlab.Pipeline{
		ID:             7,
		Coverage:       "92.1",
		UpdatedAt:      &now,
		Duration:       30,
		QueuedDuration: 5,
		Source:         goGitlab.PipelineSource("schedule"),
		Status:         "failed",
		DetailedStatus: nil,
	}

	got := NewPipeline(context.Background(), gp)

	assert.Equal(t, 7, got.ID)
	assert.Equal(t, 92.1, got.Coverage)
	assert.Equal(t, float64(now.Unix()), got.Timestamp)
	assert.Equal(t, 30.0, got.DurationSeconds)
	assert.Equal(t, 5.0, got.QueuedDurationSeconds)
	assert.Equal(t, "schedule", got.Source)
	assert.Equal(t, "failed", got.Status)
}

func TestNewPipeline_WithInvalidCoverage_ReturnsZeroCoverage(t *testing.T) {
	gp := goGitlab.Pipeline{
		ID:             9,
		Coverage:       "not-a-number",
		Duration:       10,
		QueuedDuration: 2,
		Source:         goGitlab.PipelineSource("web"),
		Status:         "success",
	}

	got := NewPipeline(context.Background(), gp)

	assert.Equal(t, 9, got.ID)
	assert.Equal(t, 0.0, got.Coverage)
	assert.Equal(t, 0.0, got.Timestamp)
	assert.Equal(t, 10.0, got.DurationSeconds)
	assert.Equal(t, 2.0, got.QueuedDurationSeconds)
	assert.Equal(t, "web", got.Source)
	assert.Equal(t, "success", got.Status)
}

func TestNewPipeline_WithEmptyCoverageAndNilUpdatedAt(t *testing.T) {
	gp := goGitlab.Pipeline{
		ID:             11,
		Coverage:       "",
		UpdatedAt:      nil,
		Duration:       0,
		QueuedDuration: 0,
		Source:         goGitlab.PipelineSource("api"),
		Status:         "running",
	}

	got := NewPipeline(context.Background(), gp)

	assert.Equal(t, 11, got.ID)
	assert.Equal(t, 0.0, got.Coverage)
	assert.Equal(t, 0.0, got.Timestamp)
	assert.Equal(t, 0.0, got.DurationSeconds)
	assert.Equal(t, 0.0, got.QueuedDurationSeconds)
	assert.Equal(t, "api", got.Source)
	assert.Equal(t, "running", got.Status)
}

func TestNewTestCase(t *testing.T) {
	gtc := &goGitlab.PipelineTestCases{
		Name:          "TestLogin",
		Classname:     "auth_test",
		ExecutionTime: 1.23,
		Status:        "success",
	}

	got := NewTestCase(gtc)

	assert.Equal(t, "TestLogin", got.Name)
	assert.Equal(t, "auth_test", got.Classname)
	assert.Equal(t, 1.23, got.ExecutionTime)
	assert.Equal(t, "success", got.Status)
}

func TestNewTestSuite(t *testing.T) {
	gts := &goGitlab.PipelineTestSuites{
		Name:         "suite-1",
		TotalTime:    12.5,
		TotalCount:   4,
		SuccessCount: 2,
		FailedCount:  1,
		SkippedCount: 1,
		ErrorCount:   0,
		TestCases: []*goGitlab.PipelineTestCases{
			{
				Name:          "TestA",
				Classname:     "suite_a",
				ExecutionTime: 0.5,
				Status:        "success",
			},
			{
				Name:          "TestB",
				Classname:     "suite_a",
				ExecutionTime: 0.7,
				Status:        "failed",
			},
		},
	}

	got := NewTestSuite(gts)

	assert.Equal(t, "suite-1", got.Name)
	assert.Equal(t, 12.5, got.TotalTime)
	assert.Equal(t, 4, got.TotalCount)
	assert.Equal(t, 2, got.SuccessCount)
	assert.Equal(t, 1, got.FailedCount)
	assert.Equal(t, 1, got.SkippedCount)
	assert.Equal(t, 0, got.ErrorCount)

	require.Len(t, got.TestCases, 2)
	assert.Equal(t, "TestA", got.TestCases[0].Name)
	assert.Equal(t, "suite_a", got.TestCases[0].Classname)
	assert.Equal(t, 0.5, got.TestCases[0].ExecutionTime)
	assert.Equal(t, "success", got.TestCases[0].Status)

	assert.Equal(t, "TestB", got.TestCases[1].Name)
	assert.Equal(t, "suite_a", got.TestCases[1].Classname)
	assert.Equal(t, 0.7, got.TestCases[1].ExecutionTime)
	assert.Equal(t, "failed", got.TestCases[1].Status)
}

func TestNewTestReport(t *testing.T) {
	gtr := goGitlab.PipelineTestReport{
		TotalTime:    20.0,
		TotalCount:   6,
		SuccessCount: 4,
		FailedCount:  1,
		SkippedCount: 1,
		ErrorCount:   0,
		TestSuites: []*goGitlab.PipelineTestSuites{
			{
				Name:         "suite-1",
				TotalTime:    10.0,
				TotalCount:   3,
				SuccessCount: 2,
				FailedCount:  1,
				SkippedCount: 0,
				ErrorCount:   0,
				TestCases: []*goGitlab.PipelineTestCases{
					{
						Name:          "TestA",
						Classname:     "suite1",
						ExecutionTime: 0.3,
						Status:        "success",
					},
				},
			},
			{
				Name:         "suite-2",
				TotalTime:    10.0,
				TotalCount:   3,
				SuccessCount: 2,
				FailedCount:  0,
				SkippedCount: 1,
				ErrorCount:   0,
				TestCases: []*goGitlab.PipelineTestCases{
					{
						Name:          "TestB",
						Classname:     "suite2",
						ExecutionTime: 0.9,
						Status:        "skipped",
					},
				},
			},
		},
	}

	got := NewTestReport(gtr)

	assert.Equal(t, 20.0, got.TotalTime)
	assert.Equal(t, 6, got.TotalCount)
	assert.Equal(t, 4, got.SuccessCount)
	assert.Equal(t, 1, got.FailedCount)
	assert.Equal(t, 1, got.SkippedCount)
	assert.Equal(t, 0, got.ErrorCount)

	require.Len(t, got.TestSuites, 2)

	assert.Equal(t, "suite-1", got.TestSuites[0].Name)
	require.Len(t, got.TestSuites[0].TestCases, 1)
	assert.Equal(t, "TestA", got.TestSuites[0].TestCases[0].Name)

	assert.Equal(t, "suite-2", got.TestSuites[1].Name)
	require.Len(t, got.TestSuites[1].TestCases, 1)
	assert.Equal(t, "TestB", got.TestSuites[1].TestCases[0].Name)
}
