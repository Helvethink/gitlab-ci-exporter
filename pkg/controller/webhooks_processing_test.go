package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/taskq/v4"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

const (
	whProject     = "group/project"
	whSecretToken = "wh-test-secret"
	whZeroSHA     = "0000000000000000000000000000000000000000"
)

// whQueuedTask is a task reservation observed through Store.QueueTask, which
// ScheduleTask calls synchronously before publishing the job.
type whQueuedTask struct {
	Type schemas.TaskType
	ID   string
}

// whRecordingStore wraps a real store and records every task reservation.
type whRecordingStore struct {
	store.Store

	mu     sync.Mutex
	queued []whQueuedTask
}

func (s *whRecordingStore) QueueTask(ctx context.Context, tt schemas.TaskType, uniqueID, owner string) (bool, error) {
	s.mu.Lock()
	s.queued = append(s.queued, whQueuedTask{Type: tt, ID: uniqueID})
	s.mu.Unlock()
	return s.Store.QueueTask(ctx, tt, uniqueID, owner)
}

func (s *whRecordingStore) tasks() []whQueuedTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]whQueuedTask(nil), s.queued...)
}

// whExecuted records the arguments that task handlers actually received.
type whExecuted struct {
	mu       sync.Mutex
	refs     []schemas.Ref
	envs     []schemas.Environment
	projects map[string]config.ProjectPull
}

func (e *whExecuted) envByName(name string) (schemas.Environment, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, env := range e.envs {
		if env.Name == name {
			return env, true
		}
	}
	return schemas.Environment{}, false
}

func (e *whExecuted) project(name string) (config.ProjectPull, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	pull, ok := e.projects[name]
	return pull, ok
}

func (e *whExecuted) refCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.refs)
}

// whFixture is a controller backed by a local store, an in-memory task queue
// with recording handlers, and a fake GitLab API.
type whFixture struct {
	c        *Controller
	store    *whRecordingStore
	executed *whExecuted

	mu       sync.Mutex
	apiPaths []string
}

func (f *whFixture) gitlabPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.apiPaths...)
}

// whNewFixture builds the fixture. gitlab may be nil, in which case any GitLab
// API call fails the test.
func whNewFixture(t *testing.T, gitlab http.HandlerFunc) *whFixture {
	t.Helper()

	f := &whFixture{executed: &whExecuted{projects: map[string]config.ProjectPull{}}}
	f.c = newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.apiPaths = append(f.apiPaths, r.URL.EscapedPath())
		f.mu.Unlock()
		if gitlab == nil {
			t.Errorf("unexpected GitLab API call: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		gitlab(w, r)
	})
	f.store = &whRecordingStore{Store: f.c.Store}
	f.c.Store = f.store
	f.c.UUID = uuid.New()
	f.c.TaskController = NewTaskController(context.Background(), nil, 64)
	t.Cleanup(func() { _ = f.c.TaskController.Queue.Close() })

	exec := f.executed
	_, err := f.c.TaskController.TaskMap.Register(string(schemas.TaskTypePullRefMetrics), &taskq.TaskConfig{
		Handler: func(_ context.Context, ref schemas.Ref) {
			exec.mu.Lock()
			exec.refs = append(exec.refs, ref)
			exec.mu.Unlock()
		},
	})
	require.NoError(t, err)
	_, err = f.c.TaskController.TaskMap.Register(string(schemas.TaskTypePullEnvironmentMetrics), &taskq.TaskConfig{
		Handler: func(_ context.Context, env schemas.Environment) {
			exec.mu.Lock()
			exec.envs = append(exec.envs, env)
			exec.mu.Unlock()
		},
	})
	require.NoError(t, err)
	_, err = f.c.TaskController.TaskMap.Register(string(schemas.TaskTypePullProject), &taskq.TaskConfig{
		Handler: func(_ context.Context, name string, pull config.ProjectPull) {
			exec.mu.Lock()
			exec.projects[name] = pull
			exec.mu.Unlock()
		},
	})
	require.NoError(t, err)

	return f
}

// whConfiguredProject returns a project exporting branches matching ^main$,
// tags matching ^v\d and merge requests, plus environments matching ^prod.
func whConfiguredProject() schemas.Project {
	p := schemas.NewProject(whProject)
	p.Pull.Refs.Branches.Enabled = true
	p.Pull.Refs.Branches.Regexp = `^main$`
	p.Pull.Refs.Tags.Enabled = true
	p.Pull.Refs.Tags.Regexp = `^v\d`
	p.Pull.Refs.MergeRequests.Enabled = true
	p.Pull.Environments.Enabled = true
	p.Pull.Environments.Regexp = `^prod`
	return p
}

func whRef(project string, kind schemas.RefKind, name string) schemas.Ref {
	return schemas.NewRef(schemas.NewProject(project), kind, name)
}

