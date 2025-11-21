# ccdash

Version: **0.1.0**

A lightweight TUI (Terminal User Interface) dashboard for monitoring system resources, Claude Code token usage, and tmux sessions.

## What is ccdash?

ccdash is a real-time dashboard application that provides:

- **System Resource Monitoring**: Track CPU usage, memory consumption, disk space, and network activity
- **Claude Code Token Tracking**: Monitor token usage across your Claude Code projects by reading from `~/.claude/projects`
- **Tmux Session Management**: View active tmux sessions and their status (optional, works without tmux)

All in a beautiful, terminal-based interface built with Bubble Tea.

## Key Features

- Real-time system metrics display
- Token usage analytics from Claude Code projects
- Tmux session monitoring (when available)
- Lightweight and fast - minimal system overhead
- Beautiful TUI with clean, organized layout
- Keyboard-driven navigation
- No external dependencies beyond Go and optional tmux

## Requirements

- **Go 1.21 or higher**
- **~/.claude/projects directory** (for Claude Code token tracking)
- **tmux** (optional, for session monitoring features)

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/jedarden/ccdash.git
cd ccdash

# Build and install
make install
```

### Using Go Install

```bash
go install github.com/jedarden/ccdash/cmd/ccdash@latest
```

### Manual Build

```bash
# Build the binary
make build

# The binary will be available at ./bin/ccdash
./bin/ccdash
```

## Usage

Simply run the application:

```bash
ccdash
```

### Keyboard Controls

- `q` or `Ctrl+C` - Quit
- `r` - Refresh metrics
- `Tab` - Switch between panels
- Arrow keys - Navigate within panels

## Development

### Building

```bash
make build
```

### Running Tests

```bash
make test
```

### Installing Dependencies

```bash
make deps
```

### Cleaning Build Artifacts

```bash
make clean
```

## Project Structure

```
ccdash/
├── cmd/ccdash/          # Main application entry point
├── internal/
│   ├── metrics/         # Metrics collectors (system, tokens, tmux)
│   └── ui/              # Bubble Tea UI components
├── Makefile             # Build automation
└── README.md            # This file
```

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## License

MIT License - see LICENSE file for details

## Acknowledgments

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [gopsutil](https://github.com/shirou/gopsutil) - System metrics collection
