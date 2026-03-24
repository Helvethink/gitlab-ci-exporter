package schemas

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func checksumKey(s string) MetricKey {
	return MetricKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(s)))))
}

func TestMetricKey_PipelineMetric(t *testing.T) {
	m := Metric{
		Kind: MetricKindCoverage,
		Labels: prometheus.Labels{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   `{"foo":"bar"}`,
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 85.5,
	}

	expectedRaw := strconv.Itoa(int(MetricKindCoverage)) + fmt.Sprintf("%v", []string{
		"group/project",
		"branch",
		"main",
		"push",
		`{"foo":"bar"}`,
		"123",
		"success",
	})

	assert.Equal(t, checksumKey(expectedRaw), m.Key())
}

func TestMetricKey_JobMetric(t *testing.T) {
	m := Metric{
		Kind: MetricKindJobDurationSeconds,
		Labels: prometheus.Labels{
			"project":        "group/project",
			"kind":           "branch",
			"ref":            "main",
			"stage":          "test",
			"tag_list":       "docker,linux",
			"job_name":       "unit-tests",
			"job_id":         "456",
			"pipeline_id":    "123",
			"status":         "success",
			"failure_reason": "",
		},
		Value: 42,
	}

	expectedRaw := strconv.Itoa(int(MetricKindJobDurationSeconds)) + fmt.Sprintf("%v", []string{
		"group/project",
		"branch",
		"main",
		"test",
		"docker,linux",
		"unit-tests",
		"456",
		"123",
		"success",
		"",
	})

	assert.Equal(t, checksumKey(expectedRaw), m.Key())
}

func TestMetricKey_EnvironmentMetric(t *testing.T) {
	m := Metric{
		Kind: MetricKindEnvironmentInformation,
		Labels: prometheus.Labels{
			"project":     "group/project",
			"environment": "production",
		},
		Value: 1,
	}

	expectedRaw := strconv.Itoa(int(MetricKindEnvironmentInformation)) + fmt.Sprintf("%v", []string{
		"group/project",
		"production",
	})

	assert.Equal(t, checksumKey(expectedRaw), m.Key())
}

func TestMetricKey_TestSuiteMetric(t *testing.T) {
	m := Metric{
		Kind: MetricKindTestSuiteTotalCount,
		Labels: prometheus.Labels{
			"project":         "group/project",
			"kind":            "branch",
			"ref":             "main",
			"test_suite_name": "suite-a",
		},
		Value: 12,
	}

	expectedRaw := strconv.Itoa(int(MetricKindTestSuiteTotalCount)) + fmt.Sprintf("%v", []string{
		"group/project",
		"branch",
		"main",
		"suite-a",
	})

	assert.Equal(t, checksumKey(expectedRaw), m.Key())
}

func TestMetricKey_TestCaseMetric(t *testing.T) {
	m := Metric{
		Kind: MetricKindTestCaseExecutionTime,
		Labels: prometheus.Labels{
			"project":             "group/project",
			"kind":                "branch",
			"ref":                 "main",
			"test_suite_name":     "suite-a",
			"test_case_name":      "TestLogin",
			"test_case_classname": "auth_test",
		},
		Value: 1.23,
	}

	expectedRaw := strconv.Itoa(int(MetricKindTestCaseExecutionTime)) + fmt.Sprintf("%v", []string{
		"group/project",
		"branch",
		"main",
		"suite-a",
		"TestLogin",
		"auth_test",
	})

	assert.Equal(t, checksumKey(expectedRaw), m.Key())
}

func TestMetricKey_RunnerMetric(t *testing.T) {
	m := Metric{
		Kind: MetricKindRunner,
		Labels: prometheus.Labels{
			"project":                 "group/project",
			"kind":                    "runner",
			"runner_id":               "12",
			"runner_description":      "shared-runner",
			"runner_groups":           `[{"id":1,"name":"group1"}]`,
			"runner_projects":         `[{"id":2,"name":"project1"}]`,
			"runner_maintenance_note": "maintenance",
			"contacted_at":            "2026-03-24T10:00:00Z",
			"paused":                  "false",
			"runner_type":             "instance_type",
			"tag_list":                "docker,linux",
			"is_shared":               "true",
			"active":                  "true",
		},
		Value: 1,
	}

	expectedRaw := strconv.Itoa(int(MetricKindRunner)) + fmt.Sprintf("%v", []string{
		"group/project",
		"runner",
		"12",
		"shared-runner",
		`[{"id":1,"name":"group1"}]`,
		`[{"id":2,"name":"project1"}]`,
		"maintenance",
		"2026-03-24T10:00:00Z",
		"false",
		"instance_type",
		"docker,linux",
		"true",
		"true",
	})

	assert.Equal(t, checksumKey(expectedRaw), m.Key())
}

func TestMetricKey_ValueDoesNotAffectKey(t *testing.T) {
	m1 := Metric{
		Kind: MetricKindCoverage,
		Labels: prometheus.Labels{
			"project":     "group/project",
			"kind":        "branch",
			"ref":         "main",
			"source":      "push",
			"variables":   "",
			"pipeline_id": "123",
			"status":      "success",
		},
		Value: 10,
	}

	m2 := Metric{
		Kind:   m1.Kind,
		Labels: m1.Labels,
		Value:  99,
	}

	assert.Equal(t, m1.Key(), m2.Key())
}

func TestMetricKey_JobStatusDependsOnStatus(t *testing.T) {
	base := prometheus.Labels{
		"project":        "group/project",
		"kind":           "branch",
		"ref":            "main",
		"stage":          "test",
		"tag_list":       "docker",
		"job_name":       "unit-tests",
		"job_id":         "456",
		"pipeline_id":    "123",
		"failure_reason": "",
	}

	m1 := Metric{
		Kind: MetricKindJobStatus,
		Labels: func() prometheus.Labels {
			l := prometheus.Labels{}
			for k, v := range base {
				l[k] = v
			}
			l["status"] = "success"
			return l
		}(),
	}

	m2 := Metric{
		Kind: MetricKindJobStatus,
		Labels: func() prometheus.Labels {
			l := prometheus.Labels{}
			for k, v := range base {
				l[k] = v
			}
			l["status"] = "failed"
			return l
		}(),
	}

	assert.NotEqual(t, m1.Key(), m2.Key())
}

func TestMetricKey_EnvironmentDeploymentStatusDependsOnStatus(t *testing.T) {
	m1 := Metric{
		Kind: MetricKindEnvironmentDeploymentStatus,
		Labels: prometheus.Labels{
			"project":     "group/project",
			"environment": "production",
			"status":      "success",
		},
	}

	m2 := Metric{
		Kind: MetricKindEnvironmentDeploymentStatus,
		Labels: prometheus.Labels{
			"project":     "group/project",
			"environment": "production",
			"status":      "failed",
		},
	}

	assert.NotEqual(t, m1.Key(), m2.Key())
}
