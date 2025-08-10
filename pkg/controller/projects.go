package controller

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// PullProject fetches a GitLab project by its name, updates local store and schedules follow-up tasks.
// It takes a context, the project name, and a pull configuration as parameters.
func (c *Controller) PullProject(ctx context.Context, name string, pull config.ProjectPull) error {
	// Retrieve project details from GitLab API
	gp, err := c.Gitlab.GetProject(ctx, name)
	if err != nil {
		// Return error if fetching project from GitLab fails
		return err
	}

	// Create a new local Project schema instance using the project's path with namespace
	p := schemas.NewProject(gp.PathWithNamespace)
	// Assign the provided pull configuration to the project instance
	p.Pull = pull

	// Check if the project already exists in the local store
	projectExists, err := c.Store.ProjectExists(ctx, p.Key())
	if err != nil {
		// Return error if checking existence in the store fails
		return err
	}

	// If the project is new (does not exist in the store)
	if !projectExists {
		// Log information about the discovery of a new project
		log.WithFields(log.Fields{
			"project-name": p.Name,
		}).Info("discovered new project")

		// Attempt to store the new project in the local store
		if err := c.Store.SetProject(ctx, p); err != nil {
			// Log an error if storing the project fails, but do not return it
			log.WithContext(ctx).
				WithError(err).
				Error()
		}

		// Schedule a task to pull refs (branches, tags, MRs) related to the project
		c.ScheduleTask(ctx, schemas.TaskTypePullRefsFromProject, string(p.Key()), p)
		// Schedule a task to pull environments related to the project
		c.ScheduleTask(ctx, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()), p)
		// Schedule a task to pull runners for this project
		c.ScheduleTask(ctx, schemas.TaskTypePullRunnersFromProject, string(p.Key()), p)
	}

	// Return nil if successful or after logging errors from SetProject
	return nil
}

// PullProjectsFromWildcard searches for projects matching the given wildcard criteria,
// adds new projects to the store, and schedules related tasks.
func (c *Controller) PullProjectsFromWildcard(ctx context.Context, w config.Wildcard) error {
	// Use GitLab API to list all projects that match the wildcard criteria
	foundProjects, err := c.Gitlab.ListProjects(ctx, w)
	if err != nil {
		// Return error if the GitLab API call fails
		return err
	}

	// Iterate over each project found by the wildcard search
	for _, p := range foundProjects {
		// Check if the project already exists in the local store
		projectExists, err := c.Store.ProjectExists(ctx, p.Key())
		if err != nil {
			// Return error if checking project existence fails
			return err
		}

		// If the project does not exist in the store, treat it as new
		if !projectExists {
			// Log detailed info about the wildcard criteria and the newly discovered project
			log.WithFields(log.Fields{
				"wildcard-search":                  w.Search,
				"wildcard-owner-kind":              w.Owner.Kind,
				"wildcard-owner-name":              w.Owner.Name,
				"wildcard-owner-include-subgroups": w.Owner.IncludeSubgroups,
				"wildcard-archived":                w.Archived,
				"project-name":                     p.Name,
			}).Info("discovered new project")

			// Store the new project in the local store
			if err := c.Store.SetProject(ctx, p); err != nil {
				// Log error if storing the project fails, but continue processing other projects
				log.WithContext(ctx).
					WithError(err).
					Error()
			}

			// Schedule a task to pull references (branches, tags, merge requests) for this project
			c.ScheduleTask(ctx, schemas.TaskTypePullRefsFromProject, string(p.Key()), p)
			// Schedule a task to pull environments for this project
			c.ScheduleTask(ctx, schemas.TaskTypePullEnvironmentsFromProject, string(p.Key()), p)
			// Schedule a task to pull runners for this project
			c.ScheduleTask(ctx, schemas.TaskTypePullRunnersFromProject, string(p.Key()), p)
		}
	}

	// Return nil after processing all found projects successfully or logging errors
	return nil
}
