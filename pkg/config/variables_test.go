package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPipelineVariablesDefaultDeny(t *testing.T) {
	p := NewProject("group/project")

	assert.False(t, p.Pull.Pipeline.Variables.Enabled)
	assert.Empty(t, p.Pull.Pipeline.Variables.Regexp)
	assert.Empty(t, p.Pull.Pipeline.Variables.Keys.List())
}

func TestNewPipelineVariableKeys(t *testing.T) {
	keys, err := NewPipelineVariableKeys("RELEASE", " DEPLOY_ENV ", "RELEASE")
	require.NoError(t, err)
	assert.Equal(t, []string{"DEPLOY_ENV", "RELEASE"}, keys.List())
	assert.True(t, keys.Contains("RELEASE"))
	assert.False(t, keys.Contains("API_TOKEN"))

	for _, invalid := range []string{"", "A:B", "A,B", "A B", "A-B"} {
		_, err := NewPipelineVariableKeys(invalid)
		assert.Error(t, err, invalid)
	}
}

func TestPipelineVariableKeysYAML(t *testing.T) {
	var v ProjectPullPipelineVariables
	require.NoError(t, yaml.Unmarshal([]byte("enabled: true\nkeys: [RELEASE, DEPLOY_ENV]\n"), &v))
	assert.True(t, v.Enabled)
	assert.Equal(t, []string{"DEPLOY_ENV", "RELEASE"}, v.Keys.List())

	out, err := yaml.Marshal(v)
	require.NoError(t, err)
	assert.Contains(t, string(out), "- DEPLOY_ENV\n")

	assert.Error(t, yaml.Unmarshal([]byte("keys: [\"BAD:KEY\"]\n"), &v))
	assert.Error(t, yaml.Unmarshal([]byte("keys: RELEASE\n"), &v))
}

func TestPipelineVariablesProjectsRemainComparable(t *testing.T) {
	keys, err := NewPipelineVariableKeys("RELEASE")
	require.NoError(t, err)

	a := NewProject("group/project")
	a.Pull.Pipeline.Variables.Keys = keys
	b := NewProject("group/project")
	b.Pull.Pipeline.Variables.Keys = keys

	m := map[Project]struct{}{a: {}}
	_, found := m[b]
	assert.True(t, found)
}
