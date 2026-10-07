package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedarden/ccdash/internal/metrics"
	"github.com/jedarden/ccdash/internal/ui"
	"golang.org/x/term"
)

// version is set at build time via -ldflags "-X main.version=vX.X.X"
// If not set, defaults to "dev" for local development builds
var version = "dev"

func main() {
	// Parse command-line flags
	var (
		showVersion       = flag.Bool("version", false, "Show version information")
		showHelp          = flag.Bool("help", false, "Show help information")
		installHooks      = flag.Bool("install-hooks", false, "Install Claude Code hooks for session tracking")
		installCodexHooks = flag.Bool("install-codex-hooks", false, "Install Codex hooks for session tracking")
		checkHooks        = flag.Bool("check-hooks", false, "Check Claude Code and Codex hook installation")
		uninstallHooks    = flag.Bool("uninstall-hooks", false, "Uninstall ccdash hooks from Claude Code and Codex")
		testNotify        = flag.Bool("test-notify", false, "Test notification webhook configuration")
		extraDirs         = flag.String("extra-dirs", "", "Additional Claude project root directories to scan (comma-separated). Also set via CCDASH_EXTRA_DIRS env var (colon-separated)")
		exportFormat      = flag.String("export", "", "Export token cache to stdout (csv|json)")
		runOnce           = flag.Bool("once", false, "Run a single collection cycle and exit")
		jsonOutput        = flag.Bool("json", false, "Output metrics as JSON (use with --once)")
		sinceValue        = flag.String("since", "", "Limit token data to monday, today, 24h, 7d, or an RFC3339 timestamp (use with --once or --export)")
		attention         = flag.Bool("attention", false, "List ASKING sessions and exit 1 if any need a human (works alone or with --once)")
		doctor            = flag.Bool("doctor", false, "Check data sources, cache, config and hooks; exit 1 on a problem (add --json for JSON)")
	)

	flag.Parse()

	// Handle --version
	if *showVersion {
		fmt.Printf("ccdash version %s\n", version)
		fmt.Println("Claude Code + Codex Dashboard - A terminal UI for monitoring system resources, token usage, and tmux sessions")
		os.Exit(0)
	}

	// Handle --help
	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	if *doctor {
		os.Exit(runDoctor(*jsonOutput, *extraDirs))
	}

	since := time.Time{}
	if *sinceValue != "" {
		var err error
		since, err = parseSince(*sinceValue, time.Now())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(2)
		}
		if !*runOnce && *exportFormat == "" {
			fmt.Fprintln(os.Stderr, "Error: --since requires --once or --export")
			os.Exit(2)
		}
	}
	if *attention && *exportFormat != "" {
		fmt.Fprintln(os.Stderr, "Error: --attention cannot be combined with --export")
		os.Exit(2)
	}

	// Handle --install-hooks
	if *installHooks {
		collector, err := metrics.NewHookSessionCollector()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Always run InstallHooks - it's idempotent and will add any missing hooks
		fmt.Println("Installing Claude Code hooks for session tracking...")
		if err := collector.InstallHooks(); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing hooks: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✓ Hooks installed successfully!")
		fmt.Println()
		fmt.Println("The following hooks have been added to ~/.claude/settings.json:")
		fmt.Println("  • SessionStart       - Registers new Claude Code sessions")
		fmt.Println("  • UserPromptSubmit   - Marks session as working")
		fmt.Println("  • PreToolUse         - Refreshes activity during long-running tasks")
		fmt.Println("  • PostToolUse        - Marks session as working (resumes after approval)")
		fmt.Println("  • Stop               - Records an idle turn while preserving explicit task state")
		fmt.Println("  • Notification       - Marks actual permission requests; ignores idle prompts")
		fmt.Println("  • PermissionRequest  - Marks session as ASKING (approval needed)")
		fmt.Println("  • SessionEnd         - Unregisters sessions when they end")
		fmt.Println()
		fmt.Printf("Session data will be written to: %s/sessions/\n", collector.GetBaseDir())
		fmt.Println()
		fmt.Println("Restart any running Claude Code sessions for hooks to take effect.")
		os.Exit(0)
	}

	// Handle --install-codex-hooks
	if *installCodexHooks {
		installer := metrics.NewCodexHookInstaller()
		fmt.Println("Installing Codex hooks for session tracking...")
		if err := installer.InstallHooks(); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing Codex hooks: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Codex hooks installed successfully!")
		fmt.Println("  Hook configuration: ~/.codex/hooks.json")
		fmt.Println("  Session data: ~/.ccdash/sessions/")
		fmt.Println()
		fmt.Println("Restart any running Codex sessions for hooks to take effect.")
		os.Exit(0)
	}

	// Handle --check-hooks
	if *checkHooks {
		collector, err := metrics.NewHookSessionCollector()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		codexInstaller := metrics.NewCodexHookInstaller()
		claudeInstalled := collector.AreHooksInstalled()
		codexInstalled := codexInstaller.AreHooksInstalled()
		if claudeInstalled || codexInstalled {
			if claudeInstalled {
				fmt.Println("✓ Claude Code hooks are installed")
				fmt.Printf("  Hook scripts: %s/hooks/\n", collector.GetBaseDir())
				fmt.Printf("  Session data: %s/sessions/\n", collector.GetBaseDir())
			}
			if codexInstalled {
				fmt.Println("✓ Codex hooks are installed")
				fmt.Println("  Hook configuration: ~/.codex/hooks.json")
			}

			// Check for active sessions
			sessions, err := collector.CollectSessions()
			if err == nil {
				fmt.Printf("  Active sessions: %d\n", len(sessions))
			}
		} else {
			fmt.Println("✗ Claude Code and Codex hooks are NOT installed")
			fmt.Println()
			fmt.Println("Run 'ccdash --install-hooks' or 'ccdash --install-codex-hooks' to install them.")
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle --uninstall-hooks
	if *uninstallHooks {
		collector, err := metrics.NewHookSessionCollector()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := collector.UninstallHooks(); err != nil {
			fmt.Fprintf(os.Stderr, "Error uninstalling Claude Code hooks: %v\n", err)
			os.Exit(1)
		}
		if err := metrics.NewCodexHookInstaller().UninstallHooks(); err != nil {
			fmt.Fprintf(os.Stderr, "Error uninstalling Codex hooks: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ ccdash hooks uninstalled from Claude Code and Codex")
		os.Exit(0)
	}

	// Handle --test-notify
	if *testNotify {
		os.Exit(runTestNotify())
	}

	// Handle --export
	if *exportFormat != "" {
		cache := metrics.NewTokenCache()
		if cache == nil || cache.GetDB() == nil {
			fmt.Fprintf(os.Stderr, "Error: token cache not available\n")
			os.Exit(1)
		}

		switch strings.ToLower(*exportFormat) {
		case "csv":
			if err := exportCSVSince(cache, since); err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting CSV: %v\n", err)
				os.Exit(1)
			}
		case "json":
			if err := exportJSONSince(cache, since); err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting JSON: %v\n", err)
				os.Exit(1)
			}
		case "json-aggregated":
			if err := exportJSONAggregatedSince(cache, since); err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting aggregated JSON: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "Error: export format must be 'csv', 'json', or 'json-aggregated', got '%s'\n", *exportFormat)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle --once (single collection cycle, no TUI)
	if *runOnce {
		os.Exit(runOnceModeWithOptions(*jsonOutput, *extraDirs, since, *attention))
	}

	// A standalone attention check only collects session state, keeping it cheap
	// for scripts that need to poll for sessions waiting on a human.
	if *attention {
		os.Exit(runAttentionMode())
	}

	// Check if running in a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stderr, "Error: ccdash must be run in a terminal")
		os.Exit(1)
	}

	// Set up hook management with cleanup on exit
	hookCollector := setupHooks()
	if hookCollector != nil {
		defer hookCollector.Cleanup()

		// Set up signal handler for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			hookCollector.Cleanup()
			os.Exit(0)
		}()
	}

	// Create and run the dashboard
	dashboard := ui.NewDashboard(version)

	// Add any extra project directories specified via --extra-dirs flag
	if *extraDirs != "" {
		var dirs []string
		for _, d := range strings.Split(*extraDirs, ",") {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
		if len(dirs) > 0 {
			expandedDirs := metrics.ExpandGlobPatterns(dirs)
			dashboard.AddProjectsDirs(expandedDirs)
		}
	}

	p := tea.NewProgram(
		dashboard,
		tea.WithAltScreen(), // Use alternate screen buffer
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running dashboard: %v\n", err)
		os.Exit(1)
	}
}

