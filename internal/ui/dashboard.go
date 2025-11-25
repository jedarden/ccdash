package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
)

// Layout mode constants
type LayoutMode int

const (
	LayoutNarrow     LayoutMode = iota // <120 cols
	LayoutWide                          // 120-239 cols, >=30 lines
	LayoutUltraWide                     // >=240 cols
)

// tickMsg is sent every 2 seconds to trigger refresh
type tickMsg time.Time

// Dashboard is the main Bubble Tea model
type Dashboard struct {
	width         int
	height        int
	layoutMode    LayoutMode
	version       string

	// Metrics collectors
	systemCollector *metrics.SystemCollector
	tokenCollector  *metrics.TokenCollector
	tmuxCollector   *metrics.TmuxCollector

	// Current metrics
	systemMetrics metrics.SystemMetrics
	tokenMetrics  *metrics.TokenMetrics
	tmuxMetrics   *metrics.TmuxMetrics

	// UI state
	lastUpdate    time.Time
	err           error
	helpMode      int // 0=none, 1=system, 2=tokens, 3=tmux
}

// NewDashboard creates a new dashboard model
func NewDashboard(version string) *Dashboard {
	return &Dashboard{
		version:         version,
		systemCollector: metrics.NewSystemCollector(),
		tokenCollector:  metrics.NewTokenCollector(),
		tmuxCollector:   metrics.NewTmuxCollector(),
		lastUpdate:      time.Now(),
	}
}

// Init initializes the dashboard
func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(
		d.tick(),
		d.collectMetrics(),
	)
}

// Update handles messages
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		d.updateLayout()
		return d, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return d, tea.Quit
		case "r":
			return d, d.collectMetrics()
		case "h":
			// Cycle through help modes: 0 -> 1 -> 2 -> 3 -> 0
			d.helpMode = (d.helpMode + 1) % 4
			return d, nil
		}

	case tickMsg:
		return d, tea.Batch(d.tick(), d.collectMetrics())

	case metricsMsg:
		d.systemMetrics = msg.system
		d.tokenMetrics = msg.tokens
		d.tmuxMetrics = msg.tmux
		d.lastUpdate = time.Now()
		return d, nil

	case errMsg:
		d.err = msg.err
		return d, nil
	}

	return d, nil
}

// View renders the dashboard
func (d *Dashboard) View() string {
	if d.width == 0 {
		return "Initializing..."
	}

	var content string

	// Check if in help mode
	if d.helpMode > 0 {
		content = d.renderHelpView()
	} else {
		switch d.layoutMode {
		case LayoutUltraWide:
			content = d.renderUltraWide()
		case LayoutWide:
			content = d.renderWide()
		default:
			content = d.renderNarrow()
		}
	}

	// Add status bar
	statusBar := d.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, content, statusBar)
}

// updateLayout determines the current layout mode based on terminal size
func (d *Dashboard) updateLayout() {
	// Always use 3-column layout
	d.layoutMode = LayoutUltraWide
}

// tick returns a command that sends a tick message every 2 seconds
func (d *Dashboard) tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// metricsMsg carries collected metrics
type metricsMsg struct {
	system metrics.SystemMetrics
	tokens *metrics.TokenMetrics
	tmux   *metrics.TmuxMetrics
}

// errMsg carries errors
type errMsg struct {
	err error
}

// collectMetrics returns a command that collects all metrics
func (d *Dashboard) collectMetrics() tea.Cmd {
	return func() tea.Msg {
		system := d.systemCollector.Collect()
		tokens, _ := d.tokenCollector.Collect()
		tmux := d.tmuxCollector.Collect()

		return metricsMsg{
			system: system,
			tokens: tokens,
			tmux:   tmux,
		}
	}
}