func whRefExists(t *testing.T, s store.Store, ref schemas.Ref) bool {
	t.Helper()
	exists, err := s.RefExists(context.Background(), ref.Key())
	require.NoError(t, err)
	return exists
}

func whPipelineEvent(project, ref string, tag bool, mrIID int64) goGitlab.PipelineEvent {
	var e goGitlab.PipelineEvent
	e.Project.PathWithNamespace = project
	e.ObjectAttributes.Ref = ref
	e.ObjectAttributes.Tag = tag
	e.MergeRequest.IID = mrIID
	return e
}

func TestWebhookPipelineEventRefSelection(t *testing.T) {
	tests := []struct {
		name       string
		event      goGitlab.PipelineEvent
		configure  func(p *schemas.Project)
		skipStore  bool
		wantRef    schemas.Ref
		wantStored bool
	}{
		{
			name:       "configured branch is stored and pulled",
			event:      whPipelineEvent(whProject, "main", false, 0),
			wantRef:    whRef(whProject, schemas.RefKindBranch, "main"),
			wantStored: true,
		},
		{
			name:    "branch not matching the project regexp is ignored",
			event:   whPipelineEvent(whProject, "feature-x", false, 0),
			wantRef: whRef(whProject, schemas.RefKindBranch, "feature-x"),
		},
		{
			name:       "tag pipeline is stored as a tag ref",
			event:      whPipelineEvent(whProject, "v1.2.3", true, 0),
			wantRef:    whRef(whProject, schemas.RefKindTag, "v1.2.3"),
			wantStored: true,
		},
		{
			name:    "tag not matching the tag regexp is ignored",
			event:   whPipelineEvent(whProject, "release-1", true, 0),
			wantRef: whRef(whProject, schemas.RefKindTag, "release-1"),
		},
		{
			name:    "tag refs disabled for the project",
			event:   whPipelineEvent(whProject, "v1.2.3", true, 0),
			wantRef: whRef(whProject, schemas.RefKindTag, "v1.2.3"),
			configure: func(p *schemas.Project) {
				p.Pull.Refs.Tags.Enabled = false
			},
		},
		{
			name:       "merge request pipeline is keyed by the MR IID, not the source branch",
			event:      whPipelineEvent(whProject, "feature-x", false, 42),
			wantRef:    whRef(whProject, schemas.RefKindMergeRequest, "42"),
			wantStored: true,
		},
		{
			name:    "merge request refs disabled for the project",
			event:   whPipelineEvent(whProject, "feature-x", false, 42),
			wantRef: whRef(whProject, schemas.RefKindMergeRequest, "42"),
			configure: func(p *schemas.Project) {
				p.Pull.Refs.MergeRequests.Enabled = false
			},
		},
		{
			name:      "unknown project without wildcards is ignored",
			event:     whPipelineEvent("other/project", "main", false, 0),
			wantRef:   whRef("other/project", schemas.RefKindBranch, "main"),
			skipStore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, nil)
			p := whConfiguredProject()
			if tt.configure != nil {
				tt.configure(&p)
			}
			if !tt.skipStore {
				require.NoError(t, f.c.Store.SetProject(ctx, p))
			}

			f.c.processPipelineEvent(ctx, tt.event)

			assert.Equal(t, tt.wantStored, whRefExists(t, f.c.Store, tt.wantRef))
			if !tt.wantStored {
				assert.Empty(t, f.store.tasks(), "no task must be scheduled")
				count, err := f.c.Store.RefsCount(ctx)
				require.NoError(t, err)
				assert.Zero(t, count, "no ref must be created")
				return
			}

			assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullRefMetrics, ID: string(tt.wantRef.Key())}}, f.store.tasks())

			// The stored ref carries the project configuration from the store,
			// not the defaults of the project built from the event.
			stored := tt.wantRef
			require.NoError(t, f.c.Store.GetRef(ctx, &stored))
			assert.Equal(t, `^main$`, stored.Project.Pull.Refs.Branches.Regexp)

			require.Eventually(t, func() bool { return f.executed.refCount() == 1 }, 2*time.Second, 5*time.Millisecond)
		})
	}
}

func TestWebhookPipelineEventForExistingRefSkipsProjectLookup(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, nil)
	// The project is unknown and the branch would not match any config: a ref
	// already present in the store is pulled regardless.
	ref := whRef("legacy/project", schemas.RefKindBranch, "feature-x")
	require.NoError(t, f.c.Store.SetRef(ctx, ref))

	f.c.processPipelineEvent(ctx, whPipelineEvent("legacy/project", "feature-x", false, 0))

	assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullRefMetrics, ID: string(ref.Key())}}, f.store.tasks())
	count, err := f.c.Store.RefsCount(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
}

