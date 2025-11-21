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
	// StatusWorking indicates an active session with recent activity
	StatusWorking SessionStatus = "WORKING"
	// StatusStalled indicates a session that hasn't been used recently
	StatusStalled SessionStatus = "STALLED"
	// StatusIdle indicates a detached session with no recent activity
	StatusIdle SessionStatus = "IDLE"
	// StatusReady indicates a new or available session
	StatusReady SessionStatus = "READY"
)

// GetColor returns the ANSI color code for the status
func (s SessionStatus) GetColor() string {
	switch s {
	case StatusWorking:
		return "\033[32m" // Green
	case StatusStalled:
		return "\033[33m" // Yellow
	case StatusIdle:
		return "\033[90m" // Gray
	case StatusReady:
		return "\033[36m" // Cyan
	default:
		return "\033[0m" // Reset
	}
}

// GetEmoji returns the emoji representation for the status
func (s SessionStatus) GetEmoji() string {
	switch s {
	case StatusWorking:
		return "🟢"
	case StatusStalled:
		return "🟡"
	case StatusIdle:
		return "⚪"
	case StatusReady:
		return "🔵"
	default:
		return "❓"
	}
}

// TmuxSession represents a single tmux session
type TmuxSession struct {
	Name     string        `json:"name"`
	Windows  int           `json:"windows"`
	Attached bool          `json:"attached"`
	Status   SessionStatus `json:"status"`
	Created  time.Time     `json:"created"`
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
}

// NewTmuxCollector creates a new TmuxCollector instance
func NewTmuxCollector() *TmuxCollector {
	return &TmuxCollector{
		sessionActivityMap: make(map[string]time.Time),
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

	// Determine session status
	session.Status = tc.determineStatus(session)

	return session, nil
}

// determineStatus determines the status of a session based on various factors
func (tc *TmuxCollector) determineStatus(session TmuxSession) SessionStatus {
	now := time.Now()
	sessionAge := now.Sub(session.Created)

	// If session is attached, it's actively being worked on
	if session.Attached {
		// Update activity map
		tc.sessionActivityMap[session.Name] = now
		return StatusWorking
	}

	// Check last activity time
	lastActivity, exists := tc.sessionActivityMap[session.Name]

	// If we have activity data
	if exists {
		timeSinceActivity := now.Sub(lastActivity)

		// Stalled: detached for more than 1 hour but less than 24 hours
		if timeSinceActivity > 1*time.Hour && timeSinceActivity < 24*time.Hour {
			return StatusStalled
		}

		// Idle: detached for more than 24 hours
		if timeSinceActivity >= 24*time.Hour {
			return StatusIdle
		}

		// Ready: recently detached (within 1 hour)
		return StatusReady
	}

	// No activity data - determine by session age
	// New session (less than 5 minutes old) = Ready
	if sessionAge < 5*time.Minute {
		tc.sessionActivityMap[session.Name] = session.Created
		return StatusReady
	}

	// Older detached session with no activity data
	// Assume stalled if created more than 1 hour ago
	if sessionAge > 1*time.Hour && sessionAge < 24*time.Hour {
		return StatusStalled
	}

	// Very old session = Idle
	if sessionAge >= 24*time.Hour {
		return StatusIdle
	}

	// Default to Ready for newer sessions
	tc.sessionActivityMap[session.Name] = session.Created
	return StatusReady
}

// GetMetrics is a convenience method that returns the collected metrics
func (tc *TmuxCollector) GetMetrics() *TmuxMetrics {
	return tc.Collect()
}
