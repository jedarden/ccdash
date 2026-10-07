package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
)

// renderTmuxPanel renders the tmux sessions panel
func (d *Dashboard) renderTmuxPanel(width, height int) string {
	style := panelStyle

	if d.tmuxMetrics == nil {
		return style.Width(width).Height(height).Render("Loading tmux metrics...")
	}

	var lines []string

	// Calculate content width for right-alignment
	contentWidth := width - 4 // Account for borders and padding

	// Count sessions by status
	statusCounts := make(map[metrics.SessionStatus]int)
	for _, session := range d.tmuxMetrics.Sessions {
		statusCounts[session.Status]++
	}

	// Build status summary (right-justified)
	var statusParts []string
	for _, status := range metrics.SessionStatuses() {
		if count := statusCounts[status]; count > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%s%d", status.GetEmoji(), count))
		}
	}
	statusSummary := strings.Join(statusParts, " ")

	// Title with total count and status summary right-justified
	// Show source indicator: 🔗 for hooks, 📺 for tmux
	sourceIcon := "📺"
	sourceLabel := "Sessions"
	if d.tmuxMetrics.Source == "hooks" {
		sourceIcon = "🔗"
		sourceLabel = "Sessions"
	} else if d.tmuxMetrics.HooksInstalled && !d.tmuxMetrics.HooksAvailable {
		// Hooks installed but no hook sessions, falling back to tmux
		sourceLabel = "Sessions (tmux)"
	}
	// Only the session count. RunningProcesses counts every process named
	// "claude" on the host - subagents and sessions outside tmux included,
	// Codex/OpenCode/NEEDLE excluded - so "sessions/procs" compared two
	// different things and could read 37/36. It stays in the JSON snapshot.
	title := successStyle.Render(fmt.Sprintf("%s %s (%d)", sourceIcon, sourceLabel, d.tmuxMetrics.Total))
	if askingCount := statusCounts[metrics.StatusAsking]; askingCount > 0 {
		title += " " + askingStyle.Render(fmt.Sprintf("needs you (%d)", askingCount))
	}
	titleLen := lipgloss.Width(title)
	summaryLen := lipgloss.Width(statusSummary)
	spacing := contentWidth - titleLen - summaryLen
	if spacing < 1 {
		spacing = 1
	}

	headerLine := title + strings.Repeat(" ", spacing) + statusSummary
	lines = append(lines, headerLine)

	if !d.tmuxMetrics.Available {
		lines = append(lines, errorStyle.Render("Not Available"))
		if d.tmuxMetrics.Error != "" {
			lines = append(lines, wrapText(d.tmuxMetrics.Error, width-4))
		}
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	if len(d.tmuxMetrics.Sessions) == 0 {
		lines = append(lines, "No active sessions")
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	workers, interactive := d.groupSessions(d.tmuxMetrics.Sessions)
	contentWidth = width - 4 // borders and horizontal padding
	if contentWidth < 1 {
		contentWidth = 1
	}
	// height is the interior height: borders are drawn outside it, so only
	// the panel title row comes off the budget.
	rowBudget := height - 1
	if rowBudget < 1 {
		rowBudget = 1
	}
	interactiveCount, workerCount, columns, showMore := sessionLayout(len(interactive), len(workers), rowBudget, contentWidth)

	if interactiveCount > 0 {
		header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
		lines = append(lines, header.Render(truncateDisplayWidth(fmt.Sprintf("Interactive (%d)", len(interactive)), contentWidth)))
		lines = append(lines, d.renderSessionRows(interactive[:interactiveCount], columns, contentWidth)...)
	}
	if workerCount > 0 {
		if interactiveCount == len(interactive) && interactiveCount > 0 && workerCount == len(workers) && rowBudget >= 2+sessionRows(len(interactive), columns)+sessionRows(len(workers), columns)+1 {
			lines = append(lines, "")
		}
		header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
		lines = append(lines, header.Render(truncateDisplayWidth(fmt.Sprintf("Workers (%d)", len(workers)), contentWidth)))
		lines = append(lines, d.renderSessionRows(workers[:workerCount], columns, contentWidth)...)
	}
	if showMore {
		remaining := len(interactive) + len(workers) - interactiveCount - workerCount
		lines = append(lines, dimStyle.Render(fmt.Sprintf("... +%d more", remaining)))
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

func (d *Dashboard) renderSessionRows(sessions []metrics.TmuxSession, columns, width int) []string {
	if len(sessions) == 0 {
		return nil
	}
	if columns < 1 {
		columns = 1
	}
	cellWidth := (width - (columns - 1)) / columns
	if cellWidth < 1 {
		cellWidth = 1
	}
	rows := sessionRows(len(sessions), columns)
	lines := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		cells := make([]string, 0, columns)
		for col := 0; col < columns; col++ {
			index := col*rows + row
			if index >= len(sessions) {
				continue
			}
			cell := d.renderSessionCell(sessions[index], cellWidth)
			if columns > 1 {
				cell = lipgloss.NewStyle().Width(cellWidth).Render(cell)
			}
			cells = append(cells, cell)
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	return lines
}

func sessionRows(count, columns int) int {
	if count == 0 || columns < 1 {
		return 0
	}
	return (count + columns - 1) / columns
}

// groupSessions preserves the collector's status ordering inside each group.
// Older cached snapshots without a type are classified on demand.
func (d *Dashboard) groupSessions(sessions []metrics.TmuxSession) (workers, interactive []metrics.TmuxSession) {
	for _, session := range sessions {
		typeOfSession := session.SessionType
		if typeOfSession == "" {
			if session.Source == "needle" {
				typeOfSession = metrics.SessionTypeWorker
			} else {
				typeOfSession = metrics.DetectSessionType(session.Name)
			}
		}
		if typeOfSession == metrics.SessionTypeWorker {
			session.SessionType = metrics.SessionTypeWorker
			workers = append(workers, session)
		} else {
			session.SessionType = metrics.SessionTypeInteractive
			interactive = append(interactive, session)
		}
	}
	return workers, interactive
}

// sessionLayout chooses as many single-line columns as needed to keep the
// groups visible within the panel. It reserves rows for headers and overflow
// text, and gives interactive sessions the first rows when space is tight.
func sessionLayout(interactive, workers, rowBudget, width int) (visibleInteractive, visibleWorkers, columns int, showMore bool) {
	if rowBudget < 0 {
		rowBudget = 0
	}
	headers := 0
	if interactive > 0 {
		headers++
	}
	if workers > 0 {
		headers++
	}
	sectionGap := 0
	if interactive > 0 && workers > 0 {
		sectionGap = 1
	}
	maxColumns := width / 28
	if maxColumns < 1 {
		maxColumns = 1
	}
	if maxColumns > 4 {
		maxColumns = 4
	}
	columns = 1
	rowsFor := func(count int) int {
		return (count + columns - 1) / columns
	}
	for columns < maxColumns && headers+rowsFor(interactive)+rowsFor(workers)+sectionGap > rowBudget {
		columns++
	}
	if headers+rowsFor(interactive)+rowsFor(workers)+sectionGap <= rowBudget {
		return interactive, workers, columns, false
	}

	rowsAvailable := rowBudget - headers
	if rowsAvailable < 0 {
		rowsAvailable = 0
	}
	if rowsAvailable > 1 {
		rowsAvailable-- // Reserve one line for the overflow count.
		showMore = true
	}
	interactiveRows := rowsFor(interactive)
	workerRows := rowsFor(workers)
	if interactive > 0 && workers > 0 && rowsAvailable > 1 {
		visibleInteractiveRows := min(interactiveRows, rowsAvailable-1)
		visibleWorkerRows := min(workerRows, rowsAvailable-visibleInteractiveRows)
		visibleInteractive = min(interactive, visibleInteractiveRows*columns)
		visibleWorkers = min(workers, visibleWorkerRows*columns)
	} else {
		visibleInteractiveRows := min(interactiveRows, rowsAvailable)
		visibleWorkerRows := min(workerRows, rowsAvailable-visibleInteractiveRows)
		visibleInteractive = min(interactive, visibleInteractiveRows*columns)
		visibleWorkers = min(workers, visibleWorkerRows*columns)
	}
	renderedHeaders := 0
	if visibleInteractive > 0 {
		renderedHeaders++
	}
	if visibleWorkers > 0 {
		renderedHeaders++
	}
	renderedRows := renderedHeaders + rowsFor(visibleInteractive) + rowsFor(visibleWorkers)
	if visibleInteractive > 0 && visibleWorkers > 0 {
		renderedRows++
	}
	if visibleInteractive+visibleWorkers < interactive+workers && rowBudget > renderedRows {
		showMore = true
	}
	return visibleInteractive, visibleWorkers, columns, showMore
}

// renderSessionCell renders a single tmux session cell
func (d *Dashboard) renderSessionCell(session metrics.TmuxSession, width int) string {
	icon := "💻"
	name := session.Name
	isWorker := session.SessionType == metrics.SessionTypeWorker || session.Source == "needle"
	if isWorker {
		icon = "🤖"
		name = abbreviateWorkerName(name)
	}

	// Convert ANSI color codes to hex colors for lipgloss
	colorMap := map[string]string{
		"\033[32m": "#00ff00", // Green - WORKING (Claude processing)
		"\033[95m": "#ff00ff", // Bright magenta - ASKING (waiting for human response)
		"\033[90m": "#888888", // Gray - READY (idle, low urgency)
		"\033[33m": "#ffff00", // Yellow - ACTIVE (User in session)
		"\033[91m": "#ff5555", // Bright Red - ERROR (Error state)
		"\033[0m":  "#ffffff", // White/Reset
	}

	ansiColor := session.Status.GetColor()
	color, ok := colorMap[ansiColor]
	if !ok {
		color = "#ffffff"
	}

	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))

	statusText := compactSessionStatus(session.Status)
	idleStr := ""
	if session.IdleDuration > 0 {
		idleStr = formatDuration(session.IdleDuration)
	}
	status := statusStyle.Render(statusText)
	suffix := fmt.Sprintf(" %s %s %s", session.Status.GetEmoji(), status, idleStr)
	if session.Cost != nil {
		suffix += " " + costStyle.Render(metrics.FormatCost(*session.Cost))
	}
	if session.Attached {
		suffix += " 📎"
	}
	if width < 1 {
		return ""
	}
	workerDetails := ""
	if isWorker {
		if session.Worker != nil {
			workerDetails = " · " + formatWorkerDetails(session.Worker)
		} else if session.Source == "needle" {
			workerDetails = " · worker metadata unavailable"
		}
	}
	iconWidth := lipgloss.Width(icon + " ")
	suffixWidth := lipgloss.Width(suffix)
	nameWidth := width - iconWidth - suffixWidth
	if isWorker {
		// A worker's identity is more useful in the list than its workspace or
		// executor context. Keep the abbreviated name intact where possible,
		// and let the metadata shrink into whatever space remains.
		nameWidth = max(nameWidth, 0)
		if lipgloss.Width(name) > nameWidth {
			name = truncateDisplayWidth(name, nameWidth)
		}
		metadataWidth := max(width-iconWidth-suffixWidth-lipgloss.Width(name), 0)
		if metadataWidth >= lipgloss.Width(" · …") {
			workerDetails = truncateDisplayWidth(workerDetails, metadataWidth)
		} else {
			workerDetails = ""
		}
		return truncateDisplayWidth(icon+" "+name+workerDetails+suffix, width)
	}
	if nameWidth < 1 {
		return truncateDisplayWidth(icon+" "+name+workerDetails+suffix, width)
	}
	if session.AttentionReason != "" {
		name += " · " + strings.Join(strings.Fields(session.AttentionReason), " ")
	} else if session.TaskSummary != "" {
		name += " · " + strings.Join(strings.Fields(session.TaskSummary), " ")
	}
	name = truncateDisplayWidth(name, nameWidth)
	return truncateDisplayWidth(icon+" "+name+workerDetails+suffix, width)
}

func formatWorkerDetails(metadata *metrics.WorkerMetadata) string {
	if metadata == nil {
		return "worker metadata unavailable"
	}
	parts := make([]string, 0, 4)
	if metadata.Workspace == "" {
		parts = append(parts, "workspace unavailable")
	} else {
		parts = append(parts, "workspace "+metadata.Workspace)
	}
	executorParts := make([]string, 0, 3)
	if metadata.Agent != "" {
		executorParts = append(executorParts, metadata.Agent)
	}
	if metadata.Provider != "" {
		executorParts = append(executorParts, metadata.Provider)
	}
	if metadata.Model != "" {
		executorParts = append(executorParts, metadata.Model)
	}
	if len(executorParts) == 0 {
		parts = append(parts, "executor unavailable")
	} else {
		parts = append(parts, "executor "+strings.Join(executorParts, "/"))
	}
	if !metadata.BeadStatusAvailable {
		beadStatus := "bead status unavailable"
		if metadata.State != "" {
			beadStatus += " (" + metadata.State + ")"
		}
		parts = append(parts, beadStatus)
	} else if metadata.CurrentBead != "" {
		beadStatus := "bead " + metadata.CurrentBead
		if metadata.State != "" {
			beadStatus += " (" + metadata.State + ")"
		}
		parts = append(parts, beadStatus)
	} else if metadata.State == "EXHAUSTED" {
		parts = append(parts, "queue empty")
	} else if metadata.State != "" {
		parts = append(parts, "queue no active bead ("+metadata.State+")")
	} else {
		parts = append(parts, "queue no active bead")
	}
	if metadata.BeadsProcessed > 0 || metadata.BeadsCompleted > 0 {
		parts = append(parts, fmt.Sprintf("%d closed/%d cycles", metadata.BeadsCompleted, metadata.BeadsProcessed))
	}
	return strings.Join(parts, " · ")
}

func compactSessionStatus(status metrics.SessionStatus) string {
	switch status {
	case metrics.StatusWorking:
		return "WORK"
	case metrics.StatusAsking:
		return "ASK"
	case metrics.StatusWaitingExternal:
		return "WAIT"
	case metrics.StatusComplete:
		return "DONE"
	case metrics.StatusResumable:
		return "RESUME"
	case metrics.StatusPaused:
		return "PAUSE"
	case metrics.StatusReady:
		return "READY"
	case metrics.StatusActive:
		return "ACT"
	case metrics.StatusError:
		return "ERR"
	default:
		return string(status)
	}
}

func abbreviateWorkerName(name string) string {
	name = strings.TrimPrefix(name, "needle-")
	switch {
	case strings.HasPrefix(name, "claude-code-"):
		name = "c-" + strings.TrimPrefix(name, "claude-code-")
	case strings.HasPrefix(name, "opencode-"):
		name = "o-" + strings.TrimPrefix(name, "opencode-")
	}
	parts := strings.Split(name, "-")
	if len(parts) > 3 && (parts[0] == "c" || parts[0] == "o") && isModelVersion(parts[2]) {
		parts = append(parts[:2], parts[3:]...)
	}
	return strings.Join(parts, "-")
}

func truncateDisplayWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	var out strings.Builder
	for _, char := range value {
		candidate := out.String() + string(char)
		if lipgloss.Width(candidate)+1 > width {
			break
		}
		out.WriteRune(char)
	}
	return out.String() + "…"
}
