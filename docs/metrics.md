# GitLab CI Pipelines Exporter - Metrics

## Metrics

| Metric name | Description | Labels | Configuration |
|---|---|---|---|
| `gcpe_currently_queued_tasks_count` | Number of tasks in the queue || *available by default* |
| `gcpe_environments_count` | Number of GitLab environments being exported || *available by default* |
| `gcpe_executed_tasks_count` | Number of tasks executed || *available by default* |
| `gcpe_gitlab_api_requests_count` | GitLab API requests count || *available by default* |
| `gcpe_gitlab_api_requests_remaining` | GitLab API requests remaining in the API Limit || *available by default* |
| `gcpe_gitlab_api_requests_limit` | GitLab API requests available in the API Limit || *available by default* |
| `gcpe_metrics_count` | Number of GitLab pipelines metrics being exported || *available by default* |
| `gcpe_projects_count` | Number of GitLab projects being exported || *available by default* |
| `gcpe_refs_count` | Number of GitLab refs being exported || *available by default* |
| `gitlab_ci_environment_behind_commits_count` | Number of commits the environment is behind given its last deployment | [project], [environment] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_environment_behind_duration_seconds` | Duration in seconds the environment is behind the most recent commit given its last deployment | [project], [environment] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_environment_deployment_count` |Number of deployments for an environment | [project], [environment] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_environment_deployment_duration_seconds` | Duration in seconds of the most recent deployment of the environment | [project], [environment] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_environment_deployment_job_id` | ID of the most recent deployment job for an environment | [project], [environment] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_environment_deployment_status` | Status of the most recent deployment of the environment | [project], [environment], [status] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_environment_deployment_timestamp` | Creation date of the most recent deployment of the environment | [project], [environment] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_environment_information` | Information about the environment | [project], [environment], [environment_id], [external_url], [kind], [ref], [latest_commit_short_id], [current_commit_short_id], [available], [username] | `project_defaults.pull.environments.enabled` |
| `gitlab_ci_pipeline_coverage` | Coverage of the most recent pipeline | [project], [topics], [ref], [kind], [source], [variables] | *available by default* |
| `gitlab_ci_pipeline_duration_seconds` | Duration in seconds of the most recent pipeline | [project], [topics], [ref], [kind], [source], [variables] | *available by default* |
| `gitlab_ci_pipeline_id` | ID of the most recent pipeline | [project], [topics], [ref], [kind], [source], [variables] | *available by default* |
| `gitlab_ci_pipeline_job_artifact_size_bytes` | Artifact size in bytes (sum of all of them) of the most recent job | [project], [topics], [ref], [runner_description], [kind], [source], [variables], [stage], [job_name], [tag_list], [failure_reason] | `project_defaults.pull.pipeline.jobs.enabled` |
| `gitlab_ci_pipeline_job_duration_seconds` | Duration in seconds of the most recent job | [project], [topics], [ref], [runner_description], [kind], [source], [variables], [stage], [job_name], [tag_list], [failure_reason] | `project_defaults.pull.pipeline.jobs.enabled` |
| `gitlab_ci_pipeline_job_id` | ID of the most recent job | [project], [topics], [ref], [runner_description], [kind], [source], [variables], [stage], [job_name], [tag_list], [failure_reason] | `project_defaults.pull.pipeline.jobs.enabled` |
| `gitlab_ci_pipeline_job_queued_duration_seconds` | Duration in seconds the most recent job has been queued before starting | [project], [topics], [ref], [runner_description], [kind], [source], [variables], [stage], [job_name], [tag_list], [failure_reason] | `project_defaults.pull.pipeline.jobs.enabled` |
| `gitlab_ci_pipeline_job_run_count` | Number of executions of a job | [project], [topics], [ref], [runner_description], [kind], [source], [variables], [stage], [job_name], [tag_list], [failure_reason] | `project_defaults.pull.pipeline.jobs.enabled` |
| `gitlab_ci_pipeline_job_status` | Status of the most recent job | [project], [topics], [ref], [runner_description], [kind], [source], [variables], [stage], [job_name], [tag_list], [status], [failure_reason] | `project_defaults.pull.pipeline.jobs.enabled` |
| `gitlab_ci_pipeline_job_timestamp` | Creation date timestamp of the the most recent job | [project], [topics], [ref], [runner_description], [kind], [source], [variables], [stage], [job_name], [tag_list], [failure_reason] | `project_defaults.pull.pipeline.jobs.enabled` |
| `gitlab_ci_pipeline_queued_duration_seconds` | Duration in seconds the most recent pipeline has been queued before starting | [project], [topics], [ref], [kind], [source], [variables] | *available by default* |
| `gitlab_ci_pipeline_run_count` | Number of executions of a pipeline | [project], [topics], [ref], [kind], [source], [variables] | *available by default* |
| `gitlab_ci_pipeline_status` | Status of the most recent pipeline | [project], [topics], [ref], [kind], [source], [variables], [status] | *available by default* |
| `gitlab_ci_pipeline_timestamp` | Timestamp of the last update of the most recent pipeline | [project], [topics], [ref], [kind], [source], [variables] | *available by default* |
| `gitlab_ci_pipeline_test_report_total_time` | Duration in seconds of all the tests in the most recently finished pipeline | [project], [topics], [ref], [kind], [source], [variables] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_report_total_count` | Number of total tests in the most recently finished pipeline | [project], [topics], [ref], [kind], [source], [variables] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_report_success_count` | Number of successful tests in the most recently finished pipeline | [project], [topics], [ref], [kind], [source], [variables] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_report_failed_count` | Number of failed tests in the most recently finished pipeline | [project], [topics], [ref], [kind], [source], [variables] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_report_skipped_count` | Number of skipped tests in the most recently finished pipeline | [project], [topics], [ref], [kind], [source], [variables] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_report_error_count` | Number of errored tests in the most recently finished pipeline | [project], [topics], [ref], [kind], [source], [variables] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_suite_total_time` | Duration in seconds for the test suite | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_suite_total_count` | Number of total tests for the test suite | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_suite_success_count` | Number of successful tests for the test suite | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_suite_failed_count` | Number of failed tests for the test suite | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_suite_skipped_count` | Number of skipped tests for the test suite | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_suite_error_count` | Duration in errored tests for the test suite | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name] | `project_defaults.pull.pipeline.test_reports.enabled` |
| `gitlab_ci_pipeline_test_case_execution_time` | Duration in seconds for the test case | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name], [test_case_name], [test_case_classname] | `project_defaults.pull.pipeline.test_reports.test_cases.enabled` |
| `gitlab_ci_pipeline_test_case_status` | Status of the most recent test case | [project], [topics], [ref], [kind], [source], [variables], [test_suite_name], [test_case_name], [test_case_classname], [status] | `project_defaults.pull.pipeline.test_reports.test_cases.enabled` |

