package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// calculateTmuxPanelWidth determines the width needed for the TMUX panel
// based on session count and available height (to calculate columns needed)
func (d *Dashboard) calculateTmuxPanelWidth(panelHeight int) int {
	const minCellWidth = 28 // Minimum readable session cell
	const minWidth = 50     // Absolute minimum for 1 column
	const maxWidth = 120    // Cap to prevent TMUX from taking too much space

	// If no tmux metrics, use minimum
	if d.tmuxMetrics == nil || !d.tmuxMetrics.Available {
		return minWidth
	}

	sessionCount := len(d.tmuxMetrics.Sessions)
	if sessionCount == 0 {
		return minWidth
	}

	// Available lines for sessions: height - title(1) - borders(2)
	availableLines := panelHeight - 3
	if availableLines < 1 {
		availableLines = 1
	}

	// Calculate columns needed to fit all sessions
	cols := 1
	if sessionCount > availableLines {
		cols = (sessionCount + availableLines - 1) / availableLines // ceil division
	}
	if cols > 4 {
		cols = 4 // Reasonable maximum for readability
	}

	// Width needed: columns * cellWidth + separators + borders/padding
	// contentWidth = cols * cellWidth + (cols - 1) for separators
	// panelWidth = contentWidth + 4 for borders/padding
	neededWidth := cols*minCellWidth + (cols - 1) + 4

	if neededWidth < minWidth {
		return minWidth
	}
	if neededWidth > maxWidth {
		return maxWidth
	}
	return neededWidth
}

// getTmuxColumnCount returns the number of columns needed to display all sessions
func (d *Dashboard) getTmuxColumnCount(panelHeight int) int {
	if d.tmuxMetrics == nil || !d.tmuxMetrics.Available {
		return 1
	}

	sessionCount := len(d.tmuxMetrics.Sessions)
	if sessionCount == 0 {
		return 1
	}

	availableLines := panelHeight - 3
	if availableLines < 1 {
		availableLines = 1
	}

	cols := 1
	if sessionCount > availableLines {
		cols = (sessionCount + availableLines - 1) / availableLines
	}
	if cols > 4 {
		cols = 4
	}
	return cols
}

// renderUltraWide renders 3 panels side-by-side
// Balances space between Token and TMUX panels based on content needs
func (d *Dashboard) renderUltraWide() string {
	// Each panel adds two border and two padding columns around its content.
	totalPanelWidth := d.width - 12
	panelHeight := d.height - 3 // -2 borders, -1 status line
	if panelHeight < 0 {
		panelHeight = 0
	}

	// Step 1: System panel width. Its floor is set by its fixed-format Disk
	// I/O / Net I/O lines ("Disk I/O | Read: 1024.00 KB/s | Write: 1024.00
	// KB/s" = 51 cols worst case, verified against FormatRate's output
	// range) — nothing in the panel benefits from more than that, so it
	// never holds width the Token/Tmux panels could use for longer model
	// names or more sessions.
	const systemPanelMinContentWidth = 52 // 51-char worst case + 1 margin
	systemWidth := systemPanelMinContentWidth + tokenPanelBorderPadding

	// Step 2: Calculate minimum and ideal widths for both panels
	// A token panel narrower than tokenSideBySideMinWidth stacks the models
	// below the stats. That is fine when the panel is tall enough to show
	// them; only when it is not does the token panel claim the wider minimum.
	minTokenWidth := 46
	if usedInteriorRows(d.renderTokenPanel(minTokenWidth, 0)) > panelHeight {
		minTokenWidth = tokenSideBySideMinWidth
	}
	minTmuxWidth := d.calculateTmuxPanelWidth(panelHeight)

	// Ideal token width: side-by-side layout with comfortable model display,
	// sized from the actual longest model name currently in use so it grows
	// automatically instead of needing a hand-bumped constant.
	idealTokenWidth := d.calculateRequiredTokenWidth()

	// Ideal tmux width: minimum cell width + some padding for session names
	// Use 35 chars per cell (28 min + 7 for longer names)
	idealCellWidth := 35
	tmuxCols := d.getTmuxColumnCount(panelHeight)
	idealTmuxWidth := tmuxCols*idealCellWidth + (tmuxCols - 1) + 4

	// Available space after system panel
	availableWidth := totalPanelWidth - systemWidth

	// Start with minimums
	tokenWidth := minTokenWidth
	tmuxWidth := minTmuxWidth

	// Calculate how much each panel wants beyond minimum
	tokenWant := idealTokenWidth - minTokenWidth
	if tokenWant < 0 {
		tokenWant = 0
	}
	tmuxWant := idealTmuxWidth - minTmuxWidth
	if tmuxWant < 0 {
		tmuxWant = 0
	}

	// Distribute remaining space proportionally based on wants
	remainingAfterMins := availableWidth - minTokenWidth - minTmuxWidth
	if remainingAfterMins > 0 {
		totalWant := tokenWant + tmuxWant
		if totalWant > 0 {
			// Proportional allocation
			tokenExtra := remainingAfterMins * tokenWant / totalWant
			tmuxExtra := remainingAfterMins - tokenExtra
			tokenWidth = minTokenWidth + tokenExtra
			tmuxWidth = minTmuxWidth + tmuxExtra
		} else {
			// Neither wants more, split evenly
			tokenWidth = minTokenWidth + remainingAfterMins/2
			tmuxWidth = minTmuxWidth + remainingAfterMins - remainingAfterMins/2
		}
	} else if remainingAfterMins < 0 {
		// Not enough space for both minimums. The sessions panel gives way
		// first: it truncates names and re-flows its columns, whereas a token
		// panel below its side-by-side width drops the models list.
		tmuxWidth = availableWidth - minTokenWidth
		tokenWidth = minTokenWidth
		// Enforce absolute minimums
		if tokenWidth < 30 {
			tokenWidth = 30
			tmuxWidth = availableWidth - tokenWidth
		}
		if tmuxWidth < 30 {
			tmuxWidth = 30
			tokenWidth = availableWidth - tmuxWidth
		}
	}

	// Cap token panel - it doesn't need more than ideal
	if tokenWidth > idealTokenWidth {
		excess := tokenWidth - idealTokenWidth
		tokenWidth = idealTokenWidth
		tmuxWidth += excess
	}

	// Final safety: ensure widths are positive
	if tokenWidth < 1 {
		tokenWidth = 1
	}
	if tmuxWidth < 1 {
		tmuxWidth = 1
	}

	systemPanel := fitPanelToSize(d.renderSystemPanel(systemWidth, panelHeight), systemWidth, panelHeight)
	tokenPanel := fitPanelToSize(d.renderTokenPanel(tokenWidth, panelHeight), tokenWidth, panelHeight)
	tmuxPanel := fitPanelToSize(d.renderTmuxPanel(tmuxWidth, panelHeight), tmuxWidth, panelHeight)

	// Join horizontally with top alignment
	return lipgloss.JoinHorizontal(lipgloss.Top,
		systemPanel,
		tokenPanel,
		tmuxPanel,
	)
}

