package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/paulbellamy/ratecounter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/taskq/v4"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	gitlabclient "github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

var errSchedStoreUnavailable = errors.New("sched test: store unavailable")

// schedStore wraps a real local store, records task completions and can be
// switched into a mode where every read used by the handlers fails.
type schedStore struct {
	store.Store
	failReads atomic.Bool

	mu       sync.Mutex
	dequeued []string
}

func (s *schedStore) DequeueTask(ctx context.Context, tt schemas.TaskType, id, owner string) error {
	s.mu.Lock()
	s.dequeued = append(s.dequeued, string(tt)+"|"+id)
	s.mu.Unlock()
	return s.Store.DequeueTask(ctx, tt, id, owner)
}

func (s *schedStore) dequeuedTasks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.dequeued...)
}

func (s *schedStore) Projects(ctx context.Context) (schemas.Projects, error) {
	if s.failReads.Load() {
		return nil, errSchedStoreUnavailable
	}
	return s.Store.Projects(ctx)
}

func (s *schedStore) ProjectsCount(ctx context.Context) (int64, error) {
	if s.failReads.Load() {
		return 0, errSchedStoreUnavailable
	}
	return s.Store.ProjectsCount(ctx)
}

func (s *schedStore) Environments(ctx context.Context) (schemas.Environments, error) {
	if s.failReads.Load() {
		return nil, errSchedStoreUnavailable
	}
	return s.Store.Environments(ctx)
}

func (s *schedStore) EnvironmentsCount(ctx context.Context) (int64, error) {
	if s.failReads.Load() {
		return 0, errSchedStoreUnavailable
	}
	return s.Store.EnvironmentsCount(ctx)
}

func (s *schedStore) Runners(ctx context.Context) (schemas.Runners, error) {
	if s.failReads.Load() {
		return nil, errSchedStoreUnavailable
	}
	return s.Store.Runners(ctx)
}

func (s *schedStore) RunnersCount(ctx context.Context) (int64, error) {
	if s.failReads.Load() {
		return 0, errSchedStoreUnavailable
	}
	return s.Store.RunnersCount(ctx)
}

func (s *schedStore) Refs(ctx context.Context) (schemas.Refs, error) {
	if s.failReads.Load() {
		return nil, errSchedStoreUnavailable
	}
	return s.Store.Refs(ctx)
}

func (s *schedStore) RefsCount(ctx context.Context) (int64, error) {
	if s.failReads.Load() {
		return 0, errSchedStoreUnavailable
	}
	return s.Store.RefsCount(ctx)
}

func (s *schedStore) Metrics(ctx context.Context) (schemas.Metrics, error) {
	if s.failReads.Load() {
		return nil, errSchedStoreUnavailable
	}
	return s.Store.Metrics(ctx)
}

func (s *schedStore) GetRef(ctx context.Context, r *schemas.Ref) error {
	if s.failReads.Load() {
		return errSchedStoreUnavailable
	}
	return s.Store.GetRef(ctx, r)
}

func (s *schedStore) RunnerExists(ctx context.Context, k schemas.RunnerKey) (bool, error) {
	if s.failReads.Load() {
		return false, errSchedStoreUnavailable
	}
	return s.Store.RunnerExists(ctx, k)
}

// schedHarness is a controller backed by a fake GitLab server, a recording
// store and a queue whose publications are captured instead of executed.
type schedHarness struct {
	c  *Controller
	st *schedStore

	mu   sync.Mutex
	jobs []*taskq.Job
}

func schedNewHarness(t *testing.T, handler http.HandlerFunc) *schedHarness {
	t.Helper()

	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected GitLab call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// Retries are disabled so that 5xx answers surface immediately.
	gl, err := goGitlab.NewClient("placeholder", goGitlab.WithBaseURL(server.URL+"/api/v4"), goGitlab.WithoutRetries())
	require.NoError(t, err)

	h := &schedHarness{st: &schedStore{Store: store.NewLocalStore()}}
	h.c = &Controller{
		Store: h.st,
		Gitlab: &gitlabclient.Client{
			Client:      gl,
			RateLimiter: noopLimiterControllerRunnersTests{},
			RateCounter: ratecounter.NewRateCounter(time.Second),
		},
		TaskController: NewTaskController(context.Background(), nil, 50),
		UUID:           uuid.New(),
	}
	h.c.registerTasks()

	queue := h.c.TaskController.Queue
	t.Cleanup(func() { _ = queue.Close() })
	h.c.TaskController.Queue = publicationQueue{Queue: queue, addJob: func(_ context.Context, job *taskq.Job) error {
		h.mu.Lock()
		h.jobs = append(h.jobs, job)
		h.mu.Unlock()
		return nil
	}}

	return h
}

func (h *schedHarness) published() []*taskq.Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*taskq.Job(nil), h.jobs...)
}

func (h *schedHarness) publishedNames() []string {
	names := make([]string, 0)
	for _, j := range h.published() {
		names = append(names, j.TaskName)
	}
	return names
}

func (h *schedHarness) resetPublished() {
	h.mu.Lock()
	h.jobs = nil
	h.mu.Unlock()
}

// reserve schedules the task through the real ScheduleTask path, so the
// handler under test owns a genuine store reservation and admission slot.
func (h *schedHarness) reserve(t *testing.T, tt schemas.TaskType, id string, args ...interface{}) {
	t.Helper()
	h.c.ScheduleTask(context.Background(), tt, id, args...)
	require.Equal(t, []string{string(tt)}, h.publishedNames(), "reservation was not published")
	require.True(t, schedAdmissionHolds(h.c, tt, id), "reservation did not take an admission slot")
	h.resetPublished()
}