## Labels

### Project

Path with namespace of the project

### Topics

Topics configured on the project

### Ref Name

Name of the ref (branch, tag or merge request) used by the pipeline

### Runner Description

Description of the runner on which the most recent job ran

### Ref Kind

Type of the ref used by the pipeline. Can be either **branch**, **tag** or **merge_request**

### Source

The reason the pipeline exists.

### Variables

User defined variables for the pipelines.
Those are not fetched by default, you need to set `project_defaults.pull.pipeline.variables.enabled` to **true**

### Test Suite Name

Name of the test suite.
This is not fetched by default, you need to set `project_default.pull.pipeline.test_reports.enabled` to **true**

### Test Case Name

Name of the test case.
This is not fetched by default, you need to set `project_default.pull.pipeline.test_reports.test_cases.enabled` to **true**

### Test Case ClassName

Name of the test case classname.
This is not fetched by default, you need to set `project_default.pull.pipeline.test_reports.test_cases.enabled` to **true**

### Environment

Name of the environment

### Available

Whether the environment is available or not

### External URL

External URL of the environment

### Latest commit short ID

Most recent commit short ID on the ref which was last used to deploy to the environment

### Current commit short ID

Currently deployed commit short ID on the environment

### Username

GitLab username of the person which triggered the most recent deployment of the environment

### Status

Status of the pipeline, deployment or test case

### Stage

Stage of the job

### Job name

Name of the job

### Tag list

Tag list of the job

### Environment ID

ID of the environment

### Sparse status metrics

If the amount of status metrics generated by fetching jobs becomes a problem, you can enable `output_sparse_status_metrics` on a global, per-project or per-wildcard basis. When enabled, only labels matching the previous pipeline or job status will be submitted (with value `1`) rather than all label combinations submitted but with `0` value where the status does not match the previous run, for example:

