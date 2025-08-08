package controller

import (
	"context"
	"fmt"
	"reflect"
	"regexp"

	"dario.cat/mergo"
	log "github.com/sirupsen/logrus"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	"github.com/helvethink/gitlab-ci-exporter/pkg/store"
)

// GarbageCollectProjects removes projects from the store that are no longer configured or discovered.
func (c *Controller) GarbageCollectProjects(ctx context.Context) error {
	// Log the start of the garbage collection process
	log.Info("starting 'projects' garbage collection")
	defer log.Info("ending 'projects' garbage collection")

	// Retrieve all currently stored projects from the store
	storedProjects, err := c.Store.Projects(ctx)
	if err != nil {
		return err
	}

	// Remove all projects that are explicitly configured in c.Config.Projects from the stored list,
	// as they should be kept and not deleted
	for _, cp := range c.Config.Projects {
		p := schemas.Project{Project: cp}
		delete(storedProjects, p.Key())
	}

	// Remove projects found via configured wildcards, as they should also be kept
	for _, w := range c.Config.Wildcards {
		foundProjects, err := c.Gitlab.ListProjects(ctx, w)
		if err != nil {
			return err
		}

		for _, p := range foundProjects {
			delete(storedProjects, p.Key())
		}
	}

	// Log how many projects remain in storedProjects — these are candidates for deletion
	log.WithFields(log.Fields{
		"projects-count": len(storedProjects),
	}).Debug("found projects to garbage collect")

	// Loop over the remaining projects and delete each one from the store
	for k, p := range storedProjects {
		if err = c.Store.DelProject(ctx, k); err != nil {
			return err
		}

		// Log info for each project deleted
		log.WithFields(log.Fields{
			"project-name": p.Name,
		}).Info("deleted project from the store")
	}

	return nil
}

// GarbageCollectEnvironments removes environments from the store that are no longer valid or configured.
func (c *Controller) GarbageCollectEnvironments(ctx context.Context) error {
	// Log the start and end of the garbage collection process
	log.Info("starting 'environments' garbage collection")
	defer log.Info("ending 'environments' garbage collection")

	// Retrieve all environments currently stored
	storedEnvironments, err := c.Store.Environments(ctx)
	if err != nil {
		return err
	}

	// Map to keep track of projects which have environments that need refreshing
	envProjects := make(map[schemas.Project]bool)

	// Iterate over each stored environment
	for _, env := range storedEnvironments {
		// Create a Project object from the environment's project name
		p := schemas.NewProject(env.ProjectName)

		// Check if the associated project still exists in the store
		projectExists, err := c.Store.ProjectExists(ctx, p.Key())
		if err != nil {
			return err
		}

		// If the project no longer exists, delete the environment and continue to next
		if !projectExists {
			if err = deleteEnv(ctx, c.Store, env, "non-existent-project"); err != nil {
				return err
			}
			continue
		}

		// Retrieve the latest project data from the store
		if err = c.Store.GetProject(ctx, &p); err != nil {
			return err
		}

		// If environment pulling is disabled for this project, delete the environment
		if !p.Pull.Environments.Enabled {
			if err = deleteEnv(ctx, c.Store, env, "project-pull-environments-disabled"); err != nil {
				return err
			}
			continue
		}

		// Mark this project to refresh its environments from the GitLab API later
		envProjects[p] = true

		// Use the project's configured regex to check if this environment should be kept
		re := regexp.MustCompile(p.Pull.Environments.Regexp)
		if !re.MatchString(env.Name) {
			if err = deleteEnv(ctx, c.Store, env, "environment-not-in-regexp"); err != nil {
				return err
			}
			continue
		}

		// If the environment's OutputSparseStatusMetrics setting differs from the project's,
		// update the environment and save it back to the store
		if env.OutputSparseStatusMetrics != p.OutputSparseStatusMetrics {
			env.OutputSparseStatusMetrics = p.OutputSparseStatusMetrics

			if err = c.Store.SetEnvironment(ctx, env); err != nil {
				return err
			}

			log.WithFields(log.Fields{
				"project-name":     env.ProjectName,
				"environment-name": env.Name,
			}).Info("updated environment configuration to match associated project")
		}
	}

	// Prepare a map to hold environments refreshed from the GitLab API
	existingEnvs := make(schemas.Environments)

	// For each project with tracked environments, fetch environments from GitLab API
	for p := range envProjects {
		projectEnvs, err := c.Gitlab.GetProjectEnvironments(ctx, p)
		if err != nil {
			return err
		}

		// Merge fetched environments into the existingEnvs map
		if err = mergo.Merge(&existingEnvs, projectEnvs); err != nil {
			return err
		}
	}

	// Reload the stored environments after possible updates
	storedEnvironments, err = c.Store.Environments(ctx)
	if err != nil {
		return err
	}

	// Delete environments from the store that do not exist anymore in the API data
	for k, env := range storedEnvironments {
		if _, exists := existingEnvs[k]; !exists {
			if err = deleteEnv(ctx, c.Store, env, "non-existent-environment"); err != nil {
				return err
			}
		}
	}

	return nil
}

