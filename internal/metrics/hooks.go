package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// HooksDir is the directory name for hook-generated data
	HooksDir = ".ccdash"
	// SessionsSubdir is the subdirectory for session files
	SessionsSubdir = "sessions"
	// HooksSubdir is the subdirectory for hook scripts
	HooksSubdir = "hooks"
	// StaleSessionThreshold is how long before a session is considered stale
	StaleSessionThreshold = 5 * time.Minute
)

// HookSession represents a Claude Code session tracked via hooks
type HookSession struct {
	SessionID       string    `json:"session_id"`
	ProjectDir      string    `json:"project_dir"`
	TmuxSessionName string    `json:"tmux_session_name,omitempty"` // Name of the tmux session
	StartedAt       time.Time `json:"started_at"`
	LastActivity    time.Time `json:"last_activity"`
	LastStop        time.Time `json:"last_stop,omitempty"`
	PID             int       `json:"pid,omitempty"`
	Status          string    `json:"status"` // "active", "stopped", "working"
}

// HookSessionCollector reads session data from hook-generated files
type HookSessionCollector struct {
	baseDir     string // ~/.ccdash
	sessionsDir string // ~/.ccdash/sessions
	available   bool
}

// NewHookSessionCollector creates a new hook session collector
func NewHookSessionCollector() (*HookSessionCollector, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, HooksDir)
	sessionsDir := filepath.Join(baseDir, SessionsSubdir)

	// Check if the hooks directory exists
	available := false
	if _, err := os.Stat(sessionsDir); err == nil {
		available = true
	}

	return &HookSessionCollector{
		baseDir:     baseDir,
		sessionsDir: sessionsDir,
		available:   available,
	}, nil
}

// IsAvailable returns true if hook-based session tracking is set up
func (h *HookSessionCollector) IsAvailable() bool {
	return h.available
}

// GetBaseDir returns the base directory for hook data
func (h *HookSessionCollector) GetBaseDir() string {
	return h.baseDir
}