```bash
# output_sparse_status_metrics: false
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="canceled",topics=""} 0
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="failed",topics=""} 0
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="manual",topics=""} 0
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="pending",topics=""} 0
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="running",topics=""} 0
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="skipped",topics=""} 0
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="success",topics=""} 1

# output_sparse_status_metrics: true
gitlab_ci_pipeline_last_job_run_status{job="test",project="bar/project",ref="main",stage="test",status="success",topics=""} 1
```

This flag affect every `_status$` metrics:

- `gitlab_ci_pipeline_environment_deployment_status`
- `gitlab_ci_pipeline_job_status`
- `gitlab_ci_pipeline_status`
- `gitlab_ci_pipeline_test_case_status`

[available]: #available
[current_commit_short_id]: #current-commit-short-id
[environment]: #environment
[environment_id]: #environment-id
[external_url]: #external-url
[job_name]: #job-name
[tag_list]: #tag-list
[kind]: #ref-kind
[latest_commit_short_id]: #latest-commit-short-id
[project]: #project
[ref]: #ref-name
[runner_description]: #runner-description
[stage]: #stage
[status]: #status
[topics]: #topics
[username]: #username
[source]: #source
[variables]: #variables
[test_suite_name]: #test-suite-name
[test_case_name]: #test-case-name
[test_case_classname]: #test-case-classname


## Configuration Syntax