// GarbageCollectRefs cleans up stored refs that are no longer valid or configured.
func (c *Controller) GarbageCollectRefs(ctx context.Context) error {
	log.Info("starting 'refs' garbage collection")
	defer log.Info("ending 'refs' garbage collection")

	// Retrieve all refs currently stored
	storedRefs, err := c.Store.Refs(ctx)
	if err != nil {
		return err
	}

	// Iterate over each stored ref to validate it
	for _, ref := range storedRefs {
		// Check if the project associated with the ref still exists
		projectExists, err := c.Store.ProjectExists(ctx, ref.Project.Key())
		if err != nil {
			return err
		}

		// If the project no longer exists, delete the ref and continue
		if !projectExists {
			if err = deleteRef(ctx, c.Store, ref, "non-existent-project"); err != nil {
				return err
			}
			continue
		}

		// Check if the ref is still configured to be pulled according to the project's ref regex
		var re *regexp.Regexp

		// Get the regular expression configured for this ref's kind (branch, tag, etc.)
		if re, err = schemas.GetRefRegexp(ref.Project.Pull.Refs, ref.Kind); err != nil {
			// If the ref kind is invalid, delete the ref from the store
			if err = deleteRef(ctx, c.Store, ref, "invalid-ref-kind"); err != nil {
				return err
			}
		}

		// If the ref's name does not match the configured regex, delete it
		if !re.MatchString(ref.Name) {
			if err = deleteRef(ctx, c.Store, ref, "ref-not-matching-regexp"); err != nil {
				return err
			}
		}

		// Load the latest project configuration from the store to verify sync status
		p := ref.Project
		if err = c.Store.GetProject(ctx, &p); err != nil {
			return err
		}

		// If the stored project configuration differs from the ref's associated project,
		// update the ref with the latest project data and save it back to the store
		if !reflect.DeepEqual(ref.Project, p) {
			ref.Project = p

			if err = c.Store.SetRef(ctx, ref); err != nil {
				return err
			}

			log.WithFields(log.Fields{
				"project-name": ref.Project.Name,
				"ref":          ref.Name,
			}).Info("updated ref, associated project configuration was not in sync")
		}
	}

	// Retrieve all projects from the store to refresh expected refs from API
	projects, err := c.Store.Projects(ctx)
	if err != nil {
		return err
	}

	// Map to keep track of refs that should still exist based on API data
	expectedRefs := make(map[schemas.RefKey]bool)

	// For each project, fetch the current refs from the external API
	for _, p := range projects {
		refs, err := c.GetRefs(ctx, p)
		if err != nil {
			return err
		}

		// Mark each fetched ref as expected
		for _, ref := range refs {
			expectedRefs[ref.Key()] = true
		}
	}

	// Reload stored refs as some might have been removed above
	storedRefs, err = c.Store.Refs(ctx)
	if err != nil {
		return err
	}

	// Delete any refs from the store that are no longer expected (not present in API results)
	for k, ref := range storedRefs {
		if _, expected := expectedRefs[k]; !expected {
			if err = deleteRef(ctx, c.Store, ref, "not-expected"); err != nil {
				return err
			}
		}
	}

	return nil
}

