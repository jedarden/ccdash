package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

var claudeHookEvents = []string{
	"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse",
	"PostToolUse", "PermissionRequest", "Stop", "Notification",
}

var codexHookEvents = []string{
	"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse",
	"PostToolUse", "PermissionRequest", "Stop",
}

func TestClaudeHookInstallerLifecycleIsIdempotentAndPreservesOtherHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	collector, err := NewHookSessionCollector()
	if err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainSettings := filepath.Join(claudeDir, "settings.json")
	writeSettings(t, mainSettings, map[string]interface{}{
		"theme": "dark",
		"hooks": map[string]interface{}{
			"SessionStart": []interface{}{claudeHookEntry("/opt/other/session-start.sh")},
			"OtherEvent":   []interface{}{claudeHookEntry("/opt/other/custom.sh")},
		},
	})
	localSettings := filepath.Join(claudeDir, "settings.local.json")
	writeSettings(t, localSettings, map[string]interface{}{
		"hooks": map[string]interface{}{
			"UserPromptSubmit": []interface{}{claudeHookEntry("/opt/other/prompt-submit.sh")},
		},
	})

	if err := collector.InstallHooks(); err != nil {
		t.Fatalf("install hooks: %v", err)
	}
	if !collector.AreHooksInstalled() {
		t.Fatal("installed Claude hooks were not detected")
	}
	status := collector.GetSettingsFilesStatus()
	if !status["settings.json"] || !status["settings.local.json"] {
		t.Fatalf("expected both settings files to report installed hooks, got %#v", status)
	}
	mainInstalled := readSettings(t, mainSettings)
	if mainInstalled["theme"] != "dark" {
		t.Fatalf("unrelated setting was lost: %#v", mainInstalled["theme"])
	}
	assertClaudeEvents(t, mainInstalled, filepath.Join(collector.GetBaseDir(), HooksSubdir), 1)
	assertClaudeUnrelatedHook(t, mainInstalled, "SessionStart", "/opt/other/session-start.sh")
	assertClaudeUnrelatedHook(t, mainInstalled, "OtherEvent", "/opt/other/custom.sh")
	assertClaudeEvents(t, readSettings(t, localSettings), filepath.Join(collector.GetBaseDir(), HooksSubdir), 1)
	assertClaudeUnrelatedHook(t, readSettings(t, localSettings), "UserPromptSubmit", "/opt/other/prompt-submit.sh")

	firstInstall, err := os.ReadFile(mainSettings)
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.InstallHooks(); err != nil {
		t.Fatalf("repeat install: %v", err)
	}
	secondInstall, err := os.ReadFile(mainSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstInstall, secondInstall) {
		t.Fatal("repeated Claude installation changed settings or duplicated hooks")
	}
	assertClaudeEvents(t, readSettings(t, mainSettings), filepath.Join(collector.GetBaseDir(), HooksSubdir), 1)

	if err := collector.UninstallHooks(); err != nil {
		t.Fatalf("uninstall hooks: %v", err)
	}
	if collector.AreHooksInstalled() {
		t.Fatal("Claude hooks are still detected after uninstall")
	}
	for _, path := range []string{mainSettings, localSettings} {
		settings := readSettings(t, path)
		assertNoClaudeHooks(t, settings, filepath.Join(collector.GetBaseDir(), HooksSubdir))
	}
	assertClaudeUnrelatedHook(t, readSettings(t, mainSettings), "SessionStart", "/opt/other/session-start.sh")
	assertClaudeUnrelatedHook(t, readSettings(t, mainSettings), "OtherEvent", "/opt/other/custom.sh")
	assertClaudeUnrelatedHook(t, readSettings(t, localSettings), "UserPromptSubmit", "/opt/other/prompt-submit.sh")

	firstUninstall, err := os.ReadFile(mainSettings)
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.UninstallHooks(); err != nil {
		t.Fatalf("repeat uninstall: %v", err)
	}
	secondUninstall, err := os.ReadFile(mainSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstUninstall, secondUninstall) {
		t.Fatal("repeated Claude uninstall changed settings")
	}
}

func TestClaudeHookInstallerHandlesPartiallyPopulatedSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	collector, err := NewHookSessionCollector()
	if err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeSettings(t, settingsPath, map[string]interface{}{"theme": "light"})

	if err := collector.InstallHooks(); err != nil {
		t.Fatalf("install into settings without a hooks section: %v", err)
	}
	settings := readSettings(t, settingsPath)
	if settings["theme"] != "light" {
		t.Fatalf("existing setting was lost: %#v", settings)
	}
	assertClaudeEvents(t, settings, filepath.Join(collector.GetBaseDir(), HooksSubdir), 1)
}

