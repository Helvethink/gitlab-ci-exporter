package schemas

import (
	"hash/crc32"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
)

func TestRefKey(t *testing.T) {
	project := Project{
		Project: config.Project{
			Name: "group/project",
		},
	}
	ref := Ref{
		Kind:    RefKindBranch,
		Name:    "main",
		Project: project,
	}

	expected := RefKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(string(RefKindBranch) + project.Name + "main")))))
	assert.Equal(t, expected, ref.Key())
}

func TestRefKey_SameInputSameKey(t *testing.T) {
	project := Project{
		Project: config.Project{
			Name: "group/project",
		},
	}
	ref1 := Ref{
		Kind:    RefKindTag,
		Name:    "v1.0.0",
		Project: project,
	}
	ref2 := Ref{
		Kind:    RefKindTag,
		Name:    "v1.0.0",
		Project: project,
	}

	assert.Equal(t, ref1.Key(), ref2.Key())
}

func TestRefKey_DifferentInputDifferentKey(t *testing.T) {
	project := Project{
		Project: config.Project{
			Name: "group/project",
		},
	}

	ref1 := Ref{
		Kind:    RefKindBranch,
		Name:    "main",
		Project: project,
	}
	ref2 := Ref{
		Kind:    RefKindBranch,
		Name:    "develop",
		Project: project,
	}

	assert.NotEqual(t, ref1.Key(), ref2.Key())
}

func TestRefsCount(t *testing.T) {
	refs := Refs{
		RefKey("1"): {},
		RefKey("2"): {},
		RefKey("3"): {},
	}

	assert.Equal(t, 3, refs.Count())
}

func TestRefDefaultLabelsValues_UsesLatestPipelineByDefault(t *testing.T) {
	ref := Ref{
		Kind: RefKindBranch,
		Name: "main",
		Project: Project{
			Project: config.Project{
				Name: "group/project",
			},
			Topics: "go,ci",
		},
		LatestPipeline: Pipeline{
			Variables: `{"foo":"bar"}`,
			Source:    "push",
		},
	}

	got := ref.DefaultLabelsValues()

	expected := map[string]string{
		"kind":      "branch",
		"project":   "group/project",
		"ref":       "main",
		"topics":    "go,ci",
		"variables": `{"foo":"bar"}`,
		"source":    "push",
	}

	assert.Equal(t, expected, got)
}

func TestRefDefaultLabelsValues_UsesInputPipelineWhenProvided(t *testing.T) {
	ref := Ref{
		Kind: RefKindTag,
		Name: "v1.2.3",
		Project: Project{
			Project: config.Project{
				Name: "group/project",
			},
			Topics: "release",
		},
		LatestPipeline: Pipeline{
			Variables: `{"from":"latest"}`,
			Source:    "schedule",
		},
	}

	override := Pipeline{
		Variables: `{"from":"override"}`,
		Source:    "web",
	}

	got := ref.DefaultLabelsValues(override)

	expected := map[string]string{
		"kind":      "tag",
		"project":   "group/project",
		"ref":       "v1.2.3",
		"topics":    "release",
		"variables": `{"from":"override"}`,
		"source":    "web",
	}

	assert.Equal(t, expected, got)
}

func TestNewRef(t *testing.T) {
	project := Project{
		Project: config.Project{
			Name: "group/project",
		},
	}

	ref := NewRef(project, RefKindMergeRequest, "123")

	assert.Equal(t, RefKindMergeRequest, ref.Kind)
	assert.Equal(t, "123", ref.Name)
	assert.Equal(t, project, ref.Project)
	assert.NotNil(t, ref.LatestJobs)
	assert.Equal(t, 0, len(ref.LatestJobs))
}

func TestGetRefRegexp_Branch(t *testing.T) {
	var ppr config.ProjectPullRefs
	ppr.Branches.Regexp = "^main|develop$"

	re, err := GetRefRegexp(ppr, RefKindBranch)
	require.NoError(t, err)
	require.NotNil(t, re)

	assert.True(t, re.MatchString("main"))
	assert.True(t, re.MatchString("develop"))
	assert.False(t, re.MatchString("feature/test"))
}

func TestGetRefRegexp_Tag(t *testing.T) {
	var ppr config.ProjectPullRefs
	ppr.Tags.Regexp = "^v[0-9]+\\.[0-9]+\\.[0-9]+$"

	re, err := GetRefRegexp(ppr, RefKindTag)
	require.NoError(t, err)
	require.NotNil(t, re)

	assert.True(t, re.MatchString("v1.2.3"))
	assert.False(t, re.MatchString("main"))
}

func TestGetRefRegexp_MergeRequest(t *testing.T) {
	var ppr config.ProjectPullRefs

	re, err := GetRefRegexp(ppr, RefKindMergeRequest)
	require.NoError(t, err)
	require.NotNil(t, re)

	assert.True(t, re.MatchString("12"))
	assert.True(t, re.MatchString("refs/merge-requests/12/head"))
	assert.True(t, re.MatchString("refs/merge-requests/12/merge"))
	assert.False(t, re.MatchString("refs/heads/main"))
}

func TestGetRefRegexp_InvalidKind(t *testing.T) {
	var ppr config.ProjectPullRefs

	re, err := GetRefRegexp(ppr, RefKind("invalid"))
	assert.Nil(t, re)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ref kind")
}

func TestGetMergeRequestIIDFromRefName_DirectIID(t *testing.T) {
	iid, err := GetMergeRequestIIDFromRefName("42")
	require.NoError(t, err)
	assert.Equal(t, "42", iid)
}

func TestGetMergeRequestIIDFromRefName_HeadRef(t *testing.T) {
	iid, err := GetMergeRequestIIDFromRefName("refs/merge-requests/123/head")
	require.NoError(t, err)
	assert.Equal(t, "123", iid)
}

func TestGetMergeRequestIIDFromRefName_MergeRef(t *testing.T) {
	iid, err := GetMergeRequestIIDFromRefName("refs/merge-requests/456/merge")
	require.NoError(t, err)
	assert.Equal(t, "456", iid)
}

func TestGetMergeRequestIIDFromRefName_Invalid(t *testing.T) {
	iid, err := GetMergeRequestIIDFromRefName("refs/heads/main")
	assert.Error(t, err)
	assert.Equal(t, "refs/heads/main", iid)
	assert.Contains(t, err.Error(), "unable to extract the merge-request ID")
}
