# Coverage Baseline

Captured on 2026-11-11 from commit `a9e41e3` plus the Phase 0 working-tree fixes (in-repo OpenTelemetry logrus hook, monitor-server socket-path test fix, `toolchain go1.26.11`).

```sh
GOTOOLCHAIN=go1.26.11 go test -race -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Total statement coverage: **67.2%**.

> Measured with the pinned `go1.26.11` toolchain, which matches CI. A run with Go 1.27.0 reported noticeably higher numbers for the same code (total 76.1%, `pkg/controller` 66.4%, `pkg/monitor/ui` 36.2%), so always compare against this baseline with the same toolchain.

## Concurrency-sensitive packages

| Package | Coverage | Functions | Functions at 0% |
| --- | ---: | ---: | ---: |
| `pkg/controller` | 58.6% | 161 | 35 |
| `pkg/store` | 76.1% | 109 | 20 |
| `pkg/monitor/server` | 85.3% | 4 | 0 |
| `pkg/monitor/ui` | 13.7% | 12 | 9 |
| `pkg/ratelimit` | 86.7% | 6 | 0 |

Function counts come from `go tool cover -func`.

### Largest gaps

`pkg/controller`:

- `scheduler.go`: every `TaskHandler*` function (pull and garbage-collect handlers) and `monitorLastTaskScheduling` are at 0%.
- `webhooks.go`: `processPipelineEvent`, `processJobEvent`, `processPushEvent`, `processTagEvent`, `processMergeEvent`, `processDeploymentEvent`, `triggerRefDeletion`, `triggerRefMetricsPull`, `triggerEnvironmentMetricsPull`, and the ref/env matching helpers are at 0%.
- `handlers.go`: `processWebhookEvent` is at 0%.
- `pipelines.go`: `PullRefMetrics` is at 0%.
- `runners.go`: `PullRunnersFromProject` is at 0%.
- `jobs.go`: `PullRefMostRecentJobsMetrics` is at 25%.
- `controller.go`: `configureTracing` is at 8.6%.

`pkg/store` (Redis backend):

- The `Has*Expired` and `getTTL*Key` functions are at 0%.
- The runner CRUD functions (`SetRunner`, `GetRunner`, `DelRunner`, `RunnerExists`, `Runners`, `RunnersCount`) are at 0%.
- The pipeline cache functions (`SetPipeline`, `GetPipeline`, `PipelineExists`, `PipelineVariablesExists`) are at 0%.

`pkg/monitor/ui`:

- `newModel`, `Init`, `Update`, `View`, `streamTelemetry`, `waitForTelemetryUpdate`, `Start`, `setPaneContent`, and `renderConfigViewport` are at 0%.

## Other packages

| Package | Coverage |
| --- | ---: |
| `internal/cli` | 84.6% |
| `internal/cmd` | 83.2% |
| `internal/collectors` | 100.0% |
| `internal/httpServer` | 90.9% |
| `internal/logging` | 76.7% |
| `pkg/config` | 81.7% |
| `pkg/gitlab` | 86.8% |
| `pkg/monitor` | 95.0% |
| `pkg/monitor/client` | 88.9% |
| `pkg/schemas` | 93.5% |

`cmd/gitlab-ci-exporter` and `pkg/monitor/protobuf` (generated code) have no tests.

## After Phase 4 (2026-11-12)

Measured on branch `feature/regression-coverage` (commit `7c1c136` plus `pkg/monitor/server/failure_paths_test.go`) with the same command and toolchain.

Total statement coverage: **83.3%** (baseline 67.2%).

| Package | Baseline | After Phase 4 | Functions at 0% |
| --- | ---: | ---: | ---: |
| `pkg/controller` | 58.6% | 83.0% | 0 of 162 |
| `pkg/store` | 76.1% | 93.8% | 0 of 113 |
| `pkg/monitor/server` | 85.3% | 90.9% | 0 of 8 |
| `pkg/monitor/ui` | 13.7% | 92.6% | 0 of 12 |
| `pkg/ratelimit` | 86.7% | 86.7% | 0 of 6 |

What is left uncovered is mostly unreachable from tests:

- marshalling failures of fixed types;
- OS-level failures, such as `chmod` or `stat` on the monitoring socket;
- the part of `ui.Start` that runs the full-screen terminal program.
