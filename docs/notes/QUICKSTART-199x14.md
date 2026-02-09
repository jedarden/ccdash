# Quick Start for 199x14 Display - REVISED

**IMPORTANT:** This guide is specifically for **199x14 displays** (199 cols × 14 rows).

The 14-row height constraint requires **single-line worker cells** instead of the original multi-line design.

---

## Key Differences from Original Plan

| Aspect | Original (206x14+) | Revised (199x14) |
|--------|-------------------|------------------|
| **Worker cells** | Multi-line (3 rows each) | Single-line (1 row each) |
| **Names** | Full names | Abbreviated |
| **Status** | Full words | Abbreviated |
| **Workspace** | Full path | Last directory only |
| **Bead status** | Inline display | Deferred to expandable view |
| **Max sessions** | ~3 workers visible | ~6 workers visible |

---

## Phase 1: Basic Worker Detection (Unchanged)

### Step 1.1-1.6: Same as original QUICKSTART.md

Follow Phase 1 from `/home/coder/research/worker-tui/QUICKSTART.md` exactly.

**Result:** Workers show 🤖, interactive show 💻

**Time:** 2-4 hours

---

## Phase 2: Single-Line Grouped Layout (REVISED)

### Step 2.1: Add Abbreviation Helpers

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Add helper functions before `renderWorkerSection`:

```go
// abbreviateWorkerName shortens worker session names for display
func abbreviateWorkerName(name string) string {
	// claude-code-glm-47-alpha → c-glm-alpha
	// opencode-glm-47-bravo → o-glm-bravo
	// claude-code-sonnet-charlie → c-sonnet-charlie

	name = strings.Replace(name, "claude-code-", "c-", 1)
	name = strings.Replace(name, "opencode-", "o-", 1)
	name = strings.Replace(name, "-47", "", 1) // Remove version number

	return name
}

// abbreviateStatus shortens status for compact display
func abbreviateStatus(status metrics.SessionStatus) string {
	switch status {
	case metrics.StatusWorking:
		return "WORK"
	case metrics.StatusActive:
		return "ACT"
	case metrics.StatusReady:
		return "READY"
	case metrics.StatusError:
		return "ERR"
	default:
		return string(status)
	}
}

// abbreviateWorkspace shows just the last directory segment
func abbreviateWorkspace(path string) string {
	if path == "" {
		return ""
	}

	// Replace home directory with ~
	home := os.Getenv("HOME")
	if strings.HasPrefix(path, home) {
		path = "~" + strings.TrimPrefix(path, home)
	}

	// Extract last segment
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]

		// Truncate if too long
		if len(last) > 12 {
			return last[:9] + "..."
		}
		return last
	}

	return path
}

// formatDuration formats duration as compact human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
```

### Step 2.2: Add Single-Line Worker Cell Renderer

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

```go
// renderWorkerCell renders a single worker session as a single line
func (d *Dashboard) renderWorkerCell(worker metrics.TmuxSession, width int) string {
	icon := "🤖"
	statusEmoji := worker.Status.GetEmoji()

	// Abbreviate name
	name := abbreviateWorkerName(worker.Name)

	// Abbreviate status
	status := abbreviateStatus(worker.Status)

	// Format idle time
	idle := formatDuration(worker.IdleDuration)

	// Get workspace abbreviation (if metadata available)
	workspace := ""
	if worker.WorkerMetadata != nil {
		workspace = abbreviateWorkspace(worker.WorkerMetadata.Workspace)
	}

	// Format: [icon] [name] [status-emoji] [status] [idle] [workspace] [attached]
	attached := ""
	if worker.Attached {
		attached = "📎"
	}

	// Build line with spacing
	var line string
	if workspace != "" {
		line = fmt.Sprintf("%s %-15s %s %-5s %4s  ~/%s %s",
			icon, name, statusEmoji, status, idle, workspace, attached)
	} else {
		line = fmt.Sprintf("%s %-15s %s %-5s %4s %s",
			icon, name, statusEmoji, status, idle, attached)
	}

	// Truncate if exceeds width
	if len(line) > width {
		line = line[:width-3] + "..."
	}

	return line
}
```

### Step 2.3: Update Interactive Cell Renderer

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