// GarbageCollectRunners cleans up stored runners that are no longer valid or configured.
func (c *Controller) GarbageCollectRunners(ctx context.Context) error {
	// Log the start and end of the garbage collection process
	log.Info("starting 'runners' garbage collection")
	defer log.Info("ending 'runners' garbage collection")

	// Retrieve all Runners currently stored
	storedRunners, err := c.Store.Runners(ctx)
	if err != nil {
		return err
	}

	// Map to keep track of projects which have runners that need refreshing
	runnerProjects := make(map[schemas.Project]bool)

	// Iterate over each stored runner
	for _, runner := range storedRunners {
		// Create a Project object from the runner's project name
		p := schemas.NewProject(runner.ProjectName)

		// Check if the associated project still exists in the store
		projectExists, err := c.Store.ProjectExists(ctx, p.Key())
		if err != nil {
			return err
		}

		// If the project no longer exists, delete the runner and continue to next
		if !projectExists {
			if err = deleteRunner(ctx, c.Store, runner, "non-existent-project"); err != nil {
				return err
			}
			continue
		}

		// Retrieve the latest project data from the store
		if err = c.Store.GetProject(ctx, &p); err != nil {
			return err
		}

		// If runner pulling is disabled for this project, delete the runner
		if !p.Pull.Runners.Enabled {
			if err = deleteRunner(ctx, c.Store, runner, "project-pull-runners-disabled"); err != nil {
				return err
			}
			continue
		}

		// Mark this project to refresh its runners from the GitLab API later
		runnerProjects[p] = true

		// Use the project's configured regex to check if this runner should be kept
		re := regexp.MustCompile(p.Pull.Runners.Regexp)
		if !re.MatchString(runner.Description) {
			if err = deleteRunner(ctx, c.Store, runner, "runner-not-in-regexp"); err != nil {
				return err
			}
			continue
		}

		// If the environment's OutputSparseStatusMetrics setting differs from the project's,
		// update the environment and save it back to the store
		if runner.OutputSparseStatusMetrics != p.OutputSparseStatusMetrics {
			runner.OutputSparseStatusMetrics = p.OutputSparseStatusMetrics

			if err = c.Store.SetRunner(ctx, runner); err != nil {
				return err
			}

			log.WithFields(log.Fields{
				"project-name": runner.ProjectName,
				"runner-name":  runner.Name,
			}).Info("updated runner configuration to match associated project")
		}
	}

	// Prepare a map to hold runners refreshed from the GitLab API
	existingRunners := make(schemas.Runners)

	// For each project with tracked runners, fetch runners from GitLab API
	for p := range runnerProjects {
		projectRunners, err := c.Gitlab.GetProjectRunners(ctx, p)
		if err != nil {
			return err
		}

		// Merge fetched runners into the existingRunners map
		if err = mergo.Merge(&existingRunners, projectRunners); err != nil {
			return err
		}
	}

	// Reload the stored runners after possible updates
	storedRunners, err = c.Store.Runners(ctx)
	if err != nil {
		return err
	}

	// Delete runners from the store that do not exist anymore in the API data
	for k, runner := range storedRunners {
		if _, exists := existingRunners[k]; !exists {
			if err = deleteRunner(ctx, c.Store, runner, "non-existent-runner"); err != nil {
				return err
			}
		}
	}

	return nil
}

