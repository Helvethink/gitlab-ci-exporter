package controller

import (
	"context"
	"sort"
	"strconv"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
	log "github.com/sirupsen/logrus"
)

// uniqueSortedNonEmpty removes empty values and duplicates,
// then sorts the remaining values for stable metric labels.
func uniqueSortedNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))

	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	sort.Strings(out)
	return out
}

// runnerGroupNames returns a stable, deduplicated list of group names.
func runnerGroupNames(runner schemas.Runner) []string {
	names := make([]string, 0, len(runner.Groups))
	for _, g := range runner.Groups {
		names = append(names, g.Name)
	}
	return uniqueSortedNonEmpty(names)
}

// runnerProjectNames returns a stable, deduplicated list of project names.
// It prefers PathWithNamespace, then NameWithNamespace, then Name.
func runnerProjectNames(runner schemas.Runner) []string {
	names := make([]string, 0, len(runner.Projects))
	for _, p := range runner.Projects {
		switch {
		case p.PathWithNamespace != "":
			names = append(names, p.PathWithNamespace)
		case p.NameWithNamespace != "":
			names = append(names, p.NameWithNamespace)
		case p.Name != "":
			names = append(names, p.Name)
		}
	}
	return uniqueSortedNonEmpty(names)
}

// runnerTagNames returns a stable, deduplicated list of runner tags.
func runnerTagNames(runner schemas.Runner) []string {
	return uniqueSortedNonEmpty(runner.TagList)
}

// deleteRunnerMetrics removes all metrics currently stored for a given runner ID.
// This ensures the next export represents the latest snapshot only.
func (c *Controller) deleteRunnerMetrics(ctx context.Context, runnerID int) error {
	metrics, err := c.Store.Metrics(ctx)
	if err != nil {
		return err
	}

	runnerIDStr := strconv.Itoa(runnerID)

	for key, metric := range metrics {
		if metric.Labels["runner_id"] != runnerIDStr {
			continue
		}

		switch metric.Kind {
		case schemas.MetricKindRunner,
			schemas.MetricKindRunnerContactedAtSeconds,
			schemas.MetricKindRunnerProjectInfo,
			schemas.MetricKindRunnerTagInfo,
			schemas.MetricKindRunnerGroupInfo:
			if err := c.Store.DelMetric(ctx, key); err != nil {
				return err
			}
		}
	}

	return nil
}

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

// UpdateRunner fetches the latest runner details from GitLab and fully refreshes
// the local runner object before storing it.
func (c *Controller) UpdateRunner(ctx context.Context, runner *schemas.Runner) error {
	projectRefLogFields := log.Fields{
		"runner-id": runner.ID,
	}

	pulledRunner, err := c.Gitlab.GetRunner(ctx, runner.ProjectName, runner.ID)
	if err != nil {
		return err
	}

	// Preserve local context fields that are not guaranteed to be returned
	// by the GitLab API call.
	pulledRunner.ProjectName = runner.ProjectName
	pulledRunner.OutputSparseStatusMetrics = runner.OutputSparseStatusMetrics

	// Replace the local runner with the freshly pulled one.
	*runner = pulledRunner

	log.WithFields(projectRefLogFields).Info("update runner metrics")
	return c.Store.SetRunner(ctx, *runner)
}

// ProcessRunnerMetrics refreshes a runner from GitLab and exports:
//
// 1. One global runner info metric per runner ID
// 2. One contacted_at metric per runner ID
// 3. One runner/project relation metric per project
// 4. One runner/tag relation metric per tag
// 5. One runner/group relation metric per group
//
// This avoids exporting one runner info series per project.
func (c *Controller) ProcessRunnerMetrics(ctx context.Context, runner schemas.Runner) (err error) {
	projectRefLogFields := log.Fields{
		"project-name-or-id": runner.ProjectName,
		"runner-desc":        runner.Description,
	}

	// Refresh runner details before exporting metrics.
	if err = c.UpdateRunner(ctx, &runner); err != nil {
		return
	}

	// Remove previous runner metrics so the store only contains the latest snapshot.
	if err = c.deleteRunnerMetrics(ctx, runner.ID); err != nil {
		return
	}

	groupNames := runnerGroupNames(runner)
	projectNames := runnerProjectNames(runner)
	tagNames := runnerTagNames(runner)

	runnerID := strconv.Itoa(runner.ID)

	// Export one global runner info metric per runner.
	infoLabels := map[string]string{
		"runner_id":               runnerID,
		"runner_name":             runner.Name,
		"runner_description":      runner.Description,
		"is_shared":               strconv.FormatBool(runner.IsShared),
		"runner_type":             runner.RunnerType,
		"online":                  strconv.FormatBool(runner.Online),
		"active":                  strconv.FormatBool(!runner.Paused),
		"paused":                  strconv.FormatBool(runner.Paused),
		"status":                  runner.Status,
		"runner_maintenance_note": runner.MaintenanceNote,
	}

	log.WithFields(projectRefLogFields).Info("processing runner metrics")

	storeSetMetric(ctx, c.Store, schemas.Metric{
		Kind:   schemas.MetricKindRunner,
		Labels: infoLabels,
		Value:  1,
	})

	// Export the last contact timestamp as a numeric metric value, not as a label.
	if runner.ContactedAt != nil {
		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind: schemas.MetricKindRunnerContactedAtSeconds,
			Labels: map[string]string{
				"runner_id": runnerID,
			},
			Value: float64(runner.ContactedAt.UTC().Unix()),
		})
	}

	// Export one metric per related project.
	for _, projectName := range projectNames {
		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind: schemas.MetricKindRunnerProjectInfo,
			Labels: map[string]string{
				"runner_id": runnerID,
				"project":   projectName,
			},
			Value: 1,
		})
	}

	// Export one metric per runner tag.
	for _, tag := range tagNames {
		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind: schemas.MetricKindRunnerTagInfo,
			Labels: map[string]string{
				"runner_id": runnerID,
				"tag":       tag,
			},
			Value: 1,
		})
	}

	// Export one metric per runner group.
	for _, groupName := range groupNames {
		storeSetMetric(ctx, c.Store, schemas.Metric{
			Kind: schemas.MetricKindRunnerGroupInfo,
			Labels: map[string]string{
				"runner_id": runnerID,
				"group":     groupName,
			},
			Value: 1,
		})
	}

	return nil
}
