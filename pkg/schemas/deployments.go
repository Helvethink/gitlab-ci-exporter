package schemas

// Deployment represents a deployment structure with detailed information about a GitLab deployment.
type Deployment struct {
	JobID           int     // Unique identifier for the job associated with the deployment
	RefKind         RefKind // The kind of reference (e.g., branch, tag) associated with the deployment
	RefName         string  // The name of the reference associated with the deployment
	Username        string  // The username of the user who triggered the deployment
	Timestamp       float64 // Unix timestamp of when the deployment was created or updated
	DurationSeconds float64 // Duration of the deployment process in seconds
	CommitShortID   string  // Short identifier for the commit associated with the deployment
	Status          string  // Status of the deployment (e.g., success, failed)
}