func TestWebhookPipelineEventDoesNotRescheduleQueuedRef(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, nil)
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))
	// The recording handlers never dequeue, so the first reservation is
	// still held when the second webhook arrives.
	e := whPipelineEvent(whProject, "main", false, 0)
	f.c.processPipelineEvent(ctx, e)
	f.c.processPipelineEvent(ctx, e)

	queued, err := f.c.Store.CurrentlyQueuedTasksCount(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, queued, "the second webhook for the same ref must be deduplicated")
	assert.Len(t, f.store.tasks(), 2, "both webhooks attempt a reservation")
	require.Eventually(t, func() bool { return f.executed.refCount() == 1 }, 2*time.Second, 5*time.Millisecond)
}

func TestWebhookPipelineEventWildcardDiscovery(t *testing.T) {
	branchWildcard := func(ownerKind, ownerName, regexp string) config.Wildcard {
		w := config.NewWildcard()
		w.Owner.Kind = ownerKind
		w.Owner.Name = ownerName
		w.Pull.Refs.Branches.Enabled = true
		w.Pull.Refs.Branches.Regexp = regexp
		return w
	}

	tests := []struct {
		name        string
		wildcards   config.Wildcards
		project     string
		wantProject bool
		wantRegexp  string
	}{
		{
			name:        "owner matches the project namespace",
			wildcards:   config.Wildcards{branchWildcard("group", "team", `^main$`)},
			project:     "team/app",
			wantProject: true,
			wantRegexp:  `^main$`,
		},
		{
			name:      "owner does not match the project namespace",
			wildcards: config.Wildcards{branchWildcard("group", "team", `^main$`)},
			project:   "other/app",
		},
		{
			name:        "global wildcard (no owner kind) matches any project",
			wildcards:   config.Wildcards{branchWildcard("", "ignored", `^ma`)},
			project:     "anyone/app",
			wantProject: true,
			wantRegexp:  `^ma`,
		},
		{
			name:      "ref not matching the wildcard regexp",
			wildcards: config.Wildcards{branchWildcard("", "", `^develop$`)},
			project:   "team/app",
		},
		{
			name: "branches disabled in the wildcard",
			wildcards: func() config.Wildcards {
				w := branchWildcard("", "", `.*`)
				w.Pull.Refs.Branches.Enabled = false
				return config.Wildcards{w}
			}(),
			project: "team/app",
		},
		{
			name: "invalid regexp in one wildcard does not prevent a later match",
			wildcards: config.Wildcards{
				branchWildcard("", "", `([`),
				branchWildcard("", "", `^main$`),
			},
			project:     "team/app",
			wantProject: true,
			wantRegexp:  `^main$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, nil)
			f.c.Config.Wildcards = tt.wildcards

			f.c.processPipelineEvent(ctx, whPipelineEvent(tt.project, "main", false, 0))

			// Wildcard discovery never stores the ref directly: it delegates
			// to a project pull which will discover refs itself.
			count, err := f.c.Store.RefsCount(ctx)
			require.NoError(t, err)
			assert.Zero(t, count)

			if !tt.wantProject {
				assert.Empty(t, f.store.tasks())
				return
			}
			assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullProject, ID: tt.project}}, f.store.tasks())
			require.Eventually(t, func() bool {
				_, ok := f.executed.project(tt.project)
				return ok
			}, 2*time.Second, 5*time.Millisecond)
			pull, _ := f.executed.project(tt.project)
			assert.Equal(t, tt.wantRegexp, pull.Refs.Branches.Regexp, "the matching wildcard pull config is forwarded")
		})
	}
}

// TestWebhookWildcardOwnerMatchesOnlyItsNamespace is a regression test: owner
// matching used strings.Contains, so owner "team" also matched "otherteam/app".
func TestWebhookWildcardOwnerMatchesOnlyItsNamespace(t *testing.T) {
	tests := []struct {
		project          string
		includeSubgroups bool
		want             bool
	}{
		{project: "team/app", want: true},
		{project: "team/app", includeSubgroups: true, want: true},
		{project: "team/sub/app", want: false},
		{project: "team/sub/app", includeSubgroups: true, want: true},
		{project: "otherteam/app", includeSubgroups: true, want: false},
		{project: "x/team-tools", includeSubgroups: true, want: false},
		{project: "team-tools/app", includeSubgroups: true, want: false},
		{project: "x/team/app", includeSubgroups: true, want: false},
		{project: "team", includeSubgroups: true, want: false},
	}

	for _, tt := range tests {
		w := config.NewWildcard()
		w.Owner.Kind = "group"
		w.Owner.Name = "team"
		w.Owner.IncludeSubgroups = tt.includeSubgroups
		w.Pull.Environments.Enabled = true

		matches, err := isRefMatchingWilcard(w, whRef(tt.project, schemas.RefKindBranch, "main"))
		require.NoError(t, err)
		assert.Equal(t, tt.want, matches, "ref of %s (include_subgroups=%v)", tt.project, tt.includeSubgroups)

		matches, err = isEnvMatchingWilcard(w, schemas.Environment{ProjectName: tt.project, Name: "production"})
		require.NoError(t, err)
		assert.Equal(t, tt.want, matches, "env of %s (include_subgroups=%v)", tt.project, tt.includeSubgroups)
	}
}

func TestWebhookJobEvent(t *testing.T) {
	gitlab := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v4/projects/123" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"path_with_namespace":"group/project"}`))
	}

	tests := []struct {
		name      string
		projectID int64
		ref       string
		tag       bool
		wantRef   schemas.Ref
		wantPull  bool
	}{
		{
			name:      "branch job resolves the project path from GitLab",
			projectID: 123,
			ref:       "main",
			wantRef:   whRef(whProject, schemas.RefKindBranch, "main"),
			wantPull:  true,
		},
		{
			name:      "tag job",
			projectID: 123,
			ref:       "v2.0.0",
			tag:       true,
			wantRef:   whRef(whProject, schemas.RefKindTag, "v2.0.0"),
			wantPull:  true,
		},
		{
			name:      "non matching branch",
			projectID: 123,
			ref:       "feature-x",
			wantRef:   whRef(whProject, schemas.RefKindBranch, "feature-x"),
		},
		{
			name:      "project lookup failure stops processing",
			projectID: 999,
			ref:       "main",
			wantRef:   whRef(whProject, schemas.RefKindBranch, "main"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, gitlab)
			require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))

			f.c.processJobEvent(ctx, goGitlab.JobEvent{ProjectID: tt.projectID, Ref: tt.ref, Tag: tt.tag})

			assert.Contains(t, f.gitlabPaths(), "/api/v4/projects/"+strconv.FormatInt(tt.projectID, 10))
			assert.Equal(t, tt.wantPull, whRefExists(t, f.c.Store, tt.wantRef))
			if tt.wantPull {
				assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullRefMetrics, ID: string(tt.wantRef.Key())}}, f.store.tasks())
			} else {
				assert.Empty(t, f.store.tasks())
			}
		})
	}
}

