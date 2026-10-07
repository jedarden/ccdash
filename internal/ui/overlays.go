package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
)

// renderLookbackPicker renders the lookback time picker overlay
func (d *Dashboard) renderLookbackPicker() string {
	panelHeight := d.height - 3
	panelWidth := 60
	if panelWidth > d.width-4 {
		panelWidth = d.width - 4
	}

	var lines []string

	// Title
	lines = append(lines, boldStyle.Render("📅 Token Usage Lookback"))
	lines = append(lines, "")

	if d.lookbackCustomMode {
		// Custom date/time picker
		lines = append(lines, "Set custom start date/time:")
		lines = append(lines, "")

		// Date/time fields
		year := d.lookbackCustomDate.Year()
		month := int(d.lookbackCustomDate.Month())
		day := d.lookbackCustomDate.Day()
		hour := d.lookbackCustomDate.Hour()
		minute := d.lookbackCustomDate.Minute()

		// Highlight selected field
		fieldStyle := dimStyle
		selectedStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#00aaff")).
			Foreground(lipgloss.Color("#000000")).
			Bold(true)

		yearStr := fmt.Sprintf(" %04d ", year)
		monthStr := fmt.Sprintf(" %02d ", month)
		dayStr := fmt.Sprintf(" %02d ", day)
		hourStr := fmt.Sprintf(" %02d ", hour)
		minStr := fmt.Sprintf(" %02d ", minute)

		if d.lookbackEditField == 0 {
			yearStr = selectedStyle.Render(yearStr)
		} else {
			yearStr = fieldStyle.Render(yearStr)
		}
		if d.lookbackEditField == 1 {
			monthStr = selectedStyle.Render(monthStr)
		} else {
			monthStr = fieldStyle.Render(monthStr)
		}
		if d.lookbackEditField == 2 {
			dayStr = selectedStyle.Render(dayStr)
		} else {
			dayStr = fieldStyle.Render(dayStr)
		}
		if d.lookbackEditField == 3 {
			hourStr = selectedStyle.Render(hourStr)
		} else {
			hourStr = fieldStyle.Render(hourStr)
		}
		if d.lookbackEditField == 4 {
			minStr = selectedStyle.Render(minStr)
		} else {
			minStr = fieldStyle.Render(minStr)
		}

		lines = append(lines, fmt.Sprintf("  Date: %s-%s-%s", yearStr, monthStr, dayStr))
		lines = append(lines, fmt.Sprintf("  Time: %s:%s", hourStr, minStr))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("  ↑/↓: adjust value  ←/→/Tab: change field"))
		lines = append(lines, dimStyle.Render("  Enter: apply  Esc: back to presets"))
	} else {
		// Preset selection
		// Calculate available content lines: panelHeight - 4 (borders + padding)
		availableLines := panelHeight - 4
		// Compact mode needed if: title(1) + instruction(1) + presets(7*2=14) + nav(1) = 17 > available
		// Use compact single-line format when height is constrained
		compactMode := availableLines < 17

		lines = append(lines, "Select a lookback period:")
		if !compactMode {
			lines = append(lines, "")
		}

		for i, preset := range d.lookbackPresets {
			prefix := "  "
			style := dimStyle
			if i == d.lookbackSelectedIndex {
				prefix = "▶ "
				style = successStyle
			}

			// Show time for presets with GetTime
			timeStr := ""
			if preset.GetTime != nil {
				t := preset.GetTime()
				if !t.IsZero() {
					timeStr = fmt.Sprintf(" (%s)", t.Format("Jan 2 3:04pm"))
				}
			}

			if compactMode {
				// Single-line compact format: "▶ Name - Description (time)"
				lines = append(lines, fmt.Sprintf("%s%s %s%s",
					prefix,
					style.Render(preset.Name),
					dimStyle.Render("- "+preset.Description),
					dimStyle.Render(timeStr)))
			} else {
				// Full two-line format
				lines = append(lines, fmt.Sprintf("%s%s%s",
					prefix,
					style.Render(preset.Name),
					dimStyle.Render(timeStr)))
				lines = append(lines, fmt.Sprintf("   %s", dimStyle.Render(preset.Description)))
			}
		}

		if !compactMode {
			lines = append(lines, "")
		}
		lines = append(lines, dimStyle.Render("  ↑/↓/j/k: navigate  Enter/Space: select  Esc/l: close"))
	}

	// Build the picker panel
	content := strings.Join(lines, "\n")

	pickerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffaa00")).
		Padding(1, 2).
		Width(panelWidth).
		Height(panelHeight)

	picker := pickerStyle.Render(content)

	// Center the picker on screen
	leftPad := (d.width - panelWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	return lipgloss.NewStyle().PaddingLeft(leftPad).Render(picker)
}

