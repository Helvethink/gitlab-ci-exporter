package controller

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	goGitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// processPipelineEvent processes a GitLab pipeline event and triggers a metrics pull
// for the associated Git reference (branch, tag, or merge request).
//
// It determines the kind of ref involved in the pipeline event based on the event details:
//   - If the event is related to a merge request (indicated by a non-zero IID), it treats
//     the ref as a merge request and uses the merge request IID as the ref name.
//   - If the event is related to a tag pipeline, it treats the ref as a tag.
//   - Otherwise, it treats the ref as a branch.
//
// Finally, it constructs a new Ref object representing the pipeline's project and ref,
// and calls triggerRefMetricsPull to update metrics related to that ref.
//
// Parameters:
// - ctx: the context for tracing and cancellation
// - e: the GitLab pipeline event to process
func (c *Controller) processPipelineEvent(ctx context.Context, e goGitlab.PipelineEvent) {
	var (
		refKind schemas.RefKind          // Type of Git ref (branch, tag, or merge request)
		refName = e.ObjectAttributes.Ref // The ref name from the pipeline event (branch or tag name)
	)

	// TODO: Consider validating if the ref matches the expected pattern for a merge request ref

	// If the pipeline event is related to a merge request (non-zero IID),
	// treat the ref as a Merge Request kind and use the IID as the ref name
	if e.MergeRequest.IID != 0 {
		refKind = schemas.RefKindMergeRequest
		refName = strconv.Itoa(e.MergeRequest.IID)
	} else if e.ObjectAttributes.Tag {
		// If the pipeline event is for a tag, set ref kind to Tag
		refKind = schemas.RefKindTag
	} else {
		// Otherwise, treat the ref as a branch
		refKind = schemas.RefKindBranch
	}

	// Trigger a metrics pull for the detected ref by creating a new Ref object
	// and calling the triggerRefMetricsPull method
	c.triggerRefMetricsPull(ctx, schemas.NewRef(
		schemas.NewProject(e.Project.PathWithNamespace),
		refKind,
		refName,
	))
}

// processJobEvent handles a GitLab job event by identifying the reference type (branch or tag)
// and triggers a metrics update for the corresponding Git reference.
//
// The function performs the following steps:
// 1. Determines if the job event's ref is a tag or a branch.
// 2. Retrieves the full project details from GitLab based on the project ID from the event.
// 3. If project retrieval fails, logs the error and stops further processing.
// 4. Creates a new Ref object representing the project and ref information.
// 5. Initiates a metrics pull for the identified Ref.
//
// Parameters:
// - ctx: Context to support cancellation, deadlines, and tracing.
// - e: The GitLab job event containing job metadata.
func (c *Controller) processJobEvent(ctx context.Context, e goGitlab.JobEvent) {
	var (
		// Initialize the reference kind and name from the event's ref field
		refKind schemas.RefKind
		refName = e.Ref
	)

	// Determine the type of ref: tag or branch
	if e.Tag {
		refKind = schemas.RefKindTag
	} else {
		refKind = schemas.RefKindBranch
	}

	// Fetch the full project metadata from GitLab using the project ID from the event
	project, _, err := c.Gitlab.Projects.GetProject(e.ProjectID, nil)
	if err != nil {
		// Log the error if project retrieval fails and stop processing
		log.WithContext(ctx).
			WithError(err).
			Error("reading project from GitLab")

		return
	}

	// Create a new Ref object combining the project path, ref kind, and ref name
	ref := schemas.NewRef(
		schemas.NewProject(project.PathWithNamespace),
		refKind,
		refName,
	)

	// Trigger the process to pull metrics for the identified ref
	c.triggerRefMetricsPull(ctx, ref)
}

