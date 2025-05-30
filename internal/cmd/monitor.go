package cmd

import (
	"github.com/urfave/cli/v2"

	monitorUI "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/ui"
)

// Monitor starts the internal monitoring UI.
func Monitor(ctx *cli.Context) (int, error) {
	// Parse global flags from CLI context (e.g., internal monitoring address)
	cfg, err := parseGlobalFlags(ctx)
	if err != nil {
		return 1, err
	}

	// Start the monitoring UI with app version and configured listener address
	monitorUI.Start(
		ctx.App.Version,
		cfg.InternalMonitoringListenerAddress,
	)

	// Return success exit code
	return 0, nil
}
