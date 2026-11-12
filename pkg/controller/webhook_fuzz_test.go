package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/paulbellamy/ratecounter"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	gitlabclient "github.com/helvethink/gitlab-ci-exporter/pkg/gitlab"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

const fuzzWebhookSecret = "fuzz-webhook-secret"

var fuzzWebhookEventTypes = []string{
	string(goGitlab.EventTypePipeline),
	string(goGitlab.EventTypeJob),
	string(goGitlab.EventTypeDeployment),
	string(goGitlab.EventTypePush),
	string(goGitlab.EventTypeTagPush),
	string(goGitlab.EventTypeMergeRequest),
	string(goGitlab.EventTypeIssue),
	"",
	"Unknown Hook",
}

var fuzzWebhookPayloads = []string{
	`{"object_kind":"pipeline","object_attributes":{"id":1,"ref":"main","tag":false},"project":{"path_with_namespace":"group/project"}}`,
	`{"object_kind":"build","ref":"main","tag":false,"project_name":"group / project","pipeline_id":1}`,
	`{"object_kind":"deployment","environment":"production","project":{"path_with_namespace":"group/project"}}`,
	`{"object_kind":"push","ref":"refs/heads/main","after":"0000000000000000000000000000000000000000","project":{"path_with_namespace":"group/project"}}`,
	`{"object_kind":"tag_push","ref":"refs/tags/v1","after":"0000000000000000000000000000000000000000","project":{"path_with_namespace":"group/project"}}`,
	`{"object_kind":"merge_request","object_attributes":{"iid":1,"action":"close"},"project":{"path_with_namespace":"group/project"}}`,
	`{}`,
	`[]`,
	`null`,
	`{"object_kind":"pipeline","project":null,"object_attributes":null}`,
	``,
}

// fuzzWebhookController returns a controller whose GitLab API answers 404 to
// every request, with the webhook enabled. It is built once per fuzz target:
// starting a server per input exhausts local ports at fuzzing speed.
func fuzzWebhookController(tb testing.TB) *Controller {
	tb.Helper()

	server := httptest.NewServer(http.NotFoundHandler())
	tb.Cleanup(server.Close)

	gl, err := goGitlab.NewClient("fuzz-token", goGitlab.WithBaseURL(server.URL+"/api/v4"), goGitlab.WithoutRetries())
	if err != nil {
		tb.Fatalf("create GitLab client: %v", err)
	}

	c := &Controller{
		Store: store.NewLocalStore(),
		Gitlab: &gitlabclient.Client{
			Client:      gl,
			RateLimiter: noopLimiterControllerRunnersTests{},
			RateCounter: ratecounter.NewRateCounter(time.Second),
		},
	}
	c.Config.Server.Webhook.Enabled = true
	c.Config.Server.Webhook.SecretToken = fuzzWebhookSecret
	c.Config.Server.Webhook.MaximumBodyBytes = 64 << 10
	c.Config.Server.Webhook.MaximumConcurrentProcessing = 2
	return c
}

// FuzzWebhookHTTP sends arbitrary payloads and event types through the HTTP
// handler. The handler must never panic, must only answer with documented
// status codes, and must hand only supported event types to processing.
func FuzzWebhookHTTP(f *testing.F) {
	for _, eventType := range fuzzWebhookEventTypes {
		for _, payload := range fuzzWebhookPayloads {
			f.Add(eventType, []byte(payload), true)
		}
	}
	f.Add(string(goGitlab.EventTypePipeline), []byte(fuzzWebhookPayloads[0]), false)

	allowed := []int{
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusForbidden,
		http.StatusRequestEntityTooLarge,
		http.StatusUnprocessableEntity,
		http.StatusServiceUnavailable,
	}

	c := fuzzWebhookController(f)

	f.Fuzz(func(t *testing.T, eventType string, payload []byte, validToken bool) {
		handler, ok := c.NewWebhookHandler(context.Background()).(*webhookHandler)
		if !ok {
			t.Fatalf("unexpected handler type")
		}

		var (
			mu        sync.Mutex
			processed []any
			done      = make(chan struct{}, 1)
		)
		handler.process = func(_ context.Context, event any) {
			mu.Lock()
			processed = append(processed, event)
			mu.Unlock()
			done <- struct{}{}
		}

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set("X-Gitlab-Event", eventType)
		if validToken {
			req.Header.Set("X-Gitlab-Token", fuzzWebhookSecret)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !slices.Contains(allowed, rr.Code) {
			t.Fatalf("unexpected status %d for event %q", rr.Code, eventType)
		}
		if !validToken && rr.Code != http.StatusForbidden {
			t.Fatalf("request without token answered %d", rr.Code)
		}
		if rr.Code != http.StatusOK {
			return
		}

		<-done
		mu.Lock()
		defer mu.Unlock()
		if len(processed) != 1 || !isSupportedWebhookEvent(processed[0]) {
			t.Fatalf("unsupported event accepted: %T", processed)
		}
	})
}

// FuzzWebhookProcessing runs parsed events through the real processing code
// against a GitLab API that returns 404. Processing must never panic.
func FuzzWebhookProcessing(f *testing.F) {
	for _, eventType := range fuzzWebhookEventTypes[:6] {
		for _, payload := range fuzzWebhookPayloads {
			f.Add(eventType, []byte(payload))
		}
	}

	base := fuzzWebhookController(f)

	f.Fuzz(func(t *testing.T, eventType string, payload []byte) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.Header.Set("X-Gitlab-Event", eventType)
		event, err := goGitlab.ParseHook(goGitlab.HookEventType(req), payload)
		if err != nil || !isSupportedWebhookEvent(event) {
			return
		}

		c := *base
		c.Store = store.NewLocalStore()
		h := &webhookHandler{controller: &c}
		h.processWebhookEvent(context.Background(), event)
	})
}
