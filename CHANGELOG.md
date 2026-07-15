# Changelog

All notable changes to ccdash will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Removed
- **Network I/O panel**: reverted the dedicated 4th panel added in v0.9.x-era commit `57fd448` (an autonomous NEEDLE worker pickup, bead bf-d0c, never approved as core scope). ccdash is back to the intended 3-panel layout (System Resources, Token Usage, Sessions) in every mode — the Net I/O summary line inside System Resources is unchanged. `docs/plan.md` now documents the 3-panel layout as a locked decision so it isn't re-added autonomously.

## [1.0.2] - 2026-07-15

### Fixed
- **Sessions blocked on human input showed `WORKING` instead of `READY`**: Claude Code fires `Notification` (needs permission, or idle 60s awaiting a reply) and `PermissionRequest` (a permission dialog is shown — covers tool approval, `AskUserQuestion`, and `ExitPlanMode` plan approval) hooks whenever it needs a human, but ccdash never installed hooks for either event, so sessions stuck mid-turn on a question or approval kept reporting whatever status `UserPromptSubmit` last set (`working`). `--install-hooks` already claimed to install these (`cmd/ccdash/main.go` help text advertised "Marks session as asking"), but no such hook scripts existed.
  - Added `~/.ccdash/hooks/notification.sh` and `~/.ccdash/hooks/permission-request.sh`, wired to the `Notification` and `PermissionRequest` events, which set session status to a new `"waiting"` value.
  - `HookSession.ToTmuxSession()` maps `"waiting"` onto the existing `StatusReady` — `READY` already means "waiting for human input" in ccdash's model, so this reuses it rather than adding a new status/color.
  - Added `~/.ccdash/hooks/post-tool-use.sh`, wired to `PostToolUse`, which sets status back to `"working"` once a gated tool actually finishes (the earliest available signal that Claude resumed after approval/an answer).

## [1.0.1] - 2026-05-22

### Changed
- **Token usage panel now displays billions with a `B` suffix**: `FormatTokensCompact` previously topped out at `M`, so counts over a billion rendered as thousands of millions (e.g. `2500.0M`). Values that exceed a billion now show as `B` (e.g. `2.5B`). Sub-billion counts are unchanged (`M`/`K`).

## [1.0.0] - 2026-05-15

### Fixed
- **Token metrics never loading with large JSONL archives**: `Collect()` was running the full file-scan and ingest loop (64,000+ files, ~50,000 previously unprocessed) synchronously on every UI refresh cycle. With ~10ms per file the loop took 500+ seconds — far exceeding the dashboard's 3-second timeout — so `QueryTokensHybrid()` was never reached and the token panel was always blank. Fixed by moving all file I/O into a background goroutine (`startBackgroundIngestion`) that starts immediately at collector creation and re-runs every 30 seconds. `Collect()` now only executes the fast `QueryTokensHybrid()` DB query, which completes in under 100ms regardless of corpus size.

## [0.9.9] - 2026-05-15

### Fixed
- **System and token panels blank with large project counts**: With many JSONL files (64,000+), the token batch-ingest goroutine held the SQLite write mutex for several seconds per cycle. Because lease acquisition, cache reads, and cache writes all shared the same mutex, this blocked the fast gopsutil-based system metrics and cache reads, causing all three operations to time out. Fixed by splitting into two mutexes: `ingestMu` for slow file-scan/DB-ingest operations, and `metaMu` for fast lease/cache operations. Fast operations are now completely independent of ingestion and can never be blocked by it.

## [0.9.8] - 2026-05-15

