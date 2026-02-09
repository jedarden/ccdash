# Quick Start: Implementing Worker Visualization in ccdash

This guide provides step-by-step instructions for implementing worker visualization in ccdash, starting with the simplest approach (Phase 1) and progressively adding features.

---

## Prerequisites

- Go 1.21+ installed
- ccdash source code cloned: `/home/coder/ccdash/`
- Active tmux sessions (both workers and interactive) for testing
- Basic familiarity with Go and Bubble Tea framework

---

## Phase 1: Basic Worker Detection (2-4 hours)

### Step 1.1: Add Session Type Detection

**File:** `/home/coder/ccdash/internal/metrics/tmux.go`

Add type definition and helper function after the `SessionStatus` constants:

```go
// SessionType distinguishes between worker and interactive sessions
type SessionType string

const (
	SessionTypeWorker      SessionType = "worker"
	SessionTypeInteractive SessionType = "interactive"
)

// IsWorkerSession detects if a session is a bead worker
func IsWorkerSession(sessionName string) bool {
	// Method 1: Pattern matching
	executors := []string{
		"claude-code-glm-47-",
		"claude-code-sonnet-",
		"opencode-glm-47-",
	}

	for _, prefix := range executors {
		if strings.HasPrefix(sessionName, prefix) {
			return true
		}
	}

	// Method 2: Check for worker log file (fallback)
	logPath := filepath.Join(os.Getenv("HOME"), ".beads-workers", sessionName+".log")
	if _, err := os.Stat(logPath); err == nil {
		return true
	}

	return false
}
```

### Step 1.2: Extend TmuxSession Struct

**File:** `/home/coder/ccdash/internal/metrics/tmux.go`

Add `SessionType` field to the `TmuxSession` struct (around line 66):

```go
type TmuxSession struct {
	Name              string        `json:"name"`
	Windows           int           `json:"windows"`
	Attached          bool          `json:"attached"`
	Status            SessionStatus `json:"status"`
	Created           time.Time     `json:"created"`
	LastContentChange time.Time     `json:"last_content_change"`
	IdleDuration      time.Duration `json:"idle_duration"`
	LastLines         []string      `json:"last_lines,omitempty"`
	Source            string        `json:"source,omitempty"`

	// NEW: Worker detection
	SessionType       SessionType   `json:"session_type"`
}
```

### Step 1.3: Populate SessionType During Parsing

**File:** `/home/coder/ccdash/internal/metrics/tmux.go`

In the `parseSessionLine` function (around line 338), after creating the session, add:

```go
// Determine session status and populate fields
session = tc.determineStatus(session)

// NEW: Determine session type
if IsWorkerSession(session.Name) {
	session.SessionType = SessionTypeWorker
} else {
	session.SessionType = SessionTypeInteractive
}

return session, nil
```

### Step 1.4: Add Icon Helper Function

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Add helper function to get session icon (before `renderSessionCell` function):

```go
// getSessionIcon returns appropriate icon for session type
func (d *Dashboard) getSessionIcon(session metrics.TmuxSession) string {
	if session.SessionType == metrics.SessionTypeWorker {
		return "🤖" // Robot for workers
	}
	return "💻" // Computer for interactive
}
```

### Step 1.5: Update Session Cell Rendering

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

In the `renderSessionCell` function (around line 1404), replace the emoji line with:

```go
// OLD:
// emoji := session.Status.GetEmoji()

// NEW: Use icon based on session type + status emoji
icon := d.getSessionIcon(session)
statusEmoji := session.Status.GetEmoji()
```

Update the line format string:

```go
// OLD:
// line := fmt.Sprintf("%s "+nameFormat+" %s %dw %-3s %s",
//     emoji, name, statusText, windows, idleStr, attached)

// NEW:
line := fmt.Sprintf("%s %s "+nameFormat+" %s %dw %-3s %s",
    icon, statusEmoji, name, statusText, windows, idleStr, attached)
```

### Step 1.6: Test Phase 1

```bash
cd /home/coder/ccdash

# Build
go build -o ccdash cmd/ccdash/main.go

# Run
./ccdash
```

**Expected Result:**
- Worker sessions show 🤖 icon
- Interactive sessions show 💻 icon
- Status emojis (🟢🔴🟡) still appear
- Everything else works as before

