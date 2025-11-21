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

// Panel focus constants
type PanelFocus int

const (
	PanelSystem PanelFocus = iota
	PanelTokens
	PanelTmux
)

// tickMsg is sent every 2 seconds to trigger refresh
type tickMsg time.Time

// Dashboard is the main Bubble Tea model
type Dashboard struct {
	width         int
	height        int
	layoutMode    LayoutMode
	focusedPanel  PanelFocus

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
}

// NewDashboard creates a new dashboard model
func NewDashboard() *Dashboard {
	return &Dashboard{
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
		case "1":
			d.focusedPanel = PanelSystem
			return d, nil
		case "2":
			d.focusedPanel = PanelTokens
			return d, nil
		case "3":
			d.focusedPanel = PanelTmux
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

	switch d.layoutMode {
	case LayoutUltraWide:
		content = d.renderUltraWide()
	case LayoutWide:
		content = d.renderWide()
	default:
		content = d.renderNarrow()
	}

	// Add status bar
	statusBar := d.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, content, statusBar)
}

// updateLayout determines the current layout mode based on terminal size
func (d *Dashboard) updateLayout() {
	if d.width >= 240 {
		d.layoutMode = LayoutUltraWide
	} else if d.width >= 120 && d.height >= 30 {
		d.layoutMode = LayoutWide
	} else {
		d.layoutMode = LayoutNarrow
	}
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
	panelWidth := (d.width - 6) / 3 // 3 panels with spacing
	panelHeight := d.height - 3     // Leave room for status bar

	systemPanel := d.renderSystemPanel(panelWidth, panelHeight, d.focusedPanel == PanelSystem)
	tokenPanel := d.renderTokenPanel(panelWidth, panelHeight, d.focusedPanel == PanelTokens)
	tmuxPanel := d.renderTmuxPanel(panelWidth, panelHeight, d.focusedPanel == PanelTmux)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		systemPanel,
		" ",
		tokenPanel,
		" ",
		tmuxPanel,
	)
}

// renderWide renders 2 panels on top, 1 on bottom
func (d *Dashboard) renderWide() string {
	panelWidth := (d.width - 3) / 2 // 2 panels with spacing
	topHeight := (d.height - 4) / 2 // Split height
	bottomHeight := d.height - topHeight - 4

	systemPanel := d.renderSystemPanel(panelWidth, topHeight, d.focusedPanel == PanelSystem)
	tokenPanel := d.renderTokenPanel(panelWidth, topHeight, d.focusedPanel == PanelTokens)
	tmuxPanel := d.renderTmuxPanel(d.width-2, bottomHeight, d.focusedPanel == PanelTmux)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, systemPanel, " ", tokenPanel)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, tmuxPanel)
}

// renderNarrow renders panels stacked vertically
func (d *Dashboard) renderNarrow() string {
	panelWidth := d.width - 2
	panelHeight := (d.height - 5) / 3 // 3 panels stacked

	systemPanel := d.renderSystemPanel(panelWidth, panelHeight, d.focusedPanel == PanelSystem)
	tokenPanel := d.renderTokenPanel(panelWidth, panelHeight, d.focusedPanel == PanelTokens)
	tmuxPanel := d.renderTmuxPanel(panelWidth, panelHeight, d.focusedPanel == PanelTmux)

	return lipgloss.JoinVertical(lipgloss.Left,
		systemPanel,
		tokenPanel,
		tmuxPanel,
	)
}