### Fixed
- **Session working status not recognized**: Hook-tracked sessions showing `WORKING` were incorrectly downgraded to `READY` by the hybrid tmux/hook merge logic. The merge was treating tmux pane content as authoritative, but `⏵⏵ bypass permissions` (always visible in Claude Code's UI chrome) caused `isClaudeWaiting` to return true even during active processing, triggering the downgrade. The `Stop` hook is now the authoritative signal — tmux pane content can only confirm working, not negate it.
- **Long-running tasks marked stale mid-execution**: Sessions with `status=working` were overridden to `stale` after 5 minutes of inactivity (no new `UserPromptSubmit`). The stale threshold no longer applies to `working` sessions.

### Added
- **`PreToolUse` hook**: New `~/.ccdash/hooks/pre-tool-use.sh` refreshes `last_activity` on every tool call, keeping the stale threshold from triggering during long multi-step tasks. Installed automatically by `ccdash hooks install`.

## [0.9.7] - 2026-05-14

### Added
- **Multi-directory JSONL support**: Token tracking now spans multiple Claude project root directories
  - `--extra-dirs=<dirs>` CLI flag accepts a comma-separated list of additional root directories to scan
  - `CCDASH_EXTRA_DIRS` environment variable accepts colon-separated paths (stackable with `--extra-dirs`)
  - Both mechanisms stack on top of the default `~/.claude/projects` root — no replacement, only addition
  - Useful for tracking usage across separate Claude Code installations or custom data locations

## [0.9.6] - 2026-05-14

### Added
- **Subagent JSONL inclusion**: Token usage now includes costs from subagent sessions spawned via the Agent tool
  - Recursively scans `<project>/<uuid>/subagents/agent-*.jsonl` in addition to top-level JSONL files
  - Subagent JSONL format is identical to main session JSONL — no extra parsing needed
  - Provides complete cost accounting for all Claude Code activity across all projects and sessions

## [0.9.5] - 2026-05-14

### Added
- **Multi-project JSONL aggregation**: Token usage dashboard now aggregates costs and tokens across all Claude Code projects, not just the one matching the current working directory
  - Scans all directories under `~/.claude/projects/*/` automatically
  - Aggregates input/output/cache tokens and costs from every project session
  - No configuration required — auto-discovery is the default behavior
  - Updated error messages to reflect all-project scope

## [0.8.0] - 2026-02-10

### Added
- **Disk usage monitoring**: System Resources panel now shows root filesystem (/) space usage
  - Displays used/total disk space with percentage bar (e.g., "Dsk [||||| 15.9%] 66.12 GB/444.00 GB")
  - Uses same compact format as Memory and Swap for consistency
  - Color-coded progress bar: Green<60%, Yellow 60-79%, Orange 80-94%, Red≥95%
  - Updated help text to document the new disk usage metric
  - Positioned between Swap and Disk I/O for logical resource grouping

## [0.7.23] - 2026-02-09

### Fixed
- **Session PID tracking bug**: Session hooks now correctly track the Claude Code process PID instead of the hook script's PID
  - Previously, hooks stored `$$` (the hook script's PID) which became invalid immediately after the hook exited
  - This caused all sessions to show as "ready" even when actively running
  - Now walks up the process tree to find the actual `claude` process and stores its PID
- **Stale session cleanup**: Automatically removes old session files when Claude Code restarts in the same tmux window
  - Prevents accumulation of orphaned session files with dead PIDs
  - Ensures only the current active session is tracked per tmux window
- **PID refresh in prompt-submit hook**: Updates the PID when user submits a prompt
  - Handles edge cases where the Claude process may have restarted
  - Ensures PID stays current throughout the session lifecycle

### Changed
- Session hooks now search for the parent `claude` process instead of using `$$`
- `session-start.sh` hook includes cleanup logic for old session files in the same tmux session
- `prompt-submit.sh` hook now updates the PID field along with status and activity time

## [0.7.18] - 2026-01-17

### Fixed
- **Attached indicator (📎) disappearing with multiple clients**: Fixed bug where the attachment indicator would disappear when connecting to a tmux session from a second computer
  - Root cause: `#{session_attached}` returns the count of attached clients, not a boolean
  - The check `attached == 1` failed when 2+ clients were attached
  - Solution: Changed to `attached > 0` to correctly detect any attached clients

## [0.7.17] - 2026-01-16

### Added
- **Automatic cleanup of orphaned session files on startup**: New `CleanupOrphanedSessions()` method removes stale hook session files where:
  - The process (PID) is no longer running
  - The tmux session no longer exists
- Cleanup runs silently on every ccdash startup, preventing accumulation of orphaned files

### Technical Details
- Uses `tmux list-sessions` to detect which tmux sessions are still active
- Uses `kill -0` signal check to verify if PIDs are still running
- Combined with v0.7.16, provides two-level protection against phantom sessions

## [0.7.16] - 2026-01-16

### Fixed
- **Phantom sessions displaying in dashboard**: Fixed bug where hook session files from terminated tmux sessions would appear in the dashboard
  - Root cause: Hook session files persist when sessions are killed abruptly (kill -9, terminal crash) without the session-end hook firing
  - The merge logic unconditionally displayed all hook sessions regardless of whether a corresponding tmux session existed
  - Solution: Skip hook sessions that don't have a matching live tmux session

### Technical Details
- Modified `Collect()` in `tmux.go` to filter out hook sessions without tmux counterparts
- Hook sessions are only displayed if `tmuxSessionMap[session.Name]` exists

## [0.7.15] - 2026-01-12

### Fixed
- **SQLite WAL mode not activating**: Fixed issue where WAL mode was not being enabled despite connection string parameter
  - WAL mode is now explicitly set via `PRAGMA journal_mode=WAL` after database open
  - Added backup `PRAGMA busy_timeout=30000` to ensure timeout is set
  - Resolves lock contention when running multiple ccdash instances concurrently

### Technical Details
- Connection string WAL parameter (`?_journal_mode=WAL`) doesn't always work with modernc.org/sqlite
- Explicit PRAGMA execution ensures WAL mode is active (creates `.db-wal` and `.db-shm` files)
- Two concurrent ccdash instances should now work reliably via leader election

## [0.7.14] - 2026-01-12

### Added
- **File pre-aggregation for complete sessions**: Dramatically improves token metrics loading performance
  - Files not modified in 30+ minutes are automatically detected as "complete"
  - Complete files are aggregated once and stored in `file_aggregates` table
  - Future queries skip file I/O entirely for complete files, reading only pre-computed totals
  - Individual events are deleted after aggregation to reduce database size
  - Files that become active again are automatically reactivated and reprocessed

### Changed
- Token queries now use hybrid approach: pre-aggregated totals + individual events
- Schema version bumped to 3 (automatic migration on first run)
- Reduced redundant file scanning - complete files checked via DB, not filesystem

### Performance
- First load after restart: Pre-computed aggregates load instantly
- Typical session with 50+ old files: ~90% reduction in file I/O operations
- Database size: Reduced by removing individual events for complete files

### Technical Details
- New `file_aggregates` table stores per-file totals with model breakdown (JSON)
- `GetFileAggregate()` / `MarkFileComplete()` / `MarkFileActive()` cache methods
- `QueryTokensHybrid()` combines aggregates and events in single query
- `GetFileCompleteThreshold()` returns 30-minute threshold (configurable constant)

## [0.6.28] - 2025-12-18

### Fixed
- **Display bleed-through bug**: Fixed issue where external process output (like Tailscale "wgengine: reconfig" logs) would appear at the bottom of the display
  - Root cause: View() output didn't fill entire terminal height, leaving bottom rows unrendered
  - Solution: Added padding in View() to ensure output always fills the full terminal height
  - Resizing no longer required to clear stray log messages

## [0.6.0] - 2025-12-05

### Added
- **SQLite-based token cache**: Complete rewrite of caching system for better queryability
  - Cache stored in `.ccdash/tokens.db` SQLite database with WAL mode
  - Directly queryable by DuckDB, SQLite CLI, or any SQLite-compatible tool
  - Schema: `token_events` table with timestamp indexes, `file_state` for tracking
  - Batch insertions for improved performance
- **Incremental ingestion**: Smart processing of JSONL files
  - Tracks last processed line per file to avoid reprocessing
  - Automatic file invalidation on modification or truncation
  - Deduplication via unique index on (source_file, line_number)
- **SQL-based lookback queries**: Efficient time-range filtering
  - Uses indexed timestamp_unix column for fast range queries
  - Per-model aggregation computed directly in SQL
  - Recent events query for rate calculations

### Changed
- Replaced JSON cache (`.ccdash/token_cache.json`) with SQLite (`.ccdash/tokens.db`)
- Token metrics now computed via SQL aggregation instead of in-memory iteration
- Updated help pane to document SQLite/DuckDB queryable cache

### Technical Details
- New dependency: `modernc.org/sqlite` (pure Go, no CGO required)
- Cross-platform binaries without C compiler dependencies
- SQLite configured with WAL journal mode and NORMAL synchronous
- `TokenCache` struct provides thread-safe database access with RWMutex
- `InsertTokenEventBatch()` for efficient bulk inserts
- `QueryTokensSince()` returns aggregated metrics with per-model breakdown
- `QueryRecentEvents()` for rate calculation over last N seconds

## [0.5.0] - 2025-12-05

### Added
- **Two-tier log file processing**: Token metrics now use a two-tier system for efficiency
  - Tier 1: Real-time processing of entries within the lookback window
  - Tier 2: Cached processing of historical entries outside the lookback window
  - Significantly reduces CPU usage when processing large JSONL files
- **Persistent cache in .ccdash folder**: Historical token data is now cached
  - Cache stored in `.ccdash/token_cache.json` in the working directory
  - Automatically invalidates when source files are modified
  - Survives across sessions for faster startup
- **Enhanced TMUX panel title**: Now shows session count and status summary
  - Title format: "📺 TMUX Sessions (N)" where N is total count
  - Status summary right-justified: "🟢2 🔴1 🟡3" showing count per status
  - Quick visual overview without scanning individual sessions

### Changed
- Removed redundant "Total: X" line from TMUX panel (now in title)
- Token collector now initializes cache on creation
- Improved file processing with modification time tracking

### Technical Details
- New `internal/metrics/cache.go` for persistent token caching
- TokenCollector now includes cache and file line tracking
- Cache uses JSON serialization with version control for compatibility
- Two-tier processing prioritizes fresh data over cached historical data

## [0.3.0] - 2025-11-27

### Added
- **Self-update functionality**: ccdash now checks for updates automatically from GitHub releases
  - Status bar shows "⬆ vX.X.X available! Press u to update" when a new version exists
  - Press `u` to download and apply the update in-place
  - Automatic version comparison with GitHub releases API
- **Per-model cost tracking**: Token panel now shows individual costs for each Claude model
  - Displays model name with cost and token count
  - Color-coded by model type (Opus=red, Sonnet=cyan, Haiku=green)
  - Sorted by cost (highest first)
  - Smart model name shortening (e.g., "claude-opus-4-5-20251101" → "Opus 4.5")
- **Improved CPU core display alignment**
  - Square brackets now align consistently across all core displays
  - Fixed-width labels ensure proper column alignment
  - Consistent bar width calculation matching memory/swap lines

### Changed
- CPU total bar now uses the same width calculation as Memory and Swap for visual consistency
- Status bar dynamically shows available shortcuts based on update availability
- Token panel now includes empty line separator before per-model breakdown

### Technical Details
- New `internal/updater` package for update management
- Added `ModelUsage` struct for per-model token and cost tracking
- Updater uses GitHub API with 5-minute cache interval
- Self-update uses atomic file replacement with restart script

## [0.1.4] - 2025-11-21

### Fixed
- Fixed panel width calculation to properly account for padding, ensuring panels fit exactly in terminal width
- Panels now render correctly in 202-character wide terminals without right-side cutoff

### Changed
- Adjusted panel width distribution to account for lipgloss padding (0,1)
- Updated width calculation: totalPanelWidth = d.width - 6 (to account for 2 chars padding per panel)

## [0.1.3] - 2025-11-21

### Fixed
- Narrowed tmux sessions panel by additional character to prevent overflow
- Improved panel border calculations

## [0.1.2] - 2025-11-21

### Fixed
- Narrowed tmux sessions panel by 3 characters to better fit terminal width
- Fixed right-side cutoff issues in ultra-wide mode

## [0.1.1] - 2025-11-21

### Added
- Version display in status bar (bottom left)
- Version now shows as "HH:MM:SS vX.X.X" format

### Changed
- Updated help pane width calculation to match normal view (d.width - 2)

## [0.1.0] - 2025-11-21

### Added
- Initial release of ccdash
- Real-time system resource monitoring (CPU, memory, swap, disk I/O, load averages)
- Claude Code token usage tracking from ~/.claude/projects
- Tmux session monitoring with intelligent status detection
- Beautiful TUI with responsive layout modes:
  - Ultra-wide mode (≥240 cols): 3 panels side-by-side
  - Wide mode (120-239 cols, ≥30 lines): 2 panels top, 1 bottom
  - Narrow mode (<120 cols): panels stacked vertically
- Help mode (press 'h') with cycling explanations for each panel
- Smart tmux session status detection:
  - 🟢 WORKING - Claude Code actively processing
  - 🔴 READY - Waiting for user input at prompt
  - 🟡 ACTIVE - User actively in session
  - ⚠️ ERROR - Error state or undefined condition
- Detection patterns from unified-dashboard:
  - Working indicators: "Finagling...", "Puzzling...", "Listing...", etc.
  - Prompt patterns: "⏵⏵ bypass permissions", "Claude Code" + "❯"
  - Error detection in last 5 lines only
- Idle duration tracking for tmux sessions
- Dynamic CPU core display (≤6 cores: one per line, >6: multiple per line)
- 2-column help layout when text exceeds available lines
- Keyboard shortcuts:
  - q, Ctrl+C: Quit
  - r: Refresh metrics immediately
  - h: Cycle through help mode
- Status bar with time, github link, dimensions, and shortcuts
- Color-coded metrics with 4-tier thresholds
- Unified-dashboard inspired styling with vertical bars and emojis

### Technical Details
- Built with Bubble Tea TUI framework
- Uses lipgloss for terminal styling
- gopsutil for system metrics collection
- Captures last 15 lines of tmux panes for status detection
- Content change detection with timing rules
- 2-second refresh interval for metrics

[0.6.28]: https://github.com/jedarden/ccdash/releases/tag/v0.6.28
[0.6.0]: https://github.com/jedarden/ccdash/releases/tag/v0.6.0
[0.5.0]: https://github.com/jedarden/ccdash/releases/tag/v0.5.0
[0.3.0]: https://github.com/jedarden/ccdash/releases/tag/v0.3.0
[0.1.4]: https://github.com/jedarden/ccdash/releases/tag/v0.1.4
[0.1.3]: https://github.com/jedarden/ccdash/releases/tag/v0.1.3
[0.1.2]: https://github.com/jedarden/ccdash/releases/tag/v0.1.2
[0.1.1]: https://github.com/jedarden/ccdash/releases/tag/v0.1.1
[0.1.0]: https://github.com/jedarden/ccdash/releases/tag/v0.1.0
