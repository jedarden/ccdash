package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CodexHookInstaller manages the status-only hooks written to Codex's
// ~/.codex/hooks.json. Token usage is intentionally not collected by hooks;
// CodexSource reads it from rollout JSONL files instead.
type CodexHookInstaller struct {
	homeDir    string
	baseDir    string
	hooksDir   string
	configPath string
}

// NewCodexHookInstaller creates an installer for the current user's Codex
// configuration.
func NewCodexHookInstaller() *CodexHookInstaller {
	homeDir, _ := os.UserHomeDir()
	return newCodexHookInstaller(homeDir)
}

func newCodexHookInstaller(homeDir string) *CodexHookInstaller {
	baseDir := filepath.Join(homeDir, HooksDir)
	return &CodexHookInstaller{
		homeDir:    homeDir,
		baseDir:    baseDir,
		hooksDir:   filepath.Join(baseDir, HooksSubdir, "codex"),
		configPath: filepath.Join(homeDir, ".codex", "hooks.json"),
	}
}

// InstallHooks writes the Codex status scripts and merges their event entries
// into hooks.json without disturbing hooks installed by other tools.
func (h *CodexHookInstaller) InstallHooks() error {
	if h.homeDir == "" {
		return fmt.Errorf("home directory is unavailable")
	}
	if err := os.MkdirAll(h.hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create Codex hook directory: %w", err)
	}
	for name, content := range codexHookScripts {
		if err := os.WriteFile(filepath.Join(h.hooksDir, name), []byte(content), 0755); err != nil {
			return fmt.Errorf("failed to write Codex hook script %s: %w", name, err)
		}
	}
	return h.updateConfig()
}

// AreHooksInstalled reports whether the ccdash Codex SessionStart hook is
// present in hooks.json.
func (h *CodexHookInstaller) AreHooksInstalled() bool {
	return h.hasHooks()
}

// UninstallHooks removes only ccdash's Codex entries from hooks.json.
func (h *CodexHookInstaller) UninstallHooks() error {
	settings, err := h.readConfig()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}
	modified := false
	for event, raw := range hooks {
		entries, ok := raw.([]interface{})
		if !ok {
			continue
		}
		filtered := entries[:0]
		for _, entry := range entries {
			if h.entryBelongsToCcdash(entry) {
				modified = true
				continue
			}
			filtered = append(filtered, entry)
		}
		if len(filtered) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = filtered
		}
	}
	if !modified {
		return nil
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	return h.writeConfig(settings)
}

func (h *CodexHookInstaller) updateConfig() error {
	settings, err := h.readConfig()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
	}

	for event, script := range map[string]string{
		"SessionStart":      "codex-session-start.sh",
		"SessionEnd":        "codex-session-end.sh",
		"UserPromptSubmit":  "codex-prompt-submit.sh",
		"PreToolUse":        "codex-pre-tool-use.sh",
		"PostToolUse":       "codex-post-tool-use.sh",
		"PermissionRequest": "codex-permission-request.sh",
		"Stop":              "codex-stop.sh",
	} {
		if h.eventHasCcdashHook(hooks[event]) {
			continue
		}
		entry := map[string]interface{}{
			"hooks": []interface{}{map[string]interface{}{
				"type":    "command",
				"command": filepath.Join(h.hooksDir, script),
			}},
		}
		if existing, ok := hooks[event].([]interface{}); ok {
			hooks[event] = append(existing, entry)
		} else {
			hooks[event] = []interface{}{entry}
		}
	}
	settings["hooks"] = hooks
	return h.writeConfig(settings)
}

func (h *CodexHookInstaller) readConfig() (map[string]interface{}, error) {
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		return nil, err
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("invalid Codex hooks.json: %w", err)
	}
	return settings, nil
}

func (h *CodexHookInstaller) writeConfig(settings map[string]interface{}) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(h.configPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(h.configPath, data, 0600)
}

func (h *CodexHookInstaller) hasHooks() bool {
	settings, err := h.readConfig()
	if err != nil {
		return false
	}
	hooks, ok := settings["hooks"].(map[string]interface{})
	return ok && h.eventHasCcdashHook(hooks["SessionStart"])
}

