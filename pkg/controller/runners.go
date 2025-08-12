package controller

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	log "github.com/sirupsen/logrus"
)

// PullRunnersFromProject fetches the list of runners for a given project from the GitLab API,
// then checks if each runner already exists in the local store.
// For any runner not found in the store, it updates the store with the new runner,
// logs the discovery, and schedules a task to pull metrics for that runner.
func (c *Controller) PullRunnersFromProject(ctx context.Context, p schemas.Project) (err error) {
	var runners schemas.Runners

	// Retrieve runners from GitLab API for the project
	runners, err = c.Gitlab.GetProjectRunners(ctx, p)
	if err != nil {
		return
	}

	// Iterate through each runner returned from GitLab
	for k := range runners {
		var exists bool

		// Check if the runner already exists in the local store
		exists, err = c.Store.RunnerExists(ctx, k)

		if err != nil {
			return
		}

		// If the runner does not exist, add/update it in the store and schedule metric pulling
		if !exists {
			runner := runners[k]
			if err = c.UpdateRunner(ctx, &runner); err != nil {
				return
			}

			log.WithFields(log.Fields{
				"project-name":       runner.ProjectName,
				"runner-id":          runner.ID,
				"runner-name":        runner.Name,
				"runner-description": runner.Description,
				"runner-key":         k,
			}).Info("discovered new runner")

			// Schedule a task to pull runner metrics asynchronously
			c.ScheduleTask(ctx, schemas.TaskTypePullRunnersMetrics, string(runner.Key()), runner)
		}
	}

	return
}

// UpdateRunner fetches the latest state of a given runner from the GitLab API,
// updates the local runner object with the latest details (Paused, Contacted At, and maintenance notes),
// and then saves the updated runner back to the local store.
func (c *Controller) UpdateRunner(ctx context.Context, runner *schemas.Runner) error {
	// Prepare logging fields with project, job name, and job ID for contextual logging
	projectRefLogFields := log.Fields{
		"runner-id": runner.ID,
	}

	// Retrieve the latest runner data from GitLab
	pulledRunner, err := c.Gitlab.GetRunner(ctx, runner.ProjectName, runner.ID)
	if err != nil {
		return err
	}

	// Update the local runner fields with the latest data
	runner.Paused = pulledRunner.Paused
	runner.ContactedAt = pulledRunner.ContactedAt
	runner.MaintenanceNote = pulledRunner.MaintenanceNote

	log.WithFields(projectRefLogFields).Info("update runner metrics")
	// Save the updated runner back to the store
	return c.Store.SetRunner(ctx, *runner)
}

// ProcessRunnerMetrics processes metrics for a given runner and updates the store accordingly.
func (c *Controller) ProcessRunnerMetrics(ctx context.Context, runner schemas.Runner) (err error) {
	// Prepare logging fields with project, job name, and job ID for contextual logging
	projectRefLogFields := log.Fields{
		"project-name-or-id": runner.ProjectName,
		"runner-desc":        runner.Description,
	}
	if err = c.UpdateRunner(ctx, &runner); err != nil {
		return
	}

	// Initialize labels from the reference default labels and add job-specific labels
	groups := runner.Groups
	GroupsOut, err := json.Marshal(groups)
	if err != nil {
		return nil
	}
	projects := runner.Projects
	projectsOut, err := json.Marshal(projects)
	if err != nil {
		return nil
	}
	tags := strings.Join(runner.TagList, ",")

	labels := runner.DefaultLabelsValues()
	labels["runner_name"] = runner.Name
	labels["runner_id"] = strconv.Itoa(runner.ID)                                       // The unique identifier for the environment
	labels["is_shared"] = strconv.FormatBool(runner.IsShared)                           // The kind of the latest deployment's reference
	labels["runner_type"] = runner.RunnerType                                           // The name of the latest deployment's reference
	labels["online"] = strconv.FormatBool(runner.Online)                                // The short ID of the current commit
	labels["tag_list"] = tags                                                           // Placeholder for the latest commit short ID (empty in this context)
	labels["active"] = strconv.FormatBool(runner.Paused)                                // The availability status of the environment
	labels["runner_maintenance_note"] = runner.MaintenanceNote                          // Maintenance note label
	labels["contacted_at"] = strconv.FormatInt(runner.ContactedAt.UTC().UnixNano(), 10) // Last contact with gitlab server
	labels["status"] = runner.Status                                                    // The status of the runner
	labels["paused"] = strconv.FormatBool(runner.Paused)                                // Define if the runner is paused
	labels["runner_groups"] = string(GroupsOut)                                         // The groups assigned to this runner
	labels["runner_projects"] = string(projectsOut)                                     // The projects assigned to this runner

	// Log trace info indicating that job metrics are being processed
	log.WithFields(projectRefLogFields).Info("processing runner metrics")

	// Store the size of job artifacts in bytes
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindRunner,
		Labels: labels,
		Value:  1,
	})

	return nil
}