// assertReleased checks that the handler completed its own reservation.
func (h *schedHarness) assertReleased(t *testing.T, tt schemas.TaskType, id string) {
	t.Helper()
	assert.Contains(t, h.st.dequeuedTasks(), string(tt)+"|"+id)
	assert.False(t, schedAdmissionHolds(h.c, tt, id), "admission slot still held")
	executed, err := h.st.ExecutedTasksCount(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), executed)
	// The same task must be schedulable again once released.
	queued, err := h.st.QueueTask(context.Background(), tt, id, "probe")
	require.NoError(t, err)
	assert.True(t, queued, "store reservation still present")
	require.NoError(t, h.st.CancelTask(context.Background(), tt, id, "probe"))
}

func schedAdmissionHolds(c *Controller, tt schemas.TaskType, id string) bool {
	a := c.TaskController.admission
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.accepted[taskKey{typeName: tt, id: id}]
	return ok
}

func schedLastScheduled(c *Controller, tt schemas.TaskType) time.Time {
	return c.TaskController.TaskSchedulingMonitoring.Snapshot()[tt].Last
}

func schedJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func schedServerError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(`{"message":"500 Internal Server Error"}`))
}

func schedAlwaysFail(calls *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		schedServerError(w)
	}
}

func schedCancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// --- per-entity pull handlers ---------------------------------------------

func TestSchedHandlerPullProjectStoresProjectAndSchedulesFollowUps(t *testing.T) {
	h := schedNewHarness(t, func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasSuffix(r.URL.Path, "/projects/group/project"), r.URL.Path)
		schedJSON(w, `{"id": 1, "path_with_namespace": "group/project"}`)
	})
	pull := config.ProjectPull{}
	pull.Runners.Enabled = true
	h.reserve(t, schemas.TaskTypePullProject, "group/project", "group/project", pull)

	require.NoError(t, h.c.TaskHandlerPullProject(context.Background(), "group/project", pull))

	p := schemas.NewProject("group/project")
	exists, err := h.st.ProjectExists(context.Background(), p.Key())
	require.NoError(t, err)
	assert.True(t, exists)
	assert.ElementsMatch(t, []string{
		string(schemas.TaskTypePullRefsFromProject),
		string(schemas.TaskTypePullEnvironmentsFromProject),
		string(schemas.TaskTypePullRunnersFromProject),
	}, h.publishedNames())
	for _, j := range h.published() {
		require.Len(t, j.Args, 1)
		scheduled, ok := j.Args[0].(schemas.Project)
		require.True(t, ok)
		assert.Equal(t, "group/project", scheduled.Name)
		assert.True(t, scheduled.Pull.Runners.Enabled, "pull config must be propagated")
	}
	h.assertReleased(t, schemas.TaskTypePullProject, "group/project")
}

func TestSchedHandlerPullProjectGitLabErrorReleasesReservation(t *testing.T) {
	var calls atomic.Int32
	h := schedNewHarness(t, schedAlwaysFail(&calls))
	h.reserve(t, schemas.TaskTypePullProject, "group/project", "group/project", config.ProjectPull{})

	err := h.c.TaskHandlerPullProject(context.Background(), "group/project", config.ProjectPull{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Equal(t, int32(1), calls.Load())

	count, err := h.st.ProjectsCount(context.Background())
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Empty(t, h.published())
	h.assertReleased(t, schemas.TaskTypePullProject, "group/project")
}

func TestSchedHandlerPullProjectCancelledContextReturnsAndReleases(t *testing.T) {
	h := schedNewHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		schedJSON(w, `{"id": 1, "path_with_namespace": "group/project"}`)
	})
	h.reserve(t, schemas.TaskTypePullProject, "group/project", "group/project", config.ProjectPull{})

	done := make(chan error, 1)
	go func() {
		done <- h.c.TaskHandlerPullProject(schedCancelledContext(), "group/project", config.ProjectPull{})
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after cancellation")
	}
	assert.Empty(t, h.published())
	h.assertReleased(t, schemas.TaskTypePullProject, "group/project")
}

func TestSchedHandlerPullProjectsFromWildcard(t *testing.T) {
	w := config.Wildcard{Search: "app", Owner: config.WildcardOwner{Kind: "group", Name: "platform"}}

	t.Run("discovers projects", func(t *testing.T) {
		h := schedNewHarness(t, func(rw http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, "/groups/platform/projects")
			schedJSON(rw, `[{"id": 1, "path_with_namespace": "platform/app-one"}]`)
		})
		h.reserve(t, schemas.TaskTypePullProjectsFromWildcard, "0", "0", w)

		require.NoError(t, h.c.TaskHandlerPullProjectsFromWildcard(context.Background(), "0", w))

		exists, err := h.st.ProjectExists(context.Background(), schemas.NewProject("platform/app-one").Key())
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Len(t, h.published(), 3)
		h.assertReleased(t, schemas.TaskTypePullProjectsFromWildcard, "0")
	})

	t.Run("gitlab error", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		h.reserve(t, schemas.TaskTypePullProjectsFromWildcard, "0", "0", w)

		require.Error(t, h.c.TaskHandlerPullProjectsFromWildcard(context.Background(), "0", w))
		assert.Positive(t, calls.Load())
		assert.Empty(t, h.published())
		h.assertReleased(t, schemas.TaskTypePullProjectsFromWildcard, "0")
	})
}

func schedProjectWithEnvironments(enabled bool) schemas.Project {
	p := schemas.NewProject("group/project")
	p.Pull.Environments.Enabled = enabled
	p.Pull.Environments.Regexp = ".*"
	return p
}

