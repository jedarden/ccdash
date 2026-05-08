# ccdash Implementation Plan

**Project:** Lightweight TUI dashboard for Claude Code token usage, agent sessions, and system resources
**Repo:** jedarden/ccdash
**Language:** Go 1.21+
**Current Version:** v0.9.4
**Total Lines of Code:** ~7,500 lines

## Overview

ccdash is a real-time terminal dashboard that provides:
- System resource monitoring (CPU, memory, disk, network, load)
- Claude Code token usage tracking with per-model cost breakdown
- Tmux session monitoring with intelligent status detection
- Self-update functionality
- SQLite-based caching for efficient token queries

## Architecture

### Project Structure

```
ccdash/
├── cmd/ccdash/          # Main application entry point
│   └── main.go          # CLI flags, hooks, TUI initialization
├── internal/
│   ├── metrics/         # Metrics collectors
│   │   ├── system.go    # System resources (CPU, memory, disk, network)
│   │   ├── tokens.go    # Token usage from Claude Code JSONL logs
│   │   ├── tmux.go      # Tmux session monitoring
│   │   ├── hooks.go     # Hook session tracking
│   │   └── cache.go     # SQLite token caching
│   ├── ui/              # Bubble Tea UI components
│   │   ├── dashboard.go # Main TUI model and layout logic
│   │   └── styles.go    # Terminal styling
│   └── updater/         # Self-update functionality
│       └── updater.go   # GitHub releases integration
├── docs/                # Documentation
│   ├── adrs/            # Architecture Decision Records
│   └── notes/           # Design notes and research
└── Makefile             # Build automation
```

### Key Dependencies

- **Bubble Tea** - TUI framework
- **Lipgloss** - Terminal styling
- **gopsutil** - System metrics collection
- **modernc.org/sqlite** - Pure Go SQLite (no CGO)

## Implementation Phases

### Phase 1: Core - COMPLETE

**Status:** ✅ Complete (v0.1.0)

Initial release with core functionality:
- Real-time system metrics (CPU, memory, swap, disk I/O, load averages)
- Claude Code token tracking from `~/.claude/projects`
- Tmux session monitoring with status detection
- Responsive layout modes (ultra-wide, wide, narrow)
- Help mode with cycling explanations
- Keyboard shortcuts (q, r, h)

### Phase 2: Editor & Full Coverage - COMPLETE

**Status:** ✅ Complete (v0.1.1 - v0.3.5)

Enhanced features and bug fixes:
- Version display in status bar
- Panel width fixes for various terminal sizes
- Self-update functionality with GitHub releases API
- Per-model cost tracking with color coding
- SQLite-based token cache (v0.6.0)
- Two-tier log file processing (v0.5.0)
- Lookback picker for custom time windows
- TMUX panel title with session count and status summary

### Phase 3: Advanced Features - COMPLETE

**Status:** ✅ Complete (v0.6.0 - v0.9.4)

Advanced features and optimizations:
- SQLite caching with WAL mode for concurrent access
- File pre-aggregation for complete sessions (v0.7.14)
- Orphaned session cleanup (v0.7.16, v0.7.17)
- Session PID tracking bug fix (v0.7.23)
- Disk usage monitoring (v0.8.0)
- Compact layout mode for narrow terminals (v0.9.0)
- Two-line status bar with persistent repo link (v0.9.1)
- GLM pricing support (v0.9.0+)
- Hook-based session tracking with Claude Code integration

### Phase 4: Production Hardening - COMPLETE

**Status:** ✅ Complete

Production readiness improvements:
- Leader election for multi-instance support
- Automatic cleanup of stale data
- Comprehensive error handling
- Cross-platform binary releases (linux, darwin, amd64, arm64)
- SHA256 checksums for releases

### Phase 5: CI/CD Migration - IN PROGRESS

**Status:** 🔄 In Progress

Migrate from GitHub Actions to Argo Workflows:
- Create argo-workflows WorkflowTemplate
- Multi-arch Go binary builds
- GitHub release automation
- Disable GitHub Actions workflow

**Related beads:**
- bf-5c3: Migrate CI to Argo Workflows
- bf-20y: Related CI migration task

### Phase 6: Future Enhancements - PLANNED

**Status:** 📋 Planned

Potential future features:
- Multi-directory JSONL support (bf-109)
- Network I/O panel with bytes sent/received (bf-29e)
- Additional Claude Code integrations
- Plugin system for custom metrics

## Key Design Decisions

### SQLite for Token Caching (v0.6.0)

**Decision:** Migrated from JSON cache to SQLite database.

**Rationale:**
- Queryable with DuckDB and SQLite CLI
- Better performance for large datasets
- WAL mode for concurrent access
- Pure Go implementation (no CGO)

### Hook-Based Session Tracking

**Decision:** Use Claude Code hooks for precise session tracking.

**Rationale:**
- Direct integration with Claude Code lifecycle
- Accurate PID tracking of actual Claude process
- Status detection via prompt-submit hooks
- Automatic cleanup of orphaned sessions

### Responsive Layout System

**Decision:** Support multiple layout modes based on terminal size.

**Rationale:**
- Works across diverse terminal environments
- Ultra-wide mode for large displays
- Compact mode for tmux panels (199x14)
- Automatic mode switching

## Development Workflow

### Building

```bash
make build      # Build binary
make install    # Install to ~/.local/bin
make test       # Run tests
make clean      # Clean artifacts
make release    # Create release build
```

### Versioning

- Semantic versioning (MAJOR.MINOR.PATCH)
- Version set via ldflags at build time
- Git tags for releases (vX.Y.Z)
- CHANGELOG.md for notable changes

### Testing

```bash
go test ./internal/metrics/...
go test ./internal/ui/...
```

## Deployment

### Binary Releases

Releases published to GitHub Releases with:
- Multi-architecture binaries (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64)
- SHA256 checksums for verification
- Release notes with changelog

### Installation Methods

1. Pre-built binary (recommended)
2. `go install` from source
3. From source with `make install`

## References

- **GitHub:** https://github.com/jedarden/ccdash
- **Releases:** https://github.com/jedarden/ccdash/releases
- **Docs:** `docs/` folder
- **ADRs:** `docs/adrs/` folder