// Snapshot is the versioned public JSON representation emitted by --once --json.
type Snapshot struct {
	SchemaVersion int                      `json:"schema_version"`
	Timestamp     time.Time                `json:"timestamp"`
	Version       string                   `json:"version"`
	System        snapshotSystemMetrics    `json:"system"`
	Tokens        *snapshotTokenMetrics    `json:"tokens"`
	Sessions      *snapshotSessionsMetrics `json:"sessions"`
}

const snapshotSchemaVersion = 1

// These snapshot types define the external --once --json schema independently
// of the collector types. Rate fields are pointers so a single sample is
// encoded as null instead of looking like a measured zero.
type snapshotSystemMetrics struct {
	CPU        snapshotCPUMetrics       `json:"cpu"`
	Load       snapshotLoadMetrics      `json:"load"`
	Memory     snapshotMemoryMetrics    `json:"memory"`
	Swap       snapshotSwapMetrics      `json:"swap"`
	DiskUsage  snapshotDiskUsageMetrics `json:"disk_usage"`
	DiskIO     snapshotDiskIOMetrics    `json:"disk_io"`
	NetIO      snapshotNetIOMetrics     `json:"net_io"`
	LastUpdate time.Time                `json:"last_update"`
}

type snapshotCPUMetrics struct {
	TotalPercent float64   `json:"total_percent"`
	PerCore      []float64 `json:"per_core"`
	Error        *string   `json:"error,omitempty"`
}

type snapshotLoadMetrics struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	Error  *string `json:"error,omitempty"`
}

type snapshotMemoryMetrics struct {
	Used       uint64  `json:"used"`
	Total      uint64  `json:"total"`
	Percentage float64 `json:"percentage"`
	Error      *string `json:"error,omitempty"`
}

type snapshotSwapMetrics struct {
	Used       uint64  `json:"used"`
	Total      uint64  `json:"total"`
	Percentage float64 `json:"percentage"`
	Error      *string `json:"error,omitempty"`
}