// renderUltraWide renders 3 panels side-by-side
func (d *Dashboard) renderUltraWide() string {
	// Custom widths: system=65, token=48, tmux=90 (wider for session details)
	// Account for panel padding (0,1) which adds 2 chars per panel = 6 total
	totalPanelWidth := d.width - 6 // 202 - 6 = 196 for padding
	systemWidth := (totalPanelWidth * 32) / 100  // ~62 chars at 196 width
	tokenWidth := (totalPanelWidth * 24) / 100   // ~47 chars at 196 width
	tmuxWidth := totalPanelWidth - systemWidth - tokenWidth // Remaining space

	// Calculate panel content height (subtract status bar and borders)
	// Total height - status bar (1) - panel borders (2) = content height
	panelHeight := d.height - 3 // Leave room for status bar (already includes border space)

	systemPanel := d.renderSystemPanel(systemWidth, panelHeight)
	tokenPanel := d.renderTokenPanel(tokenWidth, panelHeight)
	tmuxPanel := d.renderTmuxPanel(tmuxWidth, panelHeight)

	// Ensure all panels align at top with equal heights
	// No separators - panel borders provide visual separation
	return lipgloss.JoinHorizontal(lipgloss.Top,
		systemPanel,
		tokenPanel,
		tmuxPanel,
	)
}

// renderWide renders 2 panels on top, 1 on bottom
func (d *Dashboard) renderWide() string {
	panelWidth := (d.width - 3) / 2 // 2 panels with spacing
	topHeight := (d.height - 4) / 2 // Split height
	bottomHeight := d.height - topHeight - 4

	systemPanel := d.renderSystemPanel(panelWidth, topHeight)
	tokenPanel := d.renderTokenPanel(panelWidth, topHeight)
	tmuxPanel := d.renderTmuxPanel(d.width-2, bottomHeight)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, systemPanel, " ", tokenPanel)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, tmuxPanel)
}

// renderNarrow renders panels stacked vertically
func (d *Dashboard) renderNarrow() string {
	panelWidth := d.width - 2
	panelHeight := (d.height - 5) / 3 // 3 panels stacked

	systemPanel := d.renderSystemPanel(panelWidth, panelHeight)
	tokenPanel := d.renderTokenPanel(panelWidth, panelHeight)
	tmuxPanel := d.renderTmuxPanel(panelWidth, panelHeight)

	return lipgloss.JoinVertical(lipgloss.Left,
		systemPanel,
		tokenPanel,
		tmuxPanel,
	)
}

