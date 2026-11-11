package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

type noopLimiterPipelinesTests struct{}

func (noopLimiterPipelinesTests) Take(ctx context.Context) (time.Duration, error) {
	return 0, nil
}

func newTestGitLabClientForPipelines(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	gl, err := goGitlab.NewClient(
		"test-token",
		goGitlab.WithBaseURL(server.URL+"/api/v4"),
	)
	require.NoError(t, err)

	return &Client{
		Client:      gl,
		RateLimiter: noopLimiterPipelinesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}
}

func TestGetRefPipeline(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines/123"))
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"id": 123,
			"coverage": "87.5",
			"updated_at": "2024-03-09T16:00:00Z",
			"duration": 120,
			"queued_duration": 15,
			"source": "push",
			"status": "success",
			"detailed_status": {
				"group": "waiting-for-resource"
			}
		}`))
	})

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	got, err := c.GetRefPipeline(context.Background(), ref, 123)
	require.NoError(t, err)

	assert.Equal(t, int64(123), got.ID)
	assert.Equal(t, 87.5, got.Coverage)
	assert.Equal(t, float64(1710000000), got.Timestamp)
	assert.Equal(t, 120.0, got.DurationSeconds)
	assert.Equal(t, 15.0, got.QueuedDurationSeconds)
	assert.Equal(t, "push", got.Source)
	assert.Equal(t, "waiting_for_resource", got.Status)
}

func TestGetRefPipeline_APIError(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	got, err := c.GetRefPipeline(context.Background(), ref, 123)
	assert.Error(t, err)
	assert.Equal(t, schemas.Pipeline{}, got)
	assert.Contains(t, err.Error(), "could not read content of pipeline")
}

func TestGetProjectPipelines_DefaultPagination(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines"))
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id": 1, "ref": "main"},
			{"id": 2, "ref": "develop"}
		]`))
	})

	options := &goGitlab.ListProjectPipelinesOptions{}
	pipelines, resp, err := c.GetProjectPipelines(context.Background(), "group/project", options)
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Len(t, pipelines, 2)
	assert.Equal(t, int64(1), pipelines[0].ID)
	assert.Equal(t, "main", pipelines[0].Ref)
	assert.Equal(t, int64(2), pipelines[1].ID)
	assert.Equal(t, "develop", pipelines[1].Ref)
}

func TestGetProjectPipelines_APIError(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	options := &goGitlab.ListProjectPipelinesOptions{}
	pipelines, _, err := c.GetProjectPipelines(context.Background(), "group/project", options)
	assert.Error(t, err)
	assert.Nil(t, pipelines)
	assert.Contains(t, err.Error(), "error listing project pipelines")
}

func TestGetRefPipelineVariableKeys_EmptyPipeline(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterPipelinesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	p.Pull.Pipeline.Variables.Regexp = `.*`
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	got, err := c.GetRefPipelineVariableKeys(context.Background(), ref, schemas.Pipeline{})
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestGetRefPipelineVariableKeys_InvalidRegexp(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterPipelinesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	p.Pull.Pipeline.Variables.Regexp = "["
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	got, err := c.GetRefPipelineVariableKeys(context.Background(), ref, schemas.Pipeline{ID: 123})
	assert.Error(t, err)
	assert.Equal(t, "", got)
	assert.Contains(t, err.Error(), "provided filter regex")
}

// sensitiveVariablesPayload contains values that look like credentials. None of them
// may ever be returned by GetRefPipelineVariableKeys.
const sensitiveVariablesPayload = `[
	{"key":"DEPLOY_ENV","value":"production"},
	{"key":"API_TOKEN","value":"glpat-SENSITIVEVALUE0001"},
	{"key":"DB_PASSWORD","value":"s3cr3t:with,separators"},
	{"key":"RELEASE","value":"v1.2.3"},
	{"key":"bad key:","value":"SENSITIVEVALUE0002"}
]`

var sensitiveValues = []string{"production", "SENSITIVEVALUE", "s3cr3t", "v1.2.3"}

func TestGetRefPipelineVariableKeys_DefaultDenyDoesNotCallGitLab(t *testing.T) {
	called := false
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sensitiveVariablesPayload))
	})

	p := schemas.NewProject("group/project")
	p.Pull.Pipeline.Variables.Enabled = true
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	got, err := c.GetRefPipelineVariableKeys(context.Background(), ref, schemas.Pipeline{ID: 123})
	require.NoError(t, err)
	assert.Equal(t, "", got)
	assert.False(t, called, "default configuration must not fetch pipeline variables")
}

func TestGetRefPipelineVariableKeys_ReturnsOnlyAllowlistedKeys(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		regexp string
		want   string
	}{
		{name: "explicit keys", keys: []string{"RELEASE", "DEPLOY_ENV", "MISSING"}, want: "DEPLOY_ENV,RELEASE"},
		{name: "regexp", regexp: `^(DEPLOY_ENV|RELEASE)$`, want: "DEPLOY_ENV,RELEASE"},
		{name: "keys and regexp", keys: []string{"RELEASE"}, regexp: `^DEPLOY_`, want: "DEPLOY_ENV,RELEASE"},
		{name: "match-all regexp still exports keys only", regexp: `.*`, want: "API_TOKEN,DB_PASSWORD,DEPLOY_ENV,RELEASE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
				assert.True(t, strings.Contains(r.URL.Path, "/pipelines/123/variables"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(sensitiveVariablesPayload))
			})

			p := schemas.NewProject("group/project")
			p.Pull.Pipeline.Variables.Enabled = true
			p.Pull.Pipeline.Variables.Regexp = tt.regexp
			keys, err := config.NewPipelineVariableKeys(tt.keys...)
			require.NoError(t, err)
			p.Pull.Pipeline.Variables.Keys = keys
			ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

			got, err := c.GetRefPipelineVariableKeys(context.Background(), ref, schemas.Pipeline{ID: 123})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, ":")
			for _, v := range sensitiveValues {
				assert.NotContains(t, got, v)
			}
		})
	}
}