type snapshotDiskUsageMetrics struct {
	Used       uint64  `json:"used"`
	Total      uint64  `json:"total"`
	Free       uint64  `json:"free"`
	Percentage float64 `json:"percentage"`
	Path       string  `json:"path"`
	Error      *string `json:"error,omitempty"`
}

type snapshotDiskIOMetrics struct {
	ReadBytesPerSec  *float64 `json:"read_bytes_per_sec"`
	WriteBytesPerSec *float64 `json:"write_bytes_per_sec"`
	Error            *string  `json:"error,omitempty"`
}

type snapshotNetInterface struct {
	Name            string   `json:"name"`
	RecvBytesPerSec *float64 `json:"recv_bytes_per_sec"`
	SentBytesPerSec *float64 `json:"sent_bytes_per_sec"`
	TotalRecvBytes  uint64   `json:"total_recv_bytes"`
	TotalSentBytes  uint64   `json:"total_sent_bytes"`
}

type snapshotNetIOMetrics struct {
	RecvBytesPerSec *float64               `json:"recv_bytes_per_sec"`
	SentBytesPerSec *float64               `json:"sent_bytes_per_sec"`
	Interfaces      []snapshotNetInterface `json:"interfaces"`
	Error           *string                `json:"error,omitempty"`
}

type snapshotTokenMetrics struct {
	InputTokens         int64                `json:"input_tokens"`
	OutputTokens        int64                `json:"output_tokens"`
	CacheReadTokens     int64                `json:"cache_read_tokens"`
	CacheCreationTokens int64                `json:"cache_creation_tokens"`
	TotalTokens         int64                `json:"total_tokens"`
	Prompts             int64                `json:"prompts"`
	TotalCost           float64              `json:"total_cost"`
	Rate                *float64             `json:"rate"`
	SessionAvgRate      float64              `json:"session_avg_rate"`
	TimeSpan            time.Duration        `json:"time_span"`
	EarliestTimestamp   time.Time            `json:"earliest_timestamp"`
	LatestTimestamp     time.Time            `json:"latest_timestamp"`
	LookbackFrom        time.Time            `json:"lookback_from"`
	Models              []string             `json:"models"`
	ModelUsages         []snapshotModelUsage `json:"model_usages"`
	RateHistory         []int64              `json:"rate_history"`
	Available           bool                 `json:"available"`
	Error               string               `json:"error,omitempty"`
	LastUpdate          time.Time            `json:"last_update"`
}

type snapshotModelUsage struct {
	Model               string  `json:"model"`
	Source              string  `json:"source,omitempty"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`
	PricingEstimated    bool    `json:"pricing_estimated"`
}

type snapshotSessionsMetrics struct {
	Sessions         []snapshotSession `json:"sessions"`
	Total            int               `json:"total"`
	Available        bool              `json:"available"`
	Error            string            `json:"error,omitempty"`
	LastUpdate       time.Time         `json:"last_update"`
	HooksAvailable   bool              `json:"hooks_available"`
	HooksInstalled   bool              `json:"hooks_installed"`
	Source           string            `json:"source"`
	RunningProcesses int               `json:"running_processes"`
}

type snapshotSession struct {
	Name              string                  `json:"name"`
	SessionType       string                  `json:"session_type,omitempty"`
	Worker            *snapshotWorkerMetadata `json:"worker,omitempty"`
	Windows           int                     `json:"windows"`
	Attached          bool                    `json:"attached"`
	Status            string                  `json:"status"`
	Created           time.Time               `json:"created"`
	LastContentChange time.Time               `json:"last_content_change"`
	IdleDuration      time.Duration           `json:"idle_duration"`
	LastLines         []string                `json:"last_lines,omitempty"`
	Source            string                  `json:"source,omitempty"`
	Harness           string                  `json:"harness,omitempty"`
}

type snapshotWorkerMetadata struct {
	FullName            string `json:"full_name,omitempty"`
	Workspace           string `json:"workspace,omitempty"`
	Agent               string `json:"agent,omitempty"`
	Provider            string `json:"provider,omitempty"`
	Model               string `json:"model,omitempty"`
	State               string `json:"state,omitempty"`
	CurrentBead         string `json:"current_bead,omitempty"`
	BeadStatusAvailable bool   `json:"bead_status_available"`
	BeadsProcessed      uint64 `json:"beads_processed,omitempty"`
	BeadsCompleted      uint64 `json:"beads_completed,omitempty"`
}

