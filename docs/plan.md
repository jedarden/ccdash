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
- Additional Claude Code integrations
- Plugin system for custom metrics

## Key Design Decisions

### Fixed Three-Panel Layout (locked)

**Decision:** ccdash shows exactly three panels — System Resources, Token Usage, Sessions — in every layout mode (ultra-wide, wide, narrow, compact). Network I/O is a summary line inside System Resources, not its own panel.

**Rationale:**
- This is the intended design, not an oversight. A standalone Network I/O panel was implemented and merged 2026-06-25 (bead bf-d0c, an autonomous P3 pickup from this file's own "Future Enhancements" list) and reverted 2026-07-15 at the user's explicit request — it duplicated the existing Net I/O summary line and was never a real requirement.
- **Do not re-add a fourth panel, and do not resurrect the removed `renderNetworkPanel` / per-panel network breakdown, without explicit user sign-off.** If a future bead proposes a new dedicated panel of any kind, treat the three-panel layout as a constraint to raise with the user, not a backlog item to implement autonomously.

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

## ADR-0006: 2026-07-20 — Push notifications for sessions needing human input

> Full text also lives at `docs/adrs/0006-push-notifications-for-sessions-needing-human-input.md` to keep this repo's existing `docs/adrs/` index (ADRs 0001–0005) intact; this section is the canonical copy per the fleet-wide artifact-improvement review convention.

**Status:** Proposed (2026-07-20)

### Context

ccdash's core value — knowing when a Claude Code session needs a human — only reaches the human if they happen to be looking at the terminal. In practice this workspace runs many concurrent sessions: at review time this host alone had 15 attached tmux sessions (`alpha` through `oscar`), on top of NEEDLE worker fleets on the lab server. Several existing memory notes describe the direct cost of this gap — workers idling unnoticed on a blocked/asking state, re-dispatch loops, and roamers churning CPU while waiting for something that never gets answered because nobody was watching.

v1.0.2 (2026-07-15, see `CHANGELOG.md`) already did the hard part of this problem: it wired the `Notification` and `PermissionRequest` Claude Code hooks so ccdash can distinguish "genuinely idle" from "actively blocked on a human." But today that signal only feeds the TUI's `READY` status color — it does nothing if the dashboard isn't on-screen. The dashboard is a pull-based tool for a push-shaped problem.

Separately, `jedarden/telegram-claude-bridge` already exists in this fleet (it has its own Argo Workflows build template, `telegram-claude-bridge-build`, and a declarative-config integration) as a live outbound channel from this workspace to the user's phone. It is the natural delivery mechanism — no new channel needs to be invented.

ccdash also already solves the "many concurrent instances, one source of truth" problem for its SQLite token cache via a lease-based leader election (`TryAcquireLease` / `collectorLeaseDuration` in `internal/metrics/cache.go`). The same mechanism is directly reusable to guarantee exactly one ccdash instance sends a notification for a given session, even when several people/terminals are watching the same fleet.

### Decision

Add an optional, opt-in outbound notifier to ccdash:

1. **New config file**, `~/.ccdash/config.yaml` (none exists today — ccdash is currently flag-only). Fields include `notify.enabled` (bool, default `false`) and `notify.webhook_url` (string, user-supplied — never hardcoded, since ccdash is a public OSS repo with users who have no `telegram-claude-bridge`).
2. **New package**, `internal/notify/`, with a small client that POSTs a compact JSON payload (session name, project dir, elapsed idle time — never token/cost content) to the configured webhook.
3. **Detection lives in the existing refresh loop of the lease-holding leader instance only.** Each refresh cycle, the leader diffs the previous vs. current status of every tracked session (from `HookSessionCollector`). A transition *into* `waiting`/`asking` that persists past a short debounce window (~15s, to skip transient blips already visible in the hook data) triggers one notification. The reverse transition (back to `working`) is silent — only escalations page the human, to keep the channel low-noise.
4. **Fails silently, never fatally.** An unreachable or misconfigured webhook logs a one-line warning to ccdash's existing log path and otherwise does not affect the dashboard; the feature is fully inert (zero behavior change, zero network calls) until a user opts in via config.

### Alternatives Considered

- **Fire the notification from the hook shell scripts directly** (`notification.sh` / `permission-request.sh` doing the `curl`). Rejected: those scripts are stateless per-invocation and have no visibility into debounce, dedup, or whether another ccdash instance already notified — this is exactly the class of problem the existing leader-election lease was built to solve, and reusing it beats reimplementing the same coordination in bash.
- **OS-native desktop notification** (`notify-send`, terminal bell) instead of/in addition to a webhook. Rejected as the *primary* channel: this is a headless Hetzner server reached over Tailscale — the user is routinely away from any terminal entirely, not merely unfocused on one. Kept as a candidate follow-up bead (cheap, local-only, no config needed) rather than folded into this ADR.
- **Do nothing / status quo (TUI-only signal).** Rejected: given the fleet's scale and the multiple existing memory-documented incidents of sessions silently stalling on human input, the latency between "session needs a human" and "a human notices" is a real, recurring cost, and the hook infrastructure to detect the transition precisely already shipped in 1.0.2 — only the delivery half is missing.
- **Central daemon that all ccdash instances report through**, rather than leader-election-gated notification from within existing instances. Rejected: mirrors the "Central Daemon with RPC" rejection in ADR-0005 — adds a new single point of failure and deployment unit for no benefit the existing per-instance-with-lease model doesn't already provide.

### Consequences

**Positive**
- Turns ccdash from "must be watching" into a proactive alert on exactly the fleet's most common failure mode (stalled-on-human sessions).
- Reuses two things ccdash already shipped and already tested — the 1.0.2 hook wiring and the token-cache leader-election lease — rather than inventing new coordination.
- Strictly opt-in and additive: zero effect on any user who doesn't configure a webhook, so this doesn't change ccdash's behavior for the broader public GitHub audience.

**Negative**
- Introduces ccdash's first outbound network dependency and first config file — new failure surface (silent-fail is a mitigation, but a misconfigured URL now means silent *non*-delivery too; a `ccdash --test-notify` command is the natural follow-up to make that debuggable).
- Slight scope creep from "pure dashboard" toward "alerting agent" — worth being explicit about in review since it's a one-way door for the project's identity as a lightweight, dependency-free TUI.
- Couples the public ccdash repo conceptually to a private-infra sibling project (`telegram-claude-bridge`); mitigated by keeping the endpoint fully user-supplied config with no bridge-specific assumptions baked into the payload shape (a generic JSON POST works for any webhook receiver, not just this fleet's bridge).

**Neutral**
- This is the first ccdash feature that sends any data off-host. The payload is deliberately minimal (session/project name, idle duration) and excludes token counts, cost figures, and file contents — worth stating plainly in the README once implemented, since ccdash has external users who will reasonably ask "does this phone home."