func TestSchedHandlerPullEnvironmentsFromProject(t *testing.T) {
	t.Run("enabled stores environment and schedules metrics", func(t *testing.T) {
		h := schedNewHarness(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/projects/group/project/environments/7"):
				schedJSON(w, `{"id": 7, "name": "production", "state": "available", "external_url": "https://prod.example.test"}`)
			case strings.HasSuffix(r.URL.Path, "/projects/group/project/environments"):
				schedJSON(w, `[{"id": 7, "name": "production", "state": "available"}]`)
			default:
				t.Errorf("unexpected path: %s", r.URL.Path)
				schedServerError(w)
			}
		})
		p := schedProjectWithEnvironments(true)
		h.reserve(t, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullEnvironmentsFromProject(context.Background(), p)

		env := schemas.Environment{ProjectName: "group/project", Name: "production"}
		require.NoError(t, h.st.GetEnvironment(context.Background(), &env))
		assert.Equal(t, int64(7), env.ID)
		assert.Equal(t, "https://prod.example.test", env.ExternalURL)
		assert.Equal(t, []string{string(schemas.TaskTypePullEnvironmentMetrics)}, h.publishedNames())
		h.assertReleased(t, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()))
	})

	t.Run("disabled makes no API call", func(t *testing.T) {
		h := schedNewHarness(t, nil)
		p := schedProjectWithEnvironments(false)
		h.reserve(t, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullEnvironmentsFromProject(context.Background(), p)

		assert.Empty(t, h.published())
		h.assertReleased(t, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()))
	})

	t.Run("gitlab error is swallowed and reservation released", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		p := schedProjectWithEnvironments(true)
		h.reserve(t, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullEnvironmentsFromProject(context.Background(), p)

		assert.Equal(t, int32(1), calls.Load())
		count, err := h.st.EnvironmentsCount(context.Background())
		require.NoError(t, err)
		assert.Zero(t, count)
		assert.Empty(t, h.published())
		h.assertReleased(t, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()))
	})
}

func schedProjectWithRunners(enabled bool) schemas.Project {
	p := schemas.NewProject("group/project")
	p.Pull.Runners.Enabled = enabled
	p.Pull.Runners.Regexp = ".*"
	return p
}

