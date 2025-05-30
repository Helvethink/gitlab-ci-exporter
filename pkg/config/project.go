package config

import (
	"github.com/creasty/defaults"
)

// ProjectParameters holds configuration for fetching projects and wildcards.
// It includes settings for how project pipelines, refs, and environments are pulled and processed,
// as well as output options for status metrics.
type ProjectParameters struct {
	// Pull contains detailed settings for what to pull related to projects (pipelines, refs, environments).
	Pull ProjectPull `yaml:"pull"`

	// OutputSparseStatusMetrics controls whether to export all pipeline/job statuses (false)
	// or only the last job's status (true).
	// Defaults to true for less verbose metrics output.
	OutputSparseStatusMetrics bool `default:"true" yaml:"output_sparse_status_metrics"`
}

// ProjectPull contains specific configuration for pulling different aspects of projects,
// such as environments, refs (branches, tags, MRs), and pipelines.
type ProjectPull struct {
	Environments ProjectPullEnvironments `yaml:"environments"`
	Refs         ProjectPullRefs         `yaml:"refs"`
	Pipeline     ProjectPullPipeline     `yaml:"pipeline"`
}

// ProjectPullEnvironments configures if and how environments/deployments are pulled for a project.
type ProjectPullEnvironments struct {
	// Enabled controls whether environment and deployment metrics are pulled.
	// Defaults to false.
	Enabled bool `default:"false" yaml:"enabled"`

	// Regexp is a regex filter applied to environment names to select which environments to fetch.
	// Defaults to match all (".*").
	Regexp string `default:".*" yaml:"regexp"`

	// ExcludeStopped indicates if environments that are stopped should be excluded from metrics export.
	// Defaults to true.
	ExcludeStopped bool `default:"true" yaml:"exclude_stopped"`
}

// ProjectPullRefs contains configuration for pulling refs: branches, tags, and merge requests.
type ProjectPullRefs struct {
	Branches      ProjectPullRefsBranches      `yaml:"branches"`
	Tags          ProjectPullRefsTags          `yaml:"tags"`
	MergeRequests ProjectPullRefsMergeRequests `yaml:"merge_requests"`
}

// ProjectPullRefsBranches configures which branches to monitor pipelines for and how.
type ProjectPullRefsBranches struct {
	// Enabled toggles whether branch pipelines are monitored.
	// Defaults to true.
	Enabled bool `default:"true" yaml:"enabled"`

	// Regexp filters branches to include by name using regex.
	// Default includes only "main" or "master" branches.
	Regexp string `default:"^(?:main|master)$" yaml:"regexp"`

	// MostRecent limits exported branches to the N most recently updated.
	// If zero, no limit is applied.
	MostRecent uint `default:"0" yaml:"most_recent"`

	// MaxAgeSeconds prevents exporting metrics for branches whose latest pipeline
	// update is older than this age in seconds. Zero means no age limit.
	MaxAgeSeconds uint `default:"0" yaml:"max_age_seconds"`

	// ExcludeDeleted indicates whether to skip branches marked as deleted.
	// Defaults to true.
	ExcludeDeleted bool `default:"true" yaml:"exclude_deleted"`
}

// ProjectPullRefsTags configures pulling of pipelines related to project tags.
type ProjectPullRefsTags struct {
	// Enabled toggles whether pipeline metrics for tags are monitored.
	// Defaults to true.
	Enabled bool `default:"true" yaml:"enabled"`

	// Regexp filters tags by name using a regex pattern.
	// Defaults to match all tags (".*").
	Regexp string `default:".*" yaml:"regexp"`

	// MostRecent limits export to only the N most recently updated tags.
	// If zero, no limit is applied.
	MostRecent uint `default:"0" yaml:"most_recent"`

	// MaxAgeSeconds prevents exporting metrics for tags whose most recent pipeline
	// update is older than this number of seconds. Zero means no age limit.
	MaxAgeSeconds uint `default:"0" yaml:"max_age_seconds"`

	// ExcludeDeleted controls whether to exclude tags marked as deleted.
	// Defaults to true.
	ExcludeDeleted bool `default:"true" yaml:"exclude_deleted"`
}

// ProjectPullRefsMergeRequests configures pulling of pipelines related to merge requests.
type ProjectPullRefsMergeRequests struct {
	// Enabled toggles whether pipeline metrics for merge requests are monitored.
	// No default is specified, so it must be explicitly set.
	Enabled bool `yaml:"enabled"`

	// MostRecent limits export to only the N most recently updated merge requests.
	// If zero, no limit is applied.
	MostRecent uint `default:"0" yaml:"most_recent"`

	// MaxAgeSeconds prevents exporting metrics for merge requests whose most recent pipeline
	// update is older than this number of seconds. Zero means no age limit.
	MaxAgeSeconds uint `default:"0" yaml:"max_age_seconds"`
}

