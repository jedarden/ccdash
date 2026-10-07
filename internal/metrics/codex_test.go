package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexSourceParseUsageLine(t *testing.T) {
	source := NewCodexSource()
	source.Reset()

	if _, ok, err := source.ParseUsageLine([]byte(`{"type":"turn_context","payload":{"model":"gpt-5.6-luna"}}`)); err != nil || ok {
		t.Fatalf("turn_context: ok=%v err=%v", ok, err)
	}

	raw := []byte(`{"type":"event_msg","timestamp":"2026-08-13T20:25:48.113Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":36839,"cached_input_tokens":28416,"cache_write_input_tokens":12,"output_tokens":526,"reasoning_output_tokens":192,"total_tokens":37365},"total_token_usage":{"input_tokens":105385,"cached_input_tokens":76800,"cache_write_input_tokens":12,"output_tokens":1719,"reasoning_output_tokens":465,"total_tokens":107104}}}}`)
	event, ok, err := source.ParseUsageLine(raw)
	if err != nil || !ok {
		t.Fatalf("token_count: ok=%v err=%v", ok, err)
	}
	if event.Model != "gpt-5.6-luna" || event.InputTokens != 8411 || event.CacheReadTokens != 28416 || event.CacheCreationTokens != 12 || event.OutputTokens != 526 {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.Source != "codex" {
		t.Fatalf("expected codex source, got %q", event.Source)
	}
}

func TestCodexSourceIgnoresRecordsWithoutModelOrUsage(t *testing.T) {
	source := NewCodexSource()
	if event, ok, err := source.ParseUsageLine([]byte(`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10}}}}`)); err != nil || ok || event != nil {
		t.Fatalf("expected token count without model to be ignored: event=%+v ok=%v err=%v", event, ok, err)
	}
	if event, ok, err := source.ParseUsageLine([]byte(`{"type":"event_msg","payload":{"type":"message","message":"not usage"}}`)); err != nil || ok || event != nil {
		t.Fatalf("expected non-token event to be ignored: event=%+v ok=%v err=%v", event, ok, err)
	}
}

func TestCodexPricing(t *testing.T) {
	source := NewCodexSource()
	// OpenAI standard-tier, short-context rates (see codexPricing).
	published := map[string]ModelPricing{
		"gpt-6-astra":   {InputPerMillion: 10, OutputPerMillion: 50, CacheReadPerMillion: 1, CacheCreatePerMillion: 12.5},
		"gpt-6.1-sol":   {InputPerMillion: 2, OutputPerMillion: 10, CacheReadPerMillion: 0.1, CacheCreatePerMillion: 2.5},
		"gpt-6-sol":     {InputPerMillion: 2, OutputPerMillion: 10, CacheReadPerMillion: 0.2, CacheCreatePerMillion: 2.5},
		"gpt-6-luna":    {InputPerMillion: 0.1, OutputPerMillion: 0.5, CacheReadPerMillion: 0.01, CacheCreatePerMillion: 0.125},
		"gpt-5.6-sol":   {InputPerMillion: 4, OutputPerMillion: 20, CacheReadPerMillion: 0.4, CacheCreatePerMillion: 5},
		"gpt-5.6-terra": {InputPerMillion: 2, OutputPerMillion: 12, CacheReadPerMillion: 0.2, CacheCreatePerMillion: 2.5},
		"gpt-5.6-luna":  {InputPerMillion: 0.2, OutputPerMillion: 1.2, CacheReadPerMillion: 0.02, CacheCreatePerMillion: 0.25},
	}
	for model, want := range published {
		details := source.pricingDetailsForModel(model)
		if details.estimated || details.pricing != want {
			t.Errorf("%s pricing = %+v (estimated=%v), want %+v", model, details.pricing, details.estimated, want)
		}
	}
	if unknown := source.PricingForModel("provider-private-model"); unknown != (ModelPricing{}) {
		t.Fatalf("unknown Codex models should not use Claude pricing: %+v", unknown)
	}
}

func TestCodexHookInstallerLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	installer := newCodexHookInstaller(tmpDir)
	if err := installer.InstallHooks(); err != nil {
		t.Fatal(err)
	}
	if !installer.AreHooksInstalled() {
		t.Fatal("expected Codex hooks to be installed")
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	hooks := config["hooks"].(map[string]interface{})
	for _, event := range []string{"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PermissionRequest", "Stop"} {
		if _, ok := hooks[event]; !ok {
			t.Errorf("missing Codex hook event %s", event)
		}
	}
	if err := installer.UninstallHooks(); err != nil {
		t.Fatal(err)
	}
	if installer.AreHooksInstalled() {
		t.Fatal("expected Codex hooks to be removed")
	}
}

func TestCodexLongContextRequestsAreTaggedByPromptSize(t *testing.T) {
	source := NewCodexSource()
	source.Reset()
	if _, _, err := source.ParseUsageLine([]byte(`{"type":"turn_context","payload":{"model":"gpt-6-sol"}}`)); err != nil {
		t.Fatal(err)
	}

	line := func(input, cached int64) []byte {
		return []byte(fmt.Sprintf(`{"type":"event_msg","timestamp":"2026-10-07T12:00:00Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":%d,"cached_input_tokens":%d,"output_tokens":100,"total_tokens":%d}}}}`, input, cached, input+100))
	}
	cases := []struct {
		input, cached int64
		wantModel     string
	}{
		{input: 272_000, cached: 250_000, wantModel: "gpt-6-sol"},
		// The threshold counts the whole prompt, cached input included.
		{input: 272_001, cached: 270_000, wantModel: "gpt-6-sol:long-context"},
	}
	for _, tc := range cases {
		event, ok, err := source.ParseUsageLine(line(tc.input, tc.cached))
		if err != nil || !ok {
			t.Fatalf("input %d: ok=%v err=%v", tc.input, ok, err)
		}
		if event.Model != tc.wantModel {
			t.Errorf("input %d: model = %q, want %q", tc.input, event.Model, tc.wantModel)
		}
	}
}

func TestCodexLongContextPricing(t *testing.T) {
	source := NewCodexSource()
	published := map[string]ModelPricing{
		"gpt-6-astra":   {InputPerMillion: 20, OutputPerMillion: 75, CacheReadPerMillion: 2, CacheCreatePerMillion: 25},
		"gpt-6.1-sol":   {InputPerMillion: 4, OutputPerMillion: 15, CacheReadPerMillion: 0.2, CacheCreatePerMillion: 5},
		"gpt-6-sol":     {InputPerMillion: 4, OutputPerMillion: 15, CacheReadPerMillion: 0.4, CacheCreatePerMillion: 5},
		"gpt-6-luna":    {InputPerMillion: 0.2, OutputPerMillion: 0.75, CacheReadPerMillion: 0.02, CacheCreatePerMillion: 0.25},
		"gpt-5.6-sol":   {InputPerMillion: 8, OutputPerMillion: 30, CacheReadPerMillion: 0.8, CacheCreatePerMillion: 10},
		"gpt-5.6-terra": {InputPerMillion: 4, OutputPerMillion: 18, CacheReadPerMillion: 0.4, CacheCreatePerMillion: 5},
		"gpt-5.6-luna":  {InputPerMillion: 0.4, OutputPerMillion: 1.8, CacheReadPerMillion: 0.04, CacheCreatePerMillion: 0.5},
	}
	for model, want := range published {
		details := source.pricingDetailsForModel(model + CodexLongContextSuffix)
		if details.estimated || details.pricing != want {
			t.Errorf("%s long-context pricing = %+v (estimated=%v), want %+v", model, details.pricing, details.estimated, want)
		}
	}

	// A model with no published long-context rate falls back to its
	// short-context price, marked estimated because that understates.
	fallback := source.pricingDetailsForModel("gpt-5-codex" + CodexLongContextSuffix)
	if !fallback.estimated || fallback.pricing != codexPricing["gpt-5-codex"] {
		t.Errorf("gpt-5-codex long-context fallback = %+v (estimated=%v)", fallback.pricing, fallback.estimated)
	}
}
