package schemas

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
	"time"
)

type RunnerKey string

type Runner struct {
	Paused          bool
	Description     string
	ID              int64
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
	MaximumTimeout  int64
	Projects        []struct {
		ID                int64
		Name              string
		NameWithNamespace string
		Path              string
		PathWithNamespace string
	}
	Groups []struct {
		ID     int64
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
	return RunnerKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(strconv.FormatInt(r.ID, 10))))))
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
	groupNames := make([]string, 0, len(r.Groups))
	for _, g := range r.Groups {
		if g.Name != "" {
			groupNames = append(groupNames, g.Name)
		}
	}

	projectNames := make([]string, 0, len(r.Projects))
	for _, p := range r.Projects {
		switch {
		case p.PathWithNamespace != "":
			projectNames = append(projectNames, p.PathWithNamespace)
		case p.NameWithNamespace != "":
			projectNames = append(projectNames, p.NameWithNamespace)
		case p.Name != "":
			projectNames = append(projectNames, p.Name)
		}
	}

	tags := strings.Join(r.TagList, ",")

	// Add additional detailed label values
	v["runner_name"] = r.Name                              // The name of the runner
	v["runner_id"] = strconv.FormatInt(r.ID, 10)           // The unique identifier for the environment
	v["is_shared"] = strconv.FormatBool(r.IsShared)        // The kind of the latest deployment's reference
	v["runner_type"] = r.RunnerType                        // The name of the latest deployment's reference
	v["online"] = strconv.FormatBool(r.Online)             // The short ID of the current commit
	v["tag_list"] = tags                                   // Placeholder for the latest commit short ID (empty in this context)
	v["active"] = strconv.FormatBool(r.Paused)             // The availability status of the environment
	v["status"] = r.Status                                 // The status of the runner
	v["runner_groups"] = strings.Join(groupNames, ",")     // The groups assigned to this runner
	v["runner_projects"] = strings.Join(projectNames, ",") // The projects assigned to this runner

	fmt.Printf("Runner Labels:\n%v\n", v)

	return v
}
