package controller

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// PullEnvironmentsFromProject fetches the list of environments for a given project from the GitLab API,
// then checks if each environment already exists in the local store.
// For any environment not found in the store, it updates the store with the new environment,
// logs the discovery, and schedules a task to pull metrics for that environment.
func (c *Controller) PullEnvironmentsFromProject(ctx context.Context, p schemas.Project) (err error) {
	var envs schemas.Environments

	// Retrieve environments from GitLab API for the project
	envs, err = c.Gitlab.GetProjectEnvironments(ctx, p)
	if err != nil {
		return
	}

	// Iterate through each environment returned from GitLab
	for k := range envs {
		var exists bool

		// Check if the environment already exists in the local store
		exists, err = c.Store.EnvironmentExists(ctx, k)
		if err != nil {
			return
		}

		// If the environment does not exist, add/update it in the store and schedule metric pulling
		if !exists {
			env := envs[k]
			if err = c.UpdateEnvironment(ctx, &env); err != nil {
				return
			}

			log.WithFields(log.Fields{
				"project-name":     env.ProjectName,
				"environment-id":   env.ID,
				"environment-name": env.Name,
			}).Info("discovered new environment")

			// Schedule a task to pull environment metrics asynchronously
			c.ScheduleTask(ctx, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()), env)
		}
	}

	return
}

// UpdateEnvironment fetches the latest state of a given environment from the GitLab API,
// updates the local environment object with the latest details (availability, external URL, and latest deployment),
// and then saves the updated environment back to the local store.
func (c *Controller) UpdateEnvironment(ctx context.Context, env *schemas.Environment) error {
	// Retrieve the latest environment data from GitLab
	pulledEnv, err := c.Gitlab.GetEnvironment(ctx, env.ProjectName, env.ID)
	if err != nil {
		return err
	}

	// Update the local environment fields with the latest data
	env.Available = pulledEnv.Available
	env.ExternalURL = pulledEnv.ExternalURL
	env.LatestDeployment = pulledEnv.LatestDeployment

	// Save the updated environment back to the store
	return c.Store.SetEnvironment(ctx, *env)
}