func TestWebhookPushEventDeletesBranchOnlyOnDeletion(t *testing.T) {
	tests := []struct {
		name        string
		ref         string
		checkoutSHA string
		wantDeleted bool
	}{
		{name: "branch deletion (zero after SHA, no checkout SHA)", ref: "refs/heads/feature/x", wantDeleted: true},
		{name: "regular push keeps the ref", ref: "refs/heads/feature/x", checkoutSHA: "da1560886d4f094c3e6c9ef40349f7d38b5d27d7"},
		{name: "unexpected ref format is ignored", ref: "feature/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, nil)
			deleted := whRef(whProject, schemas.RefKindBranch, "feature/x")
			sameNameOtherProject := whRef("other/project", schemas.RefKindBranch, "feature/x")
			sameNameTag := whRef(whProject, schemas.RefKindTag, "feature/x")
			for _, r := range []schemas.Ref{deleted, sameNameOtherProject, sameNameTag} {
				require.NoError(t, f.c.Store.SetRef(ctx, r))
			}

			var e goGitlab.PushEvent
			e.Ref = tt.ref
			e.After = whZeroSHA
			e.CheckoutSHA = tt.checkoutSHA
			e.Project.PathWithNamespace = whProject
			f.c.processPushEvent(ctx, e)

			assert.Equal(t, !tt.wantDeleted, whRefExists(t, f.c.Store, deleted))
			assert.True(t, whRefExists(t, f.c.Store, sameNameOtherProject), "same branch in another project must be kept")
			assert.True(t, whRefExists(t, f.c.Store, sameNameTag), "tag with the same name must be kept")
			assert.Empty(t, f.store.tasks())
		})
	}
}

func TestWebhookTagEventDeletesTagOnlyOnDeletion(t *testing.T) {
	tests := []struct {
		name        string
		ref         string
		checkoutSHA string
		wantDeleted bool
	}{
		{name: "tag deletion", ref: "refs/tags/v1.0.0", wantDeleted: true},
		{name: "tag creation keeps the ref", ref: "refs/tags/v1.0.0", checkoutSHA: "da1560886d4f094c3e6c9ef40349f7d38b5d27d7"},
		{name: "branch-style ref is ignored", ref: "refs/heads/v1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, nil)
			tag := whRef(whProject, schemas.RefKindTag, "v1.0.0")
			branch := whRef(whProject, schemas.RefKindBranch, "v1.0.0")
			require.NoError(t, f.c.Store.SetRef(ctx, tag))
			require.NoError(t, f.c.Store.SetRef(ctx, branch))

			var e goGitlab.TagEvent
			e.Ref = tt.ref
			e.After = whZeroSHA
			e.CheckoutSHA = tt.checkoutSHA
			e.Project.PathWithNamespace = whProject
			f.c.processTagEvent(ctx, e)

			assert.Equal(t, !tt.wantDeleted, whRefExists(t, f.c.Store, tag))
			assert.True(t, whRefExists(t, f.c.Store, branch), "branch with the same name must be kept")
			assert.Empty(t, f.store.tasks())
		})
	}
}