// processPushEvent handles GitLab push events, specifically focusing on branch deletion events.
//
// When a branch is deleted in GitLab, the push event’s CheckoutSHA is empty ("").
// This function detects such deletions, extracts the branch name from the ref,
// and triggers the removal of the corresponding ref from the internal store.
//
// Parameters:
// - ctx: Context for cancellation, deadlines, and tracing.
// - e: The GitLab push event containing information about the pushed ref and project.
func (c *Controller) processPushEvent(ctx context.Context, e goGitlab.PushEvent) {
	// Only proceed if CheckoutSHA is empty, which indicates a branch deletion event
	if e.CheckoutSHA == "" {
		// The ref kind is a branch for deletion events
		var (
			refKind = schemas.RefKindBranch
			refName string
		)

		// GitLab branch refs have a "refs/heads/" prefix.
		// Extract the branch name by removing this prefix.
		if branch, found := strings.CutPrefix(e.Ref, "refs/heads/"); found {
			refName = branch
		} else {
			// If the prefix is missing, log an error and return early.
			// This indicates unexpected ref format.
			log.WithContext(ctx).
				WithFields(log.Fields{
					"project-name": e.Project.Name,
					"ref":          e.Ref,
				}).
				Error("extracting branch name from ref")

			return
		}

		// Create a new Ref object representing the project and branch to be deleted.
		// Call deleteRef to remove this ref from the store, indicating it was deleted upstream.
		_ = deleteRef(ctx, c.Store, schemas.NewRef(
			schemas.NewProject(e.Project.PathWithNamespace),
			refKind,
			refName,
		), "received branch deletion push event from webhook")
	}
}

// processTagEvent handles GitLab tag events, specifically focusing on tag deletion events.
//
// When a tag is deleted in GitLab, the push event’s CheckoutSHA is empty ("").
// This function detects such deletions, extracts the tag name from the ref,
// and triggers the removal of the corresponding tag ref from the internal store.
//
// Parameters:
// - ctx: Context for cancellation, deadlines, and tracing.
// - e: The GitLab tag event containing information about the tag ref and project.
func (c *Controller) processTagEvent(ctx context.Context, e goGitlab.TagEvent) {
	// Only proceed if CheckoutSHA is empty, indicating a tag deletion event
	if e.CheckoutSHA == "" {
		// The ref kind is a tag for deletion events
		var (
			refKind = schemas.RefKindTag
			refName string
		)

		// GitLab tag refs have a "refs/tags/" prefix.
		// Extract the tag name by removing this prefix.
		if tag, found := strings.CutPrefix(e.Ref, "refs/tags/"); found {
			refName = tag
		} else {
			// If the prefix is missing, log an error and return early.
			// This indicates an unexpected ref format.
			log.WithContext(ctx).
				WithFields(log.Fields{
					"project-name": e.Project.Name,
					"ref":          e.Ref,
				}).
				Error("extracting tag name from ref")

			return
		}

		// Create a new Ref object representing the project and tag to be deleted.
		// Call deleteRef to remove this ref from the store, indicating it was deleted upstream.
		_ = deleteRef(ctx, c.Store, schemas.NewRef(
			schemas.NewProject(e.Project.PathWithNamespace),
			refKind,
			refName,
		), "received tag deletion tag event from webhook")
	}
}

// processMergeEvent handles GitLab merge request events.
//
// It processes specific merge request actions such as "close" and "merge".
// For these events, it deletes the corresponding merge request reference from the internal store,
// as the merge request is no longer active or relevant.
//
// Parameters:
// - ctx: Context for cancellation, deadlines, and tracing.
// - e: The GitLab merge event containing information about the merge request and project.
func (c *Controller) processMergeEvent(ctx context.Context, e goGitlab.MergeEvent) {
	// Create a Ref representing the merge request using the project namespace and merge request IID.
	ref := schemas.NewRef(
		schemas.NewProject(e.Project.PathWithNamespace),
		schemas.RefKindMergeRequest,
		strconv.Itoa(e.ObjectAttributes.IID),
	)

	// Handle the merge request action
	switch e.ObjectAttributes.Action {
	// On "close" event, delete the merge request ref from the store
	case "close":
		_ = deleteRef(ctx, c.Store, ref, "received merge request close event from webhook")
	// On "merge" event, delete the merge request ref from the store
	case "merge":
		_ = deleteRef(ctx, c.Store, ref, "received merge request merge event from webhook")
	// For other actions, log that they are unsupported (e.g. "open", "update")
	default:
		log.
			WithField("merge-request-event-type", e.ObjectAttributes.Action).
			Debug("received a non supported merge-request event type as a webhook")
	}
}