func TestClaudeHookInstallerRejectsMalformedSettingsWithoutOverwriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	collector, err := NewHookSessionCollector()
	if err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	for _, malformed := range [][]byte{
		[]byte(`{"hooks":`),
		[]byte(`{"hooks":"invalid"}`),
	} {
		if err := os.WriteFile(settingsPath, malformed, 0644); err != nil {
			t.Fatal(err)
		}
		if err := collector.InstallHooks(); err == nil {
			t.Fatalf("expected install to reject malformed settings %q", malformed)
		}
		assertFileBytes(t, settingsPath, malformed)
		if err := collector.UninstallHooks(); err == nil {
			t.Fatalf("expected uninstall to reject malformed settings %q", malformed)
		}
		assertFileBytes(t, settingsPath, malformed)
	}
}

func TestCodexHookInstallerLifecycleIsIdempotentAndPreservesOtherHooks(t *testing.T) {
	home := t.TempDir()
	installer := newCodexHookInstaller(home)
	configPath := installer.configPath
	writeSettings(t, configPath, map[string]interface{}{
		"model": "gpt-5.6-luna",
		"hooks": map[string]interface{}{
			"SessionStart": []interface{}{codexHookEntry("/opt/other/session-start.sh")},
			"Custom":       []interface{}{codexHookEntry("/opt/other/custom.sh")},
		},
	})

	if err := installer.InstallHooks(); err != nil {
		t.Fatalf("install hooks: %v", err)
	}
	if !installer.AreHooksInstalled() {
		t.Fatal("installed Codex hooks were not detected")
	}
	settings := readSettings(t, configPath)
	if settings["model"] != "gpt-5.6-luna" {
		t.Fatalf("unrelated Codex setting was lost: %#v", settings["model"])
	}
	assertCodexEvents(t, settings, installer.hooksDir, 1)
	assertCodexUnrelatedHook(t, settings, "SessionStart", "/opt/other/session-start.sh")
	assertCodexUnrelatedHook(t, settings, "Custom", "/opt/other/custom.sh")

	firstInstall, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallHooks(); err != nil {
		t.Fatalf("repeat install: %v", err)
	}
	secondInstall, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstInstall, secondInstall) {
		t.Fatal("repeated Codex installation changed settings or duplicated hooks")
	}
	assertCodexEvents(t, readSettings(t, configPath), installer.hooksDir, 1)

	if err := installer.UninstallHooks(); err != nil {
		t.Fatalf("uninstall hooks: %v", err)
	}
	if installer.AreHooksInstalled() {
		t.Fatal("Codex hooks are still detected after uninstall")
	}
	settings = readSettings(t, configPath)
	assertNoCodexHooks(t, settings, installer.hooksDir)
	assertCodexUnrelatedHook(t, settings, "SessionStart", "/opt/other/session-start.sh")
	assertCodexUnrelatedHook(t, settings, "Custom", "/opt/other/custom.sh")

	firstUninstall, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.UninstallHooks(); err != nil {
		t.Fatalf("repeat uninstall: %v", err)
	}
	secondUninstall, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstUninstall, secondUninstall) {
		t.Fatal("repeated Codex uninstall changed settings")
	}
}

func TestCodexHookInstallerHandlesPartiallyPopulatedSettings(t *testing.T) {
	installer := newCodexHookInstaller(t.TempDir())
	writeSettings(t, installer.configPath, map[string]interface{}{"model": "gpt-5.6-luna"})

	if err := installer.InstallHooks(); err != nil {
		t.Fatalf("install into hooks.json without a hooks section: %v", err)
	}
	settings := readSettings(t, installer.configPath)
	if settings["model"] != "gpt-5.6-luna" {
		t.Fatalf("existing setting was lost: %#v", settings)
	}
	assertCodexEvents(t, settings, installer.hooksDir, 1)
}

func TestCodexHookInstallerRejectsMalformedSettingsWithoutOverwriting(t *testing.T) {
	installer := newCodexHookInstaller(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(installer.configPath), 0700); err != nil {
		t.Fatal(err)
	}
	for _, malformed := range [][]byte{
		[]byte(`{"hooks":`),
		[]byte(`{"hooks":"invalid"}`),
	} {
		if err := os.WriteFile(installer.configPath, malformed, 0600); err != nil {
			t.Fatal(err)
		}
		if err := installer.InstallHooks(); err == nil {
			t.Fatalf("expected install to reject malformed hooks.json %q", malformed)
		}
		assertFileBytes(t, installer.configPath, malformed)
		if err := installer.UninstallHooks(); err == nil {
			t.Fatalf("expected uninstall to reject malformed hooks.json %q", malformed)
		}
		assertFileBytes(t, installer.configPath, malformed)
	}
}

