package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// pipelineVariableKeyPattern matches the characters GitLab accepts in CI/CD variable keys.
var pipelineVariableKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// PipelineVariableKeys is an allowlist of pipeline variable keys.
//
// It is stored as a canonical (sorted, de-duplicated, comma-joined) string so that
// configuration structs embedding it remain comparable and usable as map keys.
type PipelineVariableKeys struct {
	joined string
}

// NewPipelineVariableKeys builds an allowlist from the given keys.
// It returns an error if a key is not a valid GitLab variable key.
func NewPipelineVariableKeys(keys ...string) (PipelineVariableKeys, error) {
	normalized := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if !pipelineVariableKeyPattern.MatchString(k) {
			return PipelineVariableKeys{}, fmt.Errorf("invalid pipeline variable key %q: only letters, digits and '_' are allowed", k)
		}
		normalized = append(normalized, k)
	}
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)

	return PipelineVariableKeys{joined: strings.Join(normalized, ",")}, nil
}

// List returns the allowlisted keys in sorted order.
func (k PipelineVariableKeys) List() []string {
	if k.joined == "" {
		return nil
	}
	return strings.Split(k.joined, ",")
}

// Contains reports whether key is allowlisted.
func (k PipelineVariableKeys) Contains(key string) bool {
	_, found := slices.BinarySearch(k.List(), key)
	return found
}

// UnmarshalYAML decodes a YAML sequence of variable keys.
func (k *PipelineVariableKeys) UnmarshalYAML(v *yaml.Node) error {
	var keys []string
	if err := v.Decode(&keys); err != nil {
		return fmt.Errorf("pipeline variable keys must be a list of strings: %w", err)
	}
	parsed, err := NewPipelineVariableKeys(keys...)
	if err != nil {
		return err
	}
	*k = parsed
	return nil
}

// MarshalYAML encodes the keys as a YAML sequence.
func (k PipelineVariableKeys) MarshalYAML() (any, error) {
	return k.List(), nil
}
