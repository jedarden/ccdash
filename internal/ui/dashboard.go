package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
	"github.com/jedarden/ccdash/internal/updater"
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

// LookbackPreset represents a predefined lookback period
type LookbackPreset struct {
	Name        string
	Description string
	GetTime     func() time.Time
}

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

	// Lookback picker state
	lookbackMode          bool   // true when lookback picker is open
	lookbackPresets       []LookbackPreset
	lookbackSelectedIndex int
	lookbackCustomMode    bool      // true when editing custom date/time
	lookbackCustomDate    time.Time // the custom date being edited
	lookbackEditField     int       // 0=year, 1=month, 2=day, 3=hour, 4=minute

	// Update checking
	updater      *updater.Updater
	updateInfo   *updater.UpdateInfo
	updating     bool
	updateStatus string
}

// NewDashboard creates a new dashboard model with default Monday 9am lookback
func NewDashboard(version string) *Dashboard {
	presets := []LookbackPreset{
		{
			Name:        "Monday 9am",
			Description: "Since this week's Monday at 9:00 AM",
			GetTime:     metrics.GetMondayNineAM,
		},
		{
			Name:        "Today",
			Description: "Since midnight today",
			GetTime: func() time.Time {
				now := time.Now()
				return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			},
		},
		{
			Name:        "24 hours",
			Description: "Last 24 hours",
			GetTime: func() time.Time {
				return time.Now().Add(-24 * time.Hour)
			},
		},
		{
			Name:        "7 days",
			Description: "Last 7 days",
			GetTime: func() time.Time {
				return time.Now().AddDate(0, 0, -7)
			},
		},
		{
			Name:        "30 days",
			Description: "Last 30 days",
			GetTime: func() time.Time {
				return time.Now().AddDate(0, 0, -30)
			},
		},
		{
			Name:        "All time",
			Description: "Show all available data",
			GetTime: func() time.Time {
				return time.Time{} // Zero time = no filter
			},
		},
		{
			Name:        "Custom...",
			Description: "Set a custom date and time",
			GetTime:     nil, // Special case: opens custom picker
		},
	}

	return &Dashboard{
		version:            version,
		systemCollector:    metrics.NewSystemCollector(),
		tokenCollector:     metrics.NewTokenCollector(),
		tmuxCollector:      metrics.NewTmuxCollector(),
		updater:            updater.NewUpdater(version),
		lastUpdate:         time.Now(),
		lookbackPresets:    presets,
		lookbackCustomDate: time.Now().AddDate(0, 0, -1), // Default custom to yesterday
	}
}

// Init initializes the dashboard
func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(
		d.tick(),
		d.collectMetrics(),
		d.checkForUpdates(),
	)
}

// updateCheckMsg carries update check results
type updateCheckMsg struct {
	info *updater.UpdateInfo
}

// updateCompleteMsg indicates update was applied
type updateCompleteMsg struct {
	err error
}

// checkForUpdates returns a command that checks for updates
func (d *Dashboard) checkForUpdates() tea.Cmd {
	return func() tea.Msg {
		info := d.updater.CheckForUpdate()
		return updateCheckMsg{info: info}
	}
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
		// Handle lookback picker mode
		if d.lookbackMode {
			return d.handleLookbackKey(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return d, tea.Quit
		case "r":
			return d, d.collectMetrics()
		case "h":
			// Cycle through help modes: 0 -> 1 -> 2 -> 3 -> 0
			d.helpMode = (d.helpMode + 1) % 4
			return d, nil
		case "l", "L":
			// Open lookback picker
			d.lookbackMode = true
			d.helpMode = 0 // Close help if open
			return d, nil
		case "u", "U":
			// Perform update if available
			if d.updateInfo != nil && d.updateInfo.UpdateAvailable && !d.updating {
				d.updating = true
				d.updateStatus = "Downloading update..."
				return d, d.performUpdate()
			}
			return d, nil
		}

	case tickMsg:
		return d, tea.Batch(d.tick(), d.collectMetrics(), d.checkForUpdates())

	case metricsMsg:
		d.systemMetrics = msg.system
		d.tokenMetrics = msg.tokens
		d.tmuxMetrics = msg.tmux
		d.lastUpdate = time.Now()
		return d, nil

	case updateCheckMsg:
		d.updateInfo = msg.info
		return d, nil

	case updateCompleteMsg:
		d.updating = false
		if msg.err != nil {
			d.updateStatus = fmt.Sprintf("Update failed: %v", msg.err)
		} else {
			d.updateStatus = "Update complete! Restarting..."
			// The app should restart automatically
			return d, tea.Quit
		}
		return d, nil

	case errMsg:
		d.err = msg.err
		return d, nil
	}

	return d, nil
}