func TestWebhookMergeEventDeletesClosedOrMergedRequests(t *testing.T) {
	for _, tt := range []struct {
		action      string
		wantDeleted bool
	}{
		{action: "merge", wantDeleted: true},
		{action: "close", wantDeleted: true},
		{action: "open"},
		{action: "reopen"},
		{action: "update"},
		{action: "approved"},
	} {
		t.Run(tt.action, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, nil)
			mr := whRef(whProject, schemas.RefKindMergeRequest, "42")
			otherMR := whRef(whProject, schemas.RefKindMergeRequest, "43")
			branch := whRef(whProject, schemas.RefKindBranch, "42")
			for _, r := range []schemas.Ref{mr, otherMR, branch} {
				require.NoError(t, f.c.Store.SetRef(ctx, r))
			}

			var e goGitlab.MergeEvent
			e.Project.PathWithNamespace = whProject
			e.ObjectAttributes.IID = 42
			e.ObjectAttributes.Action = tt.action
			f.c.processMergeEvent(ctx, e)

			assert.Equal(t, !tt.wantDeleted, whRefExists(t, f.c.Store, mr))
			assert.True(t, whRefExists(t, f.c.Store, otherMR))
			assert.True(t, whRefExists(t, f.c.Store, branch))
			assert.Empty(t, f.store.tasks())
		})
	}
}

func TestWebhookMergeEventForUnknownRefIsHarmless(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, nil)
	var e goGitlab.MergeEvent
	e.Project.PathWithNamespace = whProject
	e.ObjectAttributes.IID = 7
	e.ObjectAttributes.Action = "merge"

	f.c.processMergeEvent(ctx, e)

	count, err := f.c.Store.RefsCount(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func whDeploymentEvent(project, env string) goGitlab.DeploymentEvent {
	var e goGitlab.DeploymentEvent
	e.Project.PathWithNamespace = project
	e.Environment = env
	return e
}

func TestWebhookDeploymentEventForKnownEnvironment(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, nil)
	env := schemas.Environment{ProjectName: whProject, Name: "staging", ID: 7}
	require.NoError(t, f.c.Store.SetEnvironment(ctx, env))

	f.c.processDeploymentEvent(ctx, whDeploymentEvent(whProject, "staging"))

	// Already exported environments are pulled even if the project config
	// would not select them, and without any GitLab call.
	assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullEnvironmentMetrics, ID: string(env.Key())}}, f.store.tasks())
	require.Eventually(t, func() bool {
		_, ok := f.executed.envByName("staging")
		return ok
	}, 2*time.Second, 5*time.Millisecond)
	got, _ := f.executed.envByName("staging")
	assert.EqualValues(t, 7, got.ID, "the environment ID is refreshed from the store")
	assert.Empty(t, f.gitlabPaths())
}

func TestWebhookDeploymentEventIgnoredEnvironments(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		env       string
		configure func(p *schemas.Project)
	}{
		{name: "unknown project without wildcards", project: "other/project", env: "production"},
		{name: "environment not matching the project regexp", project: whProject, env: "review/feature-x"},
		{
			name:    "environments disabled for the project",
			project: whProject,
			env:     "production",
			configure: func(p *schemas.Project) {
				p.Pull.Environments.Enabled = false
			},
		},
		{
			name:    "invalid environment regexp",
			project: whProject,
			env:     "production",
			configure: func(p *schemas.Project) {
				p.Pull.Environments.Regexp = `([`
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, nil)
			p := whConfiguredProject()
			if tt.configure != nil {
				tt.configure(&p)
			}
			require.NoError(t, f.c.Store.SetProject(ctx, p))

			f.c.processDeploymentEvent(ctx, whDeploymentEvent(tt.project, tt.env))

			assert.Empty(t, f.store.tasks())
			assert.Empty(t, f.gitlabPaths())
			count, err := f.c.Store.EnvironmentsCount(ctx)
			require.NoError(t, err)
			assert.Zero(t, count)
		})
	}
}

func TestWebhookDeploymentEventGitLabFailureSchedulesNothing(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))

	f.c.processDeploymentEvent(ctx, whDeploymentEvent(whProject, "production"))

	assert.NotEmpty(t, f.gitlabPaths(), "a matching new environment is looked up on GitLab")
	assert.Empty(t, f.store.tasks())
	count, err := f.c.Store.EnvironmentsCount(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestWebhookDeploymentEventResolvesNewEnvironmentByName(t *testing.T) {
	t.Skip("BUG: deployment webhook for a matching, not-yet-stored environment calls GET /projects/:id/environments/0 (the event carries no environment ID), so the environment is never resolved on real GitLab")

	ctx := context.Background()
	f := whNewFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/api/v4/projects/group%2Fproject/environments":
			_, _ = w.Write([]byte(`[{"id":11,"name":"production","state":"available"}]`))
		case "/api/v4/projects/group%2Fproject/environments/11":
			_, _ = w.Write([]byte(`{"id":11,"name":"production","state":"available"}`))
		default:
			http.NotFound(w, r)
		}
	})
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))

	f.c.processDeploymentEvent(ctx, whDeploymentEvent(whProject, "production"))

	assert.NotContains(t, f.gitlabPaths(), "/api/v4/projects/group%2Fproject/environments/0")
	env := schemas.Environment{ProjectName: whProject, Name: "production"}
	require.NoError(t, f.c.Store.GetEnvironment(ctx, &env))
	assert.EqualValues(t, 11, env.ID)
	assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullEnvironmentMetrics, ID: string(env.Key())}}, f.store.tasks())
}

