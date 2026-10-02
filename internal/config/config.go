package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the ccdash configuration file
type Config struct {
	Notify  NotifyConfig  `yaml:"notify"`
	Alerts  AlertsConfig  `yaml:"alerts"`
	Pricing PricingConfig `yaml:"pricing"`
}

// NotifyConfig contains notification settings
type NotifyConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

// AlertsConfig contains alert threshold settings
type AlertsConfig struct {
	CostThresholdUSD float64 `yaml:"cost_threshold_usd"`
}

// PricingConfig contains optional per-model pricing overrides. The preferred
// form is:
//
//	pricing:
//	  models:
//	    model-name:
//	      input_per_million: 1
//	      output_per_million: 5
//	      cache_read_per_million: 0.1
//	      cache_create_per_million: 1.25
//
// Direct model entries under pricing are accepted too for a compact config.
type PricingConfig struct {
	Models map[string]ModelPricing `yaml:"models"`
}

// ModelPricing contains prices in dollars per million tokens.
type ModelPricing struct {
	InputPerMillion       float64 `yaml:"input_per_million"`
	OutputPerMillion      float64 `yaml:"output_per_million"`
	CacheReadPerMillion   float64 `yaml:"cache_read_per_million"`
	CacheCreatePerMillion float64 `yaml:"cache_create_per_million"`
}

// UnmarshalYAML accepts both pricing.models and direct model entries under
// pricing. The latter keeps the common one-or-two-model override concise while
// the former leaves room for future pricing settings alongside models.
func (p *PricingConfig) UnmarshalYAML(value *yaml.Node) error {
	var grouped struct {
		Models map[string]ModelPricing `yaml:"models"`
	}
	if err := value.Decode(&grouped); err != nil {
		return err
	}
	if grouped.Models != nil {
		p.Models = grouped.Models
		return nil
	}

	var direct map[string]ModelPricing
	if err := value.Decode(&direct); err != nil {
		return err
	}
	p.Models = direct
	return nil
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
			Enabled:    false,
			WebhookURL: "",
		},
		Alerts: AlertsConfig{
			CostThresholdUSD: 0, // 0 means disabled (no threshold)
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