// renderSystemPanel renders the system resources panel
func (d *Dashboard) renderSystemPanel(width, height int) string {
	style := panelStyle

	var lines []string

	// Title (with emoji like unified-dashboard)
	lines = append(lines, successStyle.Render("⚡ System Resources"))

	// Load average
	if d.systemMetrics.Load.Error == nil {
		lines = append(lines, fmt.Sprintf("Load: %.2f %.2f %.2f",
			d.systemMetrics.Load.Load1,
			d.systemMetrics.Load.Load5,
			d.systemMetrics.Load.Load15))
	} else {
		lines = append(lines, errorStyle.Render("Load: N/A"))
	}

	// CPU Total
	if d.systemMetrics.CPU.Error == nil {
		// Compact format: "CPU [|||||| 45.2%]" on one line
		lines = append(lines, fmt.Sprintf("CPU %s", d.renderBar(d.systemMetrics.CPU.TotalPercent, width-8)))

		// CPU per-core - use up to 6 lines for CPU display
		maxCoreLines := 6
		totalCores := len(d.systemMetrics.CPU.PerCore)

		// Determine cores per line
		var coresPerLine int
		if totalCores <= 6 {
			coresPerLine = 1 // One core per line - bars stretch full width
		} else {
			// Multiple cores per line - calculate how many fit
			// Assume minimum 15 chars per core (e.g., "0:[||| 42%] ")
			minCharsPerCore := 15
			coresPerLine = (width - 4) / minCharsPerCore
			if coresPerLine < 2 {
				coresPerLine = 2 // At least 2 per line when splitting
			}
		}

		// Calculate bar width based on cores per line
		// Available width divided by cores per line, minus label overhead
		availableWidth := width - 4 // Account for panel padding
		var barWidth int
		if coresPerLine == 1 {
			// Single core per line - stretch to full width
			// Format: "N:[|||||||...  XX%]"
			// Overhead: 2-3 for number, 2 for ":[", 5 for " XX%]" = ~9-10 chars
			maxCoreDigits := 2 // Support up to 99 cores
			barWidth = availableWidth - maxCoreDigits - 7 // -7 for ":[ XX%]"
			if barWidth < 10 {
				barWidth = 10
			}
		} else {
			// Multiple cores per line - split width evenly
			// Account for spaces between cores
			spacesBetween := coresPerLine - 1
			widthPerCore := (availableWidth - spacesBetween) / coresPerLine
			// Subtract label overhead
			barWidth = widthPerCore - 9 // -9 for "N:[ XX%]"
			if barWidth < 5 {
				barWidth = 5
			}
		}

		// Max cores we can display with 6 lines
		maxDisplayCores := coresPerLine * maxCoreLines
		maxCores := totalCores
		if maxCores > maxDisplayCores {
			maxCores = maxDisplayCores
		}

		var coreLine strings.Builder
		linesUsed := 0
		for i := 0; i < maxCores; i++ {
			if i > 0 && i%coresPerLine == 0 {
				// Start new line
				lines = append(lines, coreLine.String())
				coreLine.Reset()
				linesUsed++
				if linesUsed >= maxCoreLines {
					break
				}
			}
			if coreLine.Len() > 0 {
				coreLine.WriteString(" ")
			}
			// Render progress bar for this core with calculated width
			percent := d.systemMetrics.CPU.PerCore[i]
			miniBar := d.renderMiniBar(percent, barWidth)
			coreLine.WriteString(fmt.Sprintf("%d:[%s]", i, miniBar))
		}
		// Add remaining cores on current line
		if coreLine.Len() > 0 {
			lines = append(lines, coreLine.String())
		}

		if totalCores > maxCores {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("+%d more cores", totalCores-maxCores)))
		}
	} else {
		lines = append(lines, errorStyle.Render("CPU: N/A"))
	}

	// Memory - always compact (one line) - use shorter bar to prevent wrapping
	if d.systemMetrics.Memory.Error == nil {
		memUsed := metrics.FormatBytes(d.systemMetrics.Memory.Used)
		memTotal := metrics.FormatBytes(d.systemMetrics.Memory.Total)
		// Calculate bar width: total width - "Mem " - " " - "XX.XG/XX.XG" - margins
		barWidth := width - 8 - len(memUsed) - len(memTotal)
		if barWidth < 10 {
			barWidth = 10
		}
		lines = append(lines, fmt.Sprintf("Mem %s %s/%s",
			d.renderBar(d.systemMetrics.Memory.Percentage, barWidth),
			memUsed, memTotal))
	} else {
		lines = append(lines, errorStyle.Render("Mem: N/A"))
	}

	// Swap - always compact (one line)
	if d.systemMetrics.Swap.Error == nil && d.systemMetrics.Swap.Total > 0 {
		swpUsed := metrics.FormatBytes(d.systemMetrics.Swap.Used)
		swpTotal := metrics.FormatBytes(d.systemMetrics.Swap.Total)
		barWidth := width - 8 - len(swpUsed) - len(swpTotal)
		if barWidth < 10 {
			barWidth = 10
		}
		lines = append(lines, fmt.Sprintf("Swp %s %s/%s",
			d.renderBar(d.systemMetrics.Swap.Percentage, barWidth),
			swpUsed, swpTotal))
	}

	// Disk I/O - verbose format with pipe separators
	if d.systemMetrics.DiskIO.Error == nil {
		lines = append(lines, fmt.Sprintf("Disk I/O | Read: %s | Write: %s",
			metrics.FormatRate(d.systemMetrics.DiskIO.ReadBytesPerSec),
			metrics.FormatRate(d.systemMetrics.DiskIO.WriteBytesPerSec)))
	} else {
		lines = append(lines, errorStyle.Render("Disk I/O | N/A"))
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// renderTokenPanel renders the token usage panel
func (d *Dashboard) renderTokenPanel(width, height int) string {
	style := panelStyle

	if d.tokenMetrics == nil {
		return style.Width(width).Height(height).Render("Loading token metrics...")
	}

	var lines []string

	// Title (with emoji like unified-dashboard)
	lines = append(lines, successStyle.Render("💰 Token Usage"))

	if !d.tokenMetrics.Available {
		lines = append(lines, errorStyle.Render("Not Available"))
		if d.tokenMetrics.Error != "" {
			lines = append(lines, wrapText(d.tokenMetrics.Error, width-4))
		}
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	// Token counts - always compact format
	lines = append(lines, fmt.Sprintf("In:  %s", metrics.FormatTokens(d.tokenMetrics.InputTokens)))
	lines = append(lines, fmt.Sprintf("Out: %s", metrics.FormatTokens(d.tokenMetrics.OutputTokens)))

	// Cache on separate lines
	if d.tokenMetrics.CacheReadTokens > 0 {
		lines = append(lines, fmt.Sprintf("Cache Read: %s",
			metrics.FormatTokens(d.tokenMetrics.CacheReadTokens)))
	}
	if d.tokenMetrics.CacheCreationTokens > 0 {
		lines = append(lines, fmt.Sprintf("Cache Create: %s",
			metrics.FormatTokens(d.tokenMetrics.CacheCreationTokens)))
	}

	lines = append(lines, fmt.Sprintf("Total: %s",
		boldStyle.Render(metrics.FormatTokens(d.tokenMetrics.TotalTokens))))
	lines = append(lines, fmt.Sprintf("Cost:  %s",
		costStyle.Render(metrics.FormatCost(d.tokenMetrics.TotalCost))))

	// Compact rates
	if d.tokenMetrics.Rate > 0 {
		lines = append(lines, fmt.Sprintf("Rate: %s", metrics.FormatTokenRate(d.tokenMetrics.Rate)))
	}

	// Session average
	if d.tokenMetrics.SessionAvgRate > 0 {
		lines = append(lines, fmt.Sprintf("Avg: %s", metrics.FormatTokenRate(d.tokenMetrics.SessionAvgRate)))
	}

	// Session duration (how long ago session started)
	if !d.tokenMetrics.EarliestTimestamp.IsZero() {
		duration := time.Since(d.tokenMetrics.EarliestTimestamp)
		lines = append(lines, fmt.Sprintf("Started: %s ago", formatDuration(duration)))
	}

	// Models with label and indentation - one per line
	if len(d.tokenMetrics.Models) > 0 {
		lines = append(lines, "Models:")
		indent := "  "
		for _, model := range d.tokenMetrics.Models {
			// Truncate if model name is too long
			maxLen := width - len(indent) - 4
			if len(model) > maxLen {
				model = model[:maxLen-3] + "..."
			}
			lines = append(lines, dimStyle.Render(indent+model))
		}
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// renderTmuxPanel renders the tmux sessions panel
func (d *Dashboard) renderTmuxPanel(width, height int) string {
	style := panelStyle

	if d.tmuxMetrics == nil {
		return style.Width(width).Height(height).Render("Loading tmux metrics...")
	}

	var lines []string

	// Title (with emoji like unified-dashboard)
	lines = append(lines, successStyle.Render("📺 TMUX Sessions"))

	if !d.tmuxMetrics.Available {
		lines = append(lines, errorStyle.Render("Not Available"))
		if d.tmuxMetrics.Error != "" {
			lines = append(lines, wrapText(d.tmuxMetrics.Error, width-4))
		}
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	// Session count
	lines = append(lines, fmt.Sprintf("Total: %d", d.tmuxMetrics.Total))

	if len(d.tmuxMetrics.Sessions) == 0 {
		lines = append(lines, "No active sessions")
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	// Render sessions in grid layout
	// Determine grid columns based on width
	cols := 1
	if width >= 160 {
		cols = 3
	} else if width >= 100 {
		cols = 2
	}

	// Calculate available lines for sessions
	// height includes borders, subtract: title(1) + blank(1) + total(1) + padding(2) = 5 lines overhead
	availableLines := height - 5
	if availableLines < 1 {
		availableLines = 1
	}

	// Calculate how many sessions we can display
	maxSessions := len(d.tmuxMetrics.Sessions)
	maxDisplayed := availableLines * cols
	if maxSessions > maxDisplayed {
		maxSessions = maxDisplayed
	}

	// Calculate cell width
	// Width includes borders and padding, so subtract them
	contentWidth := width - 4 // -4 for borders (2) and padding (2)
	cellWidth := (contentWidth - (cols - 1)) / cols

	// Render sessions in vertical columns (not horizontal rows)
	rowCount := (maxSessions + cols - 1) / cols
	for row := 0; row < rowCount; row++ {
		var rowCells []string
		for col := 0; col < cols; col++ {
			idx := col*rowCount + row
			if idx < maxSessions {
				session := d.tmuxMetrics.Sessions[idx]
				cellContent := d.renderSessionCell(session, cellWidth)
				// Apply explicit width constraint using lipgloss MaxWidth
				cellStyle := lipgloss.NewStyle().MaxWidth(cellWidth)
				cell := cellStyle.Render(cellContent)
				rowCells = append(rowCells, cell)
			} else {
				// Empty cell for alignment
				emptyCell := lipgloss.NewStyle().Width(cellWidth).Render("")
				rowCells = append(rowCells, emptyCell)
			}
		}
		// Join cells with no separator for single column, or with space for multiple
		separator := ""
		if cols > 1 {
			separator = " "
		}
		if len(rowCells) == 1 {
			lines = append(lines, rowCells[0])
		} else {
			lines = append(lines, strings.Join(rowCells, separator))
		}
	}

	// Show "... and X more" if sessions were limited
	if maxSessions < len(d.tmuxMetrics.Sessions) {
		remaining := len(d.tmuxMetrics.Sessions) - maxSessions
		lines = append(lines, dimStyle.Render(fmt.Sprintf("... +%d more", remaining)))
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// renderSessionCell renders a single tmux session cell
func (d *Dashboard) renderSessionCell(session metrics.TmuxSession, width int) string {
	emoji := session.Status.GetEmoji()

	// Convert ANSI color codes to hex colors for lipgloss
	colorMap := map[string]string{
		"\033[32m": "#00ff00", // Green - WORKING (Claude processing)
		"\033[31m": "#ff0000", // Red - READY (Waiting for input)
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

	name := session.Name
	if len(name) > width-6 {
		name = name[:width-9] + "..."
	}

	attached := ""
	if session.Attached {
		attached = "📎"
	}

	// Format: emoji name status windows idle attached
	statusText := string(session.Status)
	if len(statusText) > 7 {
		statusText = statusText[:7]
	}

	// Format idle duration
	idleStr := ""
	if session.IdleDuration > 0 {
		if session.IdleDuration < time.Minute {
			idleStr = fmt.Sprintf("%ds", int(session.IdleDuration.Seconds()))
		} else if session.IdleDuration < time.Hour {
			idleStr = fmt.Sprintf("%dm", int(session.IdleDuration.Minutes()))
		} else {
			idleStr = fmt.Sprintf("%dh", int(session.IdleDuration.Hours()))
		}
	}

	// "win" = windows, idle = time content unchanged
	// Be more conservative with spacing to account for emoji display width
	line := fmt.Sprintf("%s %-9s %s %dw %-3s %s",
		emoji,
		name,
		statusStyle.Render(fmt.Sprintf("%-7s", statusText)),
		session.Windows,
		idleStr,
		attached)

	// Emojis display as 2 columns in terminal but len() counts bytes
	// Estimate display width: add extra chars for emojis
	estimatedDisplayWidth := len(line) + 2 // +2 for emoji display width overhead

	// Ensure line doesn't exceed width
	if estimatedDisplayWidth > width {
		// Truncate name if line is too long
		maxNameLen := 6
		if len(name) > maxNameLen {
			name = name[:maxNameLen]
		}
		line = fmt.Sprintf("%s %-6s %s %dw %-3s %s",
			emoji,
			name,
			statusStyle.Render(fmt.Sprintf("%-7s", statusText)),
			session.Windows,
			idleStr,
			attached)
	}

	return line
}

// renderHelpView renders the help screen for the current help mode
func (d *Dashboard) renderHelpView() string {
	panelHeight := d.height - 3
	totalPanelWidth := d.width - 2 // Match normal view width calculation
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

Disk I/O: Read/write speeds in bytes/s or KB/s

Load: 1min, 5min, 15min averages
  Indicates overall system activity level`

	case 2: // Token Usage
		title = "Token Usage Panel"
		panel = d.renderTokenPanel(panelWidth, panelHeight)
		helpText = `Tracks Claude Code token usage from ~/.claude/projects:

Tokens:
  In/Out: Input/output tokens
  Cache Read/Create: Cache operations
  Total: All tokens combined
  Cost: Estimated API cost ($)

Rates:
  Rate: Current tok/min (60s window)
  Avg: Session average tok/min
  Started: Session duration

Models: Claude models used (one per line)
  Example: claude-sonnet-4-5-20250929

Data aggregated from all JSONL files in projects dir.`

	case 3: // TMUX Sessions
		title = "TMUX Sessions Panel"
		panel = d.renderTmuxPanel(panelWidth, panelHeight)
		helpText = `Monitors tmux sessions running Claude Code:

Status (analyzes pane content):
  🟢 WORKING - Claude Code processing
  🔴 READY - Waiting for user input
  🟡 ACTIVE - User in session
  ⚠️  ERROR - Error or undefined state

Detection: Analyzes pane for tool usage,
  prompts, errors, and activity

Info: Name, status, windows (Xw), idle time, 📎=attached

Idle: Shows how long content unchanged
  (seconds/minutes/hours)

Display: Vertically aligned`
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

// renderStatusBar renders the bottom status bar (single line, compact)
func (d *Dashboard) renderStatusBar() string {
	// Compact format: time + version, github, dimensions, shortcuts - all on one line
	left := fmt.Sprintf("%s v%s", d.lastUpdate.Format("15:04:05"), d.version)
	middle := dimStyle.Render("github.com/jedarden/ccdash")
	right := fmt.Sprintf("%dx%d h:help q:quit r:refresh", d.width, d.height)

	// Calculate spacing (account for statusBarStyle padding of 2 chars)
	totalContent := lipgloss.Width(left) + lipgloss.Width(middle) + lipgloss.Width(right)
	availableSpace := d.width - totalContent - 2 // -2 for padding

	if availableSpace < 4 {
		// Not enough space, use ultra-compact format
		return statusBarStyle.Render(fmt.Sprintf("%s v%s %dx%d h q r",
			d.lastUpdate.Format("15:04"), d.version, d.width, d.height))
	}

	// Distribute space evenly on both sides of middle
	leftSpacer := strings.Repeat(" ", max(0, availableSpace/2))
	rightSpacer := strings.Repeat(" ", max(0, availableSpace-availableSpace/2))

	statusLine := left + leftSpacer + middle + rightSpacer + right

	return statusBarStyle.Render(statusLine)
}

// renderBar renders a progress bar with percentage inside (unified-dashboard style)
func (d *Dashboard) renderBar(percent float64, width int) string {
	if width < 10 {
		return ""
	}

	// Determine color based on threshold (unified-dashboard style)
	var color string
	if percent >= 95 {
		color = "#ff0000" // Red
	} else if percent >= 80 {
		color = "#ffaa00" // Orange
	} else if percent >= 60 {
		color = "#ffff00" // Yellow
	} else {
		color = "#00ff00" // Green
	}

	// Format percentage
	percentText := fmt.Sprintf("%.1f%%", percent)

	// Calculate fill width (accounting for percentage text)
	barWidth := width - 2 // Account for brackets
	availableWidth := barWidth - len(percentText) - 1 // -1 for space before %
	fillWidth := int(percent / 100.0 * float64(availableWidth))
	if fillWidth > availableWidth {
		fillWidth = availableWidth
	}
	if fillWidth < 0 {
		fillWidth = 0
	}

	// Create filled and empty portions (vertical bar style like unified-dashboard)
	filled := strings.Repeat("|", fillWidth)
	empty := strings.Repeat(" ", availableWidth-fillWidth)

	// Apply styling
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	var bar strings.Builder
	bar.WriteString("[")
	bar.WriteString(barStyle.Render(filled))
	bar.WriteString(dimStyle.Render(empty))
	bar.WriteString(" ")
	bar.WriteString(percentText)
	bar.WriteString("]")

	return bar.String()
}

// renderMiniBar creates a compact progress bar with percentage inside
// Format: "||| 42%" for use in CPU core display
func (d *Dashboard) renderMiniBar(percent float64, barWidth int) string {
	// Determine color based on threshold
	var color string
	if percent >= 95 {
		color = "#ff0000" // Red
	} else if percent >= 80 {
		color = "#ffaa00" // Orange
	} else if percent >= 60 {
		color = "#ffff00" // Yellow
	} else {
		color = "#00ff00" // Green
	}

	// Format percentage as "42%" (3 chars max for <100%)
	percentText := fmt.Sprintf("%.0f%%", percent)

	// Calculate fill width
	availableWidth := barWidth
	fillWidth := int(percent / 100.0 * float64(availableWidth))
	if fillWidth > availableWidth {
		fillWidth = availableWidth
	}
	if fillWidth < 0 {
		fillWidth = 0
	}

	// Create filled and empty portions
	filled := strings.Repeat("|", fillWidth)
	empty := strings.Repeat(" ", availableWidth-fillWidth)

	// Apply styling
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	return barStyle.Render(filled) + dimStyle.Render(empty) + " " + percentText
}

// Styles (unified-dashboard palette)
var (
	panelStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00aaff")).
		Padding(0, 1) // Minimal padding for compact display

	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00ffff")).
		MarginBottom(1)

	boldStyle = lipgloss.NewStyle().
		Bold(true)

	successStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00ff00"))

	costStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffaa00")).
		Bold(true)

	warningStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffaa00"))

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ff0000")).
		Bold(true)

	dimStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	statusBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00ffff")).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(0, 1)
)

// Utility functions

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func wrapText(text string, width int) string {
	if len(text) <= width {
		return text
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if len(currentLine)+len(word)+1 <= width {
			if currentLine == "" {
				currentLine = word
			} else {
				currentLine += " " + word
			}
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return strings.Join(lines, "\n")
}

// wrapTextPreserveBreaks wraps text while preserving explicit line breaks
func wrapTextPreserveBreaks(text string, width int) string {
	// Split by newlines to preserve paragraph structure
	paragraphs := strings.Split(text, "\n")
	var wrappedParagraphs []string

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			// Preserve empty lines
			wrappedParagraphs = append(wrappedParagraphs, "")
			continue
		}

		// Wrap this paragraph
		if len(para) <= width {
			wrappedParagraphs = append(wrappedParagraphs, para)
			continue
		}

		// Word wrap this line
		words := strings.Fields(para)
		currentLine := ""
		for _, word := range words {
			if currentLine == "" {
				currentLine = word
			} else if len(currentLine)+len(word)+1 <= width {
				currentLine += " " + word
			} else {
				wrappedParagraphs = append(wrappedParagraphs, currentLine)
				currentLine = word
			}
		}
		if currentLine != "" {
			wrappedParagraphs = append(wrappedParagraphs, currentLine)
		}
	}

	return strings.Join(wrappedParagraphs, "\n")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
