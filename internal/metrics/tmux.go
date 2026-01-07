package metrics

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// Timeout for tmux commands to prevent hanging
	tmuxCommandTimeout = 2 * time.Second
)

// SessionStatus represents the current status of a tmux session
type SessionStatus string

const (
	// StatusWorking indicates Claude Code is currently processing
	StatusWorking SessionStatus = "WORKING"
	// StatusReady indicates Claude Code is waiting for user input
	StatusReady SessionStatus = "READY"
	// StatusActive indicates user is actively in the tmux session
	StatusActive SessionStatus = "ACTIVE"
	// StatusError indicates error state or undefined condition
	StatusError SessionStatus = "ERROR"
)

// GetColor returns the ANSI color code for the status
func (s SessionStatus) GetColor() string {
	switch s {
	case StatusWorking:
		return "\033[32m" // Green
	case StatusReady:
		return "\033[31m" // Red
	case StatusActive:
		return "\033[33m" // Yellow
	case StatusError:
		return "\033[91m" // Bright Red
	default:
		return "\033[0m" // Reset
	}
}

// GetEmoji returns the emoji representation for the status
// All emojis use single codepoints for consistent terminal rendering
func (s SessionStatus) GetEmoji() string {
	switch s {
	case StatusWorking:
		return "🟢" // U+1F7E2 - Green circle
	case StatusReady:
		return "🔴" // U+1F534 - Red circle
	case StatusActive:
		return "🟡" // U+1F7E1 - Yellow circle
	case StatusError:
		return "❌" // U+274C - Cross mark (single codepoint, consistent width)
	default:
		return "❓" // U+2753 - Question mark
	}
}

// TmuxSession represents a single tmux session
type TmuxSession struct {
	Name              string        `json:"name"`
	Windows           int           `json:"windows"`
	Attached          bool          `json:"attached"`
	Status            SessionStatus `json:"status"`
	Created           time.Time     `json:"created"`
	LastContentChange time.Time     `json:"last_content_change"`
	IdleDuration      time.Duration `json:"idle_duration"` // How long content unchanged
	LastLines         []string      `json:"last_lines,omitempty"`
	Source            string        `json:"source,omitempty"` // "tmux" or "hooks"
}

// TmuxMetrics holds information about all tmux sessions
type TmuxMetrics struct {
	Sessions       []TmuxSession `json:"sessions"`
	Total          int           `json:"total"`
	Available      bool          `json:"available"`
	Error          string        `json:"error,omitempty"`
	LastUpdate     time.Time     `json:"last_update"`
	HooksAvailable bool          `json:"hooks_available"` // Whether hook-based tracking is active
	HooksInstalled bool          `json:"hooks_installed"` // Whether hooks are installed
	Source         string        `json:"source"`          // "hooks", "tmux", or "hybrid"
}

// TmuxCollector collects metrics about tmux sessions
type TmuxCollector struct {
	// sessionActivityMap tracks the last activity time for sessions
	sessionActivityMap map[string]time.Time
	// sessionContentCache stores recent pane content for change detection
	sessionContentCache map[string]string
	// hookCollector handles hook-based session tracking
	hookCollector *HookSessionCollector
}

// NewTmuxCollector creates a new TmuxCollector instance
func NewTmuxCollector() *TmuxCollector {
	hookCollector, _ := NewHookSessionCollector()
	return &TmuxCollector{
		sessionActivityMap:  make(map[string]time.Time),
		sessionContentCache: make(map[string]string),
		hookCollector:       hookCollector,
	}
}

// GetHookCollector returns the hook session collector
func (tc *TmuxCollector) GetHookCollector() *HookSessionCollector {
	return tc.hookCollector
}

