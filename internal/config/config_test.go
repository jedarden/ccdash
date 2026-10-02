package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPricingOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ccdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	configYAML := []byte(`
pricing:
  models:
    future-model:
      input_per_million: 1.25
      output_per_million: 6.5
      cache_read_per_million: 0.125
      cache_create_per_million: 1.5625
`)
	if err := os.WriteFile(filepath.Join(home, ".ccdash", "config.yaml"), configYAML, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got, ok := cfg.Pricing.Models["future-model"]
	if !ok {
		t.Fatal("pricing.models.future-model was not loaded")
	}
	want := ModelPricing{
		InputPerMillion:       1.25,
		OutputPerMillion:      6.5,
		CacheReadPerMillion:   0.125,
		CacheCreatePerMillion: 1.5625,
	}
	if got != want {
		t.Fatalf("pricing override = %+v, want %+v", got, want)
	}
}

func TestLoadPricingOverridesAcceptsDirectModelEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ccdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	configYAML := []byte(`
pricing:
  compact-model:
    input_per_million: 2
    output_per_million: 10
    cache_read_per_million: 0.2
    cache_create_per_million: 2.5
`)
	if err := os.WriteFile(filepath.Join(home, ".ccdash", "config.yaml"), configYAML, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, ok := cfg.Pricing.Models["compact-model"]; !ok {
		t.Fatal("direct pricing model entry was not loaded")
	}
}