// renderSystemPanel renders the system resources panel
func (d *Dashboard) renderSystemPanel(width, height int, focused bool) string {
	style := d.panelStyle(focused)

	var lines []string

	// Title
	lines = append(lines, titleStyle.Render("SYSTEM RESOURCES"))
	lines = append(lines, "")

	// Load average
	if d.systemMetrics.Load.Error == nil {
		lines = append(lines, fmt.Sprintf("Load: %.2f %.2f %.2f",
			d.systemMetrics.Load.Load1,
			d.systemMetrics.Load.Load5,
			d.systemMetrics.Load.Load15))
	} else {
		lines = append(lines, errorStyle.Render("Load: N/A"))
	}
	lines = append(lines, "")

	// CPU Total
	if d.systemMetrics.CPU.Error == nil {
		lines = append(lines, "CPU Total:")
		lines = append(lines, d.renderBar(d.systemMetrics.CPU.TotalPercent, width-4))
		lines = append(lines, "")

		// CPU per-core (up to 8 cores to save space)
		maxCores := len(d.systemMetrics.CPU.PerCore)
		if maxCores > 8 {
			maxCores = 8
		}

		for i := 0; i < maxCores; i++ {
			lines = append(lines, fmt.Sprintf("CPU %d:", i))
			lines = append(lines, d.renderBar(d.systemMetrics.CPU.PerCore[i], width-4))
		}

		if len(d.systemMetrics.CPU.PerCore) > 8 {
			lines = append(lines, fmt.Sprintf("... and %d more cores", len(d.systemMetrics.CPU.PerCore)-8))
		}
		lines = append(lines, "")
	} else {
		lines = append(lines, errorStyle.Render("CPU: N/A"))
		lines = append(lines, "")
	}

	// Memory
	if d.systemMetrics.Memory.Error == nil {
		lines = append(lines, "Memory:")
		lines = append(lines, d.renderBar(d.systemMetrics.Memory.Percentage, width-4))
		lines = append(lines, fmt.Sprintf("  %s / %s",
			metrics.FormatBytes(d.systemMetrics.Memory.Used),
			metrics.FormatBytes(d.systemMetrics.Memory.Total)))
		lines = append(lines, "")
	} else {
		lines = append(lines, errorStyle.Render("Memory: N/A"))
		lines = append(lines, "")
	}

	// Swap
	if d.systemMetrics.Swap.Error == nil && d.systemMetrics.Swap.Total > 0 {
		lines = append(lines, "Swap:")
		lines = append(lines, d.renderBar(d.systemMetrics.Swap.Percentage, width-4))
		lines = append(lines, fmt.Sprintf("  %s / %s",
			metrics.FormatBytes(d.systemMetrics.Swap.Used),
			metrics.FormatBytes(d.systemMetrics.Swap.Total)))
		lines = append(lines, "")
	}

	// Disk I/O
	if d.systemMetrics.DiskIO.Error == nil {
		lines = append(lines, "Disk I/O:")
		lines = append(lines, fmt.Sprintf("  R: %s  W: %s",
			metrics.FormatRate(d.systemMetrics.DiskIO.ReadBytesPerSec),
			metrics.FormatRate(d.systemMetrics.DiskIO.WriteBytesPerSec)))
	} else {
		lines = append(lines, errorStyle.Render("Disk I/O: N/A"))
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// renderTokenPanel renders the token usage panel
func (d *Dashboard) renderTokenPanel(width, height int, focused bool) string {
	style := d.panelStyle(focused)

	if d.tokenMetrics == nil {
		return style.Width(width).Height(height).Render("Loading token metrics...")
	}

	var lines []string

	// Title
	lines = append(lines, titleStyle.Render("TOKEN USAGE"))
	lines = append(lines, "")

	if !d.tokenMetrics.Available {
		lines = append(lines, errorStyle.Render("Not Available"))
		if d.tokenMetrics.Error != "" {
			lines = append(lines, "")
			lines = append(lines, wrapText(d.tokenMetrics.Error, width-4))
		}
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	// Token counts
	lines = append(lines, fmt.Sprintf("Input Tokens:        %s", metrics.FormatTokens(d.tokenMetrics.InputTokens)))
	lines = append(lines, fmt.Sprintf("Output Tokens:       %s", metrics.FormatTokens(d.tokenMetrics.OutputTokens)))
	lines = append(lines, fmt.Sprintf("Cache Creation:      %s", metrics.FormatTokens(d.tokenMetrics.CacheCreationTokens)))
	lines = append(lines, fmt.Sprintf("Cache Read:          %s", metrics.FormatTokens(d.tokenMetrics.CacheReadTokens)))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Total Tokens:        %s",
		boldStyle.Render(metrics.FormatTokens(d.tokenMetrics.TotalTokens))))
	lines = append(lines, "")

	// Cost
	lines = append(lines, fmt.Sprintf("Total Cost:          %s",
		costStyle.Render(metrics.FormatCost(d.tokenMetrics.TotalCost))))
	lines = append(lines, "")

	// Rates
	lines = append(lines, fmt.Sprintf("Current Rate:        %s",
		metrics.FormatTokenRate(d.tokenMetrics.Rate)))
	lines = append(lines, fmt.Sprintf("Session Avg:         %s",
		metrics.FormatTokenRate(d.tokenMetrics.SessionAvgRate)))
	lines = append(lines, "")

	// Time span
	if d.tokenMetrics.TimeSpan > 0 {
		lines = append(lines, fmt.Sprintf("Time Span:           %s",
			formatDuration(d.tokenMetrics.TimeSpan)))
		lines = append(lines, "")
	}

	// Models
	if len(d.tokenMetrics.Models) > 0 {
		lines = append(lines, "Models:")
		for _, model := range d.tokenMetrics.Models {
			// Truncate long model names
			if len(model) > width-6 {
				model = model[:width-9] + "..."
			}
			lines = append(lines, fmt.Sprintf("  - %s", model))
		}
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// renderTmuxPanel renders the tmux sessions panel
func (d *Dashboard) renderTmuxPanel(width, height int, focused bool) string {
	style := d.panelStyle(focused)

	if d.tmuxMetrics == nil {
		return style.Width(width).Height(height).Render("Loading tmux metrics...")
	}

	var lines []string

	// Title
	lines = append(lines, titleStyle.Render("TMUX SESSIONS"))
	lines = append(lines, "")

	if !d.tmuxMetrics.Available {
		lines = append(lines, errorStyle.Render("Not Available"))
		if d.tmuxMetrics.Error != "" {
			lines = append(lines, "")
			lines = append(lines, wrapText(d.tmuxMetrics.Error, width-4))
		}
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	// Session count
	lines = append(lines, fmt.Sprintf("Total Sessions: %d", d.tmuxMetrics.Total))
	lines = append(lines, "")

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

	// Calculate cell width
	cellWidth := (width - 4 - (cols - 1)) / cols

	// Render sessions
	for i := 0; i < len(d.tmuxMetrics.Sessions); i += cols {
		var row []string
		for j := 0; j < cols && i+j < len(d.tmuxMetrics.Sessions); j++ {
			session := d.tmuxMetrics.Sessions[i+j]
			cell := d.renderSessionCell(session, cellWidth)
			row = append(row, cell)
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, row...))
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// renderSessionCell renders a single tmux session cell
func (d *Dashboard) renderSessionCell(session metrics.TmuxSession, width int) string {
	emoji := session.Status.GetEmoji()

	// Convert ANSI color codes to hex colors for lipgloss
	colorMap := map[string]string{
		"\033[32m": "#00ff00", // Green
		"\033[33m": "#ffff00", // Yellow
		"\033[90m": "#888888", // Gray
		"\033[36m": "#00ffff", // Cyan
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
		attached = " [A]"
	}

	// Calculate age from creation time
	age := time.Since(session.Created)
	ageStr := formatDuration(age)

	line := fmt.Sprintf("%s %s%s w:%d %s",
		emoji,
		name,
		attached,
		session.Windows,
		ageStr)

	return statusStyle.Width(width).Render(line)
}

// renderStatusBar renders the bottom status bar
func (d *Dashboard) renderStatusBar() string {
	left := fmt.Sprintf("Last Update: %s", d.lastUpdate.Format("15:04:05"))

	shortcuts := "q:quit  r:refresh  1:system  2:tokens  3:tmux"

	right := shortcuts

	// Calculate spacing
	spacer := strings.Repeat(" ", max(0, d.width-lipgloss.Width(left)-lipgloss.Width(right)))

	statusLine := left + spacer + right

	return statusBarStyle.Width(d.width).Render(statusLine)
}

// renderBar renders a progress bar with percentage inside
func (d *Dashboard) renderBar(percent float64, width int) string {
	if width < 10 {
		return ""
	}

	// Determine color based on threshold
	color := getStatusColor(percent, 60.0, 80.0)

	// Calculate filled width (reserve space for percentage text)
	percentText := fmt.Sprintf(" %.1f%% ", percent)
	barWidth := width - 2 // Account for borders
	filledWidth := int(float64(barWidth) * percent / 100.0)

	// Create bar characters
	filled := strings.Repeat("█", filledWidth)
	empty := strings.Repeat("░", barWidth-filledWidth)
	bar := filled + empty

	// Overlay percentage text in the middle
	textPos := (barWidth - len(percentText)) / 2
	if textPos < 0 {
		textPos = 0
	}

	barRunes := []rune(bar)
	textRunes := []rune(percentText)

	for i, r := range textRunes {
		if textPos+i < len(barRunes) {
			barRunes[textPos+i] = r
		}
	}

	barWithText := string(barRunes)

	// Apply color
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))

	return barStyle.Render("[" + barWithText + "]")
}

// panelStyle returns the style for a panel
func (d *Dashboard) panelStyle(focused bool) lipgloss.Style {
	style := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		Padding(1)

	if focused {
		style = style.BorderForeground(lipgloss.Color("#00ff00"))
	} else {
		style = style.BorderForeground(lipgloss.Color("#888888"))
	}

	return style
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00ffff")).
		MarginBottom(1)

	boldStyle = lipgloss.NewStyle().
		Bold(true)

	costStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffaa00")).
		Bold(true)

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ff0000")).
		Bold(true)

	statusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#333333")).
		Foreground(lipgloss.Color("#ffffff")).
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

// getStatusColor returns a color based on percentage thresholds
func getStatusColor(percent float64, warningThreshold, criticalThreshold float64) string {
	if percent >= criticalThreshold {
		return "#ff0000" // Red
	} else if percent >= warningThreshold {
		return "#ffaa00" // Orange
	}
	return "#00ff00" // Green
}