func makeSnapshot(timestamp time.Time, version string, system metrics.SystemMetrics, tokens *metrics.TokenMetrics, sessions *metrics.TmuxMetrics) Snapshot {
	result := Snapshot{
		SchemaVersion: snapshotSchemaVersion,
		Timestamp:     timestamp,
		Version:       version,
		System: snapshotSystemMetrics{
			CPU: snapshotCPUMetrics{
				TotalPercent: system.CPU.TotalPercent,
				PerCore:      system.CPU.PerCore,
				Error:        errorString(system.CPU.Error),
			},
			Load: snapshotLoadMetrics{
				Load1: system.Load.Load1, Load5: system.Load.Load5, Load15: system.Load.Load15,
				Error: errorString(system.Load.Error),
			},
			Memory: snapshotMemoryMetrics{
				Used: system.Memory.Used, Total: system.Memory.Total, Percentage: system.Memory.Percentage,
				Error: errorString(system.Memory.Error),
			},
			Swap: snapshotSwapMetrics{
				Used: system.Swap.Used, Total: system.Swap.Total, Percentage: system.Swap.Percentage,
				Error: errorString(system.Swap.Error),
			},
			DiskUsage: snapshotDiskUsageMetrics{
				Used: system.DiskUsage.Used, Total: system.DiskUsage.Total, Free: system.DiskUsage.Free,
				Percentage: system.DiskUsage.Percentage, Path: system.DiskUsage.Path,
				Error: errorString(system.DiskUsage.Error),
			},
			DiskIO: snapshotDiskIOMetrics{Error: errorString(system.DiskIO.Error)},
			NetIO: snapshotNetIOMetrics{
				Interfaces: make([]snapshotNetInterface, len(system.NetIO.Interfaces)),
				Error:      errorString(system.NetIO.Error),
			},
			LastUpdate: system.LastUpdate,
		},
	}
	if tokens != nil {
		result.Tokens = &snapshotTokenMetrics{
			InputTokens: tokens.InputTokens, OutputTokens: tokens.OutputTokens,
			CacheReadTokens: tokens.CacheReadTokens, CacheCreationTokens: tokens.CacheCreationTokens,
			TotalTokens: tokens.TotalTokens, Prompts: tokens.Prompts, TotalCost: tokens.TotalCost,
			Rate: nil, SessionAvgRate: tokens.SessionAvgRate, TimeSpan: tokens.TimeSpan,
			EarliestTimestamp: tokens.EarliestTimestamp, LatestTimestamp: tokens.LatestTimestamp,
			LookbackFrom: tokens.LookbackFrom, Models: tokens.Models,
			RateHistory: tokens.RateHistory, Available: tokens.Available, Error: tokens.Error,
			LastUpdate: tokens.LastUpdate,
		}
		result.Tokens.ModelUsages = make([]snapshotModelUsage, len(tokens.ModelUsages))
		for i, usage := range tokens.ModelUsages {
			result.Tokens.ModelUsages[i] = snapshotModelUsage{
				Model: usage.Model, Source: usage.Source, InputTokens: usage.InputTokens,
				OutputTokens: usage.OutputTokens, CacheReadTokens: usage.CacheReadTokens,
				CacheCreationTokens: usage.CacheCreationTokens, TotalTokens: usage.TotalTokens, Cost: usage.Cost,
				PricingEstimated: usage.PricingEstimated,
			}
		}
	}
	if sessions != nil {
		result.Sessions = &snapshotSessionsMetrics{
			Sessions: make([]snapshotSession, len(sessions.Sessions)),
			Total:    sessions.Total, Available: sessions.Available, Error: sessions.Error,
			LastUpdate: sessions.LastUpdate, HooksAvailable: sessions.HooksAvailable,
			HooksInstalled: sessions.HooksInstalled, Source: sessions.Source,
			RunningProcesses: sessions.RunningProcesses,
		}
		for i, session := range sessions.Sessions {
			result.Sessions.Sessions[i] = snapshotSession{
				Name: session.Name, SessionType: string(session.SessionType),
				Windows: session.Windows, Attached: session.Attached, Status: string(session.Status),
				Created: session.Created, LastContentChange: session.LastContentChange,
				IdleDuration: session.IdleDuration, LastLines: session.LastLines,
				Source: session.Source, Harness: session.Harness,
			}
			if session.Worker != nil {
				worker := session.Worker
				result.Sessions.Sessions[i].Worker = &snapshotWorkerMetadata{
					FullName: worker.FullName, Workspace: worker.Workspace, Agent: worker.Agent,
					Provider: worker.Provider, Model: worker.Model, State: worker.State,
					CurrentBead: worker.CurrentBead, BeadStatusAvailable: worker.BeadStatusAvailable,
					BeadsProcessed: worker.BeadsProcessed, BeadsCompleted: worker.BeadsCompleted,
				}
			}
		}
	}
	for i, iface := range system.NetIO.Interfaces {
		result.System.NetIO.Interfaces[i] = snapshotNetInterface{
			Name: iface.Name, TotalRecvBytes: iface.TotalRecvBytes, TotalSentBytes: iface.TotalSentBytes,
		}
	}
	return result
}

func errorString(err error) *string {
	if err == nil {
		return nil
	}
	message := err.Error()
	return &message
}

// runOnceMode runs a single collection cycle and outputs the result
func runOnceMode(asJSON bool, extraDirs string) int {
	return runOnceModeWithOptions(asJSON, extraDirs, time.Time{}, false)
}

// onceSessionSampleGap matches the dashboard's refresh interval, so a
// one-shot run calls a pane WORKING on the same evidence the TUI uses: its
// content changed between two samples.
const onceSessionSampleGap = 2 * time.Second

