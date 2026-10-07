package ui

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/config"
	"github.com/jedarden/ccdash/internal/metrics"
	"github.com/jedarden/ccdash/internal/notify"
	"github.com/jedarden/ccdash/internal/updater"
)

// Layout mode constants
type LayoutMode int

const (
	LayoutCompact   LayoutMode = iota // <140 cols: tmux top, tokens middle, system bottom
	LayoutUltraWide                   // >=140 cols: three panels side by side
)

// panelNow is the clock for time-dependent panel content; tests replace it.
var panelNow = time.Now

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
	width      int
	height     int
	layoutMode LayoutMode
	version    string
	instanceID string // Unique ID for leader election

	// Metrics collectors
	systemCollector *metrics.SystemCollector
	tokenCollector  *metrics.TokenCollector
	tmuxCollector   *metrics.TmuxCollector

	// Current metrics
	systemMetrics metrics.SystemMetrics
	tokenMetrics  *metrics.TokenMetrics
	tmuxMetrics   *metrics.TmuxMetrics

	// UI state
	lastUpdate         time.Time
	err                error
	helpMode           int // 0=none, 1=system, 2=tokens, 3=tmux
	workerDetailMode   bool
	workerDetailOffset int

	// Lookback picker state
	lookbackMode          bool // true when lookback picker is open
	lookbackPresets       []LookbackPreset
	lookbackSelectedIndex int
	lookbackCustomMode    bool      // true when editing custom date/time
	lookbackCustomDate    time.Time // the custom date being edited
	lookbackEditField     int       // 0=year, 1=month, 2=day, 3=hour, 4=minute

	// Update checking
	updater             *updater.Updater
	updateInfo          *updater.UpdateInfo
	updating            bool
	checkingUpdate      bool
	updateStatus        string
	updateStatusIsError bool
	updateDismissed     bool

	// Notification system
	notifyConfig        *config.Config
	notifyClient        *notify.Client
	notificationTracker *notify.Tracker
}

