# ccdash v0.1.0 Release Notes

**Release Date**: 2025-11-21

## Overview

ccdash is a lightweight TUI (Terminal User Interface) dashboard for monitoring system resources, Claude Code token usage, and tmux sessions. This is the initial release.

## Features

### System Metrics Monitoring
- Real-time CPU usage (total and per-core)
- Load averages (1, 5, 15 minutes)
- Memory usage with visual bars
- Swap space monitoring
- Disk I/O rates (read/write)
- htop-style progress bars with color-coded thresholds

### Token Usage Tracking
- Native JSONL parsing from `~/.claude/projects`
- Token counts (input, output, cache read, cache creation)
- Cost estimation
- Token rate calculation (60s window and session average)
- Time span tracking
- Model identification

### TMUX Session Monitoring
- Active session detection
- Status indicators (WORKING, STALLED, IDLE, READY)
- Window count tracking
- Attachment status
- Color-coded emojis
- Grid layout with adaptive columns

### User Interface
- Responsive layouts:
  - Ultra-wide (≥240 cols): 3 panels side-by-side
  - Wide (120-239 cols): 2 panels top, 1 bottom
  - Narrow (<120 cols): Stacked vertically
- Auto-refresh every 2 seconds
- Keyboard shortcuts (q, r, 1/2/3)
- Beautiful Bubble Tea + Lipgloss styling
- Graceful error handling

## Technical Details

### Built With
- Go 1.21+
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [gopsutil](https://github.com/shirou/gopsutil) - System metrics

### Architecture
```
ccdash/
├── cmd/ccdash/          # Main application entry point
├── internal/
│   ├── metrics/         # Metrics collectors
│   │   ├── system.go    # CPU, memory, disk I/O
│   │   ├── tokens.go    # Token usage tracking
│   │   ├── claude.go    # Claude Code JSONL parser
│   │   └── tmux.go      # TMUX session monitoring
│   └── ui/
│       ├── dashboard.go # Main TUI implementation
│       └── styles.go    # Lipgloss styles
└── Makefile            # Build automation
```

### Requirements
- Go 1.21 or higher
- `~/.claude/projects` directory (for token tracking)
- tmux (optional, for session monitoring)
- Terminal size: minimum 80x24, recommended 120x40

## Installation

### From Source
```bash
git clone https://github.com/jedarden/ccdash.git
cd ccdash
make install
```

### Using Go Install
```bash
go install github.com/jedarden/ccdash/cmd/ccdash@v0.1.0
```

### Manual Build
```bash
make build
./bin/ccdash
```

## Usage

```bash
# Run the dashboard
ccdash

# Show version
ccdash --version

# Show help
ccdash --help
```

### Keyboard Shortcuts
- `q` or `Ctrl+C` - Quit
- `r` - Refresh metrics
- `1` - Focus System Resources panel
- `2` - Focus Token Usage panel
- `3` - Focus TMUX Sessions panel

## Testing

Comprehensive test coverage included:
- System metrics: 10 unit tests
- Token tracking: 16 unit tests
- TMUX monitoring: 15+ unit tests
- Integration tests for all components

Run tests with:
```bash
make test
```

## Known Limitations

- First disk I/O collection shows 0 (requires two samples for rate calculation)
- TMUX status detection requires multiple collections to be accurate
- Token tracking requires Claude Code project files

## Future Enhancements (v0.2.0+)

- Network traffic monitoring
- Process listing and management
- Historical metrics graphs
- Configuration file support
- Custom refresh intervals
- Export metrics to file
- Additional layout modes

## Contributors

Built by 5 parallel agents working on different components:
- Agent 1: Project structure and setup
- Agent 2: System metrics collector
- Agent 3: Token usage tracker
- Agent 4: TMUX session monitor
- Agent 5: TUI dashboard implementation

## License

MIT License

## Links

- Repository: https://github.com/jedarden/ccdash
- Issues: https://github.com/jedarden/ccdash/issues
- Release: https://github.com/jedarden/ccdash/releases/tag/v0.1.0
