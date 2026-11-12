package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	goGitlab "gitlab.com/gitlab-org/api/client-go"
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
// every request, with the webhook enabled.
func fuzzWebhookController(t testing.TB) *Controller {
	t.Helper()
	tt, ok := t.(*testing.T)
	if !ok {
		t.Fatalf("fuzzWebhookController requires *testing.T")
	}
	c := newTestRunnerController(tt, func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})
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

	f.Fuzz(func(t *testing.T, eventType string, payload []byte, validToken bool) {
		c := fuzzWebhookController(t)
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

	f.Fuzz(func(t *testing.T, eventType string, payload []byte) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.Header.Set("X-Gitlab-Event", eventType)
		event, err := goGitlab.ParseHook(goGitlab.HookEventType(req), payload)
		if err != nil || !isSupportedWebhookEvent(event) {
			return
		}

		c := fuzzWebhookController(t)
		h := &webhookHandler{controller: c}
		h.processWebhookEvent(context.Background(), event)
	})
}