// ProjectPullPipeline holds configuration related to pipelines.
type ProjectPullPipeline struct {
	// Jobs contains configuration related to pipeline jobs.
	Jobs ProjectPullPipelineJobs `yaml:"jobs"`

	// Variables controls pipeline variable fetching settings.
	Variables ProjectPullPipelineVariables `yaml:"variables"`

	// TestReports configures the collection of pipeline test reports.
	TestReports ProjectPullPipelineTestReports `yaml:"test_reports"`
}

// ProjectPullPipelineJobs configures metrics related to pipeline jobs.
type ProjectPullPipelineJobs struct {
	// Enabled toggles pulling metrics related to pipeline jobs.
	// Defaults to false.
	Enabled bool `default:"false" yaml:"enabled"`

	// FromChildPipelines configures pulling jobs from child or downstream pipelines.
	FromChildPipelines ProjectPullPipelineJobsFromChildPipelines `yaml:"from_child_pipelines"`

	// RunnerDescription configures whether to export the description of the runner
	// that executed the job.
	RunnerDescription ProjectPullPipelineJobsRunnerDescription `yaml:"runner_description"`
}

// ProjectPullPipelineJobsFromChildPipelines configures pulling jobs from child or downstream pipelines.
type ProjectPullPipelineJobsFromChildPipelines struct {
	// Enabled toggles whether to pull pipeline jobs from child or downstream pipelines.
	// Defaults to true.
	Enabled bool `default:"true" yaml:"enabled"`
}

// ProjectPullPipelineJobsRunnerDescription configures exporting runner descriptions.
type ProjectPullPipelineJobsRunnerDescription struct {
	// Enabled toggles whether to export the description of the runner that executed the job.
	// Defaults to true.
	Enabled bool `default:"true" yaml:"enabled"`

	// AggregationRegexp is a regex to reduce cardinality by aggregating runner descriptions.
	// For example, it can extract a numeric identifier from a hostname pattern.
	// Defaults to "shared-runners-manager-(\d*)\.gitlab\.com"
	AggregationRegexp string `default:"shared-runners-manager-(\\d*)\\.gitlab\\.com" yaml:"aggregation_regexp"`
}

// ProjectPullPipelineVariables configures fetching pipeline variables.
type ProjectPullPipelineVariables struct {
	// Enabled toggles whether to fetch variables included in the pipeline.
	// Defaults to false.
	Enabled bool `default:"false" yaml:"enabled"`

	// Regexp filters pipeline variable values by regex to control which variables are fetched.
	// Defaults to match all (".*").
	Regexp string `default:".*" yaml:"regexp"`
}

// ProjectPullPipelineTestReports configures retrieval of test reports from pipelines.
type ProjectPullPipelineTestReports struct {
	// Enabled toggles whether to attempt retrieving test reports included in the pipeline.
	// Defaults to false.
	Enabled bool `default:"false" yaml:"enabled"`

	// FromChildPipelines configures whether to pull test reports from child/downstream pipelines.
	FromChildPipelines ProjectPullPipelineTestReportsFromChildPipelines `yaml:"from_child_pipelines"`

	// TestCases configures fetching details about individual test cases within the test reports.
	TestCases ProjectPullPipelineTestReportsTestCases `yaml:"test_cases"`
}

// ProjectPullPipelineTestReportsFromChildPipelines configures pulling test reports from child pipelines.
type ProjectPullPipelineTestReportsFromChildPipelines struct {
	// Enabled toggles whether to pull test reports from child/downstream pipelines.
	// Defaults to false.
	Enabled bool `default:"false" yaml:"enabled"`
}

// ProjectPullPipelineTestReportsTestCases configures retrieval of test case details
// within pipeline test reports.
type ProjectPullPipelineTestReportsTestCases struct {
	// Enabled toggles whether to attempt retrieving individual test case information
	// from the pipeline test report.
	// Defaults to false.
	Enabled bool `default:"false" yaml:"enabled"`
}

// Project holds information about a GitLab project, including its specific parameters.
type Project struct {
	// ProjectParameters embeds configuration parameters specific to this project.
	ProjectParameters `yaml:",inline"`

	// Name represents the project identifier, commonly known as path_with_namespace in GitLab.
	Name string `yaml:"name"`
}

// Projects is a slice of Project instances.
type Projects []Project

// NewProject creates a new Project instance with default parameters set,
// and assigns the given project name.
// The name usually corresponds to GitLab's path_with_namespace.
//
// Parameters:
//   - name: the GitLab project identifier (path_with_namespace).
//
// Returns:
//   - Project with default values initialized and name set.
func NewProject(name string) (p Project) {
	defaults.MustSet(&p)
	p.Name = name

	return
}