func runOnceModeWithOptions(asJSON bool, extraDirs string, since time.Time, attention bool) int {
	// Create collectors
	systemCollector := metrics.NewSystemCollector()
	tokenCollector := metrics.NewTokenCollector()
	if !since.IsZero() {
		tokenCollector.SetLookback(since)
	}
	tmuxCollector := metrics.NewTmuxCollector()

	// Add extra directories if specified
	if extraDirs != "" {
		var dirs []string
		for _, d := range strings.Split(extraDirs, ",") {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
		if len(dirs) > 0 {
			expandedDirs := metrics.ExpandGlobPatterns(dirs)
			for _, dir := range expandedDirs {
				tokenCollector.AddProjectsDir(dir)
			}
		}
	}

	// Session status compares each pane with an earlier sample, so take a
	// baseline now and the real sample after the slower collectors below,
	// at least one dashboard refresh interval later.
	tmuxCollector.Collect()
	sessionBaselineAt := time.Now()

	// Collect metrics
	snapshot := struct {
		Timestamp time.Time
		System    metrics.SystemMetrics
		Tokens    *metrics.TokenMetrics
		Sessions  *metrics.TmuxMetrics
	}{
		Timestamp: time.Now(),
		System:    systemCollector.Collect(),
	}

	// Collect token metrics (may be nil if no data available)
	tokenMetrics, err := tokenCollector.Collect()
	if err != nil {
		snapshot.Tokens = nil
	} else {
		snapshot.Tokens = tokenMetrics
	}

	// Collect session metrics
	if wait := onceSessionSampleGap - time.Since(sessionBaselineAt); wait > 0 {
		time.Sleep(wait)
	}
	snapshot.Sessions = tmuxCollector.Collect()

	// Stop background ingestion for token collector
	tokenCollector.StopBackgroundIngestion()

	// Output based on format
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(makeSnapshot(snapshot.Timestamp, version, snapshot.System, snapshot.Tokens, snapshot.Sessions)); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			return 1
		}
	} else {
		// Human-readable output
		fmt.Printf("ccdash snapshot - %s\n", snapshot.Timestamp.Format(time.RFC3339))
		fmt.Printf("Version: %s\n\n", version)

		// System metrics
		fmt.Println("=== System Resources ===")
		fmt.Printf("CPU: %.1f%% (%d cores)\n", snapshot.System.CPU.TotalPercent, len(snapshot.System.CPU.PerCore))
		fmt.Printf("Load: %.2f %.2f %.2f\n", snapshot.System.Load.Load1, snapshot.System.Load.Load5, snapshot.System.Load.Load15)
		fmt.Printf("Memory: %s / %s (%.1f%%)\n",
			metrics.FormatBytes(snapshot.System.Memory.Used),
			metrics.FormatBytes(snapshot.System.Memory.Total),
			snapshot.System.Memory.Percentage)
		if snapshot.System.Swap.Total > 0 {
			fmt.Printf("Swap: %s / %s (%.1f%%)\n",
				metrics.FormatBytes(snapshot.System.Swap.Used),
				metrics.FormatBytes(snapshot.System.Swap.Total),
				snapshot.System.Swap.Percentage)
		}
		fmt.Printf("Disk (/): %s / %s (%.1f%%)\n",
			metrics.FormatBytes(snapshot.System.DiskUsage.Used),
			metrics.FormatBytes(snapshot.System.DiskUsage.Total),
			snapshot.System.DiskUsage.Percentage)
		fmt.Printf("Disk I/O: %s read, %s write\n",
			metrics.FormatRate(snapshot.System.DiskIO.ReadBytesPerSec),
			metrics.FormatRate(snapshot.System.DiskIO.WriteBytesPerSec))
		fmt.Printf("Network: %s recv, %s sent\n",
			metrics.FormatRate(snapshot.System.NetIO.RecvBytesPerSec),
			metrics.FormatRate(snapshot.System.NetIO.SentBytesPerSec))

		// Token metrics
		fmt.Println("\n=== Token Usage ===")
		if snapshot.Tokens != nil && snapshot.Tokens.Available {
			fmt.Printf("Total Tokens: %s\n", metrics.FormatTokens(snapshot.Tokens.TotalTokens))
			fmt.Printf("  Input:  %s\n", metrics.FormatTokens(snapshot.Tokens.InputTokens))
			fmt.Printf("  Output: %s\n", metrics.FormatTokens(snapshot.Tokens.OutputTokens))
			fmt.Printf("  Cache Read: %s\n", metrics.FormatTokens(snapshot.Tokens.CacheReadTokens))
			fmt.Printf("  Cache Create: %s\n", metrics.FormatTokens(snapshot.Tokens.CacheCreationTokens))
			fmt.Printf("Total Cost: %s\n", metrics.FormatCost(snapshot.Tokens.TotalCost))
			fmt.Printf("Prompts: %d\n", snapshot.Tokens.Prompts)
			fmt.Printf("Rate: %s\n", metrics.FormatTokenRate(snapshot.Tokens.Rate))
			fmt.Printf("Session Avg: %s\n", metrics.FormatTokenRate(snapshot.Tokens.SessionAvgRate))
			fmt.Printf("Time Span: %s\n", metrics.FormatDuration(snapshot.Tokens.TimeSpan))
			if len(snapshot.Tokens.Models) > 0 {
				fmt.Println("Models:")
				for _, model := range snapshot.Tokens.Models {
					fmt.Printf("  - %s\n", model)
				}
			}
		} else {
			fmt.Println("No token data available")
			if snapshot.Tokens != nil && snapshot.Tokens.Error != "" {
				fmt.Printf("Error: %s\n", snapshot.Tokens.Error)
			}
		}

		// Session metrics
		fmt.Println("\n=== Sessions ===")
		if snapshot.Sessions.Available {
			fmt.Printf("Total Sessions: %d\n", snapshot.Sessions.Total)
			fmt.Printf("Source: %s\n", snapshot.Sessions.Source)
			fmt.Printf("Hooks Installed: %v\n", snapshot.Sessions.HooksInstalled)
			if snapshot.Sessions.RunningProcesses > 0 {
				fmt.Printf("Running Processes: %d\n", snapshot.Sessions.RunningProcesses)
			}
			if len(snapshot.Sessions.Sessions) > 0 {
				fmt.Println("Active Sessions:")
				for _, session := range snapshot.Sessions.Sessions {
					fmt.Printf("  - %s (%s) [%s]\n", session.Name, session.Harness, session.Status)
					if session.Source == "hooks" {
						fmt.Printf("    Project: %s\n", session.Name)
					}
					fmt.Printf("    Windows: %d, Attached: %v\n", session.Windows, session.Attached)
					fmt.Printf("    Idle: %s\n", metrics.FormatDuration(session.IdleDuration))
				}
			}
		} else {
			fmt.Println("No session data available")
			if snapshot.Sessions.Error != "" {
				fmt.Printf("Error: %s\n", snapshot.Sessions.Error)
			}
		}
	}

	asking := askingSessions(snapshot.Sessions.Sessions)
	if attention {
		if code := attentionExitCode(snapshot.Sessions.Sessions); code != 0 {
			printAttentionSessions(os.Stderr, asking)
			return code
		}
	}

	return 0
}

