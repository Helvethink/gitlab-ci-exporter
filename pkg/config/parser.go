package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Format represents the configuration file format type.
type Format uint8

const (
	// FormatYAML represents a config written in YAML format.
	FormatYAML Format = iota
)

// ParseFile reads the content of the given file, detects the format based on the file extension,
// and unmarshals it into a Config object.
func ParseFile(filename string) (c Config, err error) {
	var (
		t         Format
		fileBytes []byte
	)

	// Determine the config file format based on file extension.
	t, err = GetTypeFromFileExtension(filename)
	if err != nil {
		return
	}

	// Read the content of the config file safely.
	fileBytes, err = os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return
	}

	// Parse and unmarshal the content into a Config object.
	return Parse(t, fileBytes)
}

// Parse unmarshals the provided bytes using the given Format into a Config object.
func Parse(f Format, bytes []byte) (cfg Config, err error) {
	switch f {
	case FormatYAML:
		err = yaml.Unmarshal(bytes, &cfg)
	default:
		err = fmt.Errorf("unsupported config type '%+v'", f)
	}

	// Automatically update cfg.GitLab.HealthURL for self-hosted GitLab instances.
	if cfg.Gitlab.URL != "https://gitlab.com" &&
		cfg.Gitlab.HealthURL == "https://gitlab.com/explore" {
		cfg.Gitlab.HealthURL = fmt.Sprintf("%s/-/health", cfg.Gitlab.URL)
	}

	return
}

// GetTypeFromFileExtension returns the Format based on the file extension.
func GetTypeFromFileExtension(filename string) (f Format, err error) {
	switch ext := filepath.Ext(filename); ext {
	case ".yml", ".yaml":
		f = FormatYAML
	default:
		err = fmt.Errorf("unsupported config type '%s', expected .y(a)ml", ext)
	}
	return
}
