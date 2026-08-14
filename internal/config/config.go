package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the ccdash configuration file
type Config struct {
	Notify NotifyConfig `yaml:"notify"`
}

// NotifyConfig contains notification settings
type NotifyConfig struct {
	Enabled   bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

// Load reads the configuration from ~/.ccdash/config.yaml
// If the file doesn't exist or is incomplete, returns defaults
func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".ccdash", "config.yaml")

	// Default configuration if file doesn't exist
	defaults := &Config{
		Notify: NotifyConfig{
			Enabled:   false,
			WebhookURL: "",
		},
	}

	// Try to read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, return defaults
			return defaults, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Merge with defaults for any missing fields
	if config.Notify.WebhookURL == "" {
		config.Notify.WebhookURL = defaults.Notify.WebhookURL
	}

	return &config, nil
}