func (h *CodexHookInstaller) eventHasCcdashHook(raw interface{}) bool {
	entries, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, entry := range entries {
		if h.entryBelongsToCcdash(entry) {
			return true
		}
	}
	return false
}

func (h *CodexHookInstaller) entryBelongsToCcdash(raw interface{}) bool {
	entry, ok := raw.(map[string]interface{})
	if !ok {
		return false
	}
	hooks, ok := entry["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, rawHook := range hooks {
		hook, ok := rawHook.(map[string]interface{})
		if !ok {
			continue
		}
		command, _ := hook["command"].(string)
		if filepath.Dir(command) == h.hooksDir {
			return true
		}
	}
	return false
}

const codexHookCommon = `#!/usr/bin/env bash
set -e

CCDASH_DIR="$HOME/.ccdash"
SESSIONS_DIR="$CCDASH_DIR/sessions"
INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')

if [ -z "$SESSION_ID" ]; then
    exit 0
fi

SESSION_FILE="$SESSIONS_DIR/${SESSION_ID}.json"
`

// Codex hooks intentionally mirror the Claude status transitions. They never
// inspect usage fields; token counts come from Codex rollout transcripts.
var codexHookScripts = map[string]string{
	"codex-session-start.sh": codexHookCommon + `
CWD=$(echo "$INPUT" | jq -r '.cwd // empty')
TMUX_SESSION=""
if [ -n "$TMUX" ]; then
    TMUX_SESSION=$(tmux display-message -p '#S' 2>/dev/null || echo "")
fi
CODEX_PID="$PPID"
CURRENT_PID=$PPID
while [ -n "$CURRENT_PID" ] && [ "$CURRENT_PID" != "1" ]; do
    PROC_NAME=$(ps -p "$CURRENT_PID" -o comm= 2>/dev/null || echo "")
    if [ "$PROC_NAME" = "codex" ]; then
        CODEX_PID="$CURRENT_PID"
        break
    fi
    CURRENT_PID=$(ps -p "$CURRENT_PID" -o ppid= 2>/dev/null | tr -d ' ' || echo "")
done
mkdir -p "$SESSIONS_DIR"
jq -n --arg id "$SESSION_ID" --arg source "codex" --arg cwd "$CWD" \
    --arg tmux "$TMUX_SESSION" --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson pid "$CODEX_PID" \
    '{session_id:$id, source:$source, project_dir:$cwd, tmux_session_name:$tmux, started_at:$now, last_activity:$now, pid:$pid, status:"active"}' \
    > "$SESSIONS_DIR/${SESSION_ID}.json"
`,
	"codex-session-end.sh": codexHookCommon + `
rm -f "$SESSION_FILE"
`,
	"codex-prompt-submit.sh": codexHookCommon + `
if [ -f "$SESSION_FILE" ]; then
    TMP_FILE=$(mktemp)
    jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_activity=$now | .status="working"' "$SESSION_FILE" > "$TMP_FILE" && mv "$TMP_FILE" "$SESSION_FILE"
fi
`,
	"codex-pre-tool-use.sh": codexHookCommon + `
if [ -f "$SESSION_FILE" ]; then
    TMP_FILE=$(mktemp)
    jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_activity=$now' "$SESSION_FILE" > "$TMP_FILE" && mv "$TMP_FILE" "$SESSION_FILE"
fi
`,
	"codex-post-tool-use.sh": codexHookCommon + `
if [ -f "$SESSION_FILE" ]; then
    TMP_FILE=$(mktemp)
    jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_activity=$now | .status="working"' "$SESSION_FILE" > "$TMP_FILE" && mv "$TMP_FILE" "$SESSION_FILE"
fi
`,
	"codex-permission-request.sh": codexHookCommon + `
if [ -f "$SESSION_FILE" ]; then
    TMP_FILE=$(mktemp)
    jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_activity=$now | .status="waiting"' "$SESSION_FILE" > "$TMP_FILE" && mv "$TMP_FILE" "$SESSION_FILE"
fi
`,
	"codex-stop.sh": codexHookCommon + `
if [ -f "$SESSION_FILE" ]; then
    TMP_FILE=$(mktemp)
    jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_activity=$now | .last_stop=$now | .status="stopped"' "$SESSION_FILE" > "$TMP_FILE" && mv "$TMP_FILE" "$SESSION_FILE"
fi
`,
}