func claudeHookEntry(command string) map[string]interface{} {
	return map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": command}}}
}

func codexHookEntry(command string) map[string]interface{} {
	return map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": command}}}
}

func writeSettings(t *testing.T, path string, settings map[string]interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func readSettings(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode settings file %s: %v", path, err)
	}
	return settings
}

func assertClaudeEvents(t *testing.T, settings map[string]interface{}, hooksDir string, want int) {
	t.Helper()
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("settings have no hooks object: %#v", settings)
	}
	for _, event := range claudeHookEvents {
		entries, _ := hooks[event].([]interface{})
		if count := countClaudeEntries(entries, hooksDir); count != want {
			t.Errorf("Claude event %s has %d ccdash entries, want %d", event, count, want)
		}
	}
}

func countClaudeEntries(entries []interface{}, hooksDir string) int {
	count := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		hookList, _ := entry["hooks"].([]interface{})
		for _, rawHook := range hookList {
			hook, ok := rawHook.(map[string]interface{})
			if !ok {
				continue
			}
			command, _ := hook["command"].(string)
			if filepath.Dir(command) == hooksDir {
				count++
			}
		}
	}
	return count
}

func assertClaudeUnrelatedHook(t *testing.T, settings map[string]interface{}, event, command string) {
	t.Helper()
	hooks, _ := settings["hooks"].(map[string]interface{})
	entries, _ := hooks[event].([]interface{})
	if count := countClaudeCommand(entries, command); count != 1 {
		t.Errorf("Claude event %s has %d copies of unrelated hook %s, want 1", event, count, command)
	}
}

func countClaudeCommand(entries []interface{}, command string) int {
	count := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		hookList, _ := entry["hooks"].([]interface{})
		for _, rawHook := range hookList {
			hook, ok := rawHook.(map[string]interface{})
			if ok && hook["command"] == command {
				count++
			}
		}
	}
	return count
}

func assertNoClaudeHooks(t *testing.T, settings map[string]interface{}, hooksDir string) {
	t.Helper()
	hooks, _ := settings["hooks"].(map[string]interface{})
	for event, raw := range hooks {
		entries, _ := raw.([]interface{})
		if count := countClaudeEntries(entries, hooksDir); count != 0 {
			t.Errorf("Claude event %s retains %d ccdash hooks after uninstall", event, count)
		}
	}
}

func assertCodexEvents(t *testing.T, settings map[string]interface{}, hooksDir string, want int) {
	t.Helper()
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("settings have no hooks object: %#v", settings)
	}
	for _, event := range codexHookEvents {
		entries, _ := hooks[event].([]interface{})
		if count := countCodexEntries(entries, hooksDir); count != want {
			t.Errorf("Codex event %s has %d ccdash entries, want %d", event, count, want)
		}
	}
}

func countCodexEntries(entries []interface{}, hooksDir string) int {
	count := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		hookList, _ := entry["hooks"].([]interface{})
		for _, rawHook := range hookList {
			hook, ok := rawHook.(map[string]interface{})
			if !ok {
				continue
			}
			command, _ := hook["command"].(string)
			if filepath.Dir(command) == hooksDir {
				count++
			}
		}
	}
	return count
}

func assertCodexUnrelatedHook(t *testing.T, settings map[string]interface{}, event, command string) {
	t.Helper()
	hooks, _ := settings["hooks"].(map[string]interface{})
	entries, _ := hooks[event].([]interface{})
	count := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		hookList, _ := entry["hooks"].([]interface{})
		for _, rawHook := range hookList {
			hook, ok := rawHook.(map[string]interface{})
			if ok && hook["command"] == command {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("Codex event %s has %d copies of unrelated hook %s, want 1", event, count, command)
	}
}

func assertNoCodexHooks(t *testing.T, settings map[string]interface{}, hooksDir string) {
	t.Helper()
	hooks, _ := settings["hooks"].(map[string]interface{})
	for event, raw := range hooks {
		entries, _ := raw.([]interface{})
		if count := countCodexEntries(entries, hooksDir); count != 0 {
			t.Errorf("Codex event %s retains %d ccdash hooks after uninstall", event, count)
		}
	}
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file %s changed after malformed configuration was rejected: got %q, want %q", path, got, want)
	}
}
