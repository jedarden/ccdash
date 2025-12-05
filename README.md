# 📊 ccdash

**Version:** `0.6.0`

> A lightweight TUI (Terminal User Interface) dashboard for monitoring system resources, Claude Code token usage, and tmux sessions.

---

## 🎯 What is ccdash?

**ccdash** is a real-time dashboard application that provides:

- 🖥️ **System Resource Monitoring** - Track CPU usage, memory consumption, disk space, and network activity
- 🤖 **Claude Code Token Tracking** - Monitor token usage across your Claude Code projects by reading from `~/.claude/projects`
- 🪟 **Tmux Session Management** - View active tmux sessions and their status (optional, works without tmux)

All in a beautiful, terminal-based interface built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

---

## ✨ Key Features

- ⚡ Real-time system metrics display
- 📈 Token usage analytics from Claude Code projects
- 🔄 Tmux session monitoring (when available)
- 🪶 Lightweight and fast - minimal system overhead
- 🎨 Beautiful TUI with clean, organized layout
- ⌨️ Keyboard-driven navigation
- 📦 No external dependencies beyond Go and optional tmux
- 🔄 **Self-update** - Press `u` to update when new version available
- 📅 **Lookback picker** - Press `l` to change time window for token tracking
- 💰 **Per-model cost tracking** - Color-coded breakdown by Claude model
- 💾 **SQLite caching** - Queryable `.ccdash/tokens.db` with DuckDB/SQLite support

---

## 📋 Requirements

- **Go 1.21 or higher** 🔧
- **`~/.claude/projects` directory** (for Claude Code token tracking) 📂
- **tmux** (optional, for session monitoring features) 🪟

---

## 🚀 Installation

### Pre-built Binary (Recommended)

Download the latest release from the [releases page](https://github.com/jedarden/ccdash/releases):

```bash
# Download the latest release (Linux example)
curl -LO https://github.com/jedarden/ccdash/releases/download/v0.1.4/ccdash-linux-amd64
curl -LO https://github.com/jedarden/ccdash/releases/download/v0.1.4/ccdash-linux-amd64.sha256

# Verify the checksum
sha256sum -c ccdash-linux-amd64.sha256

# Make it executable
chmod +x ccdash-linux-amd64

# Move to your PATH (optional)
sudo mv ccdash-linux-amd64 /usr/local/bin/ccdash

# Run it
ccdash
```

### Using Go Install

```bash
go install github.com/jedarden/ccdash/cmd/ccdash@latest
```

### From Source

```bash
# Clone the repository
git clone https://github.com/jedarden/ccdash.git
cd ccdash

# Build and install
make install
```

### Manual Build

```bash
# Build the binary
make build

# The binary will be available at ./bin/ccdash
./bin/ccdash
```

---

## 💻 Usage

Simply run the application:

```bash
ccdash
```

### ⌨️ Keyboard Controls

| Key | Action |
|-----|--------|
| `q` or `Ctrl+C` | Quit the application |
| `r` | Refresh metrics immediately |
| `h` | Cycle through help mode (explains each panel) |
| `l` | Open lookback time picker for token tracking |
| `u` | Update to latest version (when available) |

### 🎨 Display Features

- **Smart Layout**: Automatically adjusts to terminal size
  - Ultra-wide (≥240 cols): 3 panels side-by-side
  - Wide (120-239 cols): 2 panels top, 1 bottom
  - Narrow (<120 cols): Panels stacked vertically
- **Tmux Status Indicators**:
  - 🟢 WORKING - Claude Code actively processing
  - 🔴 READY - Waiting for user input at prompt
  - 🟡 ACTIVE - User actively in session
  - ⚠️ ERROR - Error state detected
- **Help Mode**: Press `h` to cycle through detailed explanations for each panel
- **Lookback Picker**: Press `l` to select time window
  - Presets: Monday 9am, Today, 24h, 7d, 30d, All time
  - Custom: Set specific date/time with arrow keys
- **Self-Update**: Status bar shows when updates are available
  - Press `u` to download and apply update automatically
- **Per-Model Costs**: Token panel shows breakdown by model
  - Color-coded: Opus (red), Sonnet (cyan), Haiku (green)
  - Sorted by cost (highest first)

---

## 🛠️ Development

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

---

## 📁 Project Structure

```
ccdash/
├── cmd/ccdash/          # Main application entry point
├── internal/
│   ├── metrics/         # Metrics collectors (system, tokens, tmux)
│   └── ui/              # Bubble Tea UI components
├── Makefile             # Build automation
└── README.md            # This file
```

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

---

## 📄 License

MIT License - see LICENSE file for details

---

## 🙏 Acknowledgments

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [gopsutil](https://github.com/shirou/gopsutil) - System metrics collection