Update `renderSessionCell` for interactive sessions (keep it simple):

```go
// renderInteractiveCell renders a single interactive session
func (d *Dashboard) renderInteractiveCell(session metrics.TmuxSession, width int) string {
	icon := "💻"
	statusEmoji := session.Status.GetEmoji()

	// Use abbreviated status for consistency
	status := abbreviateStatus(session.Status)

	idle := formatDuration(session.IdleDuration)

	attached := ""
	if session.Attached {
		attached = "📎"
	}

	line := fmt.Sprintf("%s %-15s %s %-5s %4s %s",
		icon, session.Name, statusEmoji, status, idle, attached)

	// Truncate if needed
	if len(line) > width {
		line = line[:width-3] + "..."
	}

	return line
}
```

### Step 2.4: Update Section Renderers

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

```go
// renderWorkerSection renders the worker sessions section with single-line cells
func (d *Dashboard) renderWorkerSection(workers []metrics.TmuxSession, width int) string {
	// Section header
	headerText := fmt.Sprintf("Workers (%d)", len(workers))
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("45")). // Cyan
		Render(headerText)

	// Render each worker cell (single line)
	var cells []string
	for _, worker := range workers {
		cell := d.renderWorkerCell(worker, width)
		cells = append(cells, cell)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		lipgloss.JoinVertical(lipgloss.Left, cells...),
		"", // Empty line after section
	)
}

// renderInteractiveSection renders interactive sessions section
func (d *Dashboard) renderInteractiveSection(interactive []metrics.TmuxSession, width int) string {
	// Section header
	headerText := fmt.Sprintf("Interactive (%d)", len(interactive))
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214")). // Orange
		Render(headerText)

	// Render each interactive cell (single line)
	var cells []string
	for _, session := range interactive {
		cell := d.renderInteractiveCell(session, width)
		cells = append(cells, cell)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		lipgloss.JoinVertical(lipgloss.Left, cells...),
	)
}
```

### Step 2.5: Update Main Panel Renderer

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

In `renderTmuxPanel`, replace session rendering logic:

```go
func (d *Dashboard) renderTmuxPanel(width, height int) string {
	// ... existing title/error handling ...

	sessions := d.tmuxMetrics.Sessions
	if len(sessions) == 0 {
		return titleStyle.Render("No active tmux sessions")
	}

	// Group sessions by type
	workers, interactive := d.groupSessions(sessions)

	var sections []string

	// Render interactive section FIRST (more relevant to users)
	if len(interactive) > 0 {
		sections = append(sections, d.renderInteractiveSection(interactive, width-4))
	}

	// Render workers section SECOND (background processes)
	if len(workers) > 0 {
		sections = append(sections, d.renderWorkerSection(workers, width-4))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return panelStyle.Width(width).Height(height).Render(content)
}

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

### Step 2.6: Test Phase 2 at 199x14

```bash
cd /home/coder/ccdash
go build -o ccdash cmd/ccdash/main.go
./ccdash
```

**Expected Result at 199x14:**

```
Interactive (2)
💻 alpha          🟡 ACT   30s  📎
💻 delta          🔴 READY 10m  📎

Workers (3)
🤖 c-glm-alpha    🟢 WORK   2m  ~/kalshi
🤖 c-glm-bravo    🔴 READY  5m  ~/botburrow
🤖 o-glm-charlie  🟡 ACT    1m  ~/backtest
```

**Verify:**
- [ ] All sessions fit in panel (no overflow)
- [ ] Names abbreviated correctly (`c-glm-alpha` not `claude-code-glm-47-alpha`)
- [ ] Status abbreviated (`WORK` not `WORKING`)
- [ ] Workspace shows last dir (`~/kalshi` not full path)
- [ ] Section headers show counts
- [ ] Icons distinguish types (🤖 vs 💻)
- [ ] Everything fits in 11 rows

**Time:** 4-6 hours

---

## Phase 3: Metadata Integration (Simplified)

### Step 3.1-3.4: Same as original

Follow Phase 3 from original QUICKSTART.md for:
- Updating worker spawn script
- Adding metadata struct
- Fetching metadata during collection

### Step 3.5: Display Abbreviated Workspace (REVISED)

**Already implemented in Step 2.2** - `renderWorkerCell` checks for `worker.WorkerMetadata` and calls `abbreviateWorkspace()`.

No additional changes needed!

### Step 3.6: Test Phase 3

```bash
# Spawn test worker with metadata
cd /home/coder/claude-config
./scripts/spawn-workers.sh --workspace=/home/coder/research/worker-tui --workers=1