```yaml 
# Log configuration
log:
  # Set the logging level
  # allowed values: trace, debug, info, warning, error, fatal or panic
  # (optional, default: info)
  level: info

  # Set the logging format
  # allowed values: text or json
  # (optional, default: text)
  format: text

# OpenTelemetry configuration (currently supports tracing only)
opentelemetry:
  # Configure the OpenTelemetry collector gRPC endpoint in order to enable tracing
  # (optional, default: "")
  grpc_endpoint:

# Exporter HTTP servers configuration
server:
  # [address:port] to make the process listen
  # upon (optional, default: :8080)
  listen_address: :8080
  
  # Enable profiling pages
  # at /debug/pprof (optional, default: false)
  enable_pprof: false
  
  metrics:
    # Enable /metrics endpoint (optional, default: true)
    enabled: true
  
    # Enable OpenMetrics content encoding in
    # prometheus HTTP handler (optional, default: false)
    # see: https://godoc.org/github.com/prometheus/client_golang/prometheus/promhttp#HandlerOpts
    enable_openmetrics_encoding: true

  webhook:
    # Enable /webhook endpoint to
    # support GitLab requests (optional, default: false)
    enabled: false

    # Secret token to authenticate legitimate webhook
    # requests coming from the GitLab server
    # (required if enabled but can also be configured using
    # the --webhook-secret-token flag or $GCPE_WEBHOOK_SECRET_TOKEN
    # environment variable)
    secret_token: 063f51ec-09a4-11eb-adc1-0242ac120002

# Redis configuration, optional and solely useful for an HA setup.
# By default the data is held in memory of the exporter
redis:
  # URL used to connect onto the redis endpoint
  # format: redis[s]://[:password@]host[:port][/db-number][?option=value])
  # (required to use the feature but can also be configured using
  # the --redis-url flag or $GCPE_REDIS_URL
  # environment variable)
  url: redis://foo:bar@redis.example.net:6379

# URL and Token with sufficient permissions to access
# your GitLab's projects pipelines informations (optional)
gitlab:
  # URL of your GitLab instance (optional, default: https://gitlab.com)
  url: https://gitlab.com

  # Token to use to authenticate against the GitLab API
  # it requires api and read_repository permissions
  # (required but can also be configured using the --gitlab-token
  # flag or the $GCPE_GITLAB_TOKEN environment variable)
  token: xrN14n9-ywvAFxxxxxx

  # Alternative URL for determining health of
  # GitLab API for the readiness probe (optional, default: https://gitlab.com)
  # it can also be defined using the --gitlab-health-url flag or $GCPE_GITLAB_HEALTH_URL
  # environment variable
  health_url: https://gitlab.example.com/-/health
  
  # Enable verification of readiness for target
  # GitLab instance calling `health_url` (optional, default: true)
  enable_health_check: true

  # Enable TLS validation for target
  # GitLab instance (handy when self-hosting) (optional, default: true)
  enable_tls_verify: true

  # Maximum limit for the GitLab API requests/sec
  # (optional, default: 1)
  maximum_requests_per_second: 1

  # Rate limit for the GitLab API requests/sec
  # (optional, default: 5)
  burstable_requests_per_second: 5

  # Maximum amount of jobs to keep queue, if this limit is reached
  # newly created ones will get dropped. As a best practice you should not change this value.
  # Workarounds to avoid hitting the limit are:
  # - increase polling intervals
  # - increase API rate limit
  # - reduce the amount of projects, refs, environments or metrics you are looking into
  # - leverage webhooks instead of polling schedules
  #
  # (optional, default: 1000)
  maximum_jobs_queue_size: 1000

pull:
  projects_from_wildcards:
    # Whether to trigger a discovery or not when the
    # exporter starts (optional, default: true)
    on_init: true

    # Whether to attempt retrieving new projects from wildcards
    # on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to discover projects
    # from wildcards (optional, default: 1800)
    interval_seconds: 1800

  environments_from_projects:
    # Whether to trigger a discovery of project environments when
    # exporter starts (optional, default: true)
    on_init: true

    # Whether to attempt retrieving project environments
    # on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to discover project environments
    # (optional, default: 300)
    interval_seconds: 300

  refs_from_projects:
    # Whether to trigger a discovery of project refs from
    # branches, tags and merge requests when the
    # exporter starts (optional, default: true)
    # nb: merge requests refs discovery needs to be
    # additionally enabled on a per project basis
    on_init: true

    # Whether to attempt retrieving project refs from branches,
    # tags & merge requests on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to discover refs
    # from projects branches and tags (optional, default: 300)
    interval_seconds: 300

  metrics:
    # Whether or not to trigger a pull of the metrics when the
    # exporter starts (optional, default: true)
    on_init: true

    # Whether or not to attempt refreshing the metrics
    # on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to pull metrics from
    # discovered project refs (optional, default: 30)
    interval_seconds: 30

garbage_collect:
  projects:
    # Whether or not to trigger a garbage collection of the
    # projects when the exporter starts (optional, default: false)
    on_init: false

    # Whether or not to attempt garbage collecting the projects
    # on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to garbage collect projects
    # (optional, default: 14400)
    interval_seconds: 14400

  environments:
    # Whether or not to trigger a garbage collection of the
    # environments when the exporter starts (optional, default: false)
    on_init: false

    # Whether or not to attempt garbage collecting the environments
    # on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to garbage collect environments
    # (optional, default: 14400)
    interval_seconds: 14400

  refs:
    # Whether or not to trigger a garbage collection of the
    # projects refs when the exporter starts (optional, default: false)
    on_init: false

    # Whether or not to attempt garbage collecting the projects refs
    # on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to garbage collect projects refs
    # from projects branches and tags (optional, default: 1800)
    interval_seconds: 1800

  metrics:
    # Whether or not to trigger a garbage collection of the
    # metrics when the exporter starts (optional, default: false)
    on_init: false

    # Whether or not to attempt garbage collecting the metrics
    # on a regular basis (optional, default: true)
    scheduled: true

    # Interval in seconds to garbage collect metrics
    # (optional, default: 600)
    interval_seconds: 600

# Default settings which can be overridden at the project
# or wildcard level (optional)
project_defaults:
  # Whether to output sparse job and pipeline status metrics.
  # When enabled, only the status label matching the last run
  # of a pipeline or job will be submitted (optional, default: true)
  output_sparse_status_metrics: true

  pull:
    environments:
      # Whether or not to pull project environments & their deployments
      # (optional, default: false)
      enabled: false

      # Filter out by name environments to include
      # (optional, default: ".*")
      regexp: ".*"

      # Do not export metrics for stopped environments
      # (optional, default: true)
      exclude_stopped: true

    refs:
      branches:
        # Monitor pipelines related to project branches 
        # (optional, default: true)
        enabled: true

        # Filter for branches to include
        # (optional, default: "^(?:main|master)$" -- main/master branches)
        regexp: "^(?:main|master)$"
        
        # Only keep most 'n' recently updated branches
        # (optional, default: 0 -- disabled/keep every branch matching the regexp)"
        most_recent: 0

        # If the age of the most recently updated pipeline for the branch is greater than
        # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
        max_age_seconds: 0

        # If set to false, it will continue to export metrics for the branch even
        # if it has been deleted (optional, default: true)
        exclude_deleted: true

      tags:
        # Monitor pipelines related to project tags
        # (optional, default: true)
        enabled: true

        # Filter for tags to include
        # (optional, default: ".*" -- all tags)
        regexp: ".*"

        # Only keep most 'n' recently updated tags
        # (optional, default: 0 -- disabled/keep every tag matching the regexp)"
        most_recent: 0

        # If the age of the most recently updated pipeline for the tag is greater than
        # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
        max_age_seconds: 0

        # If set to false, it will continue to export metrics for the tag even
        # if it has been deleted (optional, default: true)
        exclude_deleted: true

      merge_requests:
        # Monitor pipelines related to project merge requests
        # (optional, default: false)
        enabled: false

        # Only keep most 'n' recently updated merge requests
        # (optional, default: 0 -- disabled/keep every merge request)
        most_recent: 0

        # If the age of the most recently updated pipeline for the merge request is greater than
        # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
        max_age_seconds: 0

    pipeline:
      jobs:
        # Whether to attempt retrieving job level metrics from pipelines.
        # Increases the number of outputed metrics significantly!
        # (optional, default: false)
        enabled: false

        from_child_pipelines:
          # Collect jobs from subsequent child/downstream pipelines
          # (optional, default: true)
          enabled: true
        
        runner_description:
          # Export the description of the runner which ran the job
          # (optional, default: true)
          enabled: true

          # Whenever the description of a runner will match this regexp
          # The regexp will be used as the value of the runner description instead
          # (optional, default: "shared-runners-manager-(\d*)\.gitlab\.com")
          aggregation_regexp: shared-runners-manager-(\d*)\.gitlab\.com

      variables:
        # Fetch pipeline variables in a separate metric (optional, default: false)
        enabled: false

        # Filter pipelines variables to include
        # (optional, default: ".*", all variables)
        regexp: ".*"
      
      test_reports:
        # Fetch test reports in a separate metric (optiona, default: false)
        enabled: false

        test_cases:
        # Fetch test cases reports in a separate metric (optional, default: false)
          enabled: false

# The list of the projects you want to monitor (optional)
projects:
  - # Name of the project (actually path with namespace) to fetch
    # (required)
    name: foo/bar

    # Here are all the project parameters which can be overriden (optional)
    pull:
      environments:
        # Whether or not to pull project environments & their deployments
        # (optional, default: false)
        enabled: false

        # Filter out by name environments to include
        # (optional, default: ".*")
        regexp: ".*"

        # Do not export metrics for stopped environments
        # (optional, default: true)
        exclude_stopped: true

      refs:
        branches:
          # Monitor pipelines related to project branches 
          # (optional, default: true)
          enabled: true

          # Filter for branches to include
          # (optional, default: "^(?:main|master)$" -- main/master branches)
          regexp: "^(?:main|master)$"
          
          # Only keep most 'n' recently updated branches
          # (optional, default: 0 -- disabled/keep every branch matching the regexp)"
          most_recent: 0

          # If the age of the most recently updated pipeline for the branch is greater than
          # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
          max_age_seconds: 0

          # If set to false, it will continue to export metrics for the branch even
          # if it has been deleted (optional, default: true)
          exclude_deleted: true

        tags:
          # Monitor pipelines related to project tags
          # (optional, default: true)
          enabled: true

          # Filter for tags to include
          # (optional, default: ".*" -- all tags)
          regexp: ".*"

          # Only keep most 'n' recently updated tags
          # (optional, default: 0 -- disabled/keep every tag matching the regexp)"
          most_recent: 0

          # If the age of the most recently updated pipeline for the tag is greater than
          # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
          max_age_seconds: 0

          # If set to false, it will continue to export metrics for the tag even
          # if it has been deleted (optional, default: true)
          exclude_deleted: true

        merge_requests:
          # Monitor pipelines related to project merge requests
          # (optional, default: false)
          enabled: false

          # Only keep most 'n' recently updated merge requests
          # (optional, default: 0 -- disabled/keep every merge request)
          most_recent: 0

          # If the age of the most recently updated pipeline for the merge request is greater than
          # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
          max_age_seconds: 0

      pipeline:
        jobs:
          # Whether to attempt retrieving job level metrics from pipelines.
          # Increases the number of outputed metrics significantly!
          # (optional, default: false)
          enabled: false

          from_child_pipelines:
            # Collect jobs from subsequent child/downstream pipelines
            # (optional, default: true)
            enabled: true

          runner_description:
            # Export the description of the runner which ran the job
            # (optional, default: true)
            enabled: true

            # Whenever the description of a runner will match this regexp
            # The regexp will be used as the value of the runner description instead
            # (optional, default: "shared-runners-manager-(\d*)\.gitlab\.com")
            aggregation_regexp: shared-runners-manager-(\d*)\.gitlab\.com

        variables:
          # Fetch pipeline variables in a separate metric (optional, default: false)
          enabled: false

          # Filter pipelines variables to include
          # (optional, default: ".*", all variables)
          regexp: ".*"
          
        test_reports:
          # Fetch test reports in a separate metric (optiona, default: false)
          enabled: false

          test_cases:
          # Fetch test cases reports in a separate metric (optional, default: false)
            enabled: false
        
        # How many per ref pipelines values (default 1)
        PerRef: 1

# Dynamically fetch projects to monitor using a wildcard (optional)
wildcards:
  - # Define the owner of the projects we want to look for (optional)
    owner:
      # Name of the owner (required)
      name: foo

      # Owner kind: can be either 'group' or 'user' (required)
      kind: group

      # if owner kind is 'group', whether to include subgroups
      # or not (optional, default: false)
      include_subgroups: false

    # Search expression to filter out projects
    # (optional, default: '' -- no filter/all projects)
    search: ''

    # Including archived projects or not
    # (optional, default: false)
    archived: false

    # Here are all the project parameters which can be overriden (optional)
    pull:
      environments:
        # Whether or not to pull project environments & their deployments
        # (optional, default: false)
        enabled: false

        # Filter out by name environments to include
        # (optional, default: ".*")
        regexp: ".*"

        # Do not export metrics for stopped environments
        # (optional, default: true)
        exclude_stopped: true

      refs:
        branches:
          # Monitor pipelines related to project branches 
          # (optional, default: true)
          enabled: true

          # Filter for branches to include
          # (optional, default: "^(?:main|master)$" -- main/master branches)
          regexp: "^(?:main|master)$"
          
          # Only keep most 'n' recently updated branches
          # (optional, default: 0 -- disabled/keep every branch matching the regexp)"
          most_recent: 0

          # If the age of the most recently updated pipeline for the branch is greater than
          # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
          max_age_seconds: 0

          # If set to false, it will continue to export metrics for the branch even
          # if it has been deleted (optional, default: true)
          exclude_deleted: true

        tags:
          # Monitor pipelines related to project tags
          # (optional, default: true)
          enabled: true

          # Filter for tags to include
          # (optional, default: ".*" -- all tags)
          regexp: ".*"

          # Only keep most 'n' recently updated tags
          # (optional, default: 0 -- disabled/keep every tag matching the regexp)"
          most_recent: 0

          # If the age of the most recently updated pipeline for the tag is greater than
          # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
          max_age_seconds: 0

          # If set to false, it will continue to export metrics for the tag even
          # if it has been deleted (optional, default: true)
          exclude_deleted: true

        merge_requests:
          # Monitor pipelines related to project merge requests
          # (optional, default: false)
          enabled: false

          # Only keep most 'n' recently updated merge requests
          # (optional, default: 0 -- disabled/keep every merge request)
          most_recent: 0

          # If the age of the most recently updated pipeline for the merge request is greater than
          # this value, the pipeline metrics won't get exported (optional, default: 0 (disabled))
          max_age_seconds: 0

      pipeline:
        jobs:
          # Whether to attempt retrieving job level metrics from pipelines.
          # Increases the number of outputed metrics significantly!
          # (optional, default: false)
          enabled: false

          from_child_pipelines:
            # Collect jobs from subsequent child/downstream pipelines
            # (optional, default: true)
            enabled: true

          runner_description:
            # Export the description of the runner which ran the job
            # (optional, default: true)
            enabled: true

            # Whenever the description of a runner will match this regexp
            # The regexp will be used as the value of the runner description instead
            # (optional, default: "shared-runners-manager-(\d*)\.gitlab\.com")
            aggregation_regexp: shared-runners-manager-(\d*)\.gitlab\.com

        variables:
          # Fetch pipeline variables in a separate metric (optional, default: false)
          enabled: false

          # Filter pipelines variables to include
          # (optional, default: ".*", all variables)
          regexp: ".*"
          
        test_reports:
          # Fetch test reports in a separate metric (optiona, default: false)
          enabled: false
          
          from_child_pipelines:
            # Combines test reports from subsequent child/downstream pipelines
            # (optional, default: false)
            enabled: false

          test_cases:
          # Fetch test cases reports in a separate metric (optional, default: false)
            enabled: false
```