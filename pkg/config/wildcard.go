package config

import (
	"github.com/creasty/defaults"
)

// Wildcard is a specific handler used to dynamically search for projects.
// It includes search criteria and filtering options to discover projects
// based on certain parameters.
//
// Fields:
//   - ProjectParameters: Embedded struct holding specific parameters related
//     to projects that will be discovered using this wildcard.
//   - Search: A search string used to match project names or attributes.
//   - Owner: Specifies the owner of the projects, including the owner's name,
//     kind (e.g., user or group), and whether to include subgroups in the search.
//   - Archived: A boolean flag indicating whether to include archived projects
//     in the search results.
type Wildcard struct {
	ProjectParameters `yaml:",inline"`

	Search   string        `yaml:"search"`
	Owner    WildcardOwner `yaml:"owner"`
	Archived bool          `yaml:"archived"`
}

// WildcardOwner contains information about the owner of the projects in the
// wildcard search filter.
//
// Fields:
//   - Name: The name of the owner (user or group).
//   - Kind: The type of the owner, e.g., "user" or "group".
//   - IncludeSubgroups: Whether to include subgroups under this owner in the search.
type WildcardOwner struct {
	Name             string `yaml:"name"`
	Kind             string `yaml:"kind"`
	IncludeSubgroups bool   `yaml:"include_subgroups"`
}

// Wildcards is a slice of Wildcard structs, representing multiple project search filters.
type Wildcards []Wildcard

// NewWildcard returns a new Wildcard instance initialized with default parameters.
// It uses the defaults.MustSet function to set default values on the Wildcard.
func NewWildcard() (w Wildcard) {
	defaults.MustSet(&w)

	return
}