// EnsureDirectories creates the necessary directories for hooks
func (h *HookSessionCollector) EnsureDirectories() error {
	dirs := []string{
		h.baseDir,
		h.sessionsDir,
		filepath.Join(h.baseDir, HooksSubdir),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	h.available = true
	return nil
}

// CollectSessions reads all active sessions from hook-generated files
func (h *HookSessionCollector) CollectSessions() ([]HookSession, error) {
	if !h.available {
		return nil, nil
	}

	entries, err := os.ReadDir(h.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []HookSession
	now := time.Now()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		sessionPath := filepath.Join(h.sessionsDir, entry.Name())
		session, err := h.readSessionFile(sessionPath)
		if err != nil {
			// Log error but continue with other sessions
			continue
		}

		// Check if session is stale (no activity for StaleSessionThreshold)
		if now.Sub(session.LastActivity) > StaleSessionThreshold {
			session.Status = "stale"
		}

		sessions = append(sessions, *session)
	}

	// Sort by last activity (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	return sessions, nil
}

// readSessionFile reads and parses a session JSON file
func (h *HookSessionCollector) readSessionFile(path string) (*HookSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var session HookSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// CleanupStaleSessions removes session files that are stale
func (h *HookSessionCollector) CleanupStaleSessions(threshold time.Duration) (int, error) {
	if !h.available {
		return 0, nil
	}

	entries, err := os.ReadDir(h.sessionsDir)
	if err != nil {
		return 0, err
	}

	cleaned := 0
	now := time.Now()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		sessionPath := filepath.Join(h.sessionsDir, entry.Name())
		session, err := h.readSessionFile(sessionPath)
		if err != nil {
			// Remove unreadable files
			os.Remove(sessionPath)
			cleaned++
			continue
		}

		if now.Sub(session.LastActivity) > threshold {
			os.Remove(sessionPath)
			cleaned++
		}
	}

	return cleaned, nil
}

// ToTmuxSession converts a HookSession to TmuxSession for UI compatibility
func (hs *HookSession) ToTmuxSession() TmuxSession {
	status := StatusActive
	switch hs.Status {
	case "working":
		status = StatusWorking
	case "stopped", "ready":
		status = StatusReady
	case "stale":
		status = StatusError
	}

	// Use tmux session name if available, otherwise fall back to project dir basename
	name := hs.TmuxSessionName
	if name == "" {
		name = filepath.Base(hs.ProjectDir)
	}
	if name == "" || name == "." {
		name = hs.SessionID[:8] // Use truncated session ID
	}

	return TmuxSession{
		Name:         name,
		Windows:      1,
		Attached:     hs.Status == "working" || hs.Status == "active",
		Created:      hs.StartedAt,
		Status:       status,
		IdleDuration: time.Since(hs.LastActivity),
		LastLines:    []string{fmt.Sprintf("Session: %s", hs.SessionID[:8])},
		Source:       "hooks", // Mark as hook-sourced
	}
}

// HookScripts contains the shell scripts to be installed as Claude Code hooks
var HookScripts = map[string]string{
	"session-start.sh": `#!/bin/bash
# Claude Code SessionStart hook - registers session with ccdash
set -e

CCDASH_DIR="$HOME/.ccdash"
SESSIONS_DIR="$CCDASH_DIR/sessions"

# Read hook input from stdin
INPUT=$(cat)

# Extract session info
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')
CWD=$(echo "$INPUT" | jq -r '.cwd // empty')

if [ -z "$SESSION_ID" ]; then
    exit 0
fi

# Get tmux session name if running inside tmux
TMUX_SESSION=""
if [ -n "$TMUX" ]; then
    TMUX_SESSION=$(tmux display-message -p '#S' 2>/dev/null || echo "")
fi

# Ensure directories exist
mkdir -p "$SESSIONS_DIR"

# Write session file
cat > "$SESSIONS_DIR/${SESSION_ID}.json" << EOF
{
  "session_id": "$SESSION_ID",
  "project_dir": "$CWD",
  "tmux_session_name": "$TMUX_SESSION",
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "last_activity": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "pid": $$,
  "status": "active"
}
EOF

exit 0
`,

	"session-end.sh": `#!/bin/bash
# Claude Code SessionEnd hook - unregisters session from ccdash
set -e

CCDASH_DIR="$HOME/.ccdash"
SESSIONS_DIR="$CCDASH_DIR/sessions"

# Read hook input from stdin
INPUT=$(cat)

# Extract session ID
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')

if [ -z "$SESSION_ID" ]; then
    exit 0
fi

# Remove session file
rm -f "$SESSIONS_DIR/${SESSION_ID}.json"

exit 0
`,

	"stop.sh": `#!/bin/bash
# Claude Code Stop hook - marks session as stopped (waiting for input)
set -e

CCDASH_DIR="$HOME/.ccdash"
SESSIONS_DIR="$CCDASH_DIR/sessions"

# Read hook input from stdin
INPUT=$(cat)

# Extract session info
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')

if [ -z "$SESSION_ID" ]; then
    exit 0
fi

SESSION_FILE="$SESSIONS_DIR/${SESSION_ID}.json"

# Update status to stopped (waiting for input)
if [ -f "$SESSION_FILE" ]; then
    TMP_FILE=$(mktemp)
    jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
       '.last_activity = $now | .last_stop = $now | .status = "stopped"' \
       "$SESSION_FILE" > "$TMP_FILE" && mv "$TMP_FILE" "$SESSION_FILE"
fi

exit 0
`,

	"prompt-submit.sh": `#!/bin/bash
# Claude Code UserPromptSubmit hook - marks session as working
set -e

CCDASH_DIR="$HOME/.ccdash"
SESSIONS_DIR="$CCDASH_DIR/sessions"

# Read hook input from stdin
INPUT=$(cat)

# Extract session info
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')

if [ -z "$SESSION_ID" ]; then
    exit 0
fi

SESSION_FILE="$SESSIONS_DIR/${SESSION_ID}.json"

# Update status to working
if [ -f "$SESSION_FILE" ]; then
    TMP_FILE=$(mktemp)
    jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
       '.last_activity = $now | .status = "working"' \
       "$SESSION_FILE" > "$TMP_FILE" && mv "$TMP_FILE" "$SESSION_FILE"
fi

exit 0
`,
}

// ClaudeHooksConfig represents the hooks section of Claude settings
type ClaudeHooksConfig struct {
	SessionStart     []HookEntry `json:"SessionStart,omitempty"`
	SessionEnd       []HookEntry `json:"SessionEnd,omitempty"`
	Stop             []HookEntry `json:"Stop,omitempty"`
	UserPromptSubmit []HookEntry `json:"UserPromptSubmit,omitempty"`
}

// HookEntry represents a single hook configuration
type HookEntry struct {
	Matcher string       `json:"matcher,omitempty"`
	Hooks   []HookConfig `json:"hooks"`
}

// HookConfig represents a hook command configuration
type HookConfig struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// InstallHooks installs the ccdash hooks into Claude Code settings
func (h *HookSessionCollector) InstallHooks() error {
	if err := h.EnsureDirectories(); err != nil {
		return err
	}

	// Write hook scripts
	hooksDir := filepath.Join(h.baseDir, HooksSubdir)
	for name, content := range HookScripts {
		scriptPath := filepath.Join(hooksDir, name)
		if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
			return fmt.Errorf("failed to write hook script %s: %w", name, err)
		}
	}

	// Update Claude settings
	return h.updateClaudeSettings()
}

