package schemas

import (
	"hash/crc32"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
)

func TestProjectKey(t *testing.T) {
	p := Project{
		Project: config.Project{
			Name: "group/project",
		},
		Topics: "go,ci",
	}

	expected := ProjectKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte("group/project")))))
	assert.Equal(t, expected, p.Key())
}

func TestProjectKey_SameNameSameKey(t *testing.T) {
	p1 := Project{
		Project: config.Project{
			Name: "group/project",
		},
	}
	p2 := Project{
		Project: config.Project{
			Name: "group/project",
		},
		Topics: "different-topics",
	}

	assert.Equal(t, p1.Key(), p2.Key())
}

func TestProjectKey_DifferentNameDifferentKey(t *testing.T) {
	p1 := Project{
		Project: config.Project{
			Name: "group/project-a",
		},
	}
	p2 := Project{
		Project: config.Project{
			Name: "group/project-b",
		},
	}

	assert.NotEqual(t, p1.Key(), p2.Key())
}

func TestNewProject(t *testing.T) {
	p := NewProject("group/project")

	assert.Equal(t, "group/project", p.Name)
	assert.Equal(t, ProjectKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte("group/project"))))), p.Key())
}

func TestProjectsCount(t *testing.T) {
	projects := Projects{
		ProjectKey("1"): {
			Project: config.Project{Name: "group/project1"},
		},
		ProjectKey("2"): {
			Project: config.Project{Name: "group/project2"},
		},
		ProjectKey("3"): {
			Project: config.Project{Name: "group/project3"},
		},
	}

	assert.Equal(t, 3, projects.Count())
}

func TestProjectsCount_Empty(t *testing.T) {
	projects := Projects{}
	assert.Equal(t, 0, projects.Count())
}