// renderCompact renders panels stacked vertically for narrow terminals (e.g. 75x68):
// tmux sessions on top, token usage in the middle, system resources on the bottom.
// Height is distributed with tmux getting extra rows since session lists are tall.
func (d *Dashboard) renderCompact() string {
	panelWidth := max(d.width-4, 1)
	available := d.height - 8 // 3×2 border rows + 1 status line + 1 repo/updater line
	if available < 0 {
		available = 0
	}

	// Give tmux a bit more room — sessions need more lines than the other
	// panels — but hand rows it leaves blank to the token and system panels.
	tmuxHeight := available / 2
	tmuxPanel := d.renderTmuxPanel(panelWidth, tmuxHeight)
	if used := usedInteriorRows(tmuxPanel); used < tmuxHeight {
		tmuxHeight = used
		tmuxPanel = d.renderTmuxPanel(panelWidth, tmuxHeight)
	}
	remaining := available - tmuxHeight
	tokenHeight := (remaining + 1) / 2
	systemHeight := remaining - tokenHeight

	tmuxPanel = fitPanelToSize(tmuxPanel, panelWidth, tmuxHeight)
	tokenPanel := fitPanelToSize(d.renderTokenPanel(panelWidth, tokenHeight), panelWidth, tokenHeight)
	systemPanel := fitPanelToSize(d.renderSystemPanel(panelWidth, systemHeight), panelWidth, systemHeight)

	return lipgloss.JoinVertical(lipgloss.Left,
		tmuxPanel,
		tokenPanel,
		systemPanel,
	)
}

// usedInteriorRows counts a bordered panel's interior rows up to its last
// non-blank one.
func usedInteriorRows(panel string) int {
	lines := strings.Split(panel, "\n")
	if len(lines) <= 2 {
		return 0
	}
	interior := lines[1 : len(lines)-1]
	used := len(interior)
	for used > 0 && strings.Trim(ansi.Strip(interior[used-1]), "│ ") == "" {
		used--
	}
	return used
}

// fitPanelToSize bounds the complete panel frame, including its border and
// padding. Lip Gloss Height pads short content but deliberately does not trim
// long content, so retain the top and bottom border rows when a tight panel
// needs to drop interior rows.
func fitPanelToSize(panel string, width, height int) string {
	panel = lipgloss.NewStyle().MaxWidth(max(width+4, 1)).Render(panel)
	lines := strings.Split(panel, "\n")
	maxHeight := max(height+2, 2)
	if len(lines) > maxHeight {
		lines = append(append([]string(nil), lines[:maxHeight-1]...), lines[len(lines)-1])
	}
	return strings.Join(lines, "\n")
}