// Collect gathers current tmux session information using a hybrid approach
// that merges both hook-based and tmux-based session tracking
func (tc *TmuxCollector) Collect() *TmuxMetrics {
	metrics := &TmuxMetrics{
		Sessions:   make([]TmuxSession, 0),
		LastUpdate: time.Now(),
		Source:     "tmux",
	}

	// Check hook availability
	if tc.hookCollector != nil {
		metrics.HooksInstalled = tc.hookCollector.AreHooksInstalled()
		metrics.HooksAvailable = tc.hookCollector.IsAvailable()
	}

	// Collect hook-based sessions (these have accurate status from Claude Code)
	hookSessionMap := make(map[string]TmuxSession) // keyed by project dir basename
	if tc.hookCollector != nil && metrics.HooksAvailable {
		hookSessions, err := tc.hookCollector.CollectSessions()
		if err == nil {
			for _, hs := range hookSessions {
				session := hs.ToTmuxSession()
				session.Source = "hooks"
				hookSessionMap[session.Name] = session
			}
		}
	}

	// Collect tmux-based sessions
	tmuxSessions := make([]TmuxSession, 0)
	if tc.isTmuxAvailable() {
		sessions, err := tc.listSessions()
		if err == nil {
			for i := range sessions {
				sessions[i].Source = "tmux"
			}
			tmuxSessions = sessions
		}
	}

	// Build a map of tmux sessions for quick lookup
	tmuxSessionMap := make(map[string]TmuxSession)
	for _, session := range tmuxSessions {
		tmuxSessionMap[session.Name] = session
	}

	// Merge sessions: prefer hook data when available (more accurate status),
	// but include all tmux sessions to catch those started before hooks were installed
	seenNames := make(map[string]bool)

	// First, add all hook-tracked sessions
	// Enhance hook sessions with tmux data (attached status, working detection)
	for _, session := range hookSessionMap {
		if tmuxSession, exists := tmuxSessionMap[session.Name]; exists {
			// Use actual tmux attached status (hooks don't track this)
			session.Attached = tmuxSession.Attached

			// If hook says session is stale/error, verify with tmux pane content
			if session.Status == StatusError {
				// Use tmux-based status detection (checks pane content for working indicators)
				if tmuxSession.Status == StatusWorking || tmuxSession.Status == StatusActive {
					session.Status = tmuxSession.Status
					session.Source = "hybrid" // Mark as hybrid since we used both sources
				}
			}
		}
		metrics.Sessions = append(metrics.Sessions, session)
		seenNames[session.Name] = true
	}

	// Then, add tmux sessions that aren't already tracked by hooks
	for _, session := range tmuxSessions {
		if !seenNames[session.Name] {
			metrics.Sessions = append(metrics.Sessions, session)
			seenNames[session.Name] = true
		}
	}

	// Determine source label
	hasHooks := len(hookSessionMap) > 0
	hasTmux := len(tmuxSessions) > 0
	switch {
	case hasHooks && hasTmux:
		metrics.Source = "hybrid"
	case hasHooks:
		metrics.Source = "hooks"
	case hasTmux:
		metrics.Source = "tmux"
	}

	metrics.Available = hasTmux || hasHooks
	metrics.Total = len(metrics.Sessions)

	if !metrics.Available && !tc.isTmuxAvailable() {
		metrics.Error = "tmux is not installed or not available in PATH"
	}

	return metrics
}

// isTmuxAvailable checks if tmux is installed and available
func (tc *TmuxCollector) isTmuxAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "which", "tmux")
	err := cmd.Run()
	return err == nil
}

// listSessions executes tmux list-sessions and parses the output
func (tc *TmuxCollector) listSessions() ([]TmuxSession, error) {
	// Execute tmux list-sessions with formatted output
	// Format: session_name:windows:attached:created
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}:#{session_windows}:#{session_attached}:#{session_created}")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("tmux list-sessions timed out")
		}
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("tmux error: %s", stderrStr)
		}
		return nil, err
	}

	output := stdout.String()
	if strings.TrimSpace(output) == "" {
		return []TmuxSession{}, nil
	}

	return tc.parseSessions(output)
}

