# ccdash

A lightweight terminal dashboard for Claude Code — shows token usage, cost, agent session status, and system resources in real time.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

---

## What it shows

**Token panel** — aggregates usage from Claude Code's JSONL logs in `~/.claude/projects`. Displays input, output, and cache tokens; total cost; tokens/min rate; and a per-model cost breakdown (Opus, Sonnet, Haiku, etc.), color-coded and sorted by spend.

**Session panel** — shows active Claude Code agent sessions and their current state:

| Status | Meaning |
|--------|---------|
| WORKING | Claude is actively processing a turn |
| ASKING | Claude asked the human a question, waiting for input |
| READY | Prompt is idle, waiting for the next message |
| ACTIVE | User is typing in the session |

Session tracking has two modes: tmux pane inspection (automatic) and hook-based tracking (more accurate, install with `ccdash --install-hooks`).

**System panel** — CPU, memory, swap, disk, network I/O, and load average via [gopsutil](https://github.com/shirou/gopsutil).

---

## Installation

### Pre-built binary

Download the latest release from the [releases page](https://github.com/jedarden/ccdash/releases), make it executable, and move it to your PATH.

### Using Go

Requires Go 1.21+.

```bash
go install github.com/jedarden/ccdash/cmd/ccdash@latest
```

### From source

```bash
git clone https://github.com/jedarden/ccdash.git
cd ccdash
make install
```

---

## Usage

```bash
ccdash
```

### Keyboard controls

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `r` | Force refresh |
| `h` | Cycle help panels (explains each section) |
| `l` | Open lookback picker (change the token measurement window) |
| `u` | Self-update to latest release (when available) |

### Lookback window

Press `l` to change how far back the token panel looks:

- Monday 9am (default — useful for weekly work tracking)
- Today
- Last 24h / 7d / 30d / All time
- Custom date and time (navigate with arrow keys)

### Layout

ccdash automatically adjusts to your terminal width:

- **Narrow** (< 120 cols): panels stacked vertically
- **Wide** (120–239 cols): two panels on top, one below
- **Ultra-wide** (≥ 240 cols): three panels side by side

---

## Hook-based session tracking

For accurate per-session status (especially the WORKING/ASKING distinction), install Claude Code hooks:

```bash
ccdash --install-hooks
```

This writes hook scripts that fire on Claude Code lifecycle events, writing session state to `~/.ccdash/sessions/`. The dashboard reads those files alongside the tmux pane inspection — hook data takes precedence when available.

Check whether hooks are installed:

```bash
ccdash --check-hooks
```

---

## Multi-project token tracking

By default, ccdash scans `~/.claude/projects` for all JSONL usage files. To include additional project root directories:

```bash
# Via flag (comma-separated)
ccdash --extra-dirs /path/to/projects,/other/path

# Via environment variable (colon-separated)
CCDASH_EXTRA_DIRS=/path/to/projects:/other/path ccdash
```

---

## Token cache

Token data is persisted to `~/.ccdash/tokens.db` (SQLite). You can query it directly with `sqlite3` or DuckDB:

```bash
sqlite3 ~/.ccdash/tokens.db "SELECT model, SUM(total_tokens), SUM(cost) FROM token_events GROUP BY model;"
```

---

## Project structure

```
ccdash/
├── cmd/ccdash/          # Entry point, CLI flags
├── internal/
│   ├── metrics/         # Collectors: system, tokens (JSONL + SQLite), tmux, hooks
│   └── ui/              # Bubble Tea dashboard model and panels
└── Makefile
```

---

## Development

```bash
make build    # Build binary to ./bin/ccdash
make test     # Run tests
make deps     # Download dependencies
make clean    # Remove build artifacts
```

---

## Requirements

- Go 1.21+
- `~/.claude/` directory (Claude Code data; created automatically when you use Claude Code)
- tmux (optional, for session panel)

---

## License

MIT