// GarbageCollectMetrics performs cleanup of obsolete or invalid metrics in the store.
// It ensures metrics linked to non-existent projects, refs, or environments are removed,
// and also respects project-level settings that may disable certain metrics.
//
// The function fetches all stored environments, refs, and metrics, then iterates over each metric:
// - It first verifies that required labels ("project" plus either "ref" or "environment") exist; otherwise, it deletes the metric.
// - If the metric is related to a Ref (has "ref" label), it checks:
//   - Whether the ref still exists; if not, deletes the metric.
//   - If job-related metrics are disabled for the ref's project, deletes the metric.
//   - If sparse output mode for status metrics is enabled on the ref's project and the metric value isn't 1, deletes the metric.
//
// - If the metric is related to an Environment (has "environment" label), it checks:
//   - Whether the environment still exists; if not, deletes the metric.
//   - If sparse output mode for environment deployment status metrics is enabled and the metric value isn't 1, deletes the metric.
//
// This keeps the metrics store consistent, up-to-date, and free from stale data, according to project configurations.
func (c *Controller) GarbageCollectMetrics(ctx context.Context) error {
	log.Info("starting 'metrics' garbage collection")
	defer log.Info("ending 'metrics' garbage collection")

	storedEnvironments, err := c.Store.Environments(ctx)
	if err != nil {
		return err
	}

	// TODO: This function must be checked (Runner metrics implementation)
	storedRunners, err := c.Store.Runners(ctx)
	if err != nil {
		return err
	}
	fmt.Println("Stored Runners Metrics:", storedRunners)

	storedRefs, err := c.Store.Refs(ctx)
	if err != nil {
		return err
	}

	storedMetrics, err := c.Store.Metrics(ctx)
	if err != nil {
		return err
	}

	for k, m := range storedMetrics {
		// Retrieve metric labels needed to identify the owning project, ref, or environment.
		metricLabelProject, metricLabelProjectExists := m.Labels["project"]
		metricLabelRef, metricLabelRefExists := m.Labels["ref"]
		metricLabelEnvironment, metricLabelEnvironmentExists := m.Labels["environment"]

		// Delete metrics missing the "project" label or both "ref" and "environment" labels.
		if !metricLabelProjectExists || (!metricLabelRefExists && !metricLabelEnvironmentExists) {
			if err = c.Store.DelMetric(ctx, k); err != nil {
				return err
			}

			log.WithFields(log.Fields{
				"metric-kind":   m.Kind,
				"metric-labels": m.Labels,
				"reason":        "project-or-ref-and-environment-label-undefined",
			}).Info("deleted metric from the store")
		}

		// Handle metrics related to a Ref (no environment label).
		if metricLabelRefExists && !metricLabelEnvironmentExists {
			refKey := schemas.NewRef(
				schemas.NewProject(metricLabelProject),
				schemas.RefKind(m.Labels["kind"]),
				metricLabelRef,
			).Key()

			ref, refExists := storedRefs[refKey]

			// Delete the metric if the referenced ref no longer exists.
			if !refExists {
				if err = c.Store.DelMetric(ctx, k); err != nil {
					return err
				}

				log.WithFields(log.Fields{
					"metric-kind":   m.Kind,
					"metric-labels": m.Labels,
					"reason":        "non-existent-ref",
				}).Info("deleted metric from the store")

				continue
			}

			// For job-related metrics, check if job metric pulling is disabled; delete if so.
			switch m.Kind {
			case schemas.MetricKindJobArtifactSizeBytes,
				schemas.MetricKindJobDurationSeconds,
				schemas.MetricKindJobID,
				schemas.MetricKindJobRunCount,
				schemas.MetricKindJobStatus,
				schemas.MetricKindJobTimestamp:
				if !ref.Project.Pull.Pipeline.Jobs.Enabled {
					if err = c.Store.DelMetric(ctx, k); err != nil {
						return err
					}

					log.WithFields(log.Fields{
						"metric-kind":   m.Kind,
						"metric-labels": m.Labels,
						"reason":        "jobs-metrics-disabled-on-ref",
					}).Info("deleted metric from the store")

					continue
				}
			default:
				// no action for other kinds here
			}

			// For status metrics, if sparse output is enabled and the metric value is not 1, delete the metric.
			switch m.Kind {
			case schemas.MetricKindJobStatus,
				schemas.MetricKindStatus:
				if ref.Project.OutputSparseStatusMetrics && m.Value != 1 {
					if err = c.Store.DelMetric(ctx, k); err != nil {
						return err
					}

					log.WithFields(log.Fields{
						"metric-kind":   m.Kind,
						"metric-labels": m.Labels,
						"reason":        "output-sparse-metrics-enabled-on-ref",
					}).Info("deleted metric from the store")

					continue
				}
			default:
				// no action for other kinds here
			}
		}
		// TODO: Handle metrics related to Runner
		/**
		switch m.Kind {
				case schemas.MetricKindRunner
		}
		*/

		// Handle metrics related to an Environment.
		if metricLabelEnvironmentExists {
			envKey := schemas.Environment{
				ProjectName: metricLabelProject,
				Name:        metricLabelEnvironment,
			}.Key()

			env, envExists := storedEnvironments[envKey]

			// Delete the metric if the environment no longer exists.
			if !envExists {
				if err = c.Store.DelMetric(ctx, k); err != nil {
					return err
				}

				log.WithFields(log.Fields{
					"metric-kind":   m.Kind,
					"metric-labels": m.Labels,
					"reason":        "non-existent-environment",
				}).Info("deleted metric from the store")

				continue
			}

			// For environment deployment status metrics, if sparse output is enabled and value is not 1, delete the metric.
			switch m.Kind {
			case schemas.MetricKindEnvironmentDeploymentStatus:
				if env.OutputSparseStatusMetrics && m.Value != 1 {
					if err = c.Store.DelMetric(ctx, k); err != nil {
						return err
					}

					log.WithFields(log.Fields{
						"metric-kind":   m.Kind,
						"metric-labels": m.Labels,
						"reason":        "output-sparse-metrics-enabled-on-environment",
					}).Info("deleted metric from the store")

					continue
				}
			}
		}
	}

	return nil
}

