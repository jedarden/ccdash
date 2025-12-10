package metrics

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
func (s SessionStatus) GetEmoji() string {
	switch s {
	case StatusWorking:
		return "🟢"
	case StatusReady:
		return "🔴"
	case StatusActive:
		return "🟡"
	case StatusError:
		return "⚠️"
	default:
		return "❓"
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
}

// TmuxMetrics holds information about all tmux sessions
type TmuxMetrics struct {
	Sessions   []TmuxSession `json:"sessions"`
	Total      int           `json:"total"`
	Available  bool          `json:"available"`
	Error      string        `json:"error,omitempty"`
	LastUpdate time.Time     `json:"last_update"`
}

// TmuxCollector collects metrics about tmux sessions
type TmuxCollector struct {
	// sessionActivityMap tracks the last activity time for sessions
	sessionActivityMap map[string]time.Time
	// sessionContentCache stores recent pane content for change detection
	sessionContentCache map[string]string
}

// NewTmuxCollector creates a new TmuxCollector instance
func NewTmuxCollector() *TmuxCollector {
	return &TmuxCollector{
		sessionActivityMap:  make(map[string]time.Time),
		sessionContentCache: make(map[string]string),
	}
}

// Collect gathers current tmux session information
func (tc *TmuxCollector) Collect() *TmuxMetrics {
	metrics := &TmuxMetrics{
		Sessions:   make([]TmuxSession, 0),
		LastUpdate: time.Now(),
	}

	// Check if tmux is available
	if !tc.isTmuxAvailable() {
		metrics.Available = false
		metrics.Error = "tmux is not installed or not available in PATH"
		return metrics
	}

	metrics.Available = true

	// Get session list
	sessions, err := tc.listSessions()
	if err != nil {
		// Not necessarily an error - could just mean no sessions are running
		if strings.Contains(err.Error(), "no server running") ||
			strings.Contains(err.Error(), "no sessions") {
			metrics.Total = 0
			return metrics
		}
		metrics.Error = err.Error()
		return metrics
	}

	metrics.Sessions = sessions
	metrics.Total = len(sessions)

	return metrics
}

// isTmuxAvailable checks if tmux is installed and available
func (tc *TmuxCollector) isTmuxAvailable() bool {
	cmd := exec.Command("which", "tmux")
	err := cmd.Run()
	return err == nil
}

// listSessions executes tmux list-sessions and parses the output
func (tc *TmuxCollector) listSessions() ([]TmuxSession, error) {
	// Execute tmux list-sessions with formatted output
	// Format: session_name:windows:attached:created
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_windows}:#{session_attached}:#{session_created}")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
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
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-15")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
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

	// Priority 1: Check if Claude Code is at a prompt (READY)
	// This must be checked FIRST because working indicators like "(esc to interrupt)"
	// can persist on screen even when Claude is waiting for input
	if tc.isClaudeWaiting(content) {
		if session.IdleDuration > 30*time.Second {
			session.Status = StatusReady
		} else {
			session.Status = StatusActive
		}
		return session
	}

	// Priority 2: Check for Claude Code-specific WORKING indicators
	if tc.isClaudeWorking(content) {
		session.Status = StatusWorking
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
		"Finagling...",
		"Puzzling...",
		"Listing...",
		"Waiting for",
		"Analyzing",
		"Processing",
		"Running…",
		"Waiting…",
		"Thought for",
		"(esc to interrupt",
		"background task",
		"Spawning agent",
		"Agent is",
		// Additional patterns from ccdash observations
		"Thinking...",
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

// hasError checks for error indicators in the session
// Uses the same approach as unified-dashboard (checks last 5 lines only)
func (tc *TmuxCollector) hasError(content string) bool {
	errorPatterns := []string{
		"error",
		"Error",
		"ERROR",
		"failed",
		"Failed",
		"FAILED",
		"panic:",
		"fatal:",
		"exception",
		"Exception",
		"traceback",
		"Traceback",
	}

	// Look in last 5 lines only (like unified-dashboard)
	lines := strings.Split(content, "\n")
	lastLines := ""
	if len(lines) > 5 {
		lastLines = strings.Join(lines[len(lines)-5:], "\n")
	} else {
		lastLines = content
	}

	for _, pattern := range errorPatterns {
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