// parseSessions parses the tmux list-sessions output
func (tc *TmuxCollector) parseSessions(output string) ([]TmuxSession, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	sessions := make([]TmuxSession, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		session, err := tc.parseSessionLine(line)
		if err != nil {
			// Skip invalid lines but continue processing
			continue
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// parseSessionLine parses a single line from tmux list-sessions output
func (tc *TmuxCollector) parseSessionLine(line string) (TmuxSession, error) {
	// Expected format: session_name:windows:attached:created
	parts := strings.Split(line, ":")
	if len(parts) < 4 {
		return TmuxSession{}, fmt.Errorf("invalid session line format: %s", line)
	}

	session := TmuxSession{
		Name: parts[0],
	}

	// Parse windows count
	windows, err := strconv.Atoi(parts[1])
	if err != nil {
		return TmuxSession{}, fmt.Errorf("invalid windows count: %s", parts[1])
	}
	session.Windows = windows

	// Parse attached status (1 = attached, 0 = detached)
	attached, err := strconv.Atoi(parts[2])
	if err != nil {
		return TmuxSession{}, fmt.Errorf("invalid attached status: %s", parts[2])
	}
	session.Attached = attached == 1

	// Parse created timestamp (Unix timestamp)
	createdUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return TmuxSession{}, fmt.Errorf("invalid created timestamp: %s", parts[3])
	}
	session.Created = time.Unix(createdUnix, 0)

	// Determine session status and populate fields
	session = tc.determineStatus(session)

	return session, nil
}

// capturePaneContent captures the visible content of a tmux pane
func (tc *TmuxCollector) capturePaneContent(sessionName string) (string, error) {
	// Capture last 15 lines of the pane (same as unified-dashboard)
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-15")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("tmux capture-pane timed out")
		}
		return "", err
	}

	return stdout.String(), nil
}

// determineStatus determines the status of a session based on Claude Code activity
func (tc *TmuxCollector) determineStatus(session TmuxSession) TmuxSession {
	now := time.Now()

	// Capture pane content to analyze Claude Code state (last 15 lines like unified-dashboard)
	content, err := tc.capturePaneContent(session.Name)
	if err != nil {
		// If we can't capture content, fall back to basic detection
		session.Status = tc.fallbackStatus(session, now)
		return session
	}

	// Check if content has changed (indicates activity)
	lastContent, hasCache := tc.sessionContentCache[session.Name]
	contentChanged := !hasCache || lastContent != content

	if contentChanged {
		tc.sessionActivityMap[session.Name] = now
		tc.sessionContentCache[session.Name] = content
		session.LastContentChange = now
	} else if lastActivity, exists := tc.sessionActivityMap[session.Name]; exists {
		session.LastContentChange = lastActivity
	} else {
		session.LastContentChange = session.Created
	}

	// Calculate idle duration (how long content unchanged)
	session.IdleDuration = now.Sub(session.LastContentChange)

	// Priority 1: Check for Claude Code-specific WORKING indicators FIRST
	// Working indicators like "(esc to interrupt)" take precedence over prompt detection
	// because both can appear on screen simultaneously while Claude is processing
	if tc.isClaudeWorking(content) {
		session.Status = StatusWorking
		return session
	}

	// Priority 2: Check if Claude Code is at a prompt (READY)
	if tc.isClaudeWaiting(content) {
		if session.IdleDuration > 30*time.Second {
			session.Status = StatusReady
		} else {
			session.Status = StatusActive
		}
		return session
	}

	// Priority 3: Check for errors (ERROR)
	if tc.hasError(content) {
		session.Status = StatusError
		return session
	}

	// Priority 4: Content change detection with timing
	if contentChanged {
		if session.IdleDuration < 30*time.Second {
			session.Status = StatusWorking
			return session
		}
	}

	// Priority 5: Check if user is actively in the session
	if session.Attached {
		session.Status = StatusActive
		return session
	}

	// Priority 6: Idle state based on time
	if session.IdleDuration > 30*time.Second {
		session.Status = StatusReady
		return session
	}

	// Default: READY (waiting for input)
	session.Status = StatusReady
	return session
}