**Troubleshooting:**
- If icons don't appear: Check terminal emoji support
- If all sessions show 💻: Verify `IsWorkerSession()` logic and worker naming patterns
- If compilation fails: Check import statements (add `"path/filepath"` and `"os"` if missing)

---

## Phase 2: Visual Grouping (4-8 hours)

### Step 2.1: Add Session Grouping Logic

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Add helper function to group sessions:

```go
// groupSessions splits sessions into workers and interactive
func (d *Dashboard) groupSessions(sessions []metrics.TmuxSession) (workers, interactive []metrics.TmuxSession) {
	for _, session := range sessions {
		if session.SessionType == metrics.SessionTypeWorker {
			workers = append(workers, session)
		} else {
			interactive = append(interactive, session)
		}
	}
	return workers, interactive
}
```

### Step 2.2: Replace renderTmuxPanel

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Find the `renderTmuxPanel` function and replace its session rendering logic:

```go
func (d *Dashboard) renderTmuxPanel(width, height int) string {
	// ... existing border/title logic ...

	if !d.tmuxMetrics.Available {
		// ... existing error handling ...
	}

	sessions := d.tmuxMetrics.Sessions
	if len(sessions) == 0 {
		return titleStyle.Render("No active tmux sessions")
	}

	// NEW: Group sessions by type
	workers, interactive := d.groupSessions(sessions)

	var sections []string

	// Render workers section
	if len(workers) > 0 {
		sections = append(sections, d.renderWorkerSection(workers, contentWidth))
	}

	// Render interactive section
	if len(interactive) > 0 {
		sections = append(sections, d.renderInteractiveSection(interactive, contentWidth))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return panelStyle.Width(width).Height(height).Render(content)
}
```

### Step 2.3: Add Section Renderers

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Add two new functions:

```go
// renderWorkerSection renders the worker sessions section
func (d *Dashboard) renderWorkerSection(workers []metrics.TmuxSession, width int) string {
	// Section header
	headerText := fmt.Sprintf("Workers (%d)", len(workers))
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("45")). // Cyan
		Render(headerText)

	// Render each worker cell
	var cells []string
	for _, worker := range workers {
		cells = append(cells, d.renderWorkerCell(worker, width))
	}

	// Add separator line
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(strings.Repeat("─", width))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		separator,
		lipgloss.JoinVertical(lipgloss.Left, cells...),
		"", // Empty line after section
	)
}

// renderInteractiveSection renders the interactive sessions section
func (d *Dashboard) renderInteractiveSection(interactive []metrics.TmuxSession, width int) string {
	// Section header
	headerText := fmt.Sprintf("Interactive (%d)", len(interactive))
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214")). // Orange
		Render(headerText)

	// Render each interactive cell
	var cells []string
	for _, session := range interactive {
		cells = append(cells, d.renderSessionCell(session, width))
	}

	// Add separator line
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(strings.Repeat("─", width))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		separator,
		lipgloss.JoinVertical(lipgloss.Left, cells...),
	)
}
```

### Step 2.4: Add renderWorkerCell

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Add enhanced worker cell renderer:

```go
// renderWorkerCell renders a single worker session cell (multi-line)
func (d *Dashboard) renderWorkerCell(worker metrics.TmuxSession, width int) string {
	icon := "🤖"
	statusEmoji := worker.Status.GetEmoji()
	statusText := string(worker.Status)

	// Format idle time
	idleStr := formatDuration(worker.IdleDuration)

	// Line 1: Icon + Name
	line1 := fmt.Sprintf("%s %s", icon, worker.Name)
	if len(line1) > width {
		line1 = line1[:width-3] + "..."
	}

	// Line 2: Status + Windows + Idle + Attached
	line2 := fmt.Sprintf("   %s %s  %dw  %s",
		statusEmoji, statusText, worker.Windows, idleStr)

	if worker.Attached {
		line2 += "  📎"
	}

	// Combine lines with subtle background
	cellStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")). // Subtle dark background
		Padding(0, 1)

	return cellStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, line1, line2),
	)
}

// formatDuration formats duration as human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
```

### Step 2.5: Update Panel Title

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

In `renderTmuxPanel`, update the title to show counts:

```go
workers, interactive := d.groupSessions(sessions)
totalSessions := len(sessions)
workerCount := len(workers)
interactiveCount := len(interactive)

title := fmt.Sprintf("Tmux Sessions (%d: %d workers, %d interactive)",
	totalSessions, workerCount, interactiveCount)
```