// setupHooks installs hooks, registers this instance, and returns the collector for cleanup
func setupHooks() *metrics.HookSessionCollector {
	collector, err := metrics.NewHookSessionCollector()
	if err != nil {
		// Silently continue - hooks are optional
		return nil
	}

	wasInstalled := collector.AreHooksInstalled()

	// Always run InstallHooks - it's idempotent and will add any missing hooks
	if err := collector.InstallHooks(); err != nil {
		// Installation failed - continue without hooks (tmux fallback will be used)
		fmt.Fprintf(os.Stderr, "Note: Could not install Claude Code hooks: %v\n", err)
		fmt.Fprintf(os.Stderr, "      Session tracking will use tmux fallback.\n")
		fmt.Fprintf(os.Stderr, "      Run 'ccdash --install-hooks' to retry.\n\n")
		return nil
	}

	// Clean up orphaned session files on startup
	// This removes sessions where the process died or tmux session was killed
	// without the session-end hook firing
	collector.CleanupOrphanedSessions()

	// Register this instance for multi-instance tracking
	if err := collector.RegisterInstance(); err != nil {
		// Non-fatal, continue without instance tracking
		return collector
	}

	// Only notify on fresh install (not on updates)
	if !wasInstalled {
		fmt.Println("✓ Installed Claude Code hooks for session tracking")
		fmt.Println("  Restart Claude Code sessions for hooks to take effect.")
		fmt.Println()
	}

	return collector
}

// exportCSV exports raw token event history to CSV format
func exportCSV(cache *metrics.TokenCache) error {
	return exportCSVSince(cache, time.Time{})
}