// performUpdate returns a command that applies the update
func (d *Dashboard) performUpdate() tea.Cmd {
	return func() tea.Msg {
		err := d.updater.PerformUpdateWithRestart(d.updateInfo)
		return updateCompleteMsg{err: err}
	}
}

// handleLookbackKey handles keyboard input when lookback picker is open
func (d *Dashboard) handleLookbackKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if d.lookbackCustomMode {
		// Custom date/time editing mode
		switch msg.String() {
		case "esc":
			d.lookbackCustomMode = false
			return d, nil
		case "enter":
			// Apply custom date and close picker
			d.tokenCollector.SetLookback(d.lookbackCustomDate)
			d.lookbackCustomMode = false
			d.lookbackMode = false
			return d, d.collectMetrics()
		case "tab", "right":
			d.lookbackEditField = (d.lookbackEditField + 1) % 5
			return d, nil
		case "shift+tab", "left":
			d.lookbackEditField = (d.lookbackEditField + 4) % 5
			return d, nil
		case "up":
			d.adjustCustomDate(1)
			return d, nil
		case "down":
			d.adjustCustomDate(-1)
			return d, nil
		}
		return d, nil
	}

	// Preset selection mode
	switch msg.String() {
	case "esc", "l", "q":
		d.lookbackMode = false
		return d, nil
	case "up", "k":
		if d.lookbackSelectedIndex > 0 {
			d.lookbackSelectedIndex--
		}
		return d, nil
	case "down", "j":
		if d.lookbackSelectedIndex < len(d.lookbackPresets)-1 {
			d.lookbackSelectedIndex++
		}
		return d, nil
	case "enter", " ":
		preset := d.lookbackPresets[d.lookbackSelectedIndex]
		if preset.GetTime == nil {
			// Custom mode - enter custom date picker
			d.lookbackCustomMode = true
			d.lookbackEditField = 0
			return d, nil
		}
		// Apply preset and close picker
		d.tokenCollector.SetLookback(preset.GetTime())
		d.lookbackMode = false
		return d, d.collectMetrics()
	}
	return d, nil
}

// adjustCustomDate adjusts the custom date based on current edit field
func (d *Dashboard) adjustCustomDate(delta int) {
	switch d.lookbackEditField {
	case 0: // Year
		d.lookbackCustomDate = d.lookbackCustomDate.AddDate(delta, 0, 0)
	case 1: // Month
		d.lookbackCustomDate = d.lookbackCustomDate.AddDate(0, delta, 0)
	case 2: // Day
		d.lookbackCustomDate = d.lookbackCustomDate.AddDate(0, 0, delta)
	case 3: // Hour
		d.lookbackCustomDate = d.lookbackCustomDate.Add(time.Duration(delta) * time.Hour)
	case 4: // Minute
		d.lookbackCustomDate = d.lookbackCustomDate.Add(time.Duration(delta) * time.Minute)
	}

	// Clamp to not be in the future
	if d.lookbackCustomDate.After(time.Now()) {
		d.lookbackCustomDate = time.Now()
	}
}

