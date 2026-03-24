package schemas

import (
	"encoding/json"
	"hash/crc32"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerKey(t *testing.T) {
	r := Runner{
		ID:          123,
		ProjectName: "group/project",
		Description: "my-runner",
	}

	expected := RunnerKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte("123")))))
	assert.Equal(t, expected, r.Key())
}

func TestRunnerKey_SameIDSameKey(t *testing.T) {
	r1 := Runner{
		ID:          42,
		ProjectName: "group/project-a",
		Description: "runner-a",
	}
	r2 := Runner{
		ID:          42,
		ProjectName: "group/project-b",
		Description: "runner-b",
	}

	assert.Equal(t, r1.Key(), r2.Key())
}

func TestRunnerKey_DifferentIDDifferentKey(t *testing.T) {
	r1 := Runner{ID: 1}
	r2 := Runner{ID: 2}

	assert.NotEqual(t, r1.Key(), r2.Key())
}

func TestRunnersCount(t *testing.T) {
	runners := Runners{
		RunnerKey("1"): {ID: 1},
		RunnerKey("2"): {ID: 2},
		RunnerKey("3"): {ID: 3},
	}

	assert.Equal(t, 3, runners.Count())
}

func TestRunnerDefaultLabelsValues(t *testing.T) {
	r := Runner{
		ProjectName: "group/project",
		Description: "shared-runner-01",
	}

	got := r.DefaultLabelsValues()

	expected := map[string]string{
		"project":            "group/project",
		"runner_description": "shared-runner-01",
	}

	assert.Equal(t, expected, got)
}

func TestRunnerInformationLabelsValues(t *testing.T) {
	r := Runner{
		Paused:          true,
		Description:     "shared-runner-01",
		ID:              101,
		IsShared:        true,
		RunnerType:      "instance_type",
		Name:            "runner-name",
		Online:          true,
		Status:          "online",
		TagList:         []string{"docker", "linux", "amd64"},
		ProjectName:     "group/project",
		MaintenanceNote: "maintenance",
		Projects: []struct {
			ID                int
			Name              string
			NameWithNamespace string
			Path              string
			PathWithNamespace string
		}{
			{
				ID:                10,
				Name:              "project1",
				NameWithNamespace: "group/project1",
				Path:              "project1",
				PathWithNamespace: "group/project1",
			},
		},
		Groups: []struct {
			ID     int
			Name   string
			WebURL string
		}{
			{
				ID:     20,
				Name:   "group1",
				WebURL: "https://gitlab.example.com/groups/group1",
			},
		},
	}

	got := r.InformationLabelsValues()
	require.NotNil(t, got)

	expectedGroups, err := json.Marshal(r.Groups)
	require.NoError(t, err)

	expectedProjects, err := json.Marshal(r.Projects)
	require.NoError(t, err)

	assert.Equal(t, "group/project", got["project"])
	assert.Equal(t, "shared-runner-01", got["runner_description"])
	assert.Equal(t, "runner-name", got["runner_name"])
	assert.Equal(t, "101", got["runner_id"])
	assert.Equal(t, "true", got["is_shared"])
	assert.Equal(t, "instance_type", got["runner_type"])
	assert.Equal(t, "true", got["online"])
	assert.Equal(t, strings.Join(r.TagList, ","), got["tag_list"])
	assert.Equal(t, "true", got["active"]) // matches current implementation: active = Paused
	assert.Equal(t, "online", got["status"])
	assert.Equal(t, string(expectedGroups), got["runner_groups"])
	assert.Equal(t, string(expectedProjects), got["runner_projects"])
}

func TestRunnerInformationLabelsValues_EmptySlices(t *testing.T) {
	r := Runner{
		ID:          7,
		Description: "runner-empty",
		ProjectName: "group/empty",
		Name:        "runner-empty-name",
		Status:      "offline",
		TagList:     []string{},
		Projects:    nil,
		Groups:      nil,
	}

	got := r.InformationLabelsValues()
	require.NotNil(t, got)

	assert.Equal(t, "group/empty", got["project"])
	assert.Equal(t, "runner-empty", got["runner_description"])
	assert.Equal(t, "runner-empty-name", got["runner_name"])
	assert.Equal(t, "7", got["runner_id"])
	assert.Equal(t, "", got["tag_list"])
	assert.Equal(t, "offline", got["status"])
	assert.Equal(t, "null", got["runner_groups"])
	assert.Equal(t, "null", got["runner_projects"])
}