func exportCSVSince(cache *metrics.TokenCache, since time.Time) error {
	// Export token events and cached file aggregates within the requested window.
	events, err := cache.ExportTokenEventsSince(since)
	if err != nil {
		return fmt.Errorf("failed to export token events: %w", err)
	}
	events = tokenEventsSince(events, since)

	// Also export cached file aggregates
	aggregates, err := cache.ExportAllFileAggregates()
	if err != nil {
		return fmt.Errorf("failed to export file aggregates: %w", err)
	}
	aggregates = fileAggregatesSince(aggregates, since)

	// Create CSV writer
	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	// Write CSV header for raw events
	header := []string{
		"type", // "event" or "aggregate"
		"timestamp",
		"timestamp_unix",
		"model",
		"source",
		"input_tokens",
		"output_tokens",
		"cache_read_tokens",
		"cache_creation_tokens",
		"source_file",
		"line_number",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write raw token events
	for _, event := range events {
		row := []string{
			"event",
			event.Timestamp.Format(time.RFC3339Nano),
			fmt.Sprintf("%d", event.TimestampUnix),
			event.Model,
			event.Source,
			fmt.Sprintf("%d", event.InputTokens),
			fmt.Sprintf("%d", event.OutputTokens),
			fmt.Sprintf("%d", event.CacheReadTokens),
			fmt.Sprintf("%d", event.CacheCreationTokens),
			event.SourceFile,
			fmt.Sprintf("%d", event.LineNumber),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write event CSV row: %w", err)
		}
	}

	// Write file aggregates (lightly-aggregated data for complete files)
	for _, agg := range aggregates {
		// Write one row per model in the breakdown
		if len(agg.ModelBreakdown) == 0 {
			// No model breakdown, write aggregate as a single row
			row := []string{
				"aggregate",
				agg.LatestTimestamp.Format(time.RFC3339Nano),
				fmt.Sprintf("%d", agg.LatestTimestamp.Unix()),
				"*", // wildcard for all models
				agg.Source,
				fmt.Sprintf("%d", agg.TotalInputTokens),
				fmt.Sprintf("%d", agg.TotalOutputTokens),
				fmt.Sprintf("%d", agg.TotalCacheRead),
				fmt.Sprintf("%d", agg.TotalCacheCreation),
				agg.SourceFile,
				fmt.Sprintf("%d", agg.EventCount),
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("failed to write aggregate CSV row: %w", err)
			}
		} else {
			// Write one row per model in the breakdown
			for model, modelData := range agg.ModelBreakdown {
				row := []string{
					"aggregate",
					agg.LatestTimestamp.Format(time.RFC3339Nano),
					fmt.Sprintf("%d", agg.LatestTimestamp.Unix()),
					model,
					agg.Source,
					fmt.Sprintf("%d", modelData.InputTokens),
					fmt.Sprintf("%d", modelData.OutputTokens),
					fmt.Sprintf("%d", modelData.CacheReadTokens),
					fmt.Sprintf("%d", modelData.CacheCreationTokens),
					agg.SourceFile,
					fmt.Sprintf("%d", agg.EventCount),
				}
				if err := writer.Write(row); err != nil {
					return fmt.Errorf("failed to write model aggregate CSV row: %w", err)
				}
			}
		}
	}

	return nil
}

// exportJSON exports raw token event history to JSON format
func exportJSON(cache *metrics.TokenCache) error {
	return exportJSONSince(cache, time.Time{})
}

func exportJSONSince(cache *metrics.TokenCache, since time.Time) error {
	// Export token events and cached file aggregates within the requested window.
	events, err := cache.ExportTokenEventsSince(since)
	if err != nil {
		return fmt.Errorf("failed to export token events: %w", err)
	}
	events = tokenEventsSince(events, since)

	// Also export cached file aggregates
	aggregates, err := cache.ExportAllFileAggregates()
	if err != nil {
		return fmt.Errorf("failed to export file aggregates: %w", err)
	}
	aggregates = fileAggregatesSince(aggregates, since)

	// Create export structure
	type ExportData struct {
		Events      []metrics.TokenEventExport    `json:"events"`
		Aggregates  []metrics.FileAggregateExport `json:"aggregates"`
		ExportedAt  time.Time                     `json:"exported_at"`
		Version     string                        `json:"version"`
		TotalEvents int                           `json:"total_events"`
	}

	export := ExportData{
		Events:      events,
		Aggregates:  aggregates,
		ExportedAt:  time.Now(),
		Version:     version,
		TotalEvents: len(events),
	}

	// Write JSON output
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(export); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	return nil
}

// exportJSONAggregated exports aggregated token data to JSON format (legacy format)
func exportJSONAggregated(cache *metrics.TokenCache) error {
	return exportJSONAggregatedSince(cache, time.Time{})
}

func exportJSONAggregatedSince(cache *metrics.TokenCache, since time.Time) error {
	// Keep the legacy 90-day default unless the caller selects a window.
	if since.IsZero() {
		since = time.Now().AddDate(0, 0, -90)
	}
	agg, err := cache.QueryTokensSince(since)
	if err != nil {
		return fmt.Errorf("failed to get token data: %w", err)
	}

	// Create export structure
	type ModelBreakdownItem struct {
		Source              string `json:"source"`
		InputTokens         int64  `json:"input_tokens"`
		OutputTokens        int64  `json:"output_tokens"`
		CacheReadTokens     int64  `json:"cache_read_tokens"`
		CacheCreationTokens int64  `json:"cache_creation_tokens"`
	}

	type ExportData struct {
		InputTokens         int64                         `json:"input_tokens"`
		OutputTokens        int64                         `json:"output_tokens"`
		CacheReadTokens     int64                         `json:"cache_read_tokens"`
		CacheCreationTokens int64                         `json:"cache_creation_tokens"`
		EarliestTimestamp   string                        `json:"earliest_timestamp"`
		LatestTimestamp     string                        `json:"latest_timestamp"`
		EventCount          int64                         `json:"event_count"`
		ModelBreakdown      map[string]ModelBreakdownItem `json:"model_breakdown"`
		ExportedAt          time.Time                     `json:"exported_at"`
		Version             string                        `json:"version"`
	}

	modelBreakdown := make(map[string]ModelBreakdownItem)
	for model, metrics := range agg.ModelMetrics {
		modelBreakdown[model] = ModelBreakdownItem{
			Source:              metrics.Source,
			InputTokens:         metrics.InputTokens,
			OutputTokens:        metrics.OutputTokens,
			CacheReadTokens:     metrics.CacheReadTokens,
			CacheCreationTokens: metrics.CacheCreationTokens,
		}
	}

	export := ExportData{
		InputTokens:         agg.InputTokens,
		OutputTokens:        agg.OutputTokens,
		CacheReadTokens:     agg.CacheReadTokens,
		CacheCreationTokens: agg.CacheCreationTokens,
		EarliestTimestamp:   agg.EarliestTimestamp.Format(time.RFC3339),
		LatestTimestamp:     agg.LatestTimestamp.Format(time.RFC3339),
		EventCount:          agg.EventCount,
		ModelBreakdown:      modelBreakdown,
		ExportedAt:          time.Now(),
		Version:             version,
	}

	// Write JSON output
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(export); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	return nil
}

func printHelp() {
	fmt.Println("ccdash - Claude Code + Codex Dashboard")
	fmt.Println()
	fmt.Printf("Version: %s\n", version)
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  ccdash [OPTIONS]")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  --version             Show version information")
	fmt.Println("  --help                Show this help message")
	fmt.Println("  --install-hooks       Install Claude Code hooks for session tracking")
	fmt.Println("  --install-codex-hooks Install Codex hooks for session tracking")
	fmt.Println("  --check-hooks         Check Claude Code and Codex hooks")
	fmt.Println("  --uninstall-hooks     Remove ccdash hooks from both harnesses")
	fmt.Println("  --test-notify         Test notification webhook configuration")
	fmt.Println("  --doctor              Check data sources, cache, config and hooks; exit 1 on a problem")
	fmt.Println("  --once                Run a single collection cycle and exit (no TUI)")
	fmt.Println("  --json                Output metrics as JSON (use with --once)")
	fmt.Println("  --since=<window>      Token window: monday, today, 24h, 7d, or an RFC3339 timestamp; use with --once or --export")
	fmt.Println("  --attention           List ASKING sessions and exit 1 if any need a human (also works with --once)")
	fmt.Println("  --extra-dirs=<dirs>   Additional Claude project root directories to scan")
	fmt.Println("                        Comma-separated list of paths")
	fmt.Println("                        Also configurable via CCDASH_EXTRA_DIRS env var (colon-separated)")
	fmt.Println("  --export=<format>      Export token cache to stdout (csv|json|json-aggregated)")
	fmt.Println("                        csv: Raw event history + file aggregates")
	fmt.Println("                        json: Raw event history + file aggregates")
	fmt.Println("                        json-aggregated: Summary statistics (legacy)")
	fmt.Println()
	fmt.Println("KEYBOARD SHORTCUTS:")
	for _, shortcut := range ui.DashboardKeyboardShortcuts() {
		fmt.Printf("  %-15s %s\n", shortcut.Keys, shortcut.Description)
	}
	fmt.Println()
	fmt.Println("PANELS:")
	fmt.Println("  System Resources  - CPU, memory, swap, disk I/O, and load averages")
	fmt.Println("  Token Usage       - Claude Code token consumption and costs")
	fmt.Println("  Sessions          - Active Claude Code and Codex sessions with status indicators")
	fmt.Println()
	fmt.Println("SESSION TRACKING:")
	fmt.Println("  ccdash supports tmux fallback and status-only hooks for Claude Code and Codex:")
	fmt.Println()
	fmt.Println("  1. Hooks (recommended) - Real-time tracking via Claude Code hooks")
	fmt.Println("     Run 'ccdash --install-hooks' to enable")
	fmt.Println("     Icon: 🔗 indicates hook-based tracking is active")
	fmt.Println()
	fmt.Println("  2. Tmux (fallback) - Monitors tmux sessions for Claude Code")
	fmt.Println("     Icon: 📺 indicates tmux-based tracking")
	fmt.Println("     Requires Claude Code to run in tmux sessions")
	fmt.Println()
	fmt.Println("LAYOUT MODES:")
	fmt.Println("  Wide (>=140 cols)                 - 3 panels side-by-side")
	fmt.Println("  Narrow (<140 cols)                - Panels stacked vertically")
	fmt.Println()
	fmt.Println("STATUS INDICATORS:")
	for _, status := range metrics.SessionStatuses() {
		fmt.Printf("  %s %-8s - %s\n", status.GetEmoji(), status, status.Description())
	}
	fmt.Println()
	fmt.Println("REQUIREMENTS:")
	fmt.Println("  - Terminal size: minimum 80x24 characters")
	fmt.Println("  - True color support recommended")
	fmt.Println("  - Claude Code with ~/.claude/projects or Codex with ~/.codex/sessions (for token usage)")
	fmt.Println("  - jq (for hooks, usually pre-installed)")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  ccdash                                    Start the dashboard")
	fmt.Println("  ccdash --install-hooks                    Install Claude Code hooks")
	fmt.Println("  ccdash --check-hooks                      Verify hooks installation")
	fmt.Println("  ccdash --version                          Show version")
	fmt.Println("  ccdash --help                             Show this help")
	fmt.Println("  ccdash --once                            Single collection cycle (human-readable)")
	fmt.Println("  ccdash --once --json                      Single collection cycle (JSON output)")
	fmt.Println("  ccdash --once --json --since=7d           JSON snapshot using the last seven days")
	fmt.Println("  ccdash --attention                        List sessions waiting for a human")
	fmt.Println("  ccdash --doctor                           Diagnose missing data, hooks or config")
	fmt.Println("  ccdash --extra-dirs=/alt/path             Scan additional project directory")
	fmt.Println("  ccdash --extra-dirs=/path1,/path2         Scan multiple extra directories")
	fmt.Println("  CCDASH_EXTRA_DIRS=/path1:/path2 ccdash    Use env var for extra directories")
	fmt.Println("  ccdash --export=csv                     Export raw token events as CSV")
	fmt.Println("  ccdash --export=json                    Export raw token events as JSON")
	fmt.Println("  ccdash --export=json-aggregated          Export aggregated summary as JSON")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/jedarden/ccdash")
}
