package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
		showVersion  = flag.Bool("version", false, "Show version information")
		showHelp     = flag.Bool("help", false, "Show help information")
		installHooks = flag.Bool("install-hooks", false, "Install Claude Code hooks for session tracking")
		checkHooks   = flag.Bool("check-hooks", false, "Check if Claude Code hooks are installed")
	)

	flag.Parse()

	// Handle --version
	if *showVersion {
		fmt.Printf("ccdash version %s\n", version)
		fmt.Println("Claude Code Dashboard - A terminal UI for monitoring system resources, token usage, and tmux sessions")
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
		fmt.Println("  • SessionStart      - Registers new Claude Code sessions")
		fmt.Println("  • UserPromptSubmit  - Marks session as working")
		fmt.Println("  • Stop              - Marks session as waiting for input")
		fmt.Println("  • SessionEnd        - Unregisters sessions when they end")
		fmt.Println()
		fmt.Printf("Session data will be written to: %s/sessions/\n", collector.GetBaseDir())
		fmt.Println()
		fmt.Println("Restart any running Claude Code sessions for hooks to take effect.")
		os.Exit(0)
	}

	// Handle --check-hooks
	if *checkHooks {
		collector, err := metrics.NewHookSessionCollector()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if collector.AreHooksInstalled() {
			fmt.Println("✓ Claude Code hooks are installed")
			fmt.Printf("  Hook scripts: %s/hooks/\n", collector.GetBaseDir())
			fmt.Printf("  Session data: %s/sessions/\n", collector.GetBaseDir())

			// Check for active sessions
			sessions, err := collector.CollectSessions()
			if err == nil {
				fmt.Printf("  Active sessions: %d\n", len(sessions))
			}
		} else {
			fmt.Println("✗ Claude Code hooks are NOT installed")
			fmt.Println()
			fmt.Println("Run 'ccdash --install-hooks' to install them.")
			os.Exit(1)
		}
		os.Exit(0)
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

func printHelp() {
	fmt.Println("ccdash - Claude Code Dashboard")
	fmt.Println()
	fmt.Printf("Version: %s\n", version)
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  ccdash [OPTIONS]")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  --version        Show version information")
	fmt.Println("  --help           Show this help message")
	fmt.Println("  --install-hooks  Install Claude Code hooks for session tracking")
	fmt.Println("  --check-hooks    Check if Claude Code hooks are installed")
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
	fmt.Println("  Sessions          - Active Claude Code sessions with status indicators")
	fmt.Println()
	fmt.Println("SESSION TRACKING:")
	fmt.Println("  ccdash supports two methods for tracking Claude Code sessions:")
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
	fmt.Println("  - Claude Code with ~/.claude/projects (for token usage)")
	fmt.Println("  - jq (for hooks, usually pre-installed)")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  ccdash                  Start the dashboard")
	fmt.Println("  ccdash --install-hooks  Install Claude Code hooks")
	fmt.Println("  ccdash --check-hooks    Verify hooks installation")
	fmt.Println("  ccdash --version        Show version")
	fmt.Println("  ccdash --help           Show this help")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/jedarden/ccdash")
}
