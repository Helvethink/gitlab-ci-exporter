package controller

import (
	"context"
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
				"project-name": runner.ProjectName,
				"runner-id":    runner.ID,
				"runner-name":  runner.Name,
			}).Info("discovered new runner")

			// Schedule a task to pull runner metrics asynchronously
			c.ScheduleTask(ctx, schemas.TaskTypePullEnvironmentMetrics, string(runner.Key()), runner)
		}
	}

	return
}

// UpdateRunner fetches the latest state of a given runner from the GitLab API,
// updates the local runner object with the latest details (Paused, Contacted At, and maintenance notes),
// and then saves the updated runner back to the local store.
func (c *Controller) UpdateRunner(ctx context.Context, runner *schemas.Runner) error {
	// Retrieve the latest runner data from GitLab
	pulledRunner, err := c.Gitlab.GetRunner(ctx, runner.ProjectName, runner.ID)
	if err != nil {
		return err
	}

	// Update the local runner fields with the latest data
	runner.Paused = pulledRunner.Paused
	runner.ContactedAt = pulledRunner.ContactedAt
	runner.MaintenanceNote = pulledRunner.MaintenanceNote

	// Save the updated runner back to the store
	return c.Store.SetRunner(ctx, *runner)
}