// deleteEnv removes the specified environment from the store and logs the deletion reason.
// It takes a context, the store interface, the environment to delete, and a reason string.
// If deletion fails, it returns the error to the caller.
func deleteEnv(ctx context.Context, s store.Store, env schemas.Environment, reason string) (err error) {
	if err = s.DelEnvironment(ctx, env.Key()); err != nil {
		return
	}

	log.WithFields(log.Fields{
		"project-name":     env.ProjectName,
		"environment-name": env.Name,
		"reason":           reason,
	}).Info("deleted environment from the store")

	return
}

// deleteRunner removes the specified runner from the store and logs the deletion reason.
// It takes a context, the store interface, the runner to delete, and a reason string.
// If deletion fails, it returns the error to the caller.
func deleteRunner(ctx context.Context, s store.Store, runner schemas.Runner, reason string) (err error) {
	if err = s.DelRunner(ctx, runner.Key()); err != nil {
		return
	}

	log.WithFields(log.Fields{
		"project-name": runner.ProjectName,
		"runner-name":  runner.Name,
		"reason":       reason,
	}).Info("deleted runner from the store")

	return
}

// deleteRef removes the specified ref from the store and logs the deletion reason.
// It takes a context, the store interface, the ref to delete, and a reason string.
// If deletion fails, it returns the error to the caller.
func deleteRef(ctx context.Context, s store.Store, ref schemas.Ref, reason string) (err error) {
	if err = s.DelRef(ctx, ref.Key()); err != nil {
		return
	}

	log.WithFields(log.Fields{
		"project-name": ref.Project.Name,
		"ref":          ref.Name,
		"ref-kind":     ref.Kind,
		"reason":       reason,
	}).Info("deleted ref from the store")

	return
}
