package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// Validate checks whether the application configuration is valid.
// It takes a CLI context (typically from urfave/cli) as input.
// Returns an exit code (int) and an error if validation fails.
func Validate(cliCtx *cli.Context) (int, error) {
	log.Debug("Validating configuration..")

	// Try to configure the application using CLI context.
	// If configuration fails, log the error and return exit code 1.
	if _, err := configure(cliCtx); err != nil {
		log.WithError(err).Error("Failed to configure")
		return 1, err
	}

	// If no errors occurred, the configuration is considered valid.
	log.Debug("Configuration is valid")

	// Return exit code 0 and no error.
	return 0, nil
}