func TestWebhookDeploymentEventWildcardDiscovery(t *testing.T) {
	envWildcard := func(ownerKind, ownerName string, enabled bool, regexp string) config.Wildcard {
		w := config.NewWildcard()
		w.Owner.Kind = ownerKind
		w.Owner.Name = ownerName
		w.Pull.Environments.Enabled = enabled
		w.Pull.Environments.Regexp = regexp
		return w
	}

	tests := []struct {
		name        string
		wildcards   config.Wildcards
		project     string
		wantProject bool
	}{
		{name: "owner and regexp match", wildcards: config.Wildcards{envWildcard("group", "team", true, `^prod`)}, project: "team/app", wantProject: true},
		{name: "global wildcard", wildcards: config.Wildcards{envWildcard("", "", true, `.*`)}, project: "any/app", wantProject: true},
		{name: "owner mismatch", wildcards: config.Wildcards{envWildcard("group", "team", true, `.*`)}, project: "other/app"},
		{name: "environments disabled", wildcards: config.Wildcards{envWildcard("", "", false, `.*`)}, project: "team/app"},
		{name: "regexp mismatch", wildcards: config.Wildcards{envWildcard("", "", true, `^staging$`)}, project: "team/app"},
		{
			name: "invalid regexp is skipped, later wildcard matches",
			wildcards: config.Wildcards{
				envWildcard("", "", true, `([`),
				envWildcard("", "", true, `^prod`),
			},
			project:     "team/app",
			wantProject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := whNewFixture(t, nil)
			f.c.Config.Wildcards = tt.wildcards

			f.c.processDeploymentEvent(ctx, whDeploymentEvent(tt.project, "production"))

			if tt.wantProject {
				assert.Equal(t, []whQueuedTask{{Type: schemas.TaskTypePullProject, ID: tt.project}}, f.store.tasks())
			} else {
				assert.Empty(t, f.store.tasks())
			}
			count, err := f.c.Store.EnvironmentsCount(ctx)
			require.NoError(t, err)
			assert.Zero(t, count)
			assert.Empty(t, f.gitlabPaths())
		})
	}
}

