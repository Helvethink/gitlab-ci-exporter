package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

const gcTTL = time.Hour

func gcTTLController(t *testing.T) (*Controller, *miniredis.Miniredis, schemas.Project) {
	t.Helper()

	// GitLab still lists both branches, so only TTL expiry can remove a ref.
	c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/repository/branches") {
			_, _ = w.Write([]byte(`[{"name":"main"},{"name":"master"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	st := store.NewRedisStore(client, store.WithTTLConfig(&store.RedisTTLConfig{
		Project: gcTTL, Environment: gcTTL, Runner: gcTTL, Refs: gcTTL, Metrics: gcTTL,
	}))

	project := schemas.NewProject("group/project")
	c.Config = config.New()
	c.Config.Projects = []config.Project{project.Project}
	c.Store = st
	c.Redis = client
	require.NoError(t, st.SetProject(context.Background(), project))

	return c, mr, project
}

func gcRefMetric(ref schemas.Ref, kind schemas.MetricKind) schemas.Metric {
	labels := ref.DefaultLabelsValues()
	labels["pipeline_id"] = "1"
	labels["status"] = "success"
	return schemas.Metric{Kind: kind, Labels: labels, Value: 1}
}

// TestGarbageCollectRefsKeepsFreshAndRemovesExpired is a regression test: the
// Redis Has*Expired methods used to be inverted, so garbage collection deleted
// refs that were still within their TTL and kept the expired ones.
func TestGarbageCollectRefsKeepsFreshAndRemovesExpired(t *testing.T) {
	ctx := context.Background()
	c, mr, project := gcTTLController(t)

	expired := schemas.NewRef(project, schemas.RefKindBranch, "master")
	require.NoError(t, c.Store.SetRef(ctx, expired))
	mr.FastForward(gcTTL + time.Minute)

	fresh := schemas.NewRef(project, schemas.RefKindBranch, "main")
	require.NoError(t, c.Store.SetRef(ctx, fresh))

	require.NoError(t, c.GarbageCollectRefs(ctx))

	freshExists, err := c.Store.RefExists(ctx, fresh.Key())
	require.NoError(t, err)
	assert.True(t, freshExists, "a ref within its TTL must be kept")

	expiredExists, err := c.Store.RefExists(ctx, expired.Key())
	require.NoError(t, err)
	assert.False(t, expiredExists, "a ref past its TTL must be deleted")
}

func TestGarbageCollectMetricsKeepsFreshAndRemovesExpired(t *testing.T) {
	ctx := context.Background()
	c, mr, project := gcTTLController(t)
	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")

	expired := gcRefMetric(ref, schemas.MetricKindCoverage)
	require.NoError(t, c.Store.SetMetric(ctx, expired))
	mr.FastForward(gcTTL + time.Minute)

	require.NoError(t, c.Store.SetRef(ctx, ref))
	fresh := gcRefMetric(ref, schemas.MetricKindID)
	require.NoError(t, c.Store.SetMetric(ctx, fresh))

	require.NoError(t, c.GarbageCollectMetrics(ctx))

	freshExists, err := c.Store.MetricExists(ctx, fresh.Key())
	require.NoError(t, err)
	assert.True(t, freshExists, "a metric within its TTL must be kept")

	expiredExists, err := c.Store.MetricExists(ctx, expired.Key())
	require.NoError(t, err)
	assert.False(t, expiredExists, "a metric past its TTL must be deleted")
}

// TestGarbageCollectRefsDeletesInvalidRefsWithoutPanicking is a regression test:
// a stored ref with an unknown kind used to make garbage collection dereference a
// nil regexp, and a deleted non-matching ref was written back to the store.
func TestGarbageCollectRefsDeletesInvalidRefsWithoutPanicking(t *testing.T) {
	ctx := context.Background()
	c, _, project := gcTTLController(t)

	invalidKind := schemas.NewRef(project, schemas.RefKind("unknown"), "main")
	notMatching := schemas.NewRef(project, schemas.RefKindBranch, "feature/x")
	require.NoError(t, c.Store.SetRef(ctx, invalidKind))
	require.NoError(t, c.Store.SetRef(ctx, notMatching))

	require.NotPanics(t, func() { require.NoError(t, c.GarbageCollectRefs(ctx)) })

	for _, ref := range []schemas.Ref{invalidKind, notMatching} {
		exists, err := c.Store.RefExists(ctx, ref.Key())
		require.NoError(t, err)
		assert.False(t, exists, "ref %s/%s must be deleted", ref.Kind, ref.Name)
	}
}
