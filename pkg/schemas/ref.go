package schemas

import (
	"fmt"
	"hash/crc32" // For calculating CRC32 checksums
	"regexp"     // For regular expression operations
	"strconv"    // For string conversion operations

	"github.com/helvethink/gitlab-ci-exporter/pkg/config" // Project configuration
)

const (
	mergeRequestRegexp string = `^((\d+)|refs/merge-requests/(\d+)/(?:head|merge))$` // Regex pattern for matching merge request references

	// RefKindBranch refers to a branch reference kind.
	RefKindBranch RefKind = "branch"

	// RefKindTag refers to a tag reference kind.
	RefKindTag RefKind = "tag"

	// RefKindMergeRequest refers to a merge request reference kind.
	RefKindMergeRequest RefKind = "merge-request"
)

// RefKind is a custom type used to determine the kind of reference.
type RefKind string

// Ref represents a reference entity on which metrics operations will be performed.
type Ref struct {
	Kind           RefKind  // The kind of the reference (branch, tag, merge-request)
	Name           string   // The name of the reference
	Project        Project  // The project associated with the reference
	LatestPipeline Pipeline // The latest pipeline associated with the reference
	LatestJobs     Jobs     // The latest jobs associated with the reference
}

// RefKey is a custom type used as a key for references.
type RefKey string

// Key generates a unique key for a Ref using a CRC32 checksum.
func (ref Ref) Key() RefKey {
	// Generate a unique key using the CRC32 checksum of the ref's kind, project name, and ref name
	return RefKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(string(ref.Kind) + ref.Project.Name + ref.Name)))))
}

// Refs is a map used to keep track of all configured or discovered references.
type Refs map[RefKey]Ref

// Count returns the number of references in the Refs map.
func (refs Refs) Count() int {
	return len(refs)
}

// DefaultLabelsValues returns a map of default label values for a Ref.
func (ref Ref) DefaultLabelsValues(input ...Pipeline) map[string]string {
	var pipeline Pipeline
	if len(input) > 0 {
		pipeline = input[0]
	} else {
		pipeline = ref.LatestPipeline
	}
	return map[string]string{
		"kind":    string(ref.Kind),   // The kind of the reference
		"project": ref.Project.Name,   // The name of the project
		"ref":     ref.Name,           // The name of the reference
		"topics":  ref.Project.Topics, // The topics associated with the project
		//"variables": ref.LatestPipeline.Variables, // The variables associated with the latest pipeline
		//"source":    ref.LatestPipeline.Source,    // The source of the latest pipeline
		"variables": pipeline.Variables, // The variables associated with the latest pipeline
		"source":    pipeline.Source,    // The source of the latest pipeline
	}
}

// NewRef is a helper function that returns a new Ref.
func NewRef(
	project Project, // The project associated with the reference
	kind RefKind, // The kind of the reference
	name string, // The name of the reference
) Ref {
	return Ref{
		Kind:       kind,
		Name:       name,
		Project:    project,
		LatestJobs: make(Jobs), // Initialize an empty map for the latest jobs
	}
}

// GetRefRegexp returns the expected regular expression for a given ProjectPullRefs config and RefKind.
func GetRefRegexp(ppr config.ProjectPullRefs, rk RefKind) (re *regexp.Regexp, err error) {
	switch rk {
	case RefKindBranch:
		return regexp.Compile(ppr.Branches.Regexp) // Compile the regex for branches
	case RefKindTag:
		return regexp.Compile(ppr.Tags.Regexp) // Compile the regex for tags
	case RefKindMergeRequest:
		return regexp.Compile(mergeRequestRegexp) // Compile the regex for merge requests
	}

	return nil, fmt.Errorf("invalid ref kind (%v)", rk) // Return an error for invalid ref kinds
}

// GetMergeRequestIIDFromRefName parses a refName to extract a merge request IID (Internal ID).
func GetMergeRequestIIDFromRefName(refName string) (string, error) {
	re := regexp.MustCompile(mergeRequestRegexp) // Compile the regex for matching merge request references
	if matches := re.FindStringSubmatch(refName); len(matches) == 4 {
		if len(matches[2]) > 0 {
			return matches[2], nil // Return the IID if found in the second capture group
		}

		if len(matches[3]) > 0 {
			return matches[3], nil // Return the IID if found in the third capture group
		}
	}

	return refName, fmt.Errorf("unable to extract the merge-request ID from the ref (%s)", refName) // Return an error if unable to extract the IID
}
