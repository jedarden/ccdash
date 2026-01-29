package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// HooksDir is the directory name for hook-generated data
	HooksDir = ".ccdash"
	// SessionsSubdir is the subdirectory for session files
	SessionsSubdir = "sessions"
	// HooksSubdir is the subdirectory for hook scripts
	HooksSubdir = "hooks"
	// InstancesSubdir is the subdirectory for instance PID files
	InstancesSubdir = "instances"
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

// CleanupOrphanedSessions removes session files where the process is dead
// or the tmux session no longer exists. This handles cases where sessions
// were terminated abruptly without the session-end hook firing.
func (h *HookSessionCollector) CleanupOrphanedSessions() (int, error) {
	if !h.available {
		return 0, nil
	}

	entries, err := os.ReadDir(h.sessionsDir)
	if err != nil {
		return 0, err
	}

	// Get list of active tmux sessions
	activeTmuxSessions := h.getActiveTmuxSessions()

	cleaned := 0

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

		shouldRemove := false

		// Check if the PID is still running
		if session.PID > 0 && !isProcessRunning(session.PID) {
			shouldRemove = true
		}

		// Check if the tmux session still exists (if one was recorded)
		if session.TmuxSessionName != "" {
			if _, exists := activeTmuxSessions[session.TmuxSessionName]; !exists {
				shouldRemove = true
			}
		}

		if shouldRemove {
			os.Remove(sessionPath)
			cleaned++
		}
	}

	return cleaned, nil
}

// getActiveTmuxSessions returns a map of active tmux session names
func (h *HookSessionCollector) getActiveTmuxSessions() map[string]bool {
	sessions := make(map[string]bool)

	// Run tmux list-sessions to get active sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// tmux not available or no sessions - return empty map
		return sessions
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			sessions[name] = true
		}
	}

	return sessions
}

// ToTmuxSession converts a HookSession to TmuxSession for UI compatibility
func (hs *HookSession) ToTmuxSession() TmuxSession {
	status := StatusActive
	switch hs.Status {
	case "working":
		status = StatusWorking
	case "stopped", "ready", "stale":
		// Stale sessions (idle > 5min) are just waiting for input, not errors
		status = StatusReady
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

// updateClaudeSettings adds ccdash hooks to all ~/.claude/settings*.json files
func (h *HookSessionCollector) updateClaudeSettings() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	claudeDir := filepath.Join(homeDir, ".claude")
	hooksDir := filepath.Join(h.baseDir, HooksSubdir)

	// Find all settings files
	settingsFiles, err := filepath.Glob(filepath.Join(claudeDir, "settings*.json"))
	if err != nil {
		return err
	}

	// Always include settings.json even if it doesn't exist yet
	mainSettings := filepath.Join(claudeDir, "settings.json")
	hasMainSettings := false
	for _, f := range settingsFiles {
		if f == mainSettings {
			hasMainSettings = true
			break
		}
	}
	if !hasMainSettings {
		settingsFiles = append(settingsFiles, mainSettings)
	}

	// Update each settings file
	var lastErr error
	for _, settingsPath := range settingsFiles {
		if err := h.updateSingleSettingsFile(settingsPath, hooksDir); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// updateSingleSettingsFile adds ccdash hooks to a single settings file
func (h *HookSessionCollector) updateSingleSettingsFile(settingsPath, hooksDir string) error {

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

// AreHooksInstalled checks if ccdash hooks are installed in the main settings.json
func (h *HookSessionCollector) AreHooksInstalled() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	return h.areHooksInSettingsFile(settingsPath)
}

// GetSettingsFilesStatus returns status of hooks in all settings files
func (h *HookSessionCollector) GetSettingsFilesStatus() map[string]bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	claudeDir := filepath.Join(homeDir, ".claude")
	settingsFiles, err := filepath.Glob(filepath.Join(claudeDir, "settings*.json"))
	if err != nil {
		return nil
	}

	status := make(map[string]bool)
	for _, f := range settingsFiles {
		status[filepath.Base(f)] = h.areHooksInSettingsFile(f)
	}
	return status
}

// areHooksInSettingsFile checks if ccdash hooks are in a specific settings file
func (h *HookSessionCollector) areHooksInSettingsFile(settingsPath string) bool {
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

	for _, hook := range sessionStart {
		if hMap, ok := hook.(map[string]interface{}); ok {
			if hList, ok := hMap["hooks"].([]interface{}); ok {
				for _, h := range hList {
					if hookConfig, ok := h.(map[string]interface{}); ok {
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

// RegisterInstance creates a PID file to track this ccdash instance
func (h *HookSessionCollector) RegisterInstance() error {
	instancesDir := filepath.Join(h.baseDir, InstancesSubdir)
	if err := os.MkdirAll(instancesDir, 0755); err != nil {
		return err
	}

	pid := os.Getpid()
	pidFile := filepath.Join(instancesDir, fmt.Sprintf("%d.pid", pid))
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

// UnregisterInstance removes this instance's PID file
func (h *HookSessionCollector) UnregisterInstance() {
	pid := os.Getpid()
	pidFile := filepath.Join(h.baseDir, InstancesSubdir, fmt.Sprintf("%d.pid", pid))
	os.Remove(pidFile)
}

// GetActiveInstanceCount returns the number of running ccdash instances
func (h *HookSessionCollector) GetActiveInstanceCount() int {
	instancesDir := filepath.Join(h.baseDir, InstancesSubdir)
	entries, err := os.ReadDir(instancesDir)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}

		// Extract PID from filename
		pidStr := strings.TrimSuffix(entry.Name(), ".pid")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			// Invalid PID file, clean it up
			os.Remove(filepath.Join(instancesDir, entry.Name()))
			continue
		}

		// Check if process is still running
		if isProcessRunning(pid) {
			count++
		} else {
			// Stale PID file, clean it up
			os.Remove(filepath.Join(instancesDir, entry.Name()))
		}
	}

	return count
}

// isProcessRunning checks if a process with the given PID is running
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Use Signal(0) to check if process exists.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// UninstallHooks removes ccdash hooks from Claude Code settings
func (h *HookSessionCollector) UninstallHooks() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	hooksDir := filepath.Join(h.baseDir, HooksSubdir)

	// Read existing settings
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil // No settings file, nothing to uninstall
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return nil // No hooks section, nothing to uninstall
	}

	// Remove ccdash hooks from each event type
	modified := false
	for event, hookList := range hooks {
		existing, ok := hookList.([]interface{})
		if !ok {
			continue
		}

		// Filter out ccdash hooks
		filtered := make([]interface{}, 0, len(existing))
		for _, h := range existing {
			isCcdashHook := false
			if hMap, ok := h.(map[string]interface{}); ok {
				if hList, ok := hMap["hooks"].([]interface{}); ok {
					for _, hook := range hList {
						if hookConfig, ok := hook.(map[string]interface{}); ok {
							if cmd, ok := hookConfig["command"].(string); ok {
								if filepath.Dir(cmd) == hooksDir {
									isCcdashHook = true
									break
								}
							}
						}
					}
				}
			}
			if !isCcdashHook {
				filtered = append(filtered, h)
			} else {
				modified = true
			}
		}

		if len(filtered) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = filtered
		}
	}

	if !modified {
		return nil // No ccdash hooks found
	}

	// Remove hooks section if empty
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}

	// Write updated settings
	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

// Cleanup removes hooks if this is the last instance, otherwise just unregisters
func (h *HookSessionCollector) Cleanup() {
	h.UnregisterInstance()

	// Only uninstall hooks if no other instances are running
	if h.GetActiveInstanceCount() == 0 {
		h.UninstallHooks()
	}
}
