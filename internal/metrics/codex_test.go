package metrics

import (
	"encoding/json"
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
