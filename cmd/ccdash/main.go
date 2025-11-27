package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedarden/ccdash/internal/ui"
	"golang.org/x/term"
)

const (
	version = "0.3.0"
)

func main() {
	// Parse command-line flags
	var (
		showVersion = flag.Bool("version", false, "Show version information")
		showHelp    = flag.Bool("help", false, "Show help information")
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

	// Check if running in a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stderr, "Error: ccdash must be run in a terminal")
		os.Exit(1)
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

func printHelp() {
	fmt.Println("ccdash - Claude Code Dashboard")
	fmt.Println()
	fmt.Printf("Version: %s\n", version)
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  ccdash [OPTIONS]")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  --version    Show version information")
	fmt.Println("  --help       Show this help message")
	fmt.Println()
	fmt.Println("KEYBOARD SHORTCUTS:")
	fmt.Println("  q, Ctrl+C    Quit the dashboard")
	fmt.Println("  r            Refresh metrics immediately")
	fmt.Println("  1            Focus on System Resources panel")
	fmt.Println("  2            Focus on Token Usage panel")
	fmt.Println("  3            Focus on TMUX Sessions panel")
	fmt.Println()
	fmt.Println("PANELS:")
	fmt.Println("  System Resources  - CPU, memory, swap, disk I/O, and load averages")
	fmt.Println("  Token Usage       - Claude Code token consumption and costs")
	fmt.Println("  TMUX Sessions     - Active tmux sessions with status indicators")
	fmt.Println()
	fmt.Println("LAYOUT MODES:")
	fmt.Println("  Ultra-wide (>=240 cols)           - 3 panels side-by-side")
	fmt.Println("  Wide (120-239 cols, >=30 lines)   - 2 panels top, 1 bottom")
	fmt.Println("  Narrow (<120 cols)                - Panels stacked vertically")
	fmt.Println()
	fmt.Println("STATUS INDICATORS (TMUX):")
	fmt.Println("  🟢 WORKING   - Claude Code is actively processing")
	fmt.Println("  🔴 READY     - Waiting for input at prompt")
	fmt.Println("  🟡 ACTIVE    - Recent activity detected")
	fmt.Println("  💤 IDLE      - No activity for >5 minutes")
	fmt.Println("  ⚠️  STALLED  - Error or no progress detected")
	fmt.Println()
	fmt.Println("REQUIREMENTS:")
	fmt.Println("  - Terminal size: minimum 80x24 characters")
	fmt.Println("  - True color support recommended")
	fmt.Println("  - tmux (optional, for session monitoring)")
	fmt.Println("  - Claude Code with ~/.claude/projects (for token usage)")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  ccdash              Start the dashboard")
	fmt.Println("  ccdash --version    Show version")
	fmt.Println("  ccdash --help       Show this help")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/jedarden/ccdash")
}