// triggerRefMetricsPull processes a reference (branch, tag, or merge request ref) and triggers
// a metrics pull if the ref is configured or matches configured wildcards.
//
// It first checks if the ref already exists in the store. If it does not exist,
// it tries to determine whether the project exists or can be matched by configured wildcards.
// If the project is found or matches a wildcard and the ref matches the project configuration,
// it stores the ref and then schedules a task to pull metrics for this ref.
//
// If the ref already exists, it immediately schedules a task to pull metrics.
//
// Parameters:
// - ctx: Context for cancellation, deadlines, and tracing.
// - ref: The reference (branch/tag/merge request) to check and possibly pull metrics for.
func (c *Controller) triggerRefMetricsPull(ctx context.Context, ref schemas.Ref) {
	logFields := log.Fields{
		"project-name": ref.Project.Name,
		"ref":          ref.Name,
		"ref-kind":     ref.Kind,
	}

	// Check if the ref already exists in the internal store
	refExists, err := c.Store.RefExists(ctx, ref.Key())
	if err != nil {
		log.WithContext(ctx).
			WithFields(logFields).
			WithError(err).
			Error("reading ref from the store")
		return
	}

	// If the ref does not exist, try to see if the project is known or can be matched by wildcards
	if !refExists {
		p := schemas.NewProject(ref.Project.Name)

		projectExists, err := c.Store.ProjectExists(ctx, p.Key())
		if err != nil {
			log.WithContext(ctx).
				WithFields(logFields).
				WithError(err).
				Error("reading project from the store")
			return
		}

		// If project does not exist, try to match wildcards configured to discover projects
		if !projectExists && len(c.Config.Wildcards) > 0 {
			for _, w := range c.Config.Wildcards {
				matches, err := isRefMatchingWilcard(w, ref)
				if err != nil {
					log.WithContext(ctx).
						WithError(err).
						Warn("checking if the ref matches the wildcard config")
					continue
				}

				if matches {
					// If matched, schedule a task to pull the entire project based on the wildcard config
					c.ScheduleTask(context.TODO(), schemas.TaskTypePullProject, ref.Project.Name, ref.Project.Name, w.Pull)
					log.WithFields(logFields).Info("project ref not currently exported but its configuration matches a wildcard, triggering a pull of the project")
				} else {
					log.WithFields(logFields).Debug("project ref not matching wildcard, skipping..")
				}
			}

			log.WithFields(logFields).Info("done looking up for wildcards matching the project ref")
			return
		}

		// If project exists, verify if the ref matches the project's pull refs configuration
		if projectExists {
			if err := c.Store.GetProject(ctx, &p); err != nil {
				log.WithContext(ctx).
					WithFields(logFields).
					WithError(err).
					Error("reading project from the store")
				return
			}

			matches, err := isRefMatchingProjectPullRefs(p.Pull.Refs, ref)
			if err != nil {
				log.WithContext(ctx).
					WithError(err).
					Error("checking if the ref matches the project config")
				return
			}

			if matches {
				ref.Project = p

				// Store the ref as it's now confirmed to be of interest
				if err = c.Store.SetRef(ctx, ref); err != nil {
					log.WithContext(ctx).
						WithFields(logFields).
						WithError(err).
						Error("writing ref in the store")
					return
				}

				// Jump to scheduling the pull task after storing the ref
				goto schedulePull
			}
		}

		log.WithFields(logFields).Info("ref not configured in the exporter, ignoring pipeline webhook")
		return
	}

schedulePull:
	// If ref exists or has just been stored, schedule a pull metrics task for this ref
	log.WithFields(logFields).Info("received a pipeline webhook from GitLab for a ref, triggering metrics pull")

	// TODO: When all metrics are sent in the webhook, this pull might be avoidable
	c.ScheduleTask(context.TODO(), schemas.TaskTypePullRefMetrics, string(ref.Key()), ref)
}

