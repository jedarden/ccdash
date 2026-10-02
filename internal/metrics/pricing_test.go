package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetPricingForModel(t *testing.T) {
	cases := []struct {
		model string
		want  ModelPricing
	}{
		// Live model IDs pulled from a real ~/.ccdash/tokens.db on 2026-08-28.
		// Before this fix these all silently fell through to defaultPricing
		// (GLM-4.5's rate) because getPricingForModel only recognized the
		// -4-5 generation.
		{"claude-opus-4-7", claudeOpusPricing},
		{"claude-opus-4-8", claudeOpusPricing},
		{"claude-sonnet-4-6", claudeSonnet45Pricing},
		{"claude-sonnet-5", modelPricing["claude-sonnet-5"]},
		{"claude-fable-5", modelPricing["claude-fable-5"]},
		{"claude-haiku-4-5-20251001", modelPricing["claude-haiku-4-5-20250929"]},
		{"glm-4.7", modelPricing["glm-4.7"]},
		{"glm-5.1", modelPricing["glm-5.1"]},
		{"glm-5-turbo", modelPricing["glm-5-turbo"]},

		// glm-5.1 must not shadow-match the bare glm-5 catch-all (the actual
		// bug: contains(model, "glm-5") matches "glm-5.1" too, so ordering
		// matters here).
		{"glm-5", modelPricing["glm-5"]},

		// Untouched pre-existing behavior.
		{"claude-opus-4-5-20251101", claudeOpusPricing},
		{"claude-sonnet-4-5-20250929", claudeSonnet45Pricing},
		{"glm-4-air", modelPricing["glm-4-air"]},
		{"totally-unknown-model", defaultPricing},
	}

	for _, c := range cases {
		if got := getPricingForModel(c.model); got != c.want {
			t.Errorf("getPricingForModel(%q) = %+v, want %+v", c.model, got, c.want)
		}
	}
}

func TestGLM51PricingDiffersFromGLM5(t *testing.T) {
	// The exact regression this fix targets: before it, "glm-5.1" matched
	// the generic contains(model, "glm-5") check and silently got GLM-5's
	// price instead of its own higher rate.
	glm5 := getPricingForModel("glm-5")
	glm51 := getPricingForModel("glm-5.1")
	if glm5 == glm51 {
		t.Fatalf("glm-5 and glm-5.1 must not price identically, got %+v for both", glm5)
	}
	if glm51.InputPerMillion != 1.4 || glm51.OutputPerMillion != 4.4 {
		t.Errorf("glm-5.1 pricing = %+v, want input 1.4 / output 4.4 per docs.z.ai", glm51)
	}
}

func TestPricingDetailsMarkFallbacks(t *testing.T) {
	exact := getPricingDetailsForModel("claude-sonnet-5")
	if exact.estimated {
		t.Fatal("exact model pricing must not be marked as estimated")
	}

	fallback := getPricingDetailsForModel("claude-sonnet-next")
	if !fallback.estimated {
		t.Fatal("substring fallback pricing must be marked as estimated")
	}
}

func TestPricingOverrideTakesPrecedenceAndIsNotEstimated(t *testing.T) {
	collector := &TokenCollector{
		pricingOverrides: map[string]ModelPricing{
			"claude-sonnet-next": {
				InputPerMillion:       9,
				OutputPerMillion:      19,
				CacheReadPerMillion:   0.9,
				CacheCreatePerMillion: 11.25,
			},
		},
	}

	pricing, estimated := collector.pricingForModel("claude", "claude-sonnet-next")
	if estimated {
		t.Fatal("configured pricing override must not be marked as estimated")
	}
	if pricing.InputPerMillion != 9 || pricing.OutputPerMillion != 19 {
		t.Fatalf("configured pricing = %+v, want input 9 / output 19", pricing)
	}
}

func TestConfiguredPricingIsLoadedByTokenCollector(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ccdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccdash", "config.yaml"), []byte(`
pricing:
  models:
    provider/future-model:
      input_per_million: 7
      output_per_million: 21
      cache_read_per_million: 0.7
      cache_create_per_million: 8.75
`), 0o600); err != nil {
		t.Fatal(err)
	}

	collector := &TokenCollector{pricingOverrides: loadPricingOverrides()}
	pricing, estimated := collector.pricingForModel("opencode", "provider/future-model")
	if estimated {
		t.Fatal("configured pricing must not be estimated")
	}
	if pricing.InputPerMillion != 7 || pricing.OutputPerMillion != 21 || pricing.CacheReadPerMillion != 0.7 || pricing.CacheCreatePerMillion != 8.75 {
		t.Fatalf("loaded pricing = %+v, want configured rates", pricing)
	}
}

func TestEstimatedPricingIsFlaggedInJSON(t *testing.T) {
	raw, err := json.Marshal(ModelUsage{Model: "claude-sonnet-next", PricingEstimated: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"pricing_estimated":true`) {
		t.Fatalf("JSON = %s, want pricing_estimated=true", raw)
	}
}
