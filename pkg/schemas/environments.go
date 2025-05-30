package schemas

import (
	"hash/crc32" // For calculating CRC32 checksums
	"strconv"    // For string conversion operations
)

// Environment represents an environment structure with detailed information about a GitLab environment.
type Environment struct {
	ProjectName      string     // Name of the project the environment belongs to
	ID               int        // Unique identifier for the environment
	Name             string     // Name of the environment
	ExternalURL      string     // External URL associated with the environment
	Available        bool       // Availability status of the environment
	LatestDeployment Deployment // Latest deployment information for the environment

	OutputSparseStatusMetrics bool // Flag to determine if sparse status metrics should be output
}

// EnvironmentKey is a custom type used as a key for identifying environments.
type EnvironmentKey string

// Key generates a unique key for an Environment using a CRC32 checksum of the project name and environment name.
func (e Environment) Key() EnvironmentKey {
	// Generate a unique key using the CRC32 checksum of the project name and environment name
	return EnvironmentKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(e.ProjectName + e.Name)))))
}

// Environments is a map used to keep track of multiple environments, with EnvironmentKey as the key.
type Environments map[EnvironmentKey]Environment

// Count returns the number of environments in the Environments map.
func (envs Environments) Count() int {
	return len(envs)
}

// DefaultLabelsValues returns a map of default label values for an Environment.
func (e Environment) DefaultLabelsValues() map[string]string {
	return map[string]string{
		"project":     e.ProjectName, // The name of the project
		"environment": e.Name,        // The name of the environment
	}
}

// InformationLabelsValues returns a map of detailed label values for an Environment, including default labels.
func (e Environment) InformationLabelsValues() (v map[string]string) {
	// Start with the default label values
	v = e.DefaultLabelsValues()

	// Add additional detailed label values
	v["environment_id"] = strconv.Itoa(e.ID)                        // The unique identifier for the environment
	v["external_url"] = e.ExternalURL                               // The external URL associated with the environment
	v["kind"] = string(e.LatestDeployment.RefKind)                  // The kind of the latest deployment's reference
	v["ref"] = e.LatestDeployment.RefName                           // The name of the latest deployment's reference
	v["current_commit_short_id"] = e.LatestDeployment.CommitShortID // The short ID of the current commit
	v["latest_commit_short_id"] = ""                                // Placeholder for the latest commit short ID (empty in this context)
	v["available"] = strconv.FormatBool(e.Available)                // The availability status of the environment
	v["username"] = e.LatestDeployment.Username                     // The username associated with the latest deployment

	return v
}
