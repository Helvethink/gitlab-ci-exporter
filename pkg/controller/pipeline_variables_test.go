package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// sensitivePipelineVariables mimics a GitLab response containing credential-like values.
const sensitivePipelineVariables = `[
	{"key":"DEPLOY_ENV","value":"SENSITIVE-production"},
	{"key":"API_TOKEN","value":"glpat-SENSITIVEVALUE0001"},
	{"key":"DB_PASSWORD","value":"SENSITIVE:pa,ss"}
]`

func TestPipelineVariableValuesNeverReachMetrics(t *testing.T) {
	tests := []struct {
		name          string
		enabled       bool
		keys          []string
		regexp        string
		wantVariables string
		wantAPICall   bool
	}{
		{name: "disabled", enabled: false, regexp: `.*`, wantVariables: ""},
		{name: "enabled with default-deny configuration", enabled: true, wantVariables: ""},
		{name: "enabled with explicit keys", enabled: true, keys: []string{"DEPLOY_ENV"}, wantVariables: "DEPLOY_ENV", wantAPICall: true},
		{name: "enabled with match-all regexp", enabled: true, regexp: `.*`, wantVariables: "API_TOKEN,DB_PASSWORD,DEPLOY_ENV", wantAPICall: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			previousOut, previousLevel := log.StandardLogger().Out, log.GetLevel()
			log.SetOutput(&logs)
			log.SetLevel(log.TraceLevel)
			t.Cleanup(func() {
				log.SetOutput(previousOut)
				log.SetLevel(previousLevel)
			})

			variablesRequested := false
			c := newTestRunnerController(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/pipelines/123/variables") {
					variablesRequested = true
					_, _ = w.Write([]byte(sensitivePipelineVariables))
					return
				}
				_, _ = w.Write([]byte(`{"id":123,"duration":12,"source":"push","status":"success","updated_at":"2024-03-09T16:00:00Z"}`))
			})

			project := schemas.NewProject("group/project")
			project.Pull.Pipeline.Variables.Enabled = tt.enabled
			project.Pull.Pipeline.Variables.Regexp = tt.regexp
			keys, err := config.NewPipelineVariableKeys(tt.keys...)
			require.NoError(t, err)
			project.Pull.Pipeline.Variables.Keys = keys
			ref := schemas.NewRef(project, schemas.RefKindBranch, "main")

			require.NoError(t, c.ProcessPipelinesMetrics(context.Background(), ref, &goGitlab.PipelineInfo{ID: 123}))
			assert.Equal(t, tt.wantAPICall, variablesRequested)

			rr := httptest.NewRecorder()
			c.MetricsHandler(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			require.Equal(t, http.StatusOK, rr.Code)
			body, err := io.ReadAll(rr.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), `variables="`+tt.wantVariables+`"`)
			assert.NotContains(t, string(body), "SENSITIVE")
			assert.NotContains(t, logs.String(), "SENSITIVE")

			stored, err := c.Store.GetPipelineVariables(context.Background(), schemas.Pipeline{ID: 123})
			require.NoError(t, err)
			assert.NotContains(t, stored, "SENSITIVE")
		})
	}
}