// processDeploymentEvent handles a GitLab deployment event by triggering a metrics pull
// for the specific environment related to the deployment.
//
// Parameters:
// - ctx: Context for cancellation, deadlines, and tracing.
// - e: The GitLab deployment event containing deployment details.
func (c *Controller) processDeploymentEvent(ctx context.Context, e goGitlab.DeploymentEvent) {
	// Construct an Environment object from the event data
	env := schemas.Environment{
		ProjectName: e.Project.PathWithNamespace, // Full project namespace (e.g. "group/project")
		Name:        e.Environment,               // Name of the environment deployed to (e.g. "production")
	}

	// Trigger a metrics pull for the specified environment
	c.triggerEnvironmentMetricsPull(ctx, env)
}

// triggerEnvironmentMetricsPull triggers a metrics pull for a given environment.
// It checks if the environment exists in the store or matches configured wildcards or project settings,
// and schedules a pull task if appropriate.
//
// Parameters:
// - ctx: context for tracing, cancellation, and deadlines.
// - env: the environment to trigger metrics pull for.
func (c *Controller) triggerEnvironmentMetricsPull(ctx context.Context, env schemas.Environment) {
	// Prepare common log fields for consistent logging
	logFields := log.Fields{
		"project-name":     env.ProjectName,
		"environment-name": env.Name,
	}

	// Check if the environment exists in the store
	envExists, err := c.Store.EnvironmentExists(ctx, env.Key())
	if err != nil {
		log.WithContext(ctx).
			WithFields(logFields).
			WithError(err).
			Error("reading environment from the store")
		return
	}

	if !envExists {
		// If environment is not found, check if the project exists
		p := schemas.NewProject(env.ProjectName)

		projectExists, err := c.Store.ProjectExists(ctx, p.Key())
		if err != nil {
			log.WithContext(ctx).
				WithFields(logFields).
				WithError(err).
				Error("reading project from the store")
			return
		}

		// If project does not exist, check if it matches any configured wildcards
		if !projectExists && len(c.Config.Wildcards) > 0 {
			for _, w := range c.Config.Wildcards {
				// Check if the environment matches the wildcard pattern
				matches, err := isEnvMatchingWilcard(w, env)
				if err != nil {
					log.WithContext(ctx).
						WithError(err).
						Warn("checking if the env matches the wildcard config")
					continue
				}

				if matches {
					// Schedule a project pull task for matching wildcard configuration
					c.ScheduleTask(context.TODO(), schemas.TaskTypePullProject, env.ProjectName, env.ProjectName, w.Pull)
					log.WithFields(logFields).Info("project environment not currently exported but its configuration matches a wildcard, triggering a pull of the project")
				} else {
					log.WithFields(logFields).Debug("project ref not matching wildcard, skipping..")
				}
			}

			log.WithFields(logFields).Info("done looking up for wildcards matching the project ref")
			return
		}

		// If the project exists, verify if environment matches the project's pull configuration
		if projectExists {
			if err := c.Store.GetProject(ctx, &p); err != nil {
				log.WithContext(ctx).
					WithFields(logFields).
					WithError(err).
					Error("reading project from the store")
			}

			matches, err := isEnvMatchingProjectPullEnvironments(p.Pull.Environments, env)
			if err != nil {
				log.WithContext(ctx).
					WithError(err).
					Error("checking if the env matches the project config")
				return
			}

			if matches {
				// Since environment ID might be missing, update environment details by querying GitLab API
				if err = c.UpdateEnvironment(ctx, &env); err != nil {
					log.WithContext(ctx).
						WithFields(logFields).
						WithError(err).
						Error("updating event from GitLab API")
					return
				}
				// Proceed to schedule metrics pull after update
				goto schedulePull
			}
		}

		// Environment is neither configured nor matched by any project or wildcard - ignore the event
		log.WithFields(logFields).
			Info("environment not configured in the exporter, ignoring deployment webhook")
		return
	}

	// If the environment ID is not set, refresh it from the store to ensure we have a valid ID
	if env.ID == 0 {
		if err = c.Store.GetEnvironment(ctx, &env); err != nil {
			log.WithContext(ctx).
				WithFields(logFields).
				WithError(err).
				Error("reading environment from the store")
		}
	}

schedulePull:
	// Log and schedule the metrics pull task for the environment
	log.WithFields(logFields).Info("received a deployment webhook from GitLab for an environment, triggering metrics pull")
	c.ScheduleTask(ctx, schemas.TaskTypePullEnvironmentMetrics, string(env.Key()), env)
}

