package logger

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

// Config Struct that holds logging configuration options.
type Config struct {
	Level        string // Logging level (e.g., "info", "debug", "error")
	Format       string // Logging format ("text" or "json")
	ReportCaller bool   // Whether to include the calling method/file in the logs
}

// Configure sets up the logger according to the provided Config settings.
func Configure(c Config) (err error) {
	// Parse and set the log level
	parsedLevel, err := log.ParseLevel(c.Level)
	if err != nil {
		return // Return error if the log level is invalid
	}
	log.SetLevel(parsedLevel)

	// Default formatter is text with full timestamp
	formatter := &log.TextFormatter{
		FullTimestamp: true,
	}
	log.SetFormatter(formatter)

	// Override formatter if the format is "json"
	switch c.Format {
	case "text":
		// Already set, no action needed
	case "json":
		log.SetFormatter(&log.JSONFormatter{}) // Use JSON format
	default:
		err = fmt.Errorf("invalid log format '%s'", c.Format)
		return // Return error for unsupported formats
	}

	// Enable or disable reporting the caller (file and line number)
	log.SetReportCaller(c.ReportCaller)

	// Set output to standard output
	log.SetOutput(os.Stdout)

	return // Return nil if everything is configured successfully
}
