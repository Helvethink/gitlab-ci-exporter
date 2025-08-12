package schemas

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strconv"
	"time"
)

type RunnerKey string

type Runner struct {
	Paused          bool
	Description     string
	ID              int
	IsShared        bool
	RunnerType      string
	ContactedAt     *time.Time
	MaintenanceNote string
	Name            string
	Online          bool
	Status          string
	Token           string
	TagList         []string
	RunUntagged     bool
	Locked          bool
	AccessLevel     string
	MaximumTimeout  int
	Projects        []struct {
		ID                int
		Name              string
		NameWithNamespace string
		Path              string
		PathWithNamespace string
	}
	Groups []struct {
		ID     int
		Name   string
		WebURL string
	}
	ProjectName               string
	OutputSparseStatusMetrics bool // Flag to determine if sparse status metrics should be output
}

type Runners map[RunnerKey]Runner

// Key generates a unique key for a Runner using a CRC32 checksum of the project name and runner name.
func (r Runner) Key() RunnerKey {
	// Generate a unique key using the CRC32 checksum of the project name and runner description
	return RunnerKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(strconv.Itoa(r.ID))))))
}

// Count returns the number of environments in the Environments map.
func (runners Runners) Count() int {
	return len(runners)
}

// DefaultLabelsValues returns a map of default label values for an Runners.
func (r Runner) DefaultLabelsValues() map[string]string {
	return map[string]string{
		"project":            r.ProjectName, // The name of the project
		"runner_description": r.Description, // The description of the runner
	}
}

// InformationLabelsValues returns a map of detailed label values for an Runner, including default labels.
func (r Runner) InformationLabelsValues() (v map[string]string) {
	// Start with the default label values
	v = r.DefaultLabelsValues()

	// Marshal Groups and projects
	groups := r.Groups
	GroupsOut, err := json.Marshal(groups)
	if err != nil {
		return nil
	}
	projects := r.Projects
	projectsOut, err := json.Marshal(projects)
	if err != nil {
		return nil
	}

	// Add additional detailed label values
	v["runner_name"] = r.Name                       // The name of the runner
	v["runner_id"] = strconv.Itoa(r.ID)             // The unique identifier for the environment
	v["is_shared"] = strconv.FormatBool(r.IsShared) // The kind of the latest deployment's reference
	v["runner_type"] = r.RunnerType                 // The name of the latest deployment's reference
	v["online"] = strconv.FormatBool(r.Online)      // The short ID of the current commit
	v["tag_list"] = fmt.Sprint(r.TagList)           // Placeholder for the latest commit short ID (empty in this context)
	v["active"] = strconv.FormatBool(r.Paused)      // The availability status of the environment
	v["status"] = r.Status                          // The status of the runner
	v["runner_groups"] = string(GroupsOut)          // The groups assigned to this runner
	v["runner_projects"] = string(projectsOut)      // The projects assigned to this runner

	fmt.Printf("Runner Labels:\n%v\n", v)

	return v
}
