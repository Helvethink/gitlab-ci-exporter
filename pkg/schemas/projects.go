package schemas

import (
	"hash/crc32" // For calculating CRC32 checksums
	"strconv"    // For string conversion operations

	"github.com/helvethink/gitlab-ci-exporter/pkg/config" // Project configuration package
)

// Project represents a project structure that extends a configuration project with additional fields.
type Project struct {
	config.Project // Embedding the Project type from the config package

	Topics string // Additional field to store topics related to the project
}

// ProjectKey is a custom type used as a key for identifying projects.
type ProjectKey string

// Projects is a map used to keep track of multiple projects, with ProjectKey as the key.
type Projects map[ProjectKey]Project

// Key generates a unique key for a Project using a CRC32 checksum of the project's name.
func (p Project) Key() ProjectKey {
	// Generate a unique key using the CRC32 checksum of the project's name
	return ProjectKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(p.Name)))))
}

// NewProject creates a new Project instance with the given name.
func NewProject(name string) Project {
	// Create a new Project by embedding a new config.Project and initializing any additional fields
	return Project{Project: config.NewProject(name)}
}