### Step 2.6: Test Phase 2

```bash
cd /home/coder/ccdash
go build -o ccdash cmd/ccdash/main.go
./ccdash
```

**Expected Result:**
- Two distinct sections: "Workers (N)" and "Interactive (N)"
- Workers have subtle background color
- Worker cells span 2 lines (icon+name, then status)
- Interactive cells remain single-line
- Clear visual separation between groups

---

## Phase 3: Worker Metadata (6-10 hours)

### Step 3.1: Update Worker Spawn Script

**File:** `/home/coder/claude-config/scripts/bead-worker.sh`

Add metadata creation near the top of the script (after setting variables):

```bash
# Create worker metadata file
mkdir -p ~/.beads-workers/metadata
cat > ~/.beads-workers/metadata/$SESSION_NAME.json <<EOF
{
  "session_name": "$SESSION_NAME",
  "executor": "$EXECUTOR",
  "workspace": "$WORKSPACE",
  "started_at": "$(date -Iseconds)",
  "pid": $$
}
EOF

# Cleanup metadata on exit
cleanup() {
    rm -f ~/.beads-workers/metadata/$SESSION_NAME.json
}
trap cleanup EXIT
```

### Step 3.2: Add Metadata Struct

**File:** `/home/coder/ccdash/internal/metrics/tmux.go`

Add new struct after `TmuxSession`:

```go
// WorkerMetadata contains rich information about a worker session
type WorkerMetadata struct {
	SessionName string    `json:"session_name"`
	Executor    string    `json:"executor"`
	Workspace   string    `json:"workspace"`
	StartedAt   time.Time `json:"started_at"`
	PID         int       `json:"pid,omitempty"`
}

// GetWorkerMetadata reads worker metadata file
func GetWorkerMetadata(sessionName string) (*WorkerMetadata, error) {
	metadataPath := filepath.Join(
		os.Getenv("HOME"),
		".beads-workers",
		"metadata",
		sessionName+".json",
	)

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata WorkerMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}
```

### Step 3.3: Extend TmuxSession with Metadata

**File:** `/home/coder/ccdash/internal/metrics/tmux.go`

Add field to `TmuxSession` struct:

```go
type TmuxSession struct {
	// ... existing fields ...
	SessionType    SessionType     `json:"session_type"`
	WorkerMetadata *WorkerMetadata `json:"worker_metadata,omitempty"` // NEW
}
```

### Step 3.4: Fetch Metadata During Collection

**File:** `/home/coder/ccdash/internal/metrics/tmux.go`

In the `Collect` function, after determining session type:

```go
// Determine session type
if IsWorkerSession(session.Name) {
	session.SessionType = SessionTypeWorker

	// NEW: Fetch worker metadata
	if metadata, err := GetWorkerMetadata(session.Name); err == nil {
		session.WorkerMetadata = metadata
	}
} else {
	session.SessionType = SessionTypeInteractive
}
```

### Step 3.5: Display Workspace Path

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Update `renderWorkerCell` to include workspace:

```go
func (d *Dashboard) renderWorkerCell(worker metrics.TmuxSession, width int) string {
	icon := "🤖"
	statusEmoji := worker.Status.GetEmoji()
	statusText := string(worker.Status)
	idleStr := formatDuration(worker.IdleDuration)

	// Line 1: Icon + Name
	line1 := fmt.Sprintf("%s %s", icon, worker.Name)

	// Line 2: Workspace path (if available)
	var line2 string
	if worker.WorkerMetadata != nil {
		workspace := worker.WorkerMetadata.Workspace
		// Abbreviate home directory
		if strings.HasPrefix(workspace, os.Getenv("HOME")) {
			workspace = "~" + strings.TrimPrefix(workspace, os.Getenv("HOME"))
		}
		line2 = fmt.Sprintf("   %s", workspace)
		if len(line2) > width {
			// Truncate from left (show end of path)
			line2 = "   ..." + line2[len(line2)-(width-6):]
		}
	}

	// Line 3: Status + Windows + Idle
	line3 := fmt.Sprintf("   %s %s  %dw  %s",
		statusEmoji, statusText, worker.Windows, idleStr)

	if worker.Attached {
		line3 += "  📎"
	}

	// Combine lines
	var lines []string
	lines = append(lines, line1)
	if line2 != "" {
		lines = append(lines, line2)
	}
	lines = append(lines, line3)

	cellStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Padding(0, 1)

	return cellStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, lines...),
	)
}
```