func TestWebhookIsRefMatchingProjectPullRefs(t *testing.T) {
	pprs := whConfiguredProject().Pull.Refs

	tests := []struct {
		name    string
		mutate  func(p *config.ProjectPullRefs)
		ref     schemas.Ref
		want    bool
		wantErr bool
	}{
		{name: "branch match", ref: whRef(whProject, schemas.RefKindBranch, "main"), want: true},
		{name: "branch regexp is anchored", ref: whRef(whProject, schemas.RefKindBranch, "main-old")},
		{name: "branch disabled", ref: whRef(whProject, schemas.RefKindBranch, "main"), mutate: func(p *config.ProjectPullRefs) { p.Branches.Enabled = false }},
		{name: "tag match", ref: whRef(whProject, schemas.RefKindTag, "v3"), want: true},
		{name: "tag disabled", ref: whRef(whProject, schemas.RefKindTag, "v3"), mutate: func(p *config.ProjectPullRefs) { p.Tags.Enabled = false }},
		{name: "merge request IID", ref: whRef(whProject, schemas.RefKindMergeRequest, "42"), want: true},
		{name: "merge request full ref", ref: whRef(whProject, schemas.RefKindMergeRequest, "refs/merge-requests/42/head"), want: true},
		{name: "merge request non numeric", ref: whRef(whProject, schemas.RefKindMergeRequest, "feature-x")},
		{name: "merge requests disabled", ref: whRef(whProject, schemas.RefKindMergeRequest, "42"), mutate: func(p *config.ProjectPullRefs) { p.MergeRequests.Enabled = false }},
		{name: "invalid branch regexp", ref: whRef(whProject, schemas.RefKindBranch, "main"), mutate: func(p *config.ProjectPullRefs) { p.Branches.Regexp = `([` }, wantErr: true},
		{name: "invalid ref kind", ref: whRef(whProject, schemas.RefKind("bogus"), "main"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := pprs
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}
			got, err := isRefMatchingProjectPullRefs(cfg, tt.ref)
			if tt.wantErr {
				require.Error(t, err)
				assert.False(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWebhookIsEnvMatchingProjectPullEnvironments(t *testing.T) {
	ppe := config.ProjectPullEnvironments{Enabled: true, Regexp: `^prod`}
	env := func(name string) schemas.Environment { return schemas.Environment{ProjectName: whProject, Name: name} }

	got, err := isEnvMatchingProjectPullEnvironments(ppe, env("production"))
	require.NoError(t, err)
	assert.True(t, got)

	got, err = isEnvMatchingProjectPullEnvironments(ppe, env("staging"))
	require.NoError(t, err)
	assert.False(t, got)

	disabled := ppe
	disabled.Enabled = false
	got, err = isEnvMatchingProjectPullEnvironments(disabled, env("production"))
	require.NoError(t, err)
	assert.False(t, got)

	invalid := ppe
	invalid.Regexp = `([`
	_, err = isEnvMatchingProjectPullEnvironments(invalid, env("production"))
	require.Error(t, err)
}

func TestWebhookWildcardOwnerFilters(t *testing.T) {
	w := config.NewWildcard()
	w.Pull.Refs.Branches.Enabled = true
	w.Pull.Refs.Branches.Regexp = `.*`
	w.Pull.Environments.Enabled = true
	w.Pull.Environments.Regexp = `.*`

	// Global wildcard: owner name is irrelevant when kind is empty.
	w.Owner = config.WildcardOwner{Name: "nobody"}
	got, err := isRefMatchingWilcard(w, whRef("team/app", schemas.RefKindBranch, "main"))
	require.NoError(t, err)
	assert.True(t, got)
	got, err = isEnvMatchingWilcard(w, schemas.Environment{ProjectName: "team/app", Name: "prod"})
	require.NoError(t, err)
	assert.True(t, got)

	// Owner-scoped wildcard.
	w.Owner = config.WildcardOwner{Kind: "group", Name: "team"}
	got, err = isRefMatchingWilcard(w, whRef("team/app", schemas.RefKindBranch, "main"))
	require.NoError(t, err)
	assert.True(t, got)
	got, err = isRefMatchingWilcard(w, whRef("other/app", schemas.RefKindBranch, "main"))
	require.NoError(t, err)
	assert.False(t, got)
	got, err = isEnvMatchingWilcard(w, schemas.Environment{ProjectName: "other/app", Name: "prod"})
	require.NoError(t, err)
	assert.False(t, got)

	// Errors from the pull config propagate.
	w.Owner = config.WildcardOwner{}
	w.Pull.Environments.Regexp = `([`
	_, err = isEnvMatchingWilcard(w, schemas.Environment{ProjectName: "team/app", Name: "prod"})
	require.Error(t, err)
}

func TestWebhookProcessWebhookEventDispatch(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"path_with_namespace":"group/project"}`))
	})
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))
	h := f.c.NewWebhookHandler(ctx).(*webhookHandler)

	branch := whRef(whProject, schemas.RefKindBranch, "gone")
	tag := whRef(whProject, schemas.RefKindTag, "v0-gone")
	mr := whRef(whProject, schemas.RefKindMergeRequest, "5")
	for _, r := range []schemas.Ref{branch, tag, mr} {
		require.NoError(t, f.c.Store.SetRef(ctx, r))
	}
	require.NoError(t, f.c.Store.SetEnvironment(ctx, schemas.Environment{ProjectName: whProject, Name: "production", ID: 3}))

	pipeline := whPipelineEvent(whProject, "main", false, 0)
	h.processWebhookEvent(ctx, &pipeline)
	assert.True(t, whRefExists(t, f.c.Store, whRef(whProject, schemas.RefKindBranch, "main")))

	job := goGitlab.JobEvent{ProjectID: 123, Ref: "v9", Tag: true}
	h.processWebhookEvent(ctx, &job)
	assert.True(t, whRefExists(t, f.c.Store, whRef(whProject, schemas.RefKindTag, "v9")))

	push := goGitlab.PushEvent{Ref: "refs/heads/gone", After: whZeroSHA}
	push.Project.PathWithNamespace = whProject
	h.processWebhookEvent(ctx, &push)
	assert.False(t, whRefExists(t, f.c.Store, branch))

	tagEvent := goGitlab.TagEvent{Ref: "refs/tags/v0-gone", After: whZeroSHA}
	tagEvent.Project.PathWithNamespace = whProject
	h.processWebhookEvent(ctx, &tagEvent)
	assert.False(t, whRefExists(t, f.c.Store, tag))

	var merge goGitlab.MergeEvent
	merge.Project.PathWithNamespace = whProject
	merge.ObjectAttributes.IID = 5
	merge.ObjectAttributes.Action = "close"
	h.processWebhookEvent(ctx, &merge)
	assert.False(t, whRefExists(t, f.c.Store, mr))

	deployment := whDeploymentEvent(whProject, "production")
	h.processWebhookEvent(ctx, &deployment)

	// Unsupported or nil events are logged and ignored.
	h.processWebhookEvent(ctx, &goGitlab.IssueEvent{})
	h.processWebhookEvent(ctx, nil)

	var types []schemas.TaskType
	for _, task := range f.store.tasks() {
		types = append(types, task.Type)
	}
	assert.Equal(t, []schemas.TaskType{
		schemas.TaskTypePullRefMetrics,
		schemas.TaskTypePullRefMetrics,
		schemas.TaskTypePullEnvironmentMetrics,
	}, types)
}

// whPostWebhook sends a webhook through the HTTP handler.
func whPostWebhook(ctx context.Context, h http.Handler, eventType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("X-Gitlab-Token", whSecretToken)
	req.Header.Set("X-Gitlab-Event", eventType)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestWebhookHandlerProcessesRealPayloads(t *testing.T) {
	ctx := context.Background()
	f := whNewFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"path_with_namespace":"group/project"}`))
	})
	f.c.Config.Server.Webhook.SecretToken = whSecretToken
	require.NoError(t, f.c.Store.SetProject(ctx, whConfiguredProject()))
	deletedBranch := whRef(whProject, schemas.RefKindBranch, "feature/old")
	deletedTag := whRef(whProject, schemas.RefKindTag, "v0.1.0")
	mergedMR := whRef(whProject, schemas.RefKindMergeRequest, "17")
	for _, r := range []schemas.Ref{deletedBranch, deletedTag, mergedMR} {
		require.NoError(t, f.c.Store.SetRef(ctx, r))
	}
	h := f.c.NewWebhookHandler(ctx)

	tests := []struct {
		eventType string
		body      string
		done      func() bool
	}{
		{
			eventType: "Pipeline Hook",
			body:      `{"object_kind":"pipeline","object_attributes":{"id":1,"ref":"main","tag":false},"project":{"id":123,"path_with_namespace":"group/project"}}`,
			done: func() bool {
				return whRefExists(t, f.c.Store, whRef(whProject, schemas.RefKindBranch, "main"))
			},
		},
		{
			eventType: "Pipeline Hook",
			body:      `{"object_kind":"pipeline","object_attributes":{"id":2,"ref":"feature-x","tag":false},"merge_request":{"iid":8},"project":{"id":123,"path_with_namespace":"group/project"}}`,
			done: func() bool {
				return whRefExists(t, f.c.Store, whRef(whProject, schemas.RefKindMergeRequest, "8"))
			},
		},
		{
			eventType: "Job Hook",
			body:      `{"object_kind":"build","ref":"v1.0.0","tag":true,"project_id":123}`,
			done: func() bool {
				return whRefExists(t, f.c.Store, whRef(whProject, schemas.RefKindTag, "v1.0.0"))
			},
		},
		{
			eventType: "Push Hook",
			body: `{"object_kind":"push","before":"da1560886d4f094c3e6c9ef40349f7d38b5d27d7","after":"` + whZeroSHA +
				`","ref":"refs/heads/feature/old","checkout_sha":null,"project":{"path_with_namespace":"group/project"}}`,
			done: func() bool { return !whRefExists(t, f.c.Store, deletedBranch) },
		},
		{
			eventType: "Tag Push Hook",
			body: `{"object_kind":"tag_push","before":"da1560886d4f094c3e6c9ef40349f7d38b5d27d7","after":"` + whZeroSHA +
				`","ref":"refs/tags/v0.1.0","checkout_sha":null,"project":{"path_with_namespace":"group/project"}}`,
			done: func() bool { return !whRefExists(t, f.c.Store, deletedTag) },
		},
		{
			eventType: "Merge Request Hook",
			body:      `{"object_kind":"merge_request","project":{"path_with_namespace":"group/project"},"object_attributes":{"iid":17,"action":"merge","state":"merged"}}`,
			done:      func() bool { return !whRefExists(t, f.c.Store, mergedMR) },
		},
	}

	for _, tt := range tests {
		rr := whPostWebhook(ctx, h, tt.eventType, tt.body)
		require.Equal(t, http.StatusOK, rr.Code, "%s: %s", tt.eventType, rr.Body.String())
		require.Eventually(t, tt.done, 2*time.Second, 5*time.Millisecond, tt.eventType)
	}
}

func TestWebhookHandlerRejectsUnsupportedEventType(t *testing.T) {
	c := &Controller{Config: config.Config{Server: config.Server{Webhook: config.ServerWebhook{SecretToken: whSecretToken}}}}
	h := c.NewWebhookHandler(context.Background()).(*webhookHandler)
	h.process = func(context.Context, any) { t.Error("unsupported events must not be processed") }

	rr := whPostWebhook(context.Background(), h, "Issue Hook", `{"object_kind":"issue"}`)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Empty(t, h.processingSlots)
}