// View renders the dashboard
func (d *Dashboard) View() string {
	if d.width == 0 {
		return "Initializing..."
	}

	var content string

	// Check if in lookback picker mode
	if d.lookbackMode {
		content = d.renderLookbackPicker()
	} else if d.helpMode > 0 {
		// Check if in help mode
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
	// Account for panel padding (0,1) which adds 2 chars per panel = 6 total
	totalPanelWidth := d.width - 6

	// Token panel needs minimum width to avoid wrapping long lines like:
	// "  Opus 4.5: $XX.XX (XXX,XXX tok)" = ~38 chars + borders/padding = ~45
	// Prioritize token panel width, then tmux, system is most flexible

	// For 190 width: totalPanelWidth = 184
	// Token: 56 chars (30%), System: 55 chars (30%), Tmux: 73 chars (40%)
	// Increased token width to accommodate longer model names without wrapping
	tokenWidth := 56
	if totalPanelWidth < 180 {
		tokenWidth = 50 // Narrower for smaller terminals
	} else if totalPanelWidth >= 200 {
		tokenWidth = 62 // Wider for larger terminals
	}

	// System panel - can compress CPU bars, so it's most flexible
	// Minimum ~50 chars for readable CPU bars
	systemWidth := 55
	if totalPanelWidth < 180 {
		systemWidth = 50
	} else if totalPanelWidth >= 200 {
		systemWidth = 62
	}

	// Tmux gets remaining space
	tmuxWidth := totalPanelWidth - systemWidth - tokenWidth
	if tmuxWidth < 60 {
		// If tmux is too narrow, steal from system
		systemWidth -= (60 - tmuxWidth)
		tmuxWidth = 60
	}

	// Calculate panel content height (subtract status bar and borders)
	// Total height - status bar (1) - panel borders (2) = content height
	panelHeight := d.height - 3 // Leave room for status bar (already includes border space)

	systemPanel := d.renderSystemPanel(systemWidth, panelHeight)
	tokenPanel := d.renderTokenPanel(tokenWidth, panelHeight)
	tmuxPanel := d.renderTmuxPanel(tmuxWidth, panelHeight)

	// Force all panels to exactly the same height using lipgloss
	// This ensures borders align even if content varies
	uniformHeight := lipgloss.NewStyle().Height(panelHeight)
	systemPanel = uniformHeight.Render(systemPanel)
	tokenPanel = uniformHeight.Render(tokenPanel)
	tmuxPanel = uniformHeight.Render(tmuxPanel)

	// Join horizontally with top alignment
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

	// Calculate content width (panel width minus borders and padding)
	contentWidth := width - 4 // -2 for borders, -2 for padding

	// CPU Total - use same calculation method as Mem/Swap for consistent bar width
	if d.systemMetrics.CPU.Error == nil {
		// Format: "CPU [|||||| XX.X%]" - "CPU " is 4 chars
		cpuBarWidth := contentWidth - 4 // Subtract "CPU " prefix
		if cpuBarWidth < 10 {
			cpuBarWidth = 10
		}
		lines = append(lines, fmt.Sprintf("CPU %s", d.renderBar(d.systemMetrics.CPU.TotalPercent, cpuBarWidth)))

		// CPU per-core - use up to 6 lines for CPU display
		maxCoreLines := 6
		totalCores := len(d.systemMetrics.CPU.PerCore)

		// Determine label width based on total cores (for alignment)
		labelWidth := 1
		if totalCores >= 100 {
			labelWidth = 3
		} else if totalCores >= 10 {
			labelWidth = 2
		}

		// Determine cores per line
		var coresPerLine int
		if totalCores <= 6 {
			coresPerLine = 1 // One core per line - bars stretch full width
		} else {
			// Multiple cores per line - calculate how many fit
			// Each core needs: labelWidth + ":[" + barContent + "]" + space
			// Minimum reasonable bar content is about 12 chars
			minCharsPerCore := labelWidth + 3 + 12 // label + ":[]" + min bar
			coresPerLine = contentWidth / minCharsPerCore
			if coresPerLine < 2 {
				coresPerLine = 2 // At least 2 per line when splitting
			}
		}

		// Calculate bar width for cores - align brackets by using fixed widths
		var barWidth int
		if coresPerLine == 1 {
			// Single core per line - match memory/swap calculation for consistency
			// Format: "NN:[||||... XXX%]"
			// labelWidth + ":[]" = labelWidth + 3 chars overhead
			barWidth = contentWidth - labelWidth - 3
			if barWidth < 10 {
				barWidth = 10
			}
		} else {
			// Multiple cores per line - split width evenly
			// Account for spaces between cores (1 space separator)
			spacesBetween := coresPerLine - 1
			widthPerCore := (contentWidth - spacesBetween) / coresPerLine
			// Subtract label overhead: labelWidth + ":[]"
			barWidth = widthPerCore - labelWidth - 3
			if barWidth < 8 {
				barWidth = 8
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
			// Use consistent label width for alignment - brackets align because
			// we use fixed-width labels and fixed-width bar content
			percent := d.systemMetrics.CPU.PerCore[i]
			miniBar := d.renderMiniBar(percent, barWidth)
			coreLine.WriteString(fmt.Sprintf("%*d:[%s]", labelWidth, i, miniBar))
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

	// Memory - always compact (one line)
	if d.systemMetrics.Memory.Error == nil {
		memUsed := metrics.FormatBytes(d.systemMetrics.Memory.Used)
		memTotal := metrics.FormatBytes(d.systemMetrics.Memory.Total)
		// Format: "Mem [||||...] XX.XX GB/XX.XX GB"
		// Calculate bar width: contentWidth - "Mem " (4) - " " (1) - "used/total" - margins
		barWidth := contentWidth - 5 - len(memUsed) - 1 - len(memTotal)
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
		// Use same calculation as Memory for consistency
		barWidth := contentWidth - 5 - len(swpUsed) - 1 - len(swpTotal)
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

	// Net I/O - verbose format with pipe separators
	if d.systemMetrics.NetIO.Error == nil {
		lines = append(lines, fmt.Sprintf("Net I/O  | Recv: %s | Sent: %s",
			metrics.FormatRate(d.systemMetrics.NetIO.RecvBytesPerSec),
			metrics.FormatRate(d.systemMetrics.NetIO.SentBytesPerSec)))
	} else {
		lines = append(lines, errorStyle.Render("Net I/O  | N/A"))
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

	// Title with lookback info aligned right
	title := successStyle.Render("💰 Token Usage")
	lookbackInfo := ""
	if d.tokenMetrics != nil && !d.tokenMetrics.LookbackFrom.IsZero() {
		lookbackInfo = dimStyle.Render(d.tokenMetrics.LookbackFrom.Format("Mon 3:04pm"))
	} else if d.tokenMetrics != nil {
		lookbackInfo = dimStyle.Render("All time")
	}

	// Calculate content width and spacing for right alignment
	contentWidth := width - 4 // Account for borders and padding
	titleLen := lipgloss.Width(title)
	lookbackLen := lipgloss.Width(lookbackInfo)
	spacing := contentWidth - titleLen - lookbackLen
	if spacing < 1 {
		spacing = 1
	}

	headerLine := title + strings.Repeat(" ", spacing) + lookbackInfo
	lines = append(lines, headerLine)

	if !d.tokenMetrics.Available {
		lines = append(lines, errorStyle.Render("Not Available"))
		if d.tokenMetrics.Error != "" {
			lines = append(lines, wrapText(d.tokenMetrics.Error, width-4))
		}
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	// Token counts - aligned format
	// Determine which optional fields are present to calculate max label width
	contentWidth = width - 6 // Account for padding and borders
	hasCacheRead := d.tokenMetrics.CacheReadTokens > 0
	hasCacheCreate := d.tokenMetrics.CacheCreationTokens > 0
	hasRate := d.tokenMetrics.Rate > 0
	hasAvg := d.tokenMetrics.SessionAvgRate > 0
	hasSpan := !d.tokenMetrics.LookbackFrom.IsZero()
	hasActive := !d.tokenMetrics.EarliestTimestamp.IsZero()

	// Calculate max label width based on which fields are shown
	// Labels: In, Out, Cache Read, Cache Create, Total, Cost, Rate, Avg, Span, Active
	maxLabelWidth := 5 // "Total" or "Cost:" minimum
	if hasCacheRead && len("Cache Read") > maxLabelWidth {
		maxLabelWidth = len("Cache Read")
	}
	if hasCacheCreate && len("Cache Create") > maxLabelWidth {
		maxLabelWidth = len("Cache Create")
	}
	if hasActive && len("Active") > maxLabelWidth {
		maxLabelWidth = len("Active")
	}

	// Helper to format aligned line
	formatAligned := func(label, value string) string {
		padding := maxLabelWidth - len(label)
		// Reduce padding for narrow panes
		if contentWidth < 35 {
			padding = 0
		} else if contentWidth < 40 {
			padding = padding / 2
		}
		return fmt.Sprintf("%s:%s %s", label, strings.Repeat(" ", padding), value)
	}

	lines = append(lines, formatAligned("In", metrics.FormatTokens(d.tokenMetrics.InputTokens)))
	lines = append(lines, formatAligned("Out", metrics.FormatTokens(d.tokenMetrics.OutputTokens)))

	// Cache on separate lines
	if hasCacheRead {
		lines = append(lines, formatAligned("Cache Read", metrics.FormatTokens(d.tokenMetrics.CacheReadTokens)))
	}
	if hasCacheCreate {
		lines = append(lines, formatAligned("Cache Create", metrics.FormatTokens(d.tokenMetrics.CacheCreationTokens)))
	}

	lines = append(lines, formatAligned("Total", boldStyle.Render(metrics.FormatTokens(d.tokenMetrics.TotalTokens))))

	// Total cost with emphasis
	lines = append(lines, formatAligned("Cost", costStyle.Render(metrics.FormatCost(d.tokenMetrics.TotalCost))))

	// Compact rates
	if hasRate {
		lines = append(lines, formatAligned("Rate", metrics.FormatTokenRate(d.tokenMetrics.Rate)))
	}

	// Session average
	if hasAvg {
		lines = append(lines, formatAligned("Avg", metrics.FormatTokenRate(d.tokenMetrics.SessionAvgRate)))
	}

	// Time span info
	if hasSpan {
		spanDuration := time.Since(d.tokenMetrics.LookbackFrom)
		lines = append(lines, formatAligned("Span", formatDuration(spanDuration)))
	}

	// First activity within lookback period
	if hasActive {
		duration := time.Since(d.tokenMetrics.EarliestTimestamp)
		lines = append(lines, formatAligned("Active", formatDuration(duration)+" ago"))
	}

	// Per-model breakdown with costs (sorted by cost, highest first)
	if len(d.tokenMetrics.ModelUsages) > 0 {
		lines = append(lines, "") // Empty line separator
		lines = append(lines, boldStyle.Render("Per-Model Costs:"))

		// First pass: calculate max display name length for alignment
		maxNameLen := 0
		displayNames := make([]string, len(d.tokenMetrics.ModelUsages))
		for i, usage := range d.tokenMetrics.ModelUsages {
			displayName := shortenModelName(usage.Model)

			// Truncate if still too long
			maxAllowedLen := contentWidth - 20 // Leave room for cost and tokens
			if len(displayName) > maxAllowedLen && maxAllowedLen > 3 {
				displayName = displayName[:maxAllowedLen-3] + "..."
			}

			displayNames[i] = displayName
			if len(displayName) > maxNameLen {
				maxNameLen = len(displayName)
			}
		}

		// Second pass: render with alignment
		for i, usage := range d.tokenMetrics.ModelUsages {
			displayName := displayNames[i]
			modelName := usage.Model

			costStr := metrics.FormatCost(usage.Cost)
			tokStr := metrics.FormatTokens(usage.TotalTokens)

			// Color-code by model type
			var modelStyle lipgloss.Style
			if strings.Contains(modelName, "opus") {
				modelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")) // Red for Opus
			} else if strings.Contains(modelName, "sonnet") {
				modelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4")) // Cyan for Sonnet
			} else if strings.Contains(modelName, "haiku") {
				modelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#95e1d3")) // Light green for Haiku
			} else {
				modelStyle = dimStyle
			}

			// Calculate padding for alignment
			// When pane is narrow, reduce padding first
			padding := maxNameLen - len(displayName)
			// If content width is tight, reduce alignment padding
			if contentWidth < 35 {
				padding = 0 // No alignment padding for narrow panes
			} else if contentWidth < 40 {
				padding = padding / 2 // Reduce padding for somewhat narrow panes
			}

			line := fmt.Sprintf("  %s:%s %s (%s)",
				modelStyle.Render(displayName),
				strings.Repeat(" ", padding),
				costStyle.Render(costStr),
				dimStyle.Render(tokStr+" tok"))
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// shortenModelName shortens common Claude model names for display
func shortenModelName(name string) string {
	// Common patterns to shorten
	replacements := map[string]string{
		"claude-opus-4-5-20251101":    "Opus 4.5",
		"claude-sonnet-4-5-20250929":  "Sonnet 4.5",
		"claude-haiku-4-5-20250929":   "Haiku 4.5",
		"claude-3-5-sonnet-20241022":  "Sonnet 3.5",
		"claude-3-5-haiku-20241022":   "Haiku 3.5",
		"claude-3-opus-20240229":      "Opus 3",
		"claude-3-sonnet-20240229":    "Sonnet 3",
		"claude-3-haiku-20240307":     "Haiku 3",
	}

	if short, ok := replacements[name]; ok {
		return short
	}

	// Try partial matches
	if strings.Contains(name, "opus-4-5") || strings.Contains(name, "opus-4.5") {
		return "Opus 4.5"
	}
	if strings.Contains(name, "sonnet-4-5") || strings.Contains(name, "sonnet-4.5") {
		return "Sonnet 4.5"
	}
	if strings.Contains(name, "haiku-4-5") || strings.Contains(name, "haiku-4.5") {
		return "Haiku 4.5"
	}
	if strings.Contains(name, "opus") {
		return "Opus"
	}
	if strings.Contains(name, "sonnet") {
		return "Sonnet"
	}
	if strings.Contains(name, "haiku") {
		return "Haiku"
	}

	return name
}

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
	if count := statusCounts[metrics.StatusWorking]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("🟢%d", count))
	}
	if count := statusCounts[metrics.StatusReady]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("🔴%d", count))
	}
	if count := statusCounts[metrics.StatusActive]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("🟡%d", count))
	}
	if count := statusCounts[metrics.StatusError]; count > 0 {
		statusParts = append(statusParts, fmt.Sprintf("⚠️%d", count))
	}
	statusSummary := strings.Join(statusParts, " ")

	// Title with total count and status summary right-justified
	title := successStyle.Render(fmt.Sprintf("📺 TMUX Sessions (%d)", d.tmuxMetrics.Total))
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

	// Calculate available lines for sessions
	// height includes borders, subtract: title(1) + borders(2) = 3 lines overhead
	availableLines := height - 3
	if availableLines < 1 {
		availableLines = 1
	}

	sessionCount := len(d.tmuxMetrics.Sessions)
	contentWidth = width - 4 // -4 for borders (2) and padding (2)

	// Determine columns dynamically based on session count and available space
	// Start with 1 column, expand to 2 if sessions won't fit
	cols := 1
	if sessionCount > availableLines {
		cols = 2 // Spill over to 2 columns
	}
	// Use 3 columns only if width is sufficient and we have many sessions
	if width >= 160 && sessionCount > availableLines*2 {
		cols = 3
	}

	// Calculate cell width
	cellWidth := (contentWidth - (cols - 1)) / cols

	// Calculate how many sessions we can display
	maxDisplayed := availableLines * cols
	maxSessions := sessionCount
	if maxSessions > maxDisplayed {
		maxSessions = maxDisplayed
	}

	// Render sessions in vertical columns (fill first column, then second, etc.)
	rowCount := (maxSessions + cols - 1) / cols
	for row := 0; row < rowCount; row++ {
		var rowCells []string
		for col := 0; col < cols; col++ {
			idx := col*rowCount + row
			if idx < maxSessions {
				session := d.tmuxMetrics.Sessions[idx]
				cellContent := d.renderSessionCell(session, cellWidth)
				// Apply explicit width constraint using lipgloss
				cellStyle := lipgloss.NewStyle().Width(cellWidth)
				cell := cellStyle.Render(cellContent)
				rowCells = append(rowCells, cell)
			} else {
				// Empty cell for alignment
				emptyCell := lipgloss.NewStyle().Width(cellWidth).Render("")
				rowCells = append(rowCells, emptyCell)
			}
		}
		// Join cells with space separator for multiple columns
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
	if maxSessions < sessionCount {
		remaining := sessionCount - maxSessions
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
		lines = append(lines, "Select a lookback period:")
		lines = append(lines, "")

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

			lines = append(lines, fmt.Sprintf("%s%s%s",
				prefix,
				style.Render(preset.Name),
				dimStyle.Render(timeStr)))
			lines = append(lines, fmt.Sprintf("   %s", dimStyle.Render(preset.Description)))
		}

		lines = append(lines, "")
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

Net I/O: Network recv/sent speeds

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

Time:
  Span: Duration of lookback period
  Active: Time since first activity in period

Lookback: Press 'l' to open time picker
  Presets: Today, 24h, 7d, 30d, All time
  Custom: Set specific date/time with arrows

Models: Per-model cost breakdown
  Color-coded: Opus(red) Sonnet(cyan) Haiku(green)
  Sorted by cost (highest first)

SQLite Cache: .ccdash/tokens.db
  Queryable with DuckDB or any SQLite tool
  Tables: token_events, file_state
  Incremental ingestion with deduplication`

	case 3: // TMUX Sessions
		title = "TMUX Sessions Panel"
		panel = d.renderTmuxPanel(panelWidth, panelHeight)
		helpText = `Monitors tmux sessions running Claude Code:

Title: Shows total count + status summary
  Format: "📺 TMUX Sessions (N) 🟢2 🔴1"

Status (analyzes pane content):
  🟢 WORKING - Claude Code processing
  🔴 READY - Waiting for user input
  🟡 ACTIVE - User in session
  ⚠️  ERROR - Error or undefined state

Detection: Analyzes last 15 lines for:
  Working indicators, prompts, errors

Session Info:
  Name, status, windows (Xw), idle, 📎=attached

Idle: Time since content changed (s/m/h)

Layout: Auto-columns based on count/width

Self-Update: Press 'u' when update available
  Status bar shows "⬆ vX.X.X available!"`
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

	// Show update status or normal middle content
	var middle string
	if d.updating {
		middle = warningStyle.Render(d.updateStatus)
	} else if d.updateStatus != "" {
		middle = errorStyle.Render(d.updateStatus)
	} else if d.updateInfo != nil && d.updateInfo.UpdateAvailable {
		middle = successStyle.Render(fmt.Sprintf("⬆ v%s available! Press u to update", d.updateInfo.LatestVersion))
	} else {
		middle = dimStyle.Render("https://github.com/jedarden/ccdash")
	}

	// Build shortcuts - include 'u' if update available
	shortcuts := "l:lookback h:help q:quit r:refresh"
	if d.updateInfo != nil && d.updateInfo.UpdateAvailable && !d.updating {
		shortcuts = "u:update l:lookback h:help q:quit r:refresh"
	}
	right := fmt.Sprintf("%dx%d %s", d.width, d.height, shortcuts)

	// Calculate spacing (account for statusBarStyle padding of 2 chars)
	totalContent := lipgloss.Width(left) + lipgloss.Width(middle) + lipgloss.Width(right)
	availableSpace := d.width - totalContent - 2 // -2 for padding

	if availableSpace < 4 {
		// Not enough space, use ultra-compact format
		compactShortcuts := "l h q r"
		if d.updateInfo != nil && d.updateInfo.UpdateAvailable {
			compactShortcuts = "u l h q r"
		}
		return statusBarStyle.Render(fmt.Sprintf("%s v%s %dx%d %s",
			d.lastUpdate.Format("15:04"), d.version, d.width, d.height, compactShortcuts))
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
// Returns a fixed-width string to ensure bracket alignment
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

	// Format percentage with fixed width (4 chars: "XXX%" or " XX%")
	percentText := fmt.Sprintf("%3.0f%%", percent)

	// Reserve space for percentage text (4 chars) + 1 space
	percentSpace := 5
	barAvailableWidth := barWidth - percentSpace
	if barAvailableWidth < 1 {
		barAvailableWidth = 1
	}

	// Calculate fill width
	fillWidth := int(percent / 100.0 * float64(barAvailableWidth))
	if fillWidth > barAvailableWidth {
		fillWidth = barAvailableWidth
	}
	if fillWidth < 0 {
		fillWidth = 0
	}

	// Create filled and empty portions
	filled := strings.Repeat("|", fillWidth)
	empty := strings.Repeat(" ", barAvailableWidth-fillWidth)

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
