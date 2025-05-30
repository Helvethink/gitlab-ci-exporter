package schemas

import (
	"context"
	"strconv" // For string conversion operations

	log "github.com/sirupsen/logrus"               // Logging library
	goGitlab "gitlab.com/gitlab-org/api/client-go" // GitLab API client
)

// Pipeline represents a pipeline structure with detailed information about a GitLab CI/CD pipeline.
type Pipeline struct {
	ID                    int        // Unique identifier for the pipeline
	Coverage              float64    // Coverage percentage of the pipeline
	Timestamp             float64    // Unix timestamp of when the pipeline was updated
	DurationSeconds       float64    // Duration of the pipeline execution in seconds
	QueuedDurationSeconds float64    // Duration the pipeline was queued in seconds
	Source                string     // Source of the pipeline trigger
	Status                string     // Status of the pipeline
	Variables             string     // Variables associated with the pipeline
	TestReport            TestReport // Test report associated with the pipeline
}

// TestReport represents a test report structure with detailed information about test results.
type TestReport struct {
	TotalTime    float64     // Total time taken by all tests in seconds
	TotalCount   int         // Total number of tests
	SuccessCount int         // Number of successful tests
	FailedCount  int         // Number of failed tests
	SkippedCount int         // Number of skipped tests
	ErrorCount   int         // Number of tests with errors
	TestSuites   []TestSuite // List of test suites in the report
}

// TestSuite represents a test suite structure with detailed information about a suite of tests.
type TestSuite struct {
	Name         string     // Name of the test suite
	TotalTime    float64    // Total time taken by the test suite in seconds
	TotalCount   int        // Total number of tests in the suite
	SuccessCount int        // Number of successful tests in the suite
	FailedCount  int        // Number of failed tests in the suite
	SkippedCount int        // Number of skipped tests in the suite
	ErrorCount   int        // Number of tests with errors in the suite
	TestCases    []TestCase // List of test cases in the suite
}

// TestCase represents a test case structure with detailed information about an individual test.
type TestCase struct {
	Name          string  // Name of the test case
	Classname     string  // Class name of the test case
	ExecutionTime float64 // Execution time of the test case in seconds
	Status        string  // Status of the test case
}

// NewPipeline creates a new Pipeline instance from a GitLab pipeline object.
func NewPipeline(ctx context.Context, gp goGitlab.Pipeline) Pipeline {
	var (
		coverage  float64 // Variable to store the coverage value
		err       error   // Variable to store any error that occurs
		timestamp float64 // Variable to store the timestamp
	)

	// Parse the coverage string into a float64 if it is not empty
	if gp.Coverage != "" {
		coverage, err = strconv.ParseFloat(gp.Coverage, 64)
		if err != nil {
			log.WithContext(ctx).
				WithField("error", err.Error()).
				Warnf("could not parse coverage string returned from GitLab API '%s' into Float64", gp.Coverage)
		}
	}

	// Convert the UpdatedAt timestamp to a float64 Unix timestamp if it is not nil
	if gp.UpdatedAt != nil {
		timestamp = float64(gp.UpdatedAt.Unix())
	}

	// Create a new Pipeline instance with the parsed data
	pipeline := Pipeline{
		ID:                    gp.ID,
		Coverage:              coverage,
		Timestamp:             timestamp,
		DurationSeconds:       float64(gp.Duration),
		QueuedDurationSeconds: float64(gp.QueuedDuration),
		Source:                string(gp.Source),
	}

	// Set the pipeline status based on detailed status or default status
	if gp.DetailedStatus != nil {
		pipeline.Status = gp.DetailedStatus.Group
	} else {
		pipeline.Status = gp.Status
	}

	return pipeline
}

// NewTestReport creates a new TestReport instance from a GitLab test report object.
func NewTestReport(gtr goGitlab.PipelineTestReport) TestReport {
	testSuites := []TestSuite{} // Slice to store test suites

	// Convert each GitLab test suite to a TestSuite and append to the slice
	for _, x := range gtr.TestSuites {
		testSuites = append(testSuites, NewTestSuite(x))
	}

	// Create a new TestReport instance with the converted data
	return TestReport{
		TotalTime:    gtr.TotalTime,
		TotalCount:   gtr.TotalCount,
		SuccessCount: gtr.SuccessCount,
		FailedCount:  gtr.FailedCount,
		SkippedCount: gtr.SkippedCount,
		ErrorCount:   gtr.ErrorCount,
		TestSuites:   testSuites,
	}
}

// NewTestSuite creates a new TestSuite instance from a GitLab test suite object.
func NewTestSuite(gts *goGitlab.PipelineTestSuites) TestSuite {
	testCases := []TestCase{} // Slice to store test cases

	// Convert each GitLab test case to a TestCase and append to the slice
	for _, x := range gts.TestCases {
		testCases = append(testCases, NewTestCase(x))
	}

	// Create a new TestSuite instance with the converted data
	return TestSuite{
		Name:         gts.Name,
		TotalTime:    gts.TotalTime,
		TotalCount:   gts.TotalCount,
		SuccessCount: gts.SuccessCount,
		FailedCount:  gts.FailedCount,
		SkippedCount: gts.SkippedCount,
		ErrorCount:   gts.ErrorCount,
		TestCases:    testCases,
	}
}

// NewTestCase creates a new TestCase instance from a GitLab test case object.
func NewTestCase(gtc *goGitlab.PipelineTestCases) TestCase {
	// Create a new TestCase instance with the provided data
	return TestCase{
		Name:          gtc.Name,
		Classname:     gtc.Classname,
		ExecutionTime: gtc.ExecutionTime,
		Status:        gtc.Status,
	}
}
