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
		fmt.Println("  • Stop               - Marks session as ready for input")
		fmt.Println("  • Notification       - Marks session as ready for input (needs permission or idle)")
		fmt.Println("  • PermissionRequest  - Marks session as ready for input (approval needed)")
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
			if err := exportCSV(cache); err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting CSV: %v\n", err)
				os.Exit(1)
			}
		case "json":
			if err := exportJSON(cache); err != nil {
				fmt.Fprintf(os.Stderr, "Error exporting JSON: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "Error: export format must be 'csv' or 'json', got '%s'\n", *exportFormat)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle --once (single collection cycle, no TUI)
	if *runOnce {
		os.Exit(runOnceMode(*jsonOutput, *extraDirs))
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
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running dashboard: %v\n", err)
		os.Exit(1)
	}
}

// Snapshot represents a single-point-in-time collection of all metrics
type Snapshot struct {
	Timestamp   time.Time               `json:"timestamp"`
	Version     string                  `json:"version"`
	System      metrics.SystemMetrics   `json:"system"`
	Tokens      *metrics.TokenMetrics   `json:"tokens"`
	Sessions    *metrics.TmuxMetrics    `json:"sessions"`
}

// runOnceMode runs a single collection cycle and outputs the result
func runOnceMode(asJSON bool, extraDirs string) int {
	// Create collectors
	systemCollector := metrics.NewSystemCollector()
	tokenCollector := metrics.NewTokenCollector()
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

	// Collect metrics
	snapshot := Snapshot{
		Timestamp: time.Now(),
		Version:   version,
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
	snapshot.Sessions = tmuxCollector.Collect()

	// Stop background ingestion for token collector
	tokenCollector.StopBackgroundIngestion()

	// Output based on format
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(snapshot); err != nil {
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

// exportCSV exports aggregated token data to CSV format
func exportCSV(cache *metrics.TokenCache) error {
	// Query aggregated data for the last 90 days
	since := time.Now().AddDate(0, 0, -90)
	agg, err := cache.QueryTokensSince(since)
	if err != nil {
		return fmt.Errorf("failed to get token data: %w", err)
	}

	// Create CSV writer
	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	// Write CSV header
	if err := writer.Write([]string{
		"Metric", "Value",
	}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write summary data
	rows := [][]string{
		{"Total Input Tokens", fmt.Sprintf("%d", agg.InputTokens)},
		{"Total Output Tokens", fmt.Sprintf("%d", agg.OutputTokens)},
		{"Total Cache Read Tokens", fmt.Sprintf("%d", agg.CacheReadTokens)},
		{"Total Cache Creation Tokens", fmt.Sprintf("%d", agg.CacheCreationTokens)},
		{"Earliest Timestamp", agg.EarliestTimestamp.Format(time.RFC3339)},
		{"Latest Timestamp", agg.LatestTimestamp.Format(time.RFC3339)},
		{"Event Count", fmt.Sprintf("%d", agg.EventCount)},
	}

	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	// Write per-model breakdown
	for model, metrics := range agg.ModelMetrics {
		modelRows := [][]string{
			{fmt.Sprintf("Model: %s (Source)", model), metrics.Source},
			{fmt.Sprintf("Model: %s (Input)", model), fmt.Sprintf("%d", metrics.InputTokens)},
			{fmt.Sprintf("Model: %s (Output)", model), fmt.Sprintf("%d", metrics.OutputTokens)},
		}
		for _, row := range modelRows {
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("failed to write model CSV row: %w", err)
			}
		}
	}

	return nil
}

// exportJSON exports aggregated token data to JSON format
func exportJSON(cache *metrics.TokenCache) error {
	// Query aggregated data for the last 90 days
	since := time.Now().AddDate(0, 0, -90)
	agg, err := cache.QueryTokensSince(since)
	if err != nil {
		return fmt.Errorf("failed to get token data: %w", err)
	}

	// Create export structure
	type ModelBreakdownItem struct {
		Source       string `json:"source"`
		InputTokens  int64  `json:"input_tokens"`
		OutputTokens int64  `json:"output_tokens"`
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
	}

	modelBreakdown := make(map[string]ModelBreakdownItem)
	for model, metrics := range agg.ModelMetrics {
		modelBreakdown[model] = ModelBreakdownItem{
			Source:       metrics.Source,
			InputTokens:  metrics.InputTokens,
			OutputTokens: metrics.OutputTokens,
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
	fmt.Println("  --once                Run a single collection cycle and exit (no TUI)")
	fmt.Println("  --json                Output metrics as JSON (use with --once)")
	fmt.Println("  --extra-dirs=<dirs>   Additional Claude project root directories to scan")
	fmt.Println("                        Comma-separated list of paths")
	fmt.Println("                        Also configurable via CCDASH_EXTRA_DIRS env var (colon-separated)")
	fmt.Println("  --export=<format>      Export token cache to stdout (csv|json)")
	fmt.Println()
	fmt.Println("KEYBOARD SHORTCUTS:")
	fmt.Println("  q, Ctrl+C    Quit the dashboard")
	fmt.Println("  r            Refresh metrics immediately")
	fmt.Println("  h            Cycle through help panels")
	fmt.Println("  l            Open token usage lookback picker")
	fmt.Println("  1            Focus on System Resources panel")
	fmt.Println("  2            Focus on Token Usage panel")
	fmt.Println("  3            Focus on Sessions panel")
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
	fmt.Println("  Ultra-wide (>=240 cols)           - 3 panels side-by-side")
	fmt.Println("  Wide (120-239 cols, >=30 lines)   - 2 panels top, 1 bottom")
	fmt.Println("  Narrow (<120 cols)                - Panels stacked vertically")
	fmt.Println()
	fmt.Println("STATUS INDICATORS:")
	fmt.Println("  🟢 WORKING   - Claude Code is actively processing")
	fmt.Println("  🔴 READY     - Waiting for input at prompt")
	fmt.Println("  🟡 ACTIVE    - Recent activity detected")
	fmt.Println("  💤 IDLE      - No activity for >5 minutes")
	fmt.Println("  ❌ STALLED  - Error or stale session detected")
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
	fmt.Println("  ccdash --extra-dirs=/alt/path             Scan additional project directory")
	fmt.Println("  ccdash --extra-dirs=/path1,/path2         Scan multiple extra directories")
	fmt.Println("  CCDASH_EXTRA_DIRS=/path1:/path2 ccdash    Use env var for extra directories")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/jedarden/ccdash")
}