// isRefMatchingProjectPullRefs checks if a given ref matches the pull refs configuration for a project.
//
// Parameters:
// - pprs: the ProjectPullRefs configuration specifying which refs are enabled and their regex patterns.
// - ref: the Ref object representing the reference to check.
//
// Returns:
// - matches: true if the ref matches the configuration and regex pattern, false otherwise.
// - err: error if an invalid ref kind is provided or regex compilation fails.
func isRefMatchingProjectPullRefs(pprs config.ProjectPullRefs, ref schemas.Ref) (matches bool, err error) {
	// First, check if the kind of ref is enabled in the project's pull refs config
	switch ref.Kind {
	case schemas.RefKindBranch:
		if !pprs.Branches.Enabled {
			// Branch refs not enabled, return no match
			return
		}
	case schemas.RefKindTag:
		if !pprs.Tags.Enabled {
			// Tag refs not enabled, return no match
			return
		}
	case schemas.RefKindMergeRequest:
		if !pprs.MergeRequests.Enabled {
			// Merge request refs not enabled, return no match
			return
		}
	default:
		// Invalid ref kind, return an error
		return false, fmt.Errorf("invalid ref kind %v", ref.Kind)
	}

	// Compile the regex pattern for the ref kind from the project's pull refs config
	var re *regexp.Regexp
	if re, err = schemas.GetRefRegexp(pprs, ref.Kind); err != nil {
		return
	}

	// Check if the ref name matches the regex pattern and return the result
	return re.MatchString(ref.Name), nil
}

// isEnvMatchingProjectPullEnvironments checks if a given environment matches
// the project's pull environments configuration.
//
// Parameters:
// - ppe: the ProjectPullEnvironments configuration, including enabled flag and regexp pattern.
// - env: the Environment object to check.
//
// Returns:
// - matches: true if the environment is enabled and matches the regexp pattern, false otherwise.
// - err: error if regexp compilation fails.
func isEnvMatchingProjectPullEnvironments(ppe config.ProjectPullEnvironments, env schemas.Environment) (matches bool, err error) {
	// Check if environment pulling is enabled in the config
	if !ppe.Enabled {
		// Pulling not enabled, so no match
		return
	}

	// Compile the regular expression for matching environment names
	var re *regexp.Regexp
	if re, err = regexp.Compile(ppe.Regexp); err != nil {
		// Return error if regexp compilation fails
		return
	}

	// Return whether the environment's name matches the compiled regexp
	return re.MatchString(env.Name), nil
}

// isRefMatchingWilcard checks if a given ref matches a wildcard configuration.
//
// It first verifies if the wildcard owner matches the ref's project name
// or if the owner is global (empty Kind means global).
// Then it checks if the ref matches the pull refs configuration of the wildcard.
//
// Returns true if all checks pass, false otherwise.
func isRefMatchingWilcard(w config.Wildcard, ref schemas.Ref) (matches bool, err error) {
	// Check if wildcard owner is specified and matches the ref's project name
	if w.Owner.Kind != "" && !strings.Contains(ref.Project.Name, w.Owner.Name) {
		return
	}

	// Check if the ref matches the pull refs pattern/configuration of the wildcard
	return isRefMatchingProjectPullRefs(w.Pull.Refs, ref)
}

// isEnvMatchingWilcard checks if a given environment matches a wildcard configuration.
//
// It first verifies if the wildcard owner matches the environment's project name
// or if the owner is global (empty Kind means global).
// Then it checks if the environment matches the pull environments configuration of the wildcard.
//
// Returns true if all checks pass, false otherwise.
func isEnvMatchingWilcard(w config.Wildcard, env schemas.Environment) (matches bool, err error) {
	// Check if wildcard owner is specified and matches the environment's project name
	if w.Owner.Kind != "" && !strings.Contains(env.ProjectName, w.Owner.Name) {
		return
	}

	// Check if the environment matches the pull environments pattern/configuration of the wildcard
	return isEnvMatchingProjectPullEnvironments(w.Pull.Environments, env)
}