func (d *Dashboard) renderHelpView() string {
	panelHeight := d.height - 3
	totalPanelWidth := d.width - 2             // Match normal view width calculation
	panelWidth := (totalPanelWidth * 40) / 100 // 40% for panel

	var panel string
	var helpText string
	var title string

	switch d.helpMode {
	case 1: // System Resources
		title = "System Resources Panel"
		panel = d.renderSystemPanel(panelWidth, panelHeight)
		helpText = `Real-time system metrics updated every 2s:

CPU: Overall + per-core usage as N:[||| XX%]
  Colors: Green<60% Yellow60-79% Orange80-94% Red≥95%
  ≤6 cores: one per line, >6: multiple per line

Memory/Swap: Used/Total with percentage bars
  Formatted in GB/MB for readability

Disk: Root filesystem (/) usage with percentage bar
  Shows used/total space in GB/TB

Disk I/O: Read/write speeds in bytes/s or KB/s

Net I/O: Network recv/sent speeds

Load: 1min, 5min, 15min averages
  Indicates overall system activity level`

	case 2: // Token Usage
		title = "Token Usage Panel"
		panel = d.renderTokenPanel(panelWidth, panelHeight)
		helpText = `Tracks token usage from ~/.claude/projects (Claude) and zai-proxy metrics (GLM):

Tokens:
  In/Out: Input/output tokens
  Cache Read/Create: Cache operations (Claude only)
  Total: All tokens combined
  Cost: Estimated API cost ($)

Rates:
  Rate: Current tok/min (60s window)
  Avg: Session average tok/min

Time:
  Span: Duration of lookback period
  Active: Time since first activity in period

Lookback: Press 'l' to open time picker
  Presets: Today, 24h, 7d, 30d, All time
  Custom: Set specific date/time with arrows

Models: Per-model cost breakdown
  Color-coded: Opus(red) Sonnet(cyan) Haiku(green) GLM(blue)
  Sorted by cost (highest first)

Data Sources:
  - Claude: ~/.claude/projects/*.jsonl (sessions)
  - Codex: ~/.codex/sessions/YYYY/MM/DD (rollouts)
  - OpenCode: ~/.local/share/opencode/opencode.db (read-only)
  - GLM: Not currently tracked (zai-proxy only exports Prometheus)

SQLite Cache: .ccdash/tokens.db
  Queryable with DuckDB or any SQLite tool
  Tables: token_events, file_state
  Incremental ingestion with deduplication`

	case 3: // TMUX Sessions
		title = "TMUX Sessions Panel"
		panel = d.renderTmuxPanel(panelWidth, panelHeight)
		helpText = `Monitors tmux sessions running Claude Code:

Title: Shows total count + status summary
  Format: "📺 TMUX Sessions (N) needs you (N) 🟣2 ⚪1"

Status (analyzes pane content):
` + sessionStatusHelpText() + `

Detection: Analyzes last 15 lines for:
  Working indicators, prompts, errors

Session Info:
  Name, status, windows (Xw), idle, 📎=attached

Idle: Time since content changed (s/m/h)

Layout: Auto-columns based on count/width

Self-Update: Press 'u' to check for an update, or install one already found
  Status bar shows "⬆ vX.X.X available!"; 'esc' dismisses its notice`
	}

	// Create help text panel with wrapping that preserves line breaks
	helpWidth := d.width - panelWidth - 6 // Remaining width for help text
	if helpWidth < 40 {
		helpWidth = 40
	}

	// Calculate available lines for help text
	// panelHeight includes borders (2 lines), padding, title (1 line), blank line (1 line)
	availableLines := panelHeight - 4 // -4 for borders, title, and spacing
	if availableLines < 5 {
		availableLines = 5
	}

	// Wrap the help text
	wrappedHelp := wrapTextPreserveBreaks(helpText, helpWidth-4)
	helpLines := strings.Split(wrappedHelp, "\n")

	// Check if we need 2-column layout
	var finalHelpText string
	if len(helpLines) > availableLines {
		// Use 2-column layout to fit more content
		columnWidth := (helpWidth - 6) / 2 // Split into 2 columns with spacing

		// Re-wrap text to narrower column width first
		wrappedForColumns := wrapTextPreserveBreaks(helpText, columnWidth-2)
		columnLines := strings.Split(wrappedForColumns, "\n")

		// Split lines into two columns at midpoint
		midPoint := (len(columnLines) + 1) / 2

		// Ensure we don't exceed available lines
		if midPoint > availableLines {
			midPoint = availableLines
		}

		leftLines := columnLines[:midPoint]
		var rightLines []string
		if midPoint < len(columnLines) {
			endPoint := midPoint * 2
			if endPoint > len(columnLines) {
				endPoint = len(columnLines)
			}
			rightLines = columnLines[midPoint:endPoint]
		}

		// Pad shorter column with empty lines for alignment
		for len(rightLines) < len(leftLines) {
			rightLines = append(rightLines, "")
		}

		// Ensure both columns fit within available lines
		if len(leftLines) > availableLines {
			leftLines = leftLines[:availableLines]
			rightLines = rightLines[:availableLines]
		}

		// Build columns with fixed width
		var leftCol, rightCol strings.Builder
		for i := 0; i < len(leftLines); i++ {
			if i > 0 {
				leftCol.WriteString("\n")
				rightCol.WriteString("\n")
			}
			leftCol.WriteString(leftLines[i])
			if i < len(rightLines) {
				rightCol.WriteString(rightLines[i])
			}
		}

		leftColPanel := lipgloss.NewStyle().Width(columnWidth).Render(leftCol.String())
		rightColPanel := lipgloss.NewStyle().Width(columnWidth).Render(rightCol.String())

		finalHelpText = lipgloss.JoinHorizontal(lipgloss.Top, leftColPanel, "  ", rightColPanel)
	} else {
		// Single column is fine
		finalHelpText = strings.Join(helpLines, "\n")
	}

	helpPanel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffaa00")). // Orange for help
		Padding(0, 1).
		Width(helpWidth).
		Height(panelHeight).
		Render(successStyle.Render(title) + "\n\n" + finalHelpText)

	// Render panel on left, help on right
	return lipgloss.JoinHorizontal(lipgloss.Top, panel, " ", helpPanel)
}

func sessionStatusHelpText() string {
	var lines strings.Builder
	for _, status := range metrics.SessionStatuses() {
		fmt.Fprintf(&lines, "  %s %-8s - %s\n", status.GetEmoji(), status, status.Description())
	}
	return strings.TrimSuffix(lines.String(), "\n")
}