// isClaudeWorking checks for active Claude Code processing indicators
// Uses the same patterns as unified-dashboard
func (tc *TmuxCollector) isClaudeWorking(content string) bool {
	workingPatterns := []string{
		// Claude Code status messages (with ellipsis character …)
		"Finagling…",
		"Puzzling…",
		"Listing…",
		"Running…",
		"Waiting…",
		"Architecting…",
		"Reasoning…",
		"Thinking…",
		"Connecting…",
		"Initializing…",
		// Fallback with three dots (...)
		"Finagling...",
		"Puzzling...",
		"Listing...",
		"Thinking...",
		// Other working indicators
		"Waiting for",
		"Analyzing",
		"Processing",
		"Thought for",
		"(esc to interrupt",
		"esc to interrupt",
		"background task",
		"Spawning agent",
		"Agent is",
		"<function_calls>",
		"<invoke",
		"Tool:",
		"━━━", // Progress bars
	}

	for _, pattern := range workingPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}

// isClaudeWaiting checks if Claude Code is at a prompt waiting for input
// Uses the same patterns as unified-dashboard
func (tc *TmuxCollector) isClaudeWaiting(content string) bool {
	// Check for Claude Code prompt indicators
	// The bypass permissions line appears when Claude is waiting for input
	if strings.Contains(content, "⏵⏵ bypass permissions") {
		return true
	}
	// Alternative prompt format
	if strings.Contains(content, "Claude Code") && strings.Contains(content, "❯") {
		return true
	}
	// Check for empty prompt "> " at end of content (common waiting state)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) > 0 {
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		// Empty prompt or just the prompt character
		if lastLine == ">" || lastLine == "> " || strings.HasSuffix(lastLine, "> ") {
			return true
		}
	}
	return false
}

// hasError checks for Claude Code specific error states
// Only detects actual Claude Code errors, not error text from command output being displayed
func (tc *TmuxCollector) hasError(content string) bool {
	// Skip error detection if Claude is at a prompt (functioning normally)
	if strings.Contains(content, "⏵⏵ bypass permissions") ||
		strings.Contains(content, "esc to interrupt") {
		return false
	}

	// Claude Code specific error patterns - these indicate actual problems with Claude
	claudeErrorPatterns := []string{
		"APIError",
		"API error",
		"RateLimitError",
		"rate limit",
		"Rate limit",
		"AuthenticationError",
		"Connection error",
		"connection refused",
		"ECONNREFUSED",
		"network error",
		"Network error",
		"timed out",
		"Request timed out",
		"Claude Code encountered",
		"session crashed",
		"Session crashed",
		"unexpected error",
		"Unexpected error",
		"panic: runtime",
		"fatal error:",
		"FATAL:",
		"Traceback (most recent call last):", // Python stack trace
		"Error: EPERM",
		"Error: EACCES",
		"Permission denied",
	}

	// Look in last 5 lines only
	lines := strings.Split(content, "\n")
	lastLines := ""
	if len(lines) > 5 {
		lastLines = strings.Join(lines[len(lines)-5:], "\n")
	} else {
		lastLines = content
	}

	for _, pattern := range claudeErrorPatterns {
		if strings.Contains(lastLines, pattern) {
			return true
		}
	}
	return false
}

// fallbackStatus provides basic status detection when pane content can't be captured
func (tc *TmuxCollector) fallbackStatus(session TmuxSession, now time.Time) SessionStatus {
	// If attached, assume active
	if session.Attached {
		tc.sessionActivityMap[session.Name] = now
		return StatusActive
	}

	// Check last activity
	if lastActivity, exists := tc.sessionActivityMap[session.Name]; exists {
		timeSinceActivity := now.Sub(lastActivity)
		if timeSinceActivity < 5*time.Minute {
			return StatusActive
		}
	}

	// Default to ready for detached sessions
	return StatusReady
}

// GetMetrics is a convenience method that returns the collected metrics
func (tc *TmuxCollector) GetMetrics() *TmuxMetrics {
	return tc.Collect()
}
