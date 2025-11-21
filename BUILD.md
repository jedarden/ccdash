# ccdash - Build Instructions

## Overview
ccdash is a terminal UI dashboard for monitoring system resources, Claude Code token usage, and tmux sessions.

## Requirements
- Go 1.22.2 or later
- Terminal with true color support (recommended)
- tmux (optional, for session monitoring)
- Claude Code with `~/.claude/projects` directory (for token usage tracking)

## Building

### From Source
```bash
cd /workspaces/test-agor/ccdash
go build -o ccdash ./cmd/ccdash
```

### Install Dependencies
```bash
go mod download
```

### Run Tests
```bash
go test ./...
```

## Running

### Start the Dashboard
```bash
./ccdash
```

### Command-line Options
- `--version` - Show version information
- `--help` - Show help message

## Architecture

### Directory Structure
```
ccdash/
├── cmd/
│   └── ccdash/
│       └── main.go           # Entry point with CLI flags
├── internal/
│   ├── metrics/              # Metrics collectors
│   │   ├── system.go         # System resource metrics
│   │   ├── tokens.go         # Token usage metrics
│   │   ├── tmux.go           # Tmux session metrics
│   │   └── claude.go         # Claude Code usage parser
│   └── ui/
│       └── dashboard.go      # Bubble Tea TUI implementation
├── go.mod
└── BUILD.md
```

### Key Components

#### 1. Metrics Collectors (`internal/metrics/`)
- **SystemCollector**: Collects CPU, memory, swap, disk I/O, and load averages using gopsutil
- **TokenCollector**: Parses Claude Code JSONL files to track token usage and costs
- **TmuxCollector**: Monitors tmux sessions and their status

#### 2. Dashboard UI (`internal/ui/dashboard.go`)
- **Bubble Tea Model**: Implements the Model-View-Update pattern
- **Three Panel Layout**: System Resources, Token Usage, and Tmux Sessions
- **Responsive Design**: Adapts to terminal size with three layout modes
- **Auto-refresh**: Updates every 2 seconds automatically

#### 3. Main Entry Point (`cmd/ccdash/main.go`)
- Command-line flag parsing (--version, --help)
- Terminal size validation
- Dashboard initialization and execution

## Features

### Layout Modes
1. **Ultra-wide (≥240 cols)**: 3 panels side-by-side
2. **Wide (120-239 cols, ≥30 lines)**: 2 panels top, 1 bottom
3. **Narrow (<120 cols)**: Panels stacked vertically

### Keyboard Shortcuts
- `q` or `Ctrl+C` - Quit the dashboard
- `r` - Refresh metrics immediately
- `1` - Focus System Resources panel
- `2` - Focus Token Usage panel
- `3` - Focus TMUX Sessions panel

### Panel Details

#### System Resources Panel
- Load averages (1, 5, 15 minutes)
- CPU usage (total + per-core with htop-style bars)
- Memory usage with bar graph
- Swap usage with bar graph
- Disk I/O rates (read/write)

#### Token Usage Panel
- Input/Output token counts
- Cache creation/read tokens
- Total tokens with formatting
- Estimated costs (Claude Sonnet 4.5 pricing)
- Current rate (tokens/min)
- Session average rate
- Time span
- Models used

#### TMUX Sessions Panel
- Grid layout of sessions
- Status indicators with emojis:
  - 🟢 WORKING - Actively processing
  - 🔴 READY - Waiting at prompt
  - 🟡 ACTIVE - Recent activity
  - 💤 IDLE - No activity >5 minutes
  - ⚠️ STALLED - Errors detected
- Window count
- Attached status
- Session age

### Styling
- Colored panels with rounded borders (lipgloss)
- htop-style progress bars with percentage inside
- Color-coded thresholds (green/yellow/orange/red)
- Focused panel highlighting (green border)
- Status bar with keyboard shortcuts

## Dependencies

### Main Dependencies
- `github.com/charmbracelet/bubbletea` v1.3.10 - TUI framework
- `github.com/charmbracelet/lipgloss` v1.1.0 - Styling library
- `github.com/shirou/gopsutil/v3` v3.24.5 - System metrics
- `golang.org/x/term` v0.37.0 - Terminal operations

### Transitive Dependencies
See `go.mod` for complete list.

## Implementation Notes

### Metrics Collection
- System metrics collected via gopsutil library
- Token metrics parsed from `~/.claude/projects/*/sessions/*.jsonl`
- Tmux metrics gathered via `tmux list-sessions` command
- All collectors designed to handle errors gracefully

### Performance
- Auto-refresh every 2 seconds
- Non-blocking metrics collection using Bubble Tea commands
- Efficient rendering with lipgloss layout system
- No external API calls (all data local)

### Error Handling
- Graceful degradation when metrics unavailable
- Error messages displayed in respective panels
- Continued operation even if one collector fails

## Future Enhancements (Not Implemented)
- Configurable refresh interval
- Panel selection persistence
- Export metrics to CSV/JSON
- Historical graphs
- Alert thresholds configuration
- Multiple Claude Code project support
- SSH remote monitoring

## Troubleshooting

### Build Errors
```bash
# Clean and rebuild
go clean
go mod tidy
go build -o ccdash ./cmd/ccdash
```

### Terminal Size Warning
Minimum recommended: 80x24 characters. Resize terminal or continue anyway.

### Token Usage Not Showing
- Ensure `~/.claude/projects` directory exists
- Check that current directory is a Claude Code project
- Verify JSONL files exist in project sessions directory

### Tmux Sessions Not Showing
- Install tmux: `sudo apt-get install tmux`
- Ensure tmux server is running: `tmux ls`

## License
See project root for license information.