// updateClaudeSettings adds ccdash hooks to ~/.claude/settings.json
func (h *HookSessionCollector) updateClaudeSettings() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	hooksDir := filepath.Join(h.baseDir, HooksSubdir)

	// Read existing settings
	var settings map[string]interface{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			settings = make(map[string]interface{})
		}
	} else {
		settings = make(map[string]interface{})
	}

	// Get or create hooks section
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
	}

	// Define ccdash hooks
	ccdashHooks := map[string][]map[string]interface{}{
		"SessionStart": {
			{
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": filepath.Join(hooksDir, "session-start.sh"),
					},
				},
			},
		},
		"SessionEnd": {
			{
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": filepath.Join(hooksDir, "session-end.sh"),
					},
				},
			},
		},
		"UserPromptSubmit": {
			{
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": filepath.Join(hooksDir, "prompt-submit.sh"),
					},
				},
			},
		},
		"Stop": {
			{
				"hooks": []map[string]interface{}{
					{
						"type":    "command",
						"command": filepath.Join(hooksDir, "stop.sh"),
					},
				},
			},
		},
	}

	// Merge hooks (append ccdash hooks to existing)
	for event, hookList := range ccdashHooks {
		existing, _ := hooks[event].([]interface{})

		// Check if ccdash hook already exists
		alreadyInstalled := false
		for _, h := range existing {
			if hMap, ok := h.(map[string]interface{}); ok {
				if hList, ok := hMap["hooks"].([]interface{}); ok {
					for _, hook := range hList {
						if hookConfig, ok := hook.(map[string]interface{}); ok {
							if cmd, ok := hookConfig["command"].(string); ok {
								if filepath.Dir(cmd) == hooksDir {
									alreadyInstalled = true
									break
								}
							}
						}
					}
				}
			}
		}

		if !alreadyInstalled {
			for _, newHook := range hookList {
				existing = append(existing, newHook)
			}
			hooks[event] = existing
		}
	}

	settings["hooks"] = hooks

	// Write updated settings
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// Ensure .claude directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

// AreHooksInstalled checks if ccdash hooks are already installed
func (h *HookSessionCollector) AreHooksInstalled() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	hooksDir := filepath.Join(h.baseDir, HooksSubdir)

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return false
	}

	// Check for SessionStart hook pointing to our scripts
	sessionStart, ok := hooks["SessionStart"].([]interface{})
	if !ok || len(sessionStart) == 0 {
		return false
	}

	for _, h := range sessionStart {
		if hMap, ok := h.(map[string]interface{}); ok {
			if hList, ok := hMap["hooks"].([]interface{}); ok {
				for _, hook := range hList {
					if hookConfig, ok := hook.(map[string]interface{}); ok {
						if cmd, ok := hookConfig["command"].(string); ok {
							if filepath.Dir(cmd) == hooksDir {
								return true
							}
						}
					}
				}
			}
		}
	}

	return false
}