### Step 3.6: Test Phase 3

```bash
# Spawn a test worker
cd /home/coder/claude-config
./scripts/spawn-workers.sh --workspace=/home/coder/research/worker-tui --workers=1 --executor=claude-code-glm-47

# Build and run ccdash
cd /home/coder/ccdash
go build -o ccdash cmd/ccdash/main.go
./ccdash
```

**Expected Result:**
- Worker cells now show 3 lines:
  1. 🤖 session-name
  2. ~/workspace/path
  3. 🟢 STATUS 1w 2m
- Workspace paths truncate gracefully if too long
- Metadata loads for newly spawned workers

---

## Testing Checklist

### Basic Functionality
- [ ] Workers display 🤖 icon
- [ ] Interactive sessions display 💻 icon
- [ ] Status emojis (🟢🔴🟡❌) still work
- [ ] Idle time updates every 2 seconds
- [ ] Attached indicator (📎) appears correctly

### Worker Detection
- [ ] `claude-code-glm-47-*` sessions detected as workers
- [ ] `claude-code-sonnet-*` sessions detected as workers
- [ ] `opencode-glm-47-*` sessions detected as workers
- [ ] Simple NATO names (`alpha`, `bravo`) detected as interactive
- [ ] Workers with log files but non-standard names still detected

### Visual Grouping
- [ ] Workers section appears first
- [ ] Interactive section appears second
- [ ] Section headers show correct counts
- [ ] Sections have visual separation (lines/spacing)
- [ ] Empty sections don't render (if no workers, skip workers section)

### Metadata Display
- [ ] Workspace paths display correctly for workers
- [ ] Home directory abbreviated to `~`
- [ ] Long paths truncate gracefully
- [ ] Missing metadata doesn't crash (fallback display)

### Edge Cases
- [ ] No tmux sessions: Shows "No active tmux sessions"
- [ ] Only workers: Only workers section renders
- [ ] Only interactive: Only interactive section renders
- [ ] 10+ workers: Layout remains readable
- [ ] Terminal resize: Layout adapts correctly

---

## Troubleshooting

### "Workers not detected"
- Check session naming: `tmux list-sessions`
- Verify `IsWorkerSession()` patterns match actual names
- Check worker log files exist: `ls ~/.beads-workers/*.log`

### "Metadata not loading"
- Verify metadata file exists: `ls ~/.beads-workers/metadata/*.json`
- Check JSON format: `cat ~/.beads-workers/metadata/<session>.json`
- Ensure bead-worker.sh changes were applied
- Respawn workers after script changes

### "Layout looks broken"
- Check terminal width: `tput cols` (should be 200+)
- Verify lipgloss imports: `go get github.com/charmbracelet/lipgloss@latest`
- Test at different widths: resize terminal

### "Compilation errors"
- Missing imports: Add to top of file:
  ```go
  import (
      "encoding/json"
      "os"
      "path/filepath"
      "strings"
  )
  ```
- Type errors: Verify `SessionType` constant definitions
- Function signature mismatches: Check function parameter types

---

## Next Steps

After completing Phases 1-3, consider:

1. **Phase 4: Bead Status Integration**
   - Query `br stats --json` for each workspace
   - Display ready/blocked/completed bead counts
   - Cache results (30s refresh interval)

2. **Phase 5: Interactive Controls**
   - Add keyboard shortcuts: `w` (workers only), `i` (interactive only), `a` (all)
   - Implement section collapse/expand
   - Add filtering and sorting

3. **Performance Optimization**
   - Profile metadata loading performance
   - Cache worker detection results
   - Lazy-load bead stats on demand

4. **Polish**
   - Add help text explaining icons
   - Configurable section colors
   - Export metrics to JSON for scripting

---

## Support

For issues or questions:
- Review `/home/coder/research/worker-tui/worker-visualization-research.md`
- Check implementation sketch: `/home/coder/research/worker-tui/implementation-sketch.go`
- Compare layouts: `/home/coder/research/worker-tui/layout-mockups.txt`

---

**Document Version:** 1.0
**Date:** 2026-02-07
**Estimated Time:** 12-22 hours total (Phases 1-3)
