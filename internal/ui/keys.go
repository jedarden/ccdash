package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedarden/ccdash/internal/metrics"
	"github.com/jedarden/ccdash/internal/updater"
)

// KeyboardShortcut describes a dashboard key binding shown by the CLI help.
type KeyboardShortcut struct {
	Keys        string
	Description string
}

type dashboardKeyBinding struct {
	KeyboardShortcut
	keys   []string
	handle func(*Dashboard) tea.Cmd
}

var dashboardKeyBindings = []dashboardKeyBinding{
	{
		KeyboardShortcut: KeyboardShortcut{Keys: "q, Ctrl+C", Description: "Quit the dashboard"},
		keys:             []string{"q", "ctrl+c"},
		handle:           func(*Dashboard) tea.Cmd { return tea.Quit },
	},
	{
		KeyboardShortcut: KeyboardShortcut{Keys: "r", Description: "Refresh metrics immediately"},
		keys:             []string{"r"},
		handle:           func(d *Dashboard) tea.Cmd { return d.collectMetrics() },
	},
	{
		KeyboardShortcut: KeyboardShortcut{Keys: "h", Description: "Cycle through help panels"},
		keys:             []string{"h"},
		handle: func(d *Dashboard) tea.Cmd {
			d.helpMode = (d.helpMode + 1) % 4
			return nil
		},
	},
	{
		KeyboardShortcut: KeyboardShortcut{Keys: "w", Description: "Open the workers view"},
		keys:             []string{"w"},
		handle: func(d *Dashboard) tea.Cmd {
			d.workerDetailMode = true
			d.workerDetailOffset = 0
			d.helpMode = 0
			return nil
		},
	},
	{
		KeyboardShortcut: KeyboardShortcut{Keys: "l, L", Description: "Open token usage lookback picker"},
		keys:             []string{"l", "L"},
		handle: func(d *Dashboard) tea.Cmd {
			d.lookbackMode = true
			d.helpMode = 0
			return nil
		},
	},
	{
		KeyboardShortcut: KeyboardShortcut{Keys: "Esc", Description: "Dismiss the update notice"},
		keys:             []string{"esc"},
		handle: func(d *Dashboard) tea.Cmd {
			if d.updateNoticeVisible() && !d.updating {
				d.updateDismissed = true
			}
			return nil
		},
	},
	{
		KeyboardShortcut: KeyboardShortcut{Keys: "u, U", Description: "Check for or install an update"},
		keys:             []string{"u", "U"},
		handle:           (*Dashboard).handleUpdateKey,
	},
}

// DashboardKeyboardShortcuts returns the main dashboard bindings documented by --help.
func DashboardKeyboardShortcuts() []KeyboardShortcut {
	shortcuts := make([]KeyboardShortcut, len(dashboardKeyBindings))
	for i, binding := range dashboardKeyBindings {
		shortcuts[i] = binding.KeyboardShortcut
	}
	return shortcuts
}

func (d *Dashboard) handleDashboardKey(key string) (tea.Cmd, bool) {
	for _, binding := range dashboardKeyBindings {
		for _, boundKey := range binding.keys {
			if key == boundKey {
				return binding.handle(d), true
			}
		}
	}
	return nil, false
}

func (d *Dashboard) handleUpdateKey() tea.Cmd {
	if !updater.IsReleaseVersion(d.version) {
		return nil
	}

	// If an update is already known available, install it. Otherwise (or if the
	// last check errored/found nothing), trigger a fresh check.
	if d.updating || d.checkingUpdate {
		return nil
	}
	if d.updateInfo != nil && d.updateInfo.UpdateAvailable {
		d.updating = true
		d.updateStatus = "Downloading update..."
		return d.performUpdate()
	}
	d.checkingUpdate = true
	d.updateStatus = ""
	return d.forceCheckForUpdates()
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

func (d *Dashboard) handleWorkerDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	workers, _ := d.groupSessions(d.workerSessions())
	if d.workerDetailOffset >= len(workers) {
		d.workerDetailOffset = max(len(workers)-1, 0)
	}
	switch msg.String() {
	case "w", "q", "esc":
		d.workerDetailMode = false
		d.workerDetailOffset = 0
	case "ctrl+c":
		return d, tea.Quit
	case "up", "k":
		if d.workerDetailOffset > 0 {
			d.workerDetailOffset--
		}
	case "down", "j":
		if d.workerDetailOffset < len(workers)-1 {
			d.workerDetailOffset++
		}
	case "r":
		return d, d.collectMetrics()
	}
	return d, nil
}

func (d *Dashboard) workerSessions() []metrics.TmuxSession {
	if d.tmuxMetrics == nil {
		return nil
	}
	return d.tmuxMetrics.Sessions
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
