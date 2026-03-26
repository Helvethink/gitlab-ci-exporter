package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
)

func TestPrettyTimeago(t *testing.T) {
	assert.Equal(t, "N/A", prettyTimeago(time.Time{}))
	assert.NotEqual(t, "N/A", prettyTimeago(time.Now().Add(-time.Minute)))
}

func TestRenderEntity(t *testing.T) {
	now := time.Unix(1710000000, 0)

	got := renderEntity("Projects", &pb.Entity{
		Count:    3,
		LastPull: timestamppb.New(now),
		LastGc:   timestamppb.New(now.Add(-time.Minute)),
		NextPull: timestamppb.New(now.Add(time.Minute)),
		NextGc:   timestamppb.New(now.Add(2 * time.Minute)),
	})

	assert.Contains(t, got, "Projects")
	assert.Contains(t, got, "Total")
	assert.Contains(t, got, "3")
	assert.Contains(t, got, "Last Pull")
	assert.Contains(t, got, "Next GC")
}

func TestRenderTelemetryViewportWithoutTelemetry(t *testing.T) {
	m := &model{}

	assert.Equal(t, "\nloading data..", m.renderTelemetryViewport())
}

func TestRenderTelemetryViewport(t *testing.T) {
	p := progress.New(progress.WithScaledGradient("#80c904", "#ff9d5c"))
	now := time.Unix(1710000000, 0)

	m := &model{
		progress: &p,
		telemetry: &pb.Telemetry{
			GitlabApiUsage:          0.5,
			GitlabApiRequestsCount:  7,
			GitlabApiRateLimit:      0.25,
			GitlabApiLimitRemaining: 5,
			TasksBufferUsage:        0.1,
			TasksExecutedCount:      9,
			Projects: &pb.Entity{
				Count:    1,
				LastPull: timestamppb.New(now),
				LastGc:   timestamppb.New(now),
				NextPull: timestamppb.New(now),
				NextGc:   timestamppb.New(now),
			},
			Envs: &pb.Entity{
				Count:    2,
				LastPull: timestamppb.New(now),
				LastGc:   timestamppb.New(now),
				NextPull: timestamppb.New(now),
				NextGc:   timestamppb.New(now),
			},
			Refs: &pb.Entity{
				Count:    3,
				LastPull: timestamppb.New(now),
				LastGc:   timestamppb.New(now),
				NextPull: timestamppb.New(now),
				NextGc:   timestamppb.New(now),
			},
			Metrics: &pb.Entity{
				Count:    4,
				LastPull: timestamppb.New(now),
				LastGc:   timestamppb.New(now),
				NextPull: timestamppb.New(now),
				NextGc:   timestamppb.New(now),
			},
		},
	}

	got := m.renderTelemetryViewport()

	assert.Contains(t, got, "GitLab API usage")
	assert.Contains(t, got, "GitLab API requests")
	assert.Contains(t, got, "Tasks executed")
	assert.Contains(t, got, "Projects")
	assert.Contains(t, got, "Environments")
	assert.Contains(t, got, "Refs")
	assert.Contains(t, got, "Metrics")
	assert.Contains(t, got, "7")
	assert.Contains(t, got, "9")
}