func schedRunnersServer(t *testing.T, detailCalls *atomic.Int32, failDetails bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects/group/project/runners"):
			schedJSON(w, `[{"id": 101, "name": "runner-101", "description": "shared"}]`)
		case strings.HasSuffix(r.URL.Path, "/runners/101"):
			detailCalls.Add(1)
			if failDetails {
				schedServerError(w)
				return
			}
			schedJSON(w, `{"id": 101, "name": "runner-101", "description": "shared", "online": true, "status": "online"}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			schedServerError(w)
		}
	}
}

func TestSchedHandlerPullRunnersFromProject(t *testing.T) {
	t.Run("enabled stores new runner once and schedules metrics", func(t *testing.T) {
		var details atomic.Int32
		h := schedNewHarness(t, schedRunnersServer(t, &details, false))
		p := schedProjectWithRunners(true)
		h.reserve(t, schemas.TaskTypePullRunnersFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullRunnersFromProject(context.Background(), p)

		runner := schemas.Runner{ProjectName: "group/project", ID: 101}
		require.NoError(t, h.st.GetRunner(context.Background(), &runner))
		assert.Equal(t, "runner-101", runner.Name)
		assert.Equal(t, "online", runner.Status)
		jobs := h.published()
		require.Len(t, jobs, 1)
		assert.Equal(t, string(schemas.TaskTypePullRunnersMetrics), jobs[0].TaskName)
		scheduled, ok := jobs[0].Args[0].(schemas.Runner)
		require.True(t, ok)
		assert.Equal(t, int64(101), scheduled.ID)
		h.assertReleased(t, schemas.TaskTypePullRunnersFromProject, string(p.Key()))

		// A known runner must not be refreshed nor re-scheduled.
		h.resetPublished()
		require.NoError(t, h.c.PullRunnersFromProject(context.Background(), p))
		assert.Empty(t, h.published())
		assert.Equal(t, int32(1), details.Load())
	})

	t.Run("disabled makes no API call", func(t *testing.T) {
		h := schedNewHarness(t, nil)
		p := schedProjectWithRunners(false)
		h.reserve(t, schemas.TaskTypePullRunnersFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullRunnersFromProject(context.Background(), p)

		assert.Empty(t, h.published())
		h.assertReleased(t, schemas.TaskTypePullRunnersFromProject, string(p.Key()))
	})

	t.Run("gitlab error is swallowed and reservation released", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		p := schedProjectWithRunners(true)
		h.reserve(t, schemas.TaskTypePullRunnersFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullRunnersFromProject(context.Background(), p)

		assert.Equal(t, int32(1), calls.Load())
		count, err := h.st.RunnersCount(context.Background())
		require.NoError(t, err)
		assert.Zero(t, count)
		h.assertReleased(t, schemas.TaskTypePullRunnersFromProject, string(p.Key()))
	})
}

func TestSchedPullRunnersFromProjectErrors(t *testing.T) {
	t.Run("invalid regexp", func(t *testing.T) {
		h := schedNewHarness(t, nil)
		p := schedProjectWithRunners(true)
		p.Pull.Runners.Regexp = "("
		require.Error(t, h.c.PullRunnersFromProject(context.Background(), p))
	})

	t.Run("store failure stops before refreshing", func(t *testing.T) {
		var details atomic.Int32
		h := schedNewHarness(t, schedRunnersServer(t, &details, false))
		h.st.failReads.Store(true)
		err := h.c.PullRunnersFromProject(context.Background(), schedProjectWithRunners(true))
		require.ErrorIs(t, err, errSchedStoreUnavailable)
		assert.Zero(t, details.Load())
		assert.Empty(t, h.published())
	})

	t.Run("runner detail failure does not store or schedule", func(t *testing.T) {
		var details atomic.Int32
		h := schedNewHarness(t, schedRunnersServer(t, &details, true))
		err := h.c.PullRunnersFromProject(context.Background(), schedProjectWithRunners(true))
		require.Error(t, err)
		assert.Equal(t, int32(1), details.Load())
		count, err := h.st.RunnersCount(context.Background())
		require.NoError(t, err)
		assert.Zero(t, count)
		assert.Empty(t, h.published())
	})
}

func TestSchedHandlerPullEnvironmentMetrics(t *testing.T) {
	env := schemas.Environment{ProjectName: "group/project", Name: "production", ID: 7}

	t.Run("refreshes environment and stores metrics", func(t *testing.T) {
		h := schedNewHarness(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/projects/group/project/environments/7") {
				t.Errorf("unexpected path: %s", r.URL.Path)
				schedServerError(w)
				return
			}
			schedJSON(w, `{"id": 7, "name": "production", "state": "available", "external_url": "https://prod.example.test"}`)
		})
		require.NoError(t, h.st.SetEnvironment(context.Background(), env))
		h.reserve(t, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()), env)

		h.c.TaskHandlerPullEnvironmentMetrics(context.Background(), env)

		stored := schemas.Environment{ProjectName: "group/project", Name: "production"}
		require.NoError(t, h.st.GetEnvironment(context.Background(), &stored))
		assert.True(t, stored.Available)
		assert.Equal(t, "https://prod.example.test", stored.ExternalURL)
		metrics, err := h.st.Metrics(context.Background())
		require.NoError(t, err)
		info := metricsByKind(metrics, schemas.MetricKindEnvironmentInformation)
		require.Len(t, info, 1)
		assert.Equal(t, "production", info[0].Labels["environment"])
		h.assertReleased(t, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()))
	})

	t.Run("gitlab error leaves store untouched", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		require.NoError(t, h.st.SetEnvironment(context.Background(), env))
		h.reserve(t, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()), env)

		h.c.TaskHandlerPullEnvironmentMetrics(context.Background(), env)

		assert.Equal(t, int32(1), calls.Load())
		count, err := h.st.MetricsCount(context.Background())
		require.NoError(t, err)
		assert.Zero(t, count)
		h.assertReleased(t, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()))
	})
}

func TestSchedHandlerPullRunnerMetrics(t *testing.T) {
	runner := schemas.Runner{ProjectName: "group/project", ID: 101}

	t.Run("stores runner metrics", func(t *testing.T) {
		var details atomic.Int32
		h := schedNewHarness(t, schedRunnersServer(t, &details, false))
		h.reserve(t, schemas.TaskTypePullRunnersMetrics, string(runner.Key()), runner)

		h.c.TaskHandlerPullRunnerMetrics(context.Background(), runner)

		metrics, err := h.st.Metrics(context.Background())
		require.NoError(t, err)
		info, ok := findMetricByKindAndLabel(metrics, schemas.MetricKindRunner, "runner_id", "101")
		require.True(t, ok)
		assert.Equal(t, "runner-101", info.Labels["runner_name"])
		h.assertReleased(t, schemas.TaskTypePullRunnersMetrics, string(runner.Key()))
	})

	t.Run("gitlab error keeps previous metrics", func(t *testing.T) {
		var details atomic.Int32
		h := schedNewHarness(t, schedRunnersServer(t, &details, true))
		previous := schemas.Metric{Kind: schemas.MetricKindRunner, Labels: map[string]string{"runner_id": "101"}, Value: 1}
		require.NoError(t, h.st.SetMetric(context.Background(), previous))
		h.reserve(t, schemas.TaskTypePullRunnersMetrics, string(runner.Key()), runner)

		h.c.TaskHandlerPullRunnerMetrics(context.Background(), runner)

		assert.Equal(t, int32(1), details.Load())
		exists, err := h.st.MetricExists(context.Background(), previous.Key())
		require.NoError(t, err)
		assert.True(t, exists, "a failed refresh must not wipe the last known metrics")
		h.assertReleased(t, schemas.TaskTypePullRunnersMetrics, string(runner.Key()))
	})
}

func TestSchedHandlerPullRefsFromProject(t *testing.T) {
	p := schemas.NewProject("group/project")
	p.Pull.Refs.Branches.Enabled = true
	p.Pull.Refs.Branches.Regexp = "^main$"
	p.Pull.Refs.Tags.Enabled = false
	p.Pull.Refs.MergeRequests.Enabled = false

	t.Run("stores new ref and schedules its metrics", func(t *testing.T) {
		h := schedNewHarness(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, "/repository/branches") {
				t.Errorf("unexpected path: %s", r.URL.Path)
				schedServerError(w)
				return
			}
			schedJSON(w, `[{"name": "main"}, {"name": "feature"}]`)
		})
		h.reserve(t, schemas.TaskTypePullRefsFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullRefsFromProject(context.Background(), p)

		refs, err := h.st.Refs(context.Background())
		require.NoError(t, err)
		require.Len(t, refs, 1)
		_, ok := refs[schemas.NewRef(p, schemas.RefKindBranch, "main").Key()]
		assert.True(t, ok)
		assert.Equal(t, []string{string(schemas.TaskTypePullRefMetrics)}, h.publishedNames())
		h.assertReleased(t, schemas.TaskTypePullRefsFromProject, string(p.Key()))
	})

	t.Run("gitlab error", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		h.reserve(t, schemas.TaskTypePullRefsFromProject, string(p.Key()), p)

		h.c.TaskHandlerPullRefsFromProject(context.Background(), p)

		assert.Positive(t, calls.Load())
		count, err := h.st.RefsCount(context.Background())
		require.NoError(t, err)
		assert.Zero(t, count)
		assert.Empty(t, h.published())
		h.assertReleased(t, schemas.TaskTypePullRefsFromProject, string(p.Key()))
	})
}

// schedPipelinesServer serves a pipeline list keyed by the `ref` query
// parameter and pipeline details for the IDs it knows.
func schedPipelinesServer(t *testing.T, lists map[string]string, details map[string]string, seenRefs *[]string, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/api/v4/projects/group/project/pipelines"
		switch {
		case r.URL.Path == prefix:
			ref := r.URL.Query().Get("ref")
			mu.Lock()
			*seenRefs = append(*seenRefs, ref)
			mu.Unlock()
			body, ok := lists[ref]
			if !ok {
				body = `[]`
			}
			schedJSON(w, body)
		case strings.HasPrefix(r.URL.Path, prefix+"/"):
			body, ok := details[strings.TrimPrefix(r.URL.Path, prefix+"/")]
			if !ok {
				schedServerError(w)
				return
			}
			schedJSON(w, body)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			schedServerError(w)
		}
	}
}

func schedRefMetricKindID(t *testing.T, s store.Store) []schemas.Metric {
	t.Helper()
	metrics, err := s.Metrics(context.Background())
	require.NoError(t, err)
	return metricsByKind(metrics, schemas.MetricKindID)
}

func TestSchedPullRefMetrics(t *testing.T) {
	project := schemas.NewProject("group/project")
	branch := schemas.NewRef(project, schemas.RefKindBranch, "main")
	pipeline123 := `{"id": 123, "status": "success", "source": "push", "duration": 12, "updated_at": "2024-03-09T16:00:00Z"}`

	t.Run("branch pipeline updates ref and metrics", func(t *testing.T) {
		var mu sync.Mutex
		var seen []string
		h := schedNewHarness(t, schedPipelinesServer(t,
			map[string]string{"main": `[{"id": 123}]`},
			map[string]string{"123": pipeline123}, &seen, &mu))
		require.NoError(t, h.st.SetRef(context.Background(), branch))

		require.NoError(t, h.c.PullRefMetrics(context.Background(), branch))

		stored := schemas.NewRef(project, schemas.RefKindBranch, "main")
		require.NoError(t, h.st.GetRef(context.Background(), &stored))
		assert.Equal(t, int64(123), stored.LatestPipeline.ID)
		ids := schedRefMetricKindID(t, h.st)
		require.Len(t, ids, 1)
		assert.Equal(t, float64(123), ids[0].Value)
		assert.Equal(t, []string{"main"}, seen)
	})

	t.Run("merge request falls back to merge ref", func(t *testing.T) {
		var mu sync.Mutex
		var seen []string
		h := schedNewHarness(t, schedPipelinesServer(t,
			map[string]string{"refs/merge-requests/7/merge": `[{"id": 200}]`},
			map[string]string{"200": `{"id": 200, "status": "running", "source": "merge_request_event"}`}, &seen, &mu))
		mr := schemas.NewRef(project, schemas.RefKindMergeRequest, "7")
		require.NoError(t, h.st.SetRef(context.Background(), mr))

		require.NoError(t, h.c.PullRefMetrics(context.Background(), mr))

		assert.Equal(t, []string{"refs/merge-requests/7/head", "refs/merge-requests/7/merge"}, seen)
		stored := schemas.NewRef(project, schemas.RefKindMergeRequest, "7")
		require.NoError(t, h.st.GetRef(context.Background(), &stored))
		assert.Equal(t, int64(200), stored.LatestPipeline.ID)
	})

	t.Run("no pipeline is a no-op", func(t *testing.T) {
		var mu sync.Mutex
		var seen []string
		h := schedNewHarness(t, schedPipelinesServer(t, nil, nil, &seen, &mu))
		require.NoError(t, h.st.SetRef(context.Background(), branch))

		require.NoError(t, h.c.PullRefMetrics(context.Background(), branch))

		count, err := h.st.MetricsCount(context.Background())
		require.NoError(t, err)
		assert.Zero(t, count)
	})

	t.Run("pipeline detail failure is logged not returned", func(t *testing.T) {
		var mu sync.Mutex
		var seen []string
		h := schedNewHarness(t, schedPipelinesServer(t,
			map[string]string{"main": `[{"id": 999}]`}, nil, &seen, &mu))
		require.NoError(t, h.st.SetRef(context.Background(), branch))

		require.NoError(t, h.c.PullRefMetrics(context.Background(), branch))

		stored := schemas.NewRef(project, schemas.RefKindBranch, "main")
		require.NoError(t, h.st.GetRef(context.Background(), &stored))
		assert.Zero(t, stored.LatestPipeline.ID)
		assert.Empty(t, schedRefMetricKindID(t, h.st))
	})

	t.Run("list failure returns error", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		err := h.c.PullRefMetrics(context.Background(), branch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error fetching project pipelines for group/project")
	})

	t.Run("merge request fallback failure returns error", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				schedJSON(w, `[]`)
				return
			}
			schedServerError(w)
		})
		err := h.c.PullRefMetrics(context.Background(), schemas.NewRef(project, schemas.RefKindMergeRequest, "7"))
		require.Error(t, err)
		assert.Equal(t, int32(2), calls.Load())
	})

	t.Run("store failure returns before calling gitlab", func(t *testing.T) {
		h := schedNewHarness(t, nil)
		h.st.failReads.Store(true)
		require.ErrorIs(t, h.c.PullRefMetrics(context.Background(), branch), errSchedStoreUnavailable)
	})
}

func TestSchedHandlerPullRefMetrics(t *testing.T) {
	project := schemas.NewProject("group/project")
	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")

	t.Run("success", func(t *testing.T) {
		var mu sync.Mutex
		var seen []string
		h := schedNewHarness(t, schedPipelinesServer(t,
			map[string]string{"main": `[{"id": 123}]`},
			map[string]string{"123": `{"id": 123, "status": "failed", "source": "push"}`}, &seen, &mu))
		require.NoError(t, h.st.SetRef(context.Background(), ref))
		h.reserve(t, schemas.TaskTypePullRefMetrics, string(ref.Key()), ref)

		h.c.TaskHandlerPullRefMetrics(context.Background(), ref)

		stored := schemas.NewRef(project, schemas.RefKindBranch, "main")
		require.NoError(t, h.st.GetRef(context.Background(), &stored))
		assert.Equal(t, "failed", stored.LatestPipeline.Status)
		h.assertReleased(t, schemas.TaskTypePullRefMetrics, string(ref.Key()))
	})

	t.Run("gitlab error", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		require.NoError(t, h.st.SetRef(context.Background(), ref))
		h.reserve(t, schemas.TaskTypePullRefMetrics, string(ref.Key()), ref)

		h.c.TaskHandlerPullRefMetrics(context.Background(), ref)

		assert.Equal(t, int32(1), calls.Load())
		count, err := h.st.MetricsCount(context.Background())
		require.NoError(t, err)
		assert.Zero(t, count)
		h.assertReleased(t, schemas.TaskTypePullRefMetrics, string(ref.Key()))
	})
}

func TestSchedPullRefMostRecentJobsMetrics(t *testing.T) {
	project := schemas.NewProject("group/project")
	project.Pull.Pipeline.Jobs.Enabled = true

	newRef := func() schemas.Ref {
		ref := schemas.NewRef(project, schemas.RefKindBranch, "main")
		ref.LatestJobs["build"] = schemas.Job{ID: 554, Name: "build", Status: "running", PipelineID: 123}
		return ref
	}

	t.Run("no job in memory makes no API call", func(t *testing.T) {
		h := schedNewHarness(t, nil)
		require.NoError(t, h.c.PullRefMostRecentJobsMetrics(context.Background(), schemas.NewRef(project, schemas.RefKindBranch, "main")))
	})

	t.Run("refreshes known jobs", func(t *testing.T) {
		h := schedNewHarness(t, func(w http.ResponseWriter, r *http.Request) {
			assert.True(t, strings.HasSuffix(r.URL.Path, "/projects/group/project/jobs"), r.URL.Path)
			schedJSON(w, `[
				{"id": 556, "name": "build", "ref": "other", "status": "success", "pipeline": {"id": 124}},
				{"id": 555, "name": "build", "ref": "main", "stage": "build", "status": "success", "pipeline": {"id": 124}},
				{"id": 557, "name": "deploy", "ref": "main", "status": "success", "pipeline": {"id": 124}}
			]`)
		})
		ref := newRef()
		require.NoError(t, h.st.SetRef(context.Background(), ref))

		require.NoError(t, h.c.PullRefMostRecentJobsMetrics(context.Background(), ref))

		stored := schemas.NewRef(project, schemas.RefKindBranch, "main")
		require.NoError(t, h.st.GetRef(context.Background(), &stored))
		require.Contains(t, stored.LatestJobs, "build")
		assert.Equal(t, int64(555), stored.LatestJobs["build"].ID)
		assert.Equal(t, "success", stored.LatestJobs["build"].Status)
		assert.NotContains(t, stored.LatestJobs, "deploy", "jobs not tracked in memory must be ignored")
	})

	t.Run("gitlab error is returned", func(t *testing.T) {
		var calls atomic.Int32
		h := schedNewHarness(t, schedAlwaysFail(&calls))
		ref := newRef()
		require.NoError(t, h.st.SetRef(context.Background(), ref))

		require.Error(t, h.c.PullRefMostRecentJobsMetrics(context.Background(), ref))

		stored := schemas.NewRef(project, schemas.RefKindBranch, "main")
		require.NoError(t, h.st.GetRef(context.Background(), &stored))
		assert.Equal(t, int64(554), stored.LatestJobs["build"].ID)
	})
}

// --- fan-out handlers -------------------------------------------------------

func TestSchedHandlerPullProjectsFromWildcards(t *testing.T) {
	h := schedNewHarness(t, nil)
	h.c.Config.Wildcards = []config.Wildcard{
		{Search: "one"},
		{Search: "two"},
	}
	h.reserve(t, schemas.TaskTypePullProjectsFromWildcards, "_")
	before := time.Now()

	h.c.TaskHandlerPullProjectsFromWildcards(context.Background())

	jobs := h.published()
	require.Len(t, jobs, 2)
	searches := make([]string, 0, len(jobs))
	for _, j := range jobs {
		assert.Equal(t, string(schemas.TaskTypePullProjectsFromWildcard), j.TaskName)
		require.Len(t, j.Args, 2)
		w, ok := j.Args[1].(config.Wildcard)
		require.True(t, ok)
		id, ok := j.Args[0].(string)
		require.True(t, ok)
		idx, err := strconv.Atoi(id)
		require.NoError(t, err)
		assert.Equal(t, h.c.Config.Wildcards[idx].Search, w.Search, "wildcard id must match its config index")
		searches = append(searches, w.Search)
	}
	assert.ElementsMatch(t, []string{"one", "two"}, searches)
	assert.False(t, schedLastScheduled(h.c, schemas.TaskTypePullProjectsFromWildcards).Before(before))
	h.assertReleased(t, schemas.TaskTypePullProjectsFromWildcards, "_")
}

func TestSchedHandlerFanOutFromStoredProjects(t *testing.T) {
	cases := []struct {
		name     string
		tt       schemas.TaskType
		followUp schemas.TaskType
		run      func(*Controller, context.Context)
	}{
		{"environments", schemas.TaskTypePullEnvironmentsFromProjects, schemas.TaskTypePullEnvironmentsFromProject, (*Controller).TaskHandlerPullEnvironmentsFromProjects},
		{"runners", schemas.TaskTypePullRunnersFromProjects, schemas.TaskTypePullRunnersFromProject, (*Controller).TaskHandlerPullRunnersFromProjects},
		{"refs", schemas.TaskTypePullRefsFromProjects, schemas.TaskTypePullRefsFromProject, (*Controller).TaskHandlerPullRefsFromProjects},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/schedules one task per project", func(t *testing.T) {
			h := schedNewHarness(t, nil)
			for _, name := range []string{"group/a", "group/b"} {
				require.NoError(t, h.st.SetProject(context.Background(), schemas.NewProject(name)))
			}
			h.reserve(t, tc.tt, "_")
			before := time.Now()

			tc.run(h.c, context.Background())

			names := make([]string, 0)
			for _, j := range h.published() {
				assert.Equal(t, string(tc.followUp), j.TaskName)
				p, ok := j.Args[0].(schemas.Project)
				require.True(t, ok)
				names = append(names, p.Name)
				assert.True(t, schedAdmissionHolds(h.c, tc.followUp, string(p.Key())))
			}
			assert.ElementsMatch(t, []string{"group/a", "group/b"}, names)
			assert.False(t, schedLastScheduled(h.c, tc.tt).Before(before))
			h.assertReleased(t, tc.tt, "_")

			// Running again while follow-ups are still queued must not duplicate them.
			h.resetPublished()
			tc.run(h.c, context.Background())
			assert.Empty(t, h.published())
		})

		t.Run(tc.name+"/store failure schedules nothing", func(t *testing.T) {
			h := schedNewHarness(t, nil)
			require.NoError(t, h.st.SetProject(context.Background(), schemas.NewProject("group/a")))
			h.reserve(t, tc.tt, "_")
			h.st.failReads.Store(true)

			tc.run(h.c, context.Background())

			assert.Empty(t, h.published())
			assert.False(t, schedLastScheduled(h.c, tc.tt).IsZero())
			h.assertReleased(t, tc.tt, "_")
		})

		t.Run(tc.name+"/cancelled context schedules nothing", func(t *testing.T) {
			h := schedNewHarness(t, nil)
			require.NoError(t, h.st.SetProject(context.Background(), schemas.NewProject("group/a")))
			h.reserve(t, tc.tt, "_")

			tc.run(h.c, schedCancelledContext())

			assert.Empty(t, h.published())
			queued, err := h.st.CurrentlyQueuedTasksCount(context.Background())
			require.NoError(t, err)
			assert.Zero(t, queued)
			h.assertReleased(t, tc.tt, "_")
		})
	}
}

func TestSchedHandlerPullMetrics(t *testing.T) {
	project := schemas.NewProject("group/project")
	env := schemas.Environment{ProjectName: project.Name, Name: "production", ID: 7}
	runner := schemas.Runner{ProjectName: project.Name, ID: 101}
	ref := schemas.NewRef(project, schemas.RefKindBranch, "main")

	seed := func(t *testing.T, h *schedHarness) {
		t.Helper()
		ctx := context.Background()
		require.NoError(t, h.st.SetEnvironment(ctx, env))
		require.NoError(t, h.st.SetRunner(ctx, runner))
		require.NoError(t, h.st.SetRef(ctx, ref))
	}

	t.Run("schedules environment, runner and ref metrics", func(t *testing.T) {
		h := schedNewHarness(t, nil)
		seed(t, h)
		h.reserve(t, schemas.TaskTypePullMetrics, "_")
		before := time.Now()

		h.c.TaskHandlerPullMetrics(context.Background())

		assert.ElementsMatch(t, []string{
			string(schemas.TaskTypePullEnvironmentMetrics),
			string(schemas.TaskTypePullRunnersMetrics),
			string(schemas.TaskTypePullRefMetrics),
		}, h.publishedNames())
		assert.True(t, schedAdmissionHolds(h.c, schemas.TaskTypePullEnvironmentMetrics, string(env.Key())))
		assert.True(t, schedAdmissionHolds(h.c, schemas.TaskTypePullRunnersMetrics, string(runner.Key())))
		assert.True(t, schedAdmissionHolds(h.c, schemas.TaskTypePullRefMetrics, string(ref.Key())))
		assert.False(t, schedLastScheduled(h.c, schemas.TaskTypePullMetrics).Before(before))
		h.assertReleased(t, schemas.TaskTypePullMetrics, "_")
	})

	t.Run("store failure schedules nothing", func(t *testing.T) {
		h := schedNewHarness(t, nil)
		seed(t, h)
		h.reserve(t, schemas.TaskTypePullMetrics, "_")
		h.st.failReads.Store(true)

		h.c.TaskHandlerPullMetrics(context.Background())

		assert.Empty(t, h.published())
		assert.False(t, schedLastScheduled(h.c, schemas.TaskTypePullMetrics).IsZero())
		h.assertReleased(t, schemas.TaskTypePullMetrics, "_")
	})
}

// --- garbage collection handlers -------------------------------------------

func TestSchedHandlerGarbageCollect(t *testing.T) {
	orphanRef := schemas.NewRef(schemas.NewProject("gone/project"), schemas.RefKindBranch, "main")
	cases := []struct {
		name  string
		tt    schemas.TaskType
		run   func(*Controller, context.Context) error
		seed  func(context.Context, store.Store) error
		count func(context.Context, store.Store) (int64, error)
	}{
		{
			name: "projects", tt: schemas.TaskTypeGarbageCollectProjects, run: (*Controller).TaskHandlerGarbageCollectProjects,
			seed: func(ctx context.Context, s store.Store) error {
				return s.SetProject(ctx, schemas.NewProject("gone/project"))
			},
			count: func(ctx context.Context, s store.Store) (int64, error) { return s.ProjectsCount(ctx) },
		},
		{
			name: "environments", tt: schemas.TaskTypeGarbageCollectEnvironments, run: (*Controller).TaskHandlerGarbageCollectEnvironments,
			seed: func(ctx context.Context, s store.Store) error {
				return s.SetEnvironment(ctx, schemas.Environment{ProjectName: "gone/project", Name: "production"})
			},
			count: func(ctx context.Context, s store.Store) (int64, error) { return s.EnvironmentsCount(ctx) },
		},
		{
			name: "refs", tt: schemas.TaskTypeGarbageCollectRefs, run: (*Controller).TaskHandlerGarbageCollectRefs,
			seed:  func(ctx context.Context, s store.Store) error { return s.SetRef(ctx, orphanRef) },
			count: func(ctx context.Context, s store.Store) (int64, error) { return s.RefsCount(ctx) },
		},
		{
			name: "runners", tt: schemas.TaskTypeGarbageCollectRunners, run: (*Controller).TaskHandlerGarbageCollectRunners,
			seed: func(ctx context.Context, s store.Store) error {
				return s.SetRunner(ctx, schemas.Runner{ProjectName: "gone/project", ID: 42})
			},
			count: func(ctx context.Context, s store.Store) (int64, error) { return s.RunnersCount(ctx) },
		},
		{
			name: "metrics", tt: schemas.TaskTypeGarbageCollectMetrics, run: (*Controller).TaskHandlerGarbageCollectMetrics,
			seed: func(ctx context.Context, s store.Store) error {
				return s.SetMetric(ctx, schemas.Metric{Kind: schemas.MetricKindCoverage, Labels: orphanRef.DefaultLabelsValues()})
			},
			count: func(ctx context.Context, s store.Store) (int64, error) { return s.MetricsCount(ctx) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/deletes orphans", func(t *testing.T) {
			h := schedNewHarness(t, nil)
			ctx := context.Background()
			require.NoError(t, tc.seed(ctx, h.st))
			h.reserve(t, tc.tt, "_")
			before := time.Now()

			require.NoError(t, tc.run(h.c, ctx))

			count, err := tc.count(ctx, h.st)
			require.NoError(t, err)
			assert.Zero(t, count)
			assert.False(t, schedLastScheduled(h.c, tc.tt).Before(before))
			h.assertReleased(t, tc.tt, "_")
		})

		t.Run(tc.name+"/store failure is returned", func(t *testing.T) {
			h := schedNewHarness(t, nil)
			ctx := context.Background()
			require.NoError(t, tc.seed(ctx, h.st))
			h.reserve(t, tc.tt, "_")
			h.st.failReads.Store(true)

			require.ErrorIs(t, tc.run(h.c, ctx), errSchedStoreUnavailable)

			h.st.failReads.Store(false)
			count, err := tc.count(ctx, h.st)
			require.NoError(t, err)
			assert.Equal(t, int64(1), count, "nothing may be deleted when the listing failed")
			assert.False(t, schedLastScheduled(h.c, tc.tt).IsZero())
			h.assertReleased(t, tc.tt, "_")
		})
	}
}

func TestSchedHandlerGarbageCollectProjectsKeepsWildcardMatches(t *testing.T) {
	h := schedNewHarness(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/groups/platform/projects")
		schedJSON(w, `[{"id": 1, "path_with_namespace": "platform/kept"}]`)
	})
	h.c.Config.Wildcards = []config.Wildcard{{Owner: config.WildcardOwner{Kind: "group", Name: "platform"}}}
	ctx := context.Background()
	require.NoError(t, h.st.SetProject(ctx, schemas.NewProject("platform/kept")))
	require.NoError(t, h.st.SetProject(ctx, schemas.NewProject("platform/gone")))
	h.reserve(t, schemas.TaskTypeGarbageCollectProjects, "_")

	require.NoError(t, h.c.TaskHandlerGarbageCollectProjects(ctx))

	projects, err := h.st.Projects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	_, kept := projects[schemas.NewProject("platform/kept").Key()]
	assert.True(t, kept)
	h.assertReleased(t, schemas.TaskTypeGarbageCollectProjects, "_")
}

func TestSchedHandlerGarbageCollectProjectsGitLabErrorKeepsProjects(t *testing.T) {
	var calls atomic.Int32
	h := schedNewHarness(t, schedAlwaysFail(&calls))
	h.c.Config.Wildcards = []config.Wildcard{{Owner: config.WildcardOwner{Kind: "group", Name: "platform"}}}
	ctx := context.Background()
	require.NoError(t, h.st.SetProject(ctx, schemas.NewProject("platform/kept")))
	h.reserve(t, schemas.TaskTypeGarbageCollectProjects, "_")

	require.Error(t, h.c.TaskHandlerGarbageCollectProjects(ctx))

	count, err := h.st.ProjectsCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "an unreachable GitLab must not cause projects to be collected")
	h.assertReleased(t, schemas.TaskTypeGarbageCollectProjects, "_")
}

// TestSchedHandlerRunsThroughRealQueue drives a registered handler through the
// in-memory taskq consumer to verify the reflection-based wiring end to end.
func TestSchedHandlerRunsThroughRealQueue(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalStore()
	c := &Controller{Store: s, TaskController: NewTaskController(ctx, nil, 1), UUID: uuid.New()}
	c.registerTasks()
	t.Cleanup(func() { _ = c.TaskController.Queue.Close() })
	require.NoError(t, s.SetProject(ctx, schemas.NewProject("gone/project")))

	c.ScheduleTask(ctx, schemas.TaskTypeGarbageCollectProjects, "_")

	require.Eventually(t, func() bool {
		executed, err := s.ExecutedTasksCount(ctx)
		return err == nil && executed == 1
	}, 2*time.Second, 5*time.Millisecond)
	count, err := s.ProjectsCount(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.False(t, schedLastScheduled(c, schemas.TaskTypeGarbageCollectProjects).IsZero())
	require.Eventually(t, func() bool {
		return len(c.TaskController.admission.slots) == 0
	}, time.Second, 5*time.Millisecond, "capacity slot was not returned")

	// With capacity 1, a second run is only possible because the slot was released.
	require.NoError(t, s.SetProject(ctx, schemas.NewProject("gone/again")))
	c.ScheduleTask(ctx, schemas.TaskTypeGarbageCollectProjects, "_")
	require.Eventually(t, func() bool {
		executed, err := s.ExecutedTasksCount(ctx)
		return err == nil && executed == 2
	}, 2*time.Second, 5*time.Millisecond)
}

func TestSchedMonitorLastTaskScheduling(t *testing.T) {
	tc := NewTaskController(context.Background(), nil, 1)
	t.Cleanup(func() { _ = tc.Queue.Close() })
	next := time.Now().Add(time.Hour)
	tc.TaskSchedulingMonitoring.SetNext(schemas.TaskTypePullMetrics, next)
	before := time.Now()

	tc.monitorLastTaskScheduling(schemas.TaskTypePullMetrics)

	status := tc.TaskSchedulingMonitoring.Snapshot()[schemas.TaskTypePullMetrics]
	assert.False(t, status.Last.Before(before))
	assert.False(t, status.Last.After(time.Now()))
	assert.True(t, status.Next.Equal(next), "recording Last must not clobber Next")
	_, other := tc.TaskSchedulingMonitoring.Snapshot()[schemas.TaskTypeGarbageCollectMetrics]
	assert.False(t, other)
}