// generateInstanceID creates a unique identifier for this dashboard instance
func generateInstanceID() string {
	return fmt.Sprintf("%d-%d", os.Getpid(), rand.Int63())
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
			Name:        "5 hours",
			Description: "Last 5 hours (2400 req limit)",
			GetTime: func() time.Time {
				return time.Now().Add(-5 * time.Hour)
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

	// Load notification config (fails gracefully if config missing). Keep the
	// client and tracker nil unless explicitly enabled so the default path is
	// fully inert.
	cfg, _ := config.Load()
	var notifyClient *notify.Client
	var notificationTracker *notify.Tracker
	if cfg != nil && cfg.Notify.Enabled {
		notifyClient = notify.NewClient(cfg.Notify.WebhookURL, true)
		notificationTracker = notify.NewTracker(notify.DebounceWindow)
	}

	return &Dashboard{
		version:             version,
		instanceID:          generateInstanceID(),
		systemCollector:     metrics.NewSystemCollector(),
		tokenCollector:      metrics.NewTokenCollector(),
		tmuxCollector:       metrics.NewTmuxCollector(),
		updater:             updater.NewUpdater(version),
		lastUpdate:          time.Now(),
		lookbackPresets:     presets,
		lookbackCustomDate:  time.Now().AddDate(0, 0, -1), // Default custom to yesterday
		notifyConfig:        cfg,
		notifyClient:        notifyClient,
		notificationTracker: notificationTracker,
	}
}

// AddProjectsDirs adds additional root directories to scan for JSONL files.
// Call this after NewDashboard to include directories beyond the default ~/.claude/projects.
func (d *Dashboard) AddProjectsDirs(dirs []string) {
	for _, dir := range dirs {
		d.tokenCollector.AddProjectsDir(dir)
	}
}

// Init initializes the dashboard
func (d *Dashboard) Init() tea.Cmd {
	commands := []tea.Cmd{
		d.tick(),
		d.collectMetrics(),
	}
	if updater.IsReleaseVersion(d.version) {
		// Run the network check asynchronously so it cannot delay the initial dashboard render.
		commands = append(commands, d.checkForUpdates())
	}
	return tea.Batch(commands...)
}

// updateCheckMsg carries update check results
type updateCheckMsg struct {
	info *updater.UpdateInfo
}

// updateCompleteMsg indicates update was applied
type updateCompleteMsg struct {
	err error
}

// checkForUpdates returns a command that checks for updates, using the
// updater's normal cache.
func (d *Dashboard) checkForUpdates() tea.Cmd {
	return func() tea.Msg {
		info := d.updater.CheckForUpdate(false)
		return updateCheckMsg{info: info}
	}
}

// forceCheckForUpdates returns a command that checks for updates immediately,
// bypassing the cache - for a user-initiated recheck (pressing 'u' with no
// update currently known).
func (d *Dashboard) forceCheckForUpdates() tea.Cmd {
	return func() tea.Msg {
		info := d.updater.CheckForUpdate(true)
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
		if d.workerDetailMode {
			return d.handleWorkerDetailKey(msg)
		}

		// Handle lookback picker mode
		if d.lookbackMode {
			return d.handleLookbackKey(msg)
		}

		if cmd, handled := d.handleDashboardKey(msg.String()); handled {
			return d, cmd
		}

	case tickMsg:
		return d, tea.Batch(d.tick(), d.collectMetrics())

	case metricsMsg:
		d.systemMetrics = msg.system
		d.tokenMetrics = msg.tokens
		d.tmuxMetrics = msg.tmux
		d.lastUpdate = time.Now()

		// Only the lease holder receives raw hook snapshots. This keeps
		// notification diffing leader-only even when other dashboards render
		// cached tmux metrics.
		if msg.isLeader && msg.hookSessionsCollected && d.notificationTracker != nil {
			for _, payload := range d.notificationTracker.Update(msg.hookSessions) {
				payload := payload
				go d.notifyClient.Send(&payload)
			}
		}
		return d, nil

	case updateCheckMsg:
		wasManualCheck := d.checkingUpdate
		d.checkingUpdate = false
		d.updateInfo = msg.info
		if wasManualCheck {
			switch {
			case msg.info.Error != "":
				d.updateStatus = fmt.Sprintf("Update check failed: %s", msg.info.Error)
				d.updateStatusIsError = true
			case msg.info.UpdateAvailable:
				// Clear any stale status so the "available" notice renders.
				d.updateStatus = ""
				d.updateStatusIsError = false
			default:
				d.updateStatus = fmt.Sprintf("Up to date (v%s)", msg.info.CurrentVersion)
				d.updateStatusIsError = false
			}
		}
		return d, nil

	case updateCompleteMsg:
		d.updating = false
		if msg.err != nil {
			d.updateStatus = fmt.Sprintf("Update failed: %v", msg.err)
			d.updateStatusIsError = true
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

// updateNoticeVisible reports whether the update notice should be shown.
func (d *Dashboard) updateNoticeVisible() bool {
	return updater.IsReleaseVersion(d.version) && d.updateInfo != nil && d.updateInfo.UpdateAvailable && !d.updateDismissed
}

// View renders the dashboard
func (d *Dashboard) View() string {
	if d.width == 0 {
		return "Initializing..."
	}

	var content string

	// Check if in lookback picker mode
	if d.workerDetailMode {
		content = d.renderWorkerDetailView()
	} else if d.lookbackMode {
		content = d.renderLookbackPicker()
	} else if d.helpMode > 0 {
		// Check if in help mode
		content = d.renderHelpView()
	} else {
		if d.layoutMode == LayoutUltraWide {
			content = d.renderUltraWide()
		} else {
			content = d.renderCompact()
		}
	}

	// Add status bar
	statusBar := d.renderStatusBar()

	// Join content and status bar
	output := lipgloss.JoinVertical(lipgloss.Left, content, statusBar)

	// CRITICAL: Ensure output fills the entire terminal height to prevent
	// external process output (like Tailscale logs) from bleeding through
	// at the bottom of the screen. Without this, the alternate screen buffer
	// may show underlying terminal content in unfilled rows.
	outputHeight := lipgloss.Height(output)
	if outputHeight < d.height {
		// Pad with empty lines to fill the screen
		padding := strings.Repeat("\n", d.height-outputHeight)
		output = output + padding
	}

	// Panel renderers intentionally favor readable content, which can exceed
	// their requested size when a terminal is too small. Keep the final frame
	// inside the actual terminal dimensions in every layout and view mode.
	return lipgloss.NewStyle().MaxWidth(d.width).MaxHeight(d.height).Render(output)
}

// updateLayout determines the current layout mode based on terminal size
func (d *Dashboard) updateLayout() {
	// Below 140 columns three panels cannot fit side by side: the fixed
	// 56-column system panel leaves the token panel too narrow for its own
	// header (verified at 120-139 columns), so stack them instead.
	if d.width < 140 {
		// Compact stacked layout: tmux top, tokens middle, system bottom
		d.layoutMode = LayoutCompact
	} else {
		// Always use 3-column layout for wide terminals
		d.layoutMode = LayoutUltraWide
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
	system                metrics.SystemMetrics
	tokens                *metrics.TokenMetrics
	tmux                  *metrics.TmuxMetrics
	isLeader              bool
	hookSessions          []metrics.HookSession
	hookSessionsCollected bool
}

// errMsg carries errors
type errMsg struct {
	err error
}

// Metric type constants for cache keys
const (
	metricTypeSystem = "system"
	metricTypeTmux   = "tmux"
)

// collectMetrics returns a command that collects all metrics
// Uses leader election: only one instance collects, others read from cache
func (d *Dashboard) collectMetrics() tea.Cmd {
	return func() tea.Msg {
		cache := d.tokenCollector.GetCache()
		isLeader := cache.TryAcquireLease(d.instanceID)

		var system metrics.SystemMetrics
		var tokens *metrics.TokenMetrics
		var tmux *metrics.TmuxMetrics
		var hookSessions []metrics.HookSession
		var hookSessionsCollected bool

		// Use channels to collect results with timeout
		type systemResult struct {
			metrics metrics.SystemMetrics
		}
		type tokenResult struct {
			metrics *metrics.TokenMetrics
		}
		type tmuxResult struct {
			metrics               *metrics.TmuxMetrics
			hookSessions          []metrics.HookSession
			hookSessionsCollected bool
		}

		systemChan := make(chan systemResult, 1)
		tokenChan := make(chan tokenResult, 1)
		tmuxChan := make(chan tmuxResult, 1)

		// System metrics: try cache first, collect if leader or cache miss
		go func() {
			if !isLeader {
				if data, ok := cache.GetCachedMetrics(metricTypeSystem); ok {
					var cached metrics.SystemMetrics
					if json.Unmarshal(data, &cached) == nil {
						systemChan <- systemResult{metrics: cached}
						return
					}
				}
			}
			// Leader or cache miss: collect fresh
			m := d.systemCollector.Collect()
			if isLeader {
				if data, err := json.Marshal(m); err == nil {
					cache.SetCachedMetrics(metricTypeSystem, data)
				}
			}
			systemChan <- systemResult{metrics: m}
		}()

		// Token metrics: always collect (uses shared DB, cheap to query)
		go func() {
			t, _ := d.tokenCollector.Collect()
			tokenChan <- tokenResult{metrics: t}
		}()

		// Tmux metrics: try cache first, collect if leader or cache miss
		go func() {
			if !isLeader {
				if data, ok := cache.GetCachedMetrics(metricTypeTmux); ok {
					var cached metrics.TmuxMetrics
					if json.Unmarshal(data, &cached) == nil {
						tmuxChan <- tmuxResult{metrics: &cached}
						return
					}
				}
			}
			// Leader or cache miss: collect fresh
			m := d.tmuxCollector.Collect()
			var hookSessions []metrics.HookSession
			hookSessionsCollected := false
			if isLeader && d.notificationTracker != nil {
				hookSessionsCollected = true
				if hookCollector := d.tmuxCollector.GetHookCollector(); hookCollector != nil {
					var err error
					hookSessions, err = hookCollector.CollectSessions()
					hookSessionsCollected = err == nil
				}
			}
			if isLeader {
				if data, err := json.Marshal(m); err == nil {
					cache.SetCachedMetrics(metricTypeTmux, data)
				}
			}
			tmuxChan <- tmuxResult{
				metrics:               m,
				hookSessions:          hookSessions,
				hookSessionsCollected: hookSessionsCollected,
			}
		}()

		// Wait for results with 3 second timeout
		timeout := time.After(3 * time.Second)

		// Collect results as they come in, or timeout
		for i := 0; i < 3; i++ {
			select {
			case r := <-systemChan:
				system = r.metrics
			case r := <-tokenChan:
				tokens = r.metrics
			case r := <-tmuxChan:
				tmux = r.metrics
				hookSessions = r.hookSessions
				hookSessionsCollected = r.hookSessionsCollected
			case <-timeout:
				// Return whatever we have so far
				return metricsMsg{
					system:                system,
					tokens:                tokens,
					tmux:                  tmux,
					isLeader:              isLeader,
					hookSessions:          hookSessions,
					hookSessionsCollected: hookSessionsCollected,
				}
			}
		}

		metrics.AttachSessionCosts(d.tokenCollector, tmux)
		return metricsMsg{
			system:                system,
			tokens:                tokens,
			tmux:                  tmux,
			isLeader:              isLeader,
			hookSessions:          hookSessions,
			hookSessionsCollected: hookSessionsCollected,
		}
	}
}
