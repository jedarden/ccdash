package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
)

func (d *Dashboard) renderWorkerDetailView() string {
	workers, _ := d.groupSessions(d.workerSessions())
	width := max(d.width, 1)
	height := d.height - 1 // Leave the status bar visible.
	if d.layoutMode == LayoutCompact {
		height-- // Compact status bars use two rows.
	}
	if height < 1 {
		height = 1
	}

	lines := wrapDetailText(fmt.Sprintf("Worker Details (%d) · ↑/↓ or j/k browse · w/q/Esc return", len(workers)), width)
	if len(lines) > height {
		lines = lines[:height]
	}
	if len(workers) == 0 {
		for _, line := range wrapDetailText("No workers currently detected.", width) {
			if len(lines) == height {
				break
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	}

	start := min(d.workerDetailOffset, len(workers)-1)
	for index := start; index < len(workers); index++ {
		block := workerDetailLines(workers[index], width)
		separatorRows := 0
		if index > start {
			separatorRows = 1
		}
		if len(lines)+separatorRows+len(block) > height {
			if index == start && len(lines) < height {
				for _, line := range wrapDetailText("Terminal too short for this worker; resize to show full details.", width) {
					if len(lines) == height {
						break
					}
					lines = append(lines, line)
				}
			}
			break
		}
		if separatorRows > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, block...)
	}
	return strings.Join(lines, "\n")
}

func workerDetailLines(session metrics.TmuxSession, width int) []string {
	name := session.Name
	if session.Worker != nil && session.Worker.FullName != "" {
		name = session.Worker.FullName
	}
	if name == "" {
		name = "unknown session"
	}
	lines := wrapDetailText("🤖 "+name, width)

	metadata := session.Worker
	workspace := "unavailable"
	executor := "unavailable"
	if metadata != nil {
		if metadata.Workspace != "" {
			workspace = metadata.Workspace
		}
		executorParts := make([]string, 0, 3)
		for _, part := range []string{metadata.Agent, metadata.Provider, metadata.Model} {
			if part != "" {
				executorParts = append(executorParts, part)
			}
		}
		if len(executorParts) > 0 {
			executor = strings.Join(executorParts, " / ")
		}
	}
	lines = append(lines, wrapDetailText("Workspace: "+workspace, width)...)

	status := string(session.Status)
	if status == "" {
		status = "UNKNOWN"
	}
	statusParts := []string{status}
	if session.IdleDuration > 0 {
		statusParts = append(statusParts, "idle "+formatDuration(session.IdleDuration))
	}
	if !session.Created.IsZero() && time.Since(session.Created) >= 0 {
		statusParts = append(statusParts, "uptime "+formatDuration(time.Since(session.Created)))
	}
	if session.Windows > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d windows", session.Windows))
	}
	if session.Attached && session.Source != "needle" {
		statusParts = append(statusParts, "attached")
	}
	lines = append(lines, wrapDetailText("Status: "+strings.Join(statusParts, " · "), width)...)
	lines = append(lines, wrapDetailText("Executor: "+executor, width)...)

	if metadata == nil || !metadata.BeadStatusAvailable {
		beadStatus := "Bead status: unavailable"
		if metadata != nil && metadata.State != "" {
			beadStatus += " (worker state: " + metadata.State + ")"
		}
		lines = append(lines, wrapDetailText(beadStatus, width)...)
	} else if metadata.CurrentBead != "" {
		beadStatus := "Current bead: " + metadata.CurrentBead
		if metadata.State != "" {
			beadStatus += " · worker state: " + metadata.State
		}
		lines = append(lines, wrapDetailText(beadStatus, width)...)
	} else if metadata.State == "EXHAUSTED" {
		lines = append(lines, "Queue: empty (EXHAUSTED)")
	} else {
		beadStatus := "Current bead: none"
		if metadata.State != "" {
			beadStatus += " · worker state: " + metadata.State
		}
		lines = append(lines, wrapDetailText(beadStatus, width)...)
	}
	if metadata != nil && (metadata.BeadsProcessed > 0 || metadata.BeadsCompleted > 0) {
		lines = append(lines, wrapDetailText(fmt.Sprintf("Cycles: %d completed / %d processed", metadata.BeadsCompleted, metadata.BeadsProcessed), width)...)
	}
	return lines
}

// wrapDetailText wraps at terminal display width and preserves every rune,
// including long session names and workspace paths without spaces.
func wrapDetailText(value string, width int) []string {
	if width < 1 {
		return nil
	}
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "\r", " ")
	lines := make([]string, 0, 1)
	var line strings.Builder
	lineWidth := 0
	for _, r := range value {
		runeText := string(r)
		runeWidth := lipgloss.Width(runeText)
		if line.Len() > 0 && lineWidth+runeWidth > width {
			lines = append(lines, line.String())
			line.Reset()
			lineWidth = 0
		}
		line.WriteRune(r)
		lineWidth += runeWidth
	}
	if line.Len() > 0 || len(lines) == 0 {
		lines = append(lines, line.String())
	}
	return lines
}