// PullEnvironmentMetrics retrieves and updates metrics related to a specific environment.
// It ensures the environment is up to date before collecting metrics, compares commits to
// calculate how far behind the environment is, updates deployment counts and durations,
// and sets various metrics in the store.
//
// The function workflow is as follows:
// 1. Refresh the environment from the store to avoid working on stale data.
// 2. Store the current deployment JobID for comparison later.
// 3. Update the environment from the GitLab API.
// 4. Depending on whether the latest deployment's ref is a branch or tag, fetch the latest commit ID and timestamp.
// 5. Calculate how many commits behind the environment is compared to the latest commit.
// 6. Update or reuse stored metrics for environment commit counts and behind durations.
// 7. Update deployment count if a new deployment job has started.
// 8. Store various deployment-related metrics including durations, job IDs, timestamps, and status.
// 9. Emit status metrics considering if sparse output is enabled.
//
// Returns an error if any step fails.
func (c *Controller) PullEnvironmentMetrics(ctx context.Context, env schemas.Environment) (err error) {
	// Refresh the environment from the store to ensure we have the latest local data
	if err := c.Store.GetEnvironment(ctx, &env); err != nil {
		return err
	}

	// Keep the existing deployment JobID before updating the environment
	deploymentJobID := env.LatestDeployment.JobID

	// Update environment details by pulling latest info from GitLab
	if err = c.UpdateEnvironment(ctx, &env); err != nil {
		return
	}

	var (
		infoLabels = env.InformationLabelsValues() // Labels to be attached to info metrics
		commitDate float64                         // Timestamp of the latest commit
	)

	// Depending on the kind of ref (branch or tag), fetch the latest commit short ID and date
	switch env.LatestDeployment.RefKind {
	case schemas.RefKindBranch:
		infoLabels["latest_commit_short_id"], commitDate, err = c.Gitlab.GetBranchLatestCommit(ctx, env.ProjectName, env.LatestDeployment.RefName)
	case schemas.RefKindTag:
		// For tags, fetch the most recent tag commit (implementation can be improved)
		infoLabels["latest_commit_short_id"], commitDate, err = c.Gitlab.GetProjectMostRecentTagCommit(ctx, env.ProjectName, ".*")
	default:
		// Fallback: use stored commit ID and timestamp
		infoLabels["latest_commit_short_id"] = env.LatestDeployment.CommitShortID
		commitDate = env.LatestDeployment.Timestamp
	}

	if err != nil {
		return err
	}

	var (
		envBehindDurationSeconds float64 // How many seconds environment is behind latest commit
		envBehindCommitCount     float64 // How many commits environment is behind
	)

	// Prepare metric to track number of commits behind
	behindCommitsCountMetric := schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentBehindCommitsCount,
		Labels: env.DefaultLabelsValues(),
	}

	// Only calculate commits behind if the latest commit ID has changed since last info metric
	if infoLabels["latest_commit_short_id"] != infoLabels["current_commit_short_id"] {
		infoMetric := schemas.Metric{
			Kind:   schemas.MetricKindEnvironmentInformation,
			Labels: env.DefaultLabelsValues(),
		}

		var commitCount int

		// Retrieve the last emitted info metric to compare commit IDs
		if err = c.Store.GetMetric(ctx, &infoMetric); err != nil {
			return err
		}

		// If commit IDs differ, fetch the number of commits between the two
		if infoMetric.Labels["latest_commit_short_id"] != infoLabels["latest_commit_short_id"] ||
			infoMetric.Labels["current_commit_short_id"] != infoLabels["current_commit_short_id"] {
			commitCount, err = c.Gitlab.GetCommitCountBetweenRefs(ctx, env.ProjectName, infoLabels["current_commit_short_id"], infoLabels["latest_commit_short_id"])
			if err != nil {
				return err
			}

			envBehindCommitCount = float64(commitCount)
		} else {
			// Otherwise, reuse the stored metric value (TODO: optimize this retrieval)
			if err = c.Store.GetMetric(ctx, &behindCommitsCountMetric); err != nil {
				return err
			}

			envBehindCommitCount = behindCommitsCountMetric.Value
		}
	}

	// Store the metric for commits behind
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentBehindCommitsCount,
		Labels: env.DefaultLabelsValues(),
		Value:  envBehindCommitCount,
	})

	// Calculate how long the environment is behind in seconds, if applicable
	if commitDate-env.LatestDeployment.Timestamp > 0 {
		envBehindDurationSeconds = commitDate - env.LatestDeployment.Timestamp
	}

	// Retrieve deployment count metric for the environment
	envDeploymentCount := schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentDeploymentCount,
		Labels: env.DefaultLabelsValues(),
	}

	storeGetMetric(ctx, c.Store, &envDeploymentCount)

	// If a new deployment job is detected (JobID increased), increment deployment count
	if env.LatestDeployment.JobID > deploymentJobID {
		envDeploymentCount.Value++
	}

	// Save updated deployment count metric
	storeSetMetric(ctx, c.Store, envDeploymentCount)

	// Store other deployment-related metrics: behind duration, deployment duration, job ID, deployment timestamp
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentBehindDurationSeconds,
		Labels: env.DefaultLabelsValues(),
		Value:  envBehindDurationSeconds,
	})

	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentDeploymentDurationSeconds,
		Labels: env.DefaultLabelsValues(),
		Value:  env.LatestDeployment.DurationSeconds,
	})

	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentDeploymentJobID,
		Labels: env.DefaultLabelsValues(),
		Value:  float64(env.LatestDeployment.JobID),
	})

	// Emit deployment status metric, respecting sparse output settings
	emitStatusMetric(
		ctx,
		c.Store,
		schemas.MetricKindEnvironmentDeploymentStatus,
		env.DefaultLabelsValues(),
		statusesList[:],
		env.LatestDeployment.Status,
		env.OutputSparseStatusMetrics,
	)

	// Store the deployment timestamp metric
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentDeploymentTimestamp,
		Labels: env.DefaultLabelsValues(),
		Value:  env.LatestDeployment.Timestamp,
	})

	// Store an information metric indicating the current state of the environment
	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindEnvironmentInformation,
		Labels: infoLabels,
		Value:  1,
	})

	return nil
}
