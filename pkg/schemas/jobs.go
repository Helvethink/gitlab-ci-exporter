package schemas

import (
	"strings" // For string manipulation operations

	goGitlab "gitlab.com/gitlab-org/api/client-go" // GitLab API client
)

// Job represents a job structure with detailed information about a GitLab CI/CD job.
type Job struct {
	ID                    int        // Unique identifier for the job
	Name                  string     // Name of the job
	Stage                 string     // Stage of the job in the pipeline
	Timestamp             float64    // Unix timestamp of when the job was created
	DurationSeconds       float64    // Duration of the job execution in seconds
	QueuedDurationSeconds float64    // Duration the job was queued in seconds
	Status                string     // Status of the job
	PipelineID            int        // PipelineID of the job
	TagList               string     // Comma-separated list of tags associated with the job
	ArtifactSize          float64    // Total size of artifacts produced by the job in bytes
	FailureReason         string     // Reason for failure if the job failed
	Runner                RunnerDesc // Runner information associated with the job
}

// RunnerDesc represents information about a GitLab runner.
type RunnerDesc struct {
	Description string // Description of the runner
}

// Jobs is a map used to keep track of multiple jobs, with job names as the keys.
type Jobs map[string]Job

// NewJob creates a new Job instance from a GitLab job object.
func NewJob(gj goGitlab.Job) Job {
	var (
		artifactSize float64 // Variable to store the total size of artifacts
		timestamp    float64 // Variable to store the timestamp
	)

	// Calculate the total size of all artifacts
	for _, artifact := range gj.Artifacts {
		artifactSize += float64(artifact.Size)
	}

	// Convert the CreatedAt timestamp to a float64 Unix timestamp if it is not nil
	if gj.CreatedAt != nil {
		timestamp = float64(gj.CreatedAt.Unix())
	}

	// Create a new Job instance with the parsed data
	return Job{
		ID:                    gj.ID,
		Name:                  gj.Name,
		Stage:                 gj.Stage,
		Timestamp:             timestamp,
		DurationSeconds:       gj.Duration,
		QueuedDurationSeconds: gj.QueuedDuration,
		Status:                gj.Status,
		PipelineID:            gj.Pipeline.ID,
		TagList:               strings.Join(gj.TagList, ","), // Join the tag list into a comma-separated string
		ArtifactSize:          artifactSize,
		FailureReason:         gj.FailureReason,

		Runner: RunnerDesc{
			Description: gj.Runner.Description, // Set the runner description
		},
	}
}
