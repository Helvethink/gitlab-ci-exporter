package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

var errCacheUnavailable = errors.New("cache unavailable")

// faultyPipelineStore injects failures into the pipeline cache operations.
type faultyPipelineStore struct {
	store.Store
	failVariablesExists bool
	failGetVariables    bool
	failSetVariables    bool
	failGetPipeline     bool
	failSetPipeline     bool
	setPipelineCalls    atomic.Int32
}

func (s *faultyPipelineStore) PipelineVariablesExists(ctx context.Context, p schemas.Pipeline) (bool, error) {
	if s.failVariablesExists {
		return false, errCacheUnavailable
	}
	return s.Store.PipelineVariablesExists(ctx, p)
}

func (s *faultyPipelineStore) GetPipelineVariables(ctx context.Context, p schemas.Pipeline) (string, error) {
	if s.failGetVariables {
		return "", errCacheUnavailable
	}
	return s.Store.GetPipelineVariables(ctx, p)
}

func (s *faultyPipelineStore) SetPipelineVariables(ctx context.Context, p schemas.Pipeline, v string) error {
	if s.failSetVariables {
		return errCacheUnavailable
	}
	return s.Store.SetPipelineVariables(ctx, p, v)
}

func (s *faultyPipelineStore) GetPipeline(ctx context.Context, p *schemas.Pipeline) error {
	if s.failGetPipeline {
		return errCacheUnavailable
	}
	return s.Store.GetPipeline(ctx, p)
}

func (s *faultyPipelineStore) SetPipeline(ctx context.Context, p schemas.Pipeline) error {
	s.setPipelineCalls.Add(1)
	if s.failSetPipeline {
		return errCacheUnavailable
	}
	return s.Store.SetPipeline(ctx, p)
}

type pipelineAPI struct {
	variablesCalls atomic.Int32
}

func (a *pipelineAPI) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/pipelines/123/variables") {
		a.variablesCalls.Add(1)
		_, _ = w.Write([]byte(`[{"key":"DEPLOY_ENV","value":"SENSITIVE"}]`))
		return
	}
	_, _ = w.Write([]byte(`{"id":123,"duration":12,"source":"push","status":"success","updated_at":"2024-03-09T16:00:00Z"}`))
}

func newPipelineCacheTest(t *testing.T, faults *faultyPipelineStore) (*Controller, *pipelineAPI, schemas.Ref) {
	t.Helper()

	api := &pipelineAPI{}
	c := newTestRunnerController(t, api.handler)
	faults.Store = c.Store
	c.Store = faults

	project := schemas.NewProject("group/project")
	project.Pull.Pipeline.Variables.Enabled = true
	keys, err := config.NewPipelineVariableKeys("DEPLOY_ENV")
	require.NoError(t, err)
	project.Pull.Pipeline.Variables.Keys = keys

	return c, api, schemas.NewRef(project, schemas.RefKindBranch, "main")
}

func storedRunCount(t *testing.T, c *Controller, variables string) schemas.Metric {
	t.Helper()
	m := schemas.Metric{Kind: schemas.MetricKindRunCount, Labels: map[string]string{
		"project": "group/project", "topics": "", "kind": "branch", "ref": "main",
		"source": "push", "variables": variables, "pipeline_id": "123", "status": "success",
	}}
	exists, err := c.Store.MetricExists(context.Background(), m.Key())
	require.NoError(t, err)
	require.True(t, exists, "run count metric must be stored")
	return m
}

func TestPipelineCacheUsesStoredPipelineAndVariables(t *testing.T) {
	ctx := context.Background()
	faults := &faultyPipelineStore{}
	c, api, ref := newPipelineCacheTest(t, faults)

	require.NoError(t, c.ProcessPipelinesMetrics(ctx, ref, &goGitlab.PipelineInfo{ID: 123}))
	require.NoError(t, c.ProcessPipelinesMetrics(ctx, ref, &goGitlab.PipelineInfo{ID: 123}))

	assert.Equal(t, int32(1), api.variablesCalls.Load(), "variables must be served from the cache on the second pull")
	assert.Equal(t, int32(1), faults.setPipelineCalls.Load(), "an unchanged pipeline must be recognized from the cache")
	storedRunCount(t, c, "DEPLOY_ENV")
}

func TestPipelineCacheReadFailuresAreBestEffort(t *testing.T) {
	tests := map[string]*faultyPipelineStore{
		"variables exists fails": {failVariablesExists: true},
		"get variables fails":    {failGetVariables: true},
		"set variables fails":    {failSetVariables: true},
		"get pipeline fails":     {failGetPipeline: true},
		"every read fails":       {failVariablesExists: true, failGetVariables: true, failSetVariables: true, failGetPipeline: true},
	}

	for name, faults := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			c, api, ref := newPipelineCacheTest(t, faults)

			require.NoError(t, c.ProcessPipelinesMetrics(ctx, ref, &goGitlab.PipelineInfo{ID: 123}))
			require.NoError(t, c.ProcessPipelinesMetrics(ctx, ref, &goGitlab.PipelineInfo{ID: 123}))

			assert.GreaterOrEqual(t, api.variablesCalls.Load(), int32(1))
			storedRunCount(t, c, "DEPLOY_ENV")

			storedRef := schemas.NewRef(ref.Project, schemas.RefKindBranch, "main")
			require.NoError(t, c.Store.GetRef(ctx, &storedRef))
			assert.Equal(t, int64(123), storedRef.LatestPipeline.ID)
		})
	}
}

func TestPipelineCacheWriteFailureIsRequired(t *testing.T) {
	c, _, ref := newPipelineCacheTest(t, &faultyPipelineStore{failSetPipeline: true})

	err := c.ProcessPipelinesMetrics(context.Background(), ref, &goGitlab.PipelineInfo{ID: 123})
	require.ErrorIs(t, err, errCacheUnavailable)
}

func TestUnchangedPipelineStillRefreshesRefAndMetrics(t *testing.T) {
	ctx := context.Background()
	c, _, ref := newPipelineCacheTest(t, &faultyPipelineStore{})

	require.NoError(t, c.ProcessPipelinesMetrics(ctx, ref, &goGitlab.PipelineInfo{ID: 123}))

	// Simulate TTL expiry and garbage collection of the ref and its metrics.
	m := storedRunCount(t, c, "DEPLOY_ENV")
	require.NoError(t, c.Store.DelMetric(ctx, m.Key()))
	require.NoError(t, c.Store.DelRef(ctx, ref.Key()))

	require.NoError(t, c.ProcessPipelinesMetrics(ctx, ref, &goGitlab.PipelineInfo{ID: 123}))

	storedRunCount(t, c, "DEPLOY_ENV")
	exists, err := c.Store.RefExists(ctx, ref.Key())
	require.NoError(t, err)
	assert.True(t, exists)
}
