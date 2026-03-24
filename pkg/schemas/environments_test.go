package schemas

import (
	"hash/crc32"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentKey(t *testing.T) {
	e := Environment{
		ProjectName: "group/project",
		Name:        "production",
	}

	expected := EnvironmentKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte("group/project" + "production")))))
	assert.Equal(t, expected, e.Key())
}

func TestEnvironmentKey_SameInputSameKey(t *testing.T) {
	e1 := Environment{
		ProjectName: "group/project",
		Name:        "staging",
		ID:          1,
	}
	e2 := Environment{
		ProjectName: "group/project",
		Name:        "staging",
		ID:          999,
	}

	assert.Equal(t, e1.Key(), e2.Key())
}

func TestEnvironmentKey_DifferentInputDifferentKey(t *testing.T) {
	e1 := Environment{
		ProjectName: "group/project",
		Name:        "staging",
	}
	e2 := Environment{
		ProjectName: "group/project",
		Name:        "production",
	}

	assert.NotEqual(t, e1.Key(), e2.Key())
}

func TestEnvironmentsCount(t *testing.T) {
	envs := Environments{
		EnvironmentKey("1"): {ID: 1},
		EnvironmentKey("2"): {ID: 2},
		EnvironmentKey("3"): {ID: 3},
	}

	assert.Equal(t, 3, envs.Count())
}

func TestEnvironmentsCount_Empty(t *testing.T) {
	envs := Environments{}
	assert.Equal(t, 0, envs.Count())
}

func TestEnvironmentDefaultLabelsValues(t *testing.T) {
	e := Environment{
		ProjectName: "group/project",
		Name:        "production",
	}

	got := e.DefaultLabelsValues()

	expected := map[string]string{
		"project":     "group/project",
		"environment": "production",
	}

	assert.Equal(t, expected, got)
}

func TestEnvironmentInformationLabelsValues(t *testing.T) {
	e := Environment{
		ProjectName: "group/project",
		ID:          42,
		Name:        "production",
		ExternalURL: "https://example.com",
		Available:   true,
		LatestDeployment: Deployment{
			JobID:           1001,
			RefKind:         RefKindBranch,
			RefName:         "main",
			Username:        "jdoe",
			Timestamp:       1710000000,
			DurationSeconds: 12.5,
			CommitShortID:   "abc123",
			Status:          "success",
		},
	}

	got := e.InformationLabelsValues()

	expected := map[string]string{
		"project":                 "group/project",
		"environment":             "production",
		"environment_id":          "42",
		"external_url":            "https://example.com",
		"kind":                    "branch",
		"ref":                     "main",
		"current_commit_short_id": "abc123",
		"latest_commit_short_id":  "",
		"available":               "true",
		"username":                "jdoe",
	}

	assert.Equal(t, expected, got)
}

func TestEnvironmentInformationLabelsValues_WithZeroValues(t *testing.T) {
	e := Environment{
		ProjectName: "group/project",
		ID:          0,
		Name:        "staging",
		ExternalURL: "",
		Available:   false,
		LatestDeployment: Deployment{
			JobID:           0,
			RefKind:         "",
			RefName:         "",
			Username:        "",
			Timestamp:       0,
			DurationSeconds: 0,
			CommitShortID:   "",
			Status:          "",
		},
	}

	got := e.InformationLabelsValues()

	expected := map[string]string{
		"project":                 "group/project",
		"environment":             "staging",
		"environment_id":          "0",
		"external_url":            "",
		"kind":                    "",
		"ref":                     "",
		"current_commit_short_id": "",
		"latest_commit_short_id":  "",
		"available":               "false",
		"username":                "",
	}

	assert.Equal(t, expected, got)
}
