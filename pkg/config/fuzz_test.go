package config

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fuzzSecret is a recognizable credential used in seeds.
const fuzzSecret = "fuzzsecretvalue0123456789"

// fuzzSentinel is assigned to the token and webhook secret after parsing. No seed
// contains it, so finding it in an output can only be a redaction failure.
const fuzzSentinel = "sentinelTOKENnotinanyseed42"

var configSeeds = []string{
	"",
	"gitlab:\n  url: https://gitlab.example.com\n  token: " + fuzzSecret + "\nprojects:\n  - name: group/project\n",
	"wildcards:\n  - owner:\n      name: group\n      kind: group\n      include_subgroups: true\n",
	"redis:\n  url: redis://default:" + fuzzSecret + "@redis:6379/0\nwildcards:\n  - {}\n",
	"opentelemetry:\n  grpc_endpoint: localhost:4317\n  insecure: true\n",
	"project_defaults:\n  pull:\n    pipeline:\n      variables:\n        enabled: true\n        keys: [DEPLOY_ENV, RELEASE]\n        regexp: ^CI_\n",
	"projects:\n  - name: a\n    pull:\n      refs:\n        branches:\n          regexp: '['\n",
	"projects: not-a-list\n",
	"log: [unclosed",
	"projects:\n  - &a {name: x}\n  - *a\n",
}

// FuzzParse checks that arbitrary configuration input never panics, and that a
// successfully parsed configuration can be serialized, re-parsed, and summarized
// without leaking the GitLab token, webhook secret, or Redis credentials.
func FuzzParse(f *testing.F) {
	for _, seed := range configSeeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := Parse(FormatYAML, data)
		if err != nil {
			return
		}
		_ = cfg.Validate()

		cfg.Gitlab.Token = fuzzSentinel
		cfg.Server.Webhook.SecretToken = fuzzSentinel

		serialized := cfg.ToYAML()
		if strings.Contains(serialized, fuzzSentinel) {
			t.Fatalf("ToYAML leaked a secret:\n%s", serialized)
		}
		if _, err := Parse(FormatYAML, []byte(serialized)); err != nil {
			t.Fatalf("ToYAML output does not parse back: %v\n%s", err, serialized)
		}

		view, err := cfg.MonitoringView().YAML()
		if err != nil {
			t.Fatalf("render monitoring view: %v", err)
		}
		if strings.Contains(view, fuzzSentinel) || strings.Contains(view, fuzzSecret) {
			t.Fatalf("monitoring view leaked a secret:\n%s", view)
		}
	})
}

// FuzzPipelineVariableKeys checks the allowlist invariants: accepted keys are
// valid GitLab variable keys, sorted, unique, and survive a YAML round trip.
func FuzzPipelineVariableKeys(f *testing.F) {
	for _, seed := range []string{"", "RELEASE", "DEPLOY_ENV,RELEASE,RELEASE", " A ,b_2", "A:B", "A,,B", "ÄÖ", "A\nB"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		keys, err := NewPipelineVariableKeys(strings.Split(raw, ",")...)
		if err != nil {
			return
		}

		list := keys.List()
		if !slices.IsSorted(list) {
			t.Fatalf("keys are not sorted: %q", list)
		}
		if len(slices.Compact(slices.Clone(list))) != len(list) {
			t.Fatalf("keys are not unique: %q", list)
		}
		for _, k := range list {
			if !pipelineVariableKeyPattern.MatchString(k) {
				t.Fatalf("invalid key accepted: %q", k)
			}
			if !keys.Contains(k) {
				t.Fatalf("Contains(%q) = false", k)
			}
		}

		out, err := yaml.Marshal(struct {
			Keys PipelineVariableKeys `yaml:"keys"`
		}{keys})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var back struct {
			Keys PipelineVariableKeys `yaml:"keys"`
		}
		if err := yaml.Unmarshal(out, &back); err != nil {
			t.Fatalf("unmarshal %q: %v", out, err)
		}
		if back.Keys != keys {
			t.Fatalf("round trip changed keys: %q -> %q", list, back.Keys.List())
		}
	})
}

// FuzzRedactURL checks that a password in the user info of a URL never survives
// redaction.
func FuzzRedactURL(f *testing.F) {
	for _, seed := range []string{
		"redis://default:pw@redis:6379/0",
		"https://user:pw@gitlab.example.com/?private_token=pw",
		"rediss://:pw@[::1]:6380",
		"not a url %zz",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		redacted := RedactURL(raw)

		_, err := url.Parse(raw)
		if err != nil {
			if redacted != "" && redacted != "*******" {
				t.Fatalf("unparseable URL not fully redacted: %q -> %q", raw, redacted)
			}
			return
		}

		// Structural check on the output: every sensitive component is either
		// absent or replaced by the "redacted" marker.
		out, err := url.Parse(redacted)
		if err != nil {
			t.Fatalf("redacted URL does not parse: %q -> %q: %v", raw, redacted, err)
		}
		if out.User != nil && out.User.String() != "redacted" {
			t.Fatalf("user info survived redaction: %q -> %q", raw, redacted)
		}
		if out.RawQuery != "" && out.RawQuery != "redacted" {
			t.Fatalf("query survived redaction: %q -> %q", raw, redacted)
		}
		if out.Fragment != "" && out.Fragment != "redacted" {
			t.Fatalf("fragment survived redaction: %q -> %q", raw, redacted)
		}
		if at := strings.LastIndex(out.Opaque, "@"); at >= 0 && out.Opaque[:at] != "redacted" {
			t.Fatalf("opaque credentials survived redaction: %q -> %q", raw, redacted)
		}
	})
}
