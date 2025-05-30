package controller

import (
	"context"

	"dario.cat/mergo"
	log "github.com/sirupsen/logrus"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// GetRefs retrieves the references (branches, tags, merge requests) for a given project `p`,
// based on the project's pull configuration settings.
//
// The method returns a map of references keyed by their identifiers and any error encountered during retrieval.
//
// Behavior details:
// - Initializes an empty collection of refs to accumulate results.
//
// - If branch pulling is enabled:
//   - If any of the following branch filters are active:
//   - Include deleted branches (ExcludeDeleted is false),
//   - Limit to most recent branches (MostRecent > 0),
//   - Limit by age of branches (MaxAgeSeconds > 0),
//     then branches are fetched via the pipelines API (GetRefsFromPipelines) for branches.
//   - Otherwise, branches are fetched using the direct project branches API (GetProjectBranches).
//   - Merges the pulled branches into the refs map.
//
// - If tag pulling is enabled:
//   - Similar to branches, if any tag filters are active (ExcludeDeleted, MostRecent, MaxAgeSeconds),
//     tags are fetched from pipelines API (GetRefsFromPipelines) for tags,
//   - Otherwise, tags are fetched via the project tags API (GetProjectTags).
//   - Merges the pulled tags into the refs map.
//
// - If merge requests pulling is enabled:
//   - Merge requests are always fetched from the pipelines API (GetRefsFromPipelines).
//   - Merges the pulled merge requests into the refs map.
//
// - Returns the combined refs map and any error encountered.
//
// This method ensures that refs are fetched efficiently according to filtering options,
// using the pipelines API when advanced filtering is required, otherwise falling back to simpler APIs
func (c *Controller) GetRefs(ctx context.Context, p schemas.Project) (refs schemas.Refs, err error) {
	var pulledRefs schemas.Refs

	refs = make(schemas.Refs)

	if p.Pull.Refs.Branches.Enabled {
		// If one of these parameter is set, we will need to fetch the branches from the
		// pipelines API instead of the branches one
		if !p.Pull.Refs.Branches.ExcludeDeleted ||
			p.Pull.Refs.Branches.MostRecent > 0 ||
			p.Pull.Refs.Branches.MaxAgeSeconds > 0 {
			if pulledRefs, err = c.Gitlab.GetRefsFromPipelines(ctx, p, schemas.RefKindBranch); err != nil {
				return
			}
		} else {
			if pulledRefs, err = c.Gitlab.GetProjectBranches(ctx, p); err != nil {
				return
			}
		}

		if err = mergo.Merge(&refs, pulledRefs); err != nil {
			return
		}
	}

	if p.Pull.Refs.Tags.Enabled {
		// If one of these parameter is set, we will need to fetch the tags from the
		// pipelines API instead of the tags one
		if !p.Pull.Refs.Tags.ExcludeDeleted ||
			p.Pull.Refs.Tags.MostRecent > 0 ||
			p.Pull.Refs.Tags.MaxAgeSeconds > 0 {
			if pulledRefs, err = c.Gitlab.GetRefsFromPipelines(ctx, p, schemas.RefKindTag); err != nil {
				return
			}
		} else {
			if pulledRefs, err = c.Gitlab.GetProjectTags(ctx, p); err != nil {
				return
			}
		}

		if err = mergo.Merge(&refs, pulledRefs); err != nil {
			return
		}
	}

	if p.Pull.Refs.MergeRequests.Enabled {
		if pulledRefs, err = c.Gitlab.GetRefsFromPipelines(
			ctx,
			p,
			schemas.RefKindMergeRequest,
		); err != nil {
			return
		}

		if err = mergo.Merge(&refs, pulledRefs); err != nil {
			return
		}
	}

	return
}

// PullRefsFromProject retrieves all refs (branches, tags, merge requests) for the given project `p`
// by calling GetRefs. For each ref retrieved, it checks if the ref already exists in the store.
// If the ref does not exist, it logs the discovery of a new ref, saves it to the store,
// and schedules a task to pull metrics related to that ref.
//
// This function returns an error if any operation fails (fetching refs, checking existence,
// storing the ref). Otherwise, it returns nil.
//
// The process helps keep the store updated with the latest refs and triggers metrics collection
// for newly discovered refs.
func (c *Controller) PullRefsFromProject(ctx context.Context, p schemas.Project) error {
	refs, err := c.GetRefs(ctx, p)
	if err != nil {
		return err
	}

	for _, ref := range refs {
		refExists, err := c.Store.RefExists(ctx, ref.Key())
		if err != nil {
			return err
		}

		if !refExists {
			log.WithFields(log.Fields{
				"project-name": ref.Project.Name,
				"ref":          ref.Name,
				"ref-kind":     ref.Kind,
			}).Info("discovered new ref")

			if err = c.Store.SetRef(ctx, ref); err != nil {
				return err
			}

			c.ScheduleTask(ctx, schemas.TaskTypePullRefMetrics, string(ref.Key()), ref)
		}
	}

	return nil
}