func TestGetRefsFromPipelines_BranchesMostRecent(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines"))
		assert.Equal(t, "branches", r.URL.Query().Get("scope"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id": 1, "ref": "main"},
			{"id": 2, "ref": "feature/test"},
			{"id": 3, "ref": "develop"}
		]`))
	})

	p := schemas.NewProject("group/project")
	p.Pull.Refs.Branches.Regexp = `^(main|develop)$`
	p.Pull.Refs.Branches.MostRecent = 1
	p.Pull.Refs.Branches.ExcludeDeleted = false

	refs, err := c.GetRefsFromPipelines(context.Background(), p, schemas.RefKindBranch)
	require.NoError(t, err)

	expected := schemas.NewRef(p, schemas.RefKindBranch, "main")
	require.Len(t, refs, 1)
	assert.Contains(t, refs, expected.Key())
	assert.Equal(t, expected, refs[expected.Key()])
}

func TestGetRefsFromPipelines_MergeRequests(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Next-Page", "0")

		_, _ = w.Write([]byte(`[
			{"id": 1, "ref": "refs/merge-requests/12/head"},
			{"id": 2, "ref": "refs/merge-requests/99/merge"}
		]`))
	})

	p := schemas.NewProject("group/project")
	p.Pull.Refs.MergeRequests.MostRecent = 2

	refs, err := c.GetRefsFromPipelines(context.Background(), p, schemas.RefKindMergeRequest)
	require.NoError(t, err)

	expected1 := schemas.NewRef(p, schemas.RefKindMergeRequest, "12")
	expected2 := schemas.NewRef(p, schemas.RefKindMergeRequest, "99")

	require.Len(t, refs, 2)
	assert.Contains(t, refs, expected1.Key())
	assert.Contains(t, refs, expected2.Key())
	assert.Equal(t, expected1, refs[expected1.Key()])
	assert.Equal(t, expected2, refs[expected2.Key()])
}

func TestGetRefsFromPipelines_UnsupportedKind(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterPipelinesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")

	refs, err := c.GetRefsFromPipelines(context.Background(), p, schemas.RefKind("unsupported"))
	assert.Error(t, err)
	assert.Empty(t, refs)
	assert.Contains(t, err.Error(), "invalid ref kind")
}

func TestGetRefPipelineTestReport_EmptyLatestPipeline(t *testing.T) {
	c := &Client{
		RateLimiter: noopLimiterPipelinesTests{},
		RateCounter: ratecounter.NewRateCounter(time.Second),
	}

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")

	report, err := c.GetRefPipelineTestReport(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, schemas.TestReport{}, report)
}

func TestGetRefPipelineTestReport_SinglePipeline(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/pipelines/321/test_report"))
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{
			"total_time": 20,
			"total_count": 6,
			"success_count": 4,
			"failed_count": 1,
			"skipped_count": 1,
			"error_count": 0,
			"test_suites": [
				{
					"name": "suite-1",
					"total_time": 20,
					"total_count": 6,
					"success_count": 4,
					"failed_count": 1,
					"skipped_count": 1,
					"error_count": 0,
					"test_cases": [
						{
							"name": "TestA",
							"classname": "suite1",
							"execution_time": 0.3,
							"status": "success"
						}
					]
				}
			]
		}`))
	})

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{ID: 321}

	report, err := c.GetRefPipelineTestReport(context.Background(), ref)
	require.NoError(t, err)

	assert.Equal(t, 20.0, report.TotalTime)
	assert.Equal(t, int64(6), report.TotalCount)
	assert.Equal(t, int64(4), report.SuccessCount)
	assert.Equal(t, int64(1), report.FailedCount)
	assert.Equal(t, int64(1), report.SkippedCount)
	assert.Equal(t, int64(0), report.ErrorCount)
	require.Len(t, report.TestSuites, 1)
	assert.Equal(t, "suite-1", report.TestSuites[0].Name)
	require.Len(t, report.TestSuites[0].TestCases, 1)
	assert.Equal(t, "TestA", report.TestSuites[0].TestCases[0].Name)
}

func TestGetRefPipelineTestReport_APIError(t *testing.T) {
	c := newTestGitLabClientForPipelines(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	p := schemas.NewProject("group/project")
	ref := schemas.NewRef(p, schemas.RefKindBranch, "main")
	ref.LatestPipeline = schemas.Pipeline{ID: 321}

	report, err := c.GetRefPipelineTestReport(context.Background(), ref)
	assert.Error(t, err)
	assert.Equal(t, schemas.TestReport{}, report)
	assert.Contains(t, err.Error(), "could not fetch test report")
}