# Build and run ccdash
cd /home/coder/ccdash
go build -o ccdash cmd/ccdash/main.go
./ccdash
```

**Expected Result:**
```
Interactive (0)
(no interactive sessions)

Workers (1)
🤖 c-glm-alpha    🟢 WORK   2m  ~/worker-tui
```

**Verify:**
- [ ] Workspace displays abbreviated (`~/worker-tui`)
- [ ] Full path stored in metadata (check with debug print)
- [ ] Missing metadata doesn't crash (no workspace displayed)

**Time:** 4-8 hours

---

## Phase 4: Deferred (Bead Status)

**Skip for now** - not enough space in single-line format.

**Future:** Implement in expandable detail view (Phase 5).

---

## Phase 5: Expandable Detail View (Optional)

Add keyboard shortcut to show full worker details:

### Step 5.1: Add Key Handler

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

In the `Update` method, add key handler:

```go
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return d, tea.Quit
		case "h":
			d.helpMode = (d.helpMode + 1) % 4
		case "l":
			d.lookbackMode = !d.lookbackMode
		case "w":
			// NEW: Toggle worker detail view
			d.workerDetailMode = !d.workerDetailMode
		}
	// ... rest of Update method
}
```

### Step 5.2: Add Detail Renderer

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

```go
// renderWorkerDetailView shows full worker information
func (d *Dashboard) renderWorkerDetailView(width, height int) string {
	if !d.tmuxMetrics.Available {
		return "No worker data available"
	}

	// Filter to only workers
	var workers []metrics.TmuxSession
	for _, session := range d.tmuxMetrics.Sessions {
		if session.SessionType == metrics.SessionTypeWorker {
			workers = append(workers, session)
		}
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("45")).
		Render(fmt.Sprintf("Worker Details (%d) - Press 'w' to return", len(workers)))

	var lines []string
	lines = append(lines, title, "")

	for _, worker := range workers {
		// Full name
		lines = append(lines, fmt.Sprintf("🤖 %s", worker.Name))

		// Workspace path (full)
		if worker.WorkerMetadata != nil {
			workspace := worker.WorkerMetadata.Workspace
			if strings.HasPrefix(workspace, os.Getenv("HOME")) {
				workspace = "~" + strings.TrimPrefix(workspace, os.Getenv("HOME"))
			}
			lines = append(lines, fmt.Sprintf("   Workspace: %s", workspace))
		}

		// Status line
		statusEmoji := worker.Status.GetEmoji()
		statusText := string(worker.Status)
		idle := formatDuration(worker.IdleDuration)
		attached := ""
		if worker.Attached {
			attached = " 📎"
		}
		lines = append(lines, fmt.Sprintf("   Status: %s %s  %dw  %s%s",
			statusEmoji, statusText, worker.Windows, idle, attached))

		// TODO: Bead stats (Phase 4 integration)
		// if worker.WorkerMetadata != nil && worker.WorkerMetadata.BeadStats != nil {
		//     lines = append(lines, fmt.Sprintf("   Beads: %s", FormatBeadStatus(worker.WorkerMetadata.BeadStats)))
		// }

		lines = append(lines, "") // Empty line between workers
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
```

### Step 5.3: Update Main Render Logic

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`

In `View` method:

```go
func (d *Dashboard) View() string {
	// ... existing layout detection ...

	// NEW: Check if in worker detail mode
	if d.workerDetailMode {
		// Show full-screen worker detail
		return d.renderWorkerDetailView(d.width, d.height)
	}

	// ... rest of normal rendering ...
}
```

### Step 5.4: Test Phase 5

```bash
cd /home/coder/ccdash
go build -o ccdash cmd/ccdash/main.go
./ccdash

# Press 'w' to toggle worker detail view
# Press 'w' again to return to summary
```

**Expected:**
- Default view: Compact single-line display
- After 'w': Full worker details with complete names and paths
- After 'w' again: Return to compact view

**Time:** 8-12 hours

---

## Abbreviation Reference Card

### Worker Names

| Full Name | Abbreviated | Saved Chars |
|-----------|-------------|-------------|
| `claude-code-glm-47-alpha` | `c-glm-alpha` | 17 |
| `claude-code-sonnet-bravo` | `c-sonnet-bravo` | 12 |
| `opencode-glm-47-charlie` | `o-glm-charlie` | 13 |

### Status

| Full | Abbreviated | Color |
|------|-------------|-------|
| `WORKING` | `WORK` | 🟢 Green |
| `ACTIVE` | `ACT` | 🟡 Yellow |
| `READY` | `READY` | 🔴 Red |
| `ERROR` | `ERR` | ❌ Red X |

### Workspace Paths

| Full Path | Abbreviated |
|-----------|-------------|
| `/home/coder/prompts/kalshi-improvement` | `~/kalshi-imp...` |
| `/home/coder/ardenone-cluster/botburrow` | `~/botburrow` |
| `/home/coder/trading/backtest-engine` | `~/backtest-e...` |

---

## Testing Checklist for 199x14

### Display Constraints
- [ ] Terminal is 199 cols × 14 rows (verify with `tput cols` and `tput lines`)
- [ ] All 3 panels visible (System, Token, Tmux)
- [ ] No scrolling required
- [ ] Content fits in 11 rows (panel height)

### Worker Display
- [ ] Workers show 🤖 icon
- [ ] Interactive show 💻 icon
- [ ] Section headers show correct counts
- [ ] Names abbreviated correctly
- [ ] Status abbreviated correctly
- [ ] Workspace abbreviated correctly
- [ ] Can see 3 workers + 2 interactive comfortably

### Edge Cases
- [ ] Missing metadata: Worker displays without workspace
- [ ] Long workspace names: Truncated to ~12 chars
- [ ] 6+ sessions: All fit in single view
- [ ] No workers: Only interactive section shown
- [ ] No interactive: Only workers section shown

### Expandable View (Phase 5)
- [ ] 'w' key toggles detail view
- [ ] Detail view shows full names
- [ ] Detail view shows full workspace paths
- [ ] Return to summary works ('w' again)

---

## Troubleshooting for 199x14

### "Layout looks cramped"

**Check:**
- Verify terminal size: `echo $COLUMNS x $LINES`
- Reduce padding if needed (adjust panelStyle)
- Consider hiding system/token panels temporarily to test tmux panel

### "Worker names still too long"

**Solution:**
- Increase abbreviation aggressiveness:
  ```go
  // More aggressive: c-glm-a instead of c-glm-alpha
  name = strings.Replace(name, "claude-code-glm-47-", "cg-", 1)
  ```

### "Can't see all sessions"

**Options:**
1. Reduce to single-line format (already done)
2. Implement scrolling (future enhancement)
3. Use expandable detail view (Phase 5)
4. Increase terminal height (resize to 199x20)

### "Workspace paths unhelpful"

**Solution:**
- Show parent dir too: `prompts/kalshi` instead of `kalshi`
- Add tooltip on hover (Phase 5)
- Show full path in detail view (Phase 5)

---

## Performance Considerations

At 199x14, refresh every 2 seconds:

**Operations per refresh:**
- Session detection: ~5-10 sessions
- Metadata reads: ~3-5 workers
- Name abbreviation: String ops (< 1ms)
- Rendering: Minimal (single-line cells)

**Total overhead:** < 10ms per refresh (negligible)

---

## Conclusion for 199x14 Implementation

**Phases 1-3 (Basic + Grouped + Metadata):** 10-18 hours
**Phase 5 (Expandable detail):** 8-12 hours optional

**Key Success Factors:**
1. Single-line cell format (fits in 11 rows)
2. Aggressive abbreviation (fits in 69 cols)
3. Clear grouping (workers/interactive sections)
4. Graceful degradation (missing metadata doesn't break)
5. Optional detail view (full info on demand)

This approach provides the best balance for **199x14 displays** between information density and space constraints.

---

**Document Version:** 1.0
**Display:** 199x14 (199 cols × 14 rows)
**Supersedes:** Original QUICKSTART.md (designed for 206x14+)
**Date:** 2026-02-07
