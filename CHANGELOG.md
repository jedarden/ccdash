# Changelog

All notable changes to ccdash will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.1] - 2026-10-07

### Fixed
- **Model names truncated with space to spare**: the token panel sized model names against a fixed 22-column worst case for the cost and token text, so `gpt-6-astra`, `gpt-6.1-sol` and `gpt-5.6-luna` were cut to 10 characters while the models column had room. Names now get the width the actual cost and token text leaves, and the panel's preferred width uses the same measure, never below what the side-by-side layout needs.

## [1.2.0] - 2026-10-07

### Added
- **`ccdash --doctor`**: checks token sources (Claude transcripts, Codex rollout logs, OpenCode store), the token cache and which dashboard holds the collector lease, `config.yaml`, notifications (webhook shown as host only), Claude/Codex hooks and recent hook errors, and tmux. Prints OK/WARN/FAIL per check, exits 1 on a problem, `--json` for a machine-readable report.
- **`--watch --json`**: streams one snapshot per `--interval` (default 2s) as a JSON line until interrupted. Unlike `--once`, disk/network I/O and `tokens.rate` are measured rather than `null`.
- **Weekly spend projection and budget bar**: in the default Monday-9am window, after the first day, the token panel shows `Proj: $X/wk` (spend so far x 7 days / elapsed). With `alerts.cost_threshold_usd` set it also shows a `Budget` bar. `--once --json` gains `tokens.projected_week_cost`, `budget_usd` and `budget_used_percent`.
- **Per-session cost**: hook-tracked Claude Code and Codex sessions show their own spend in the token window, from the transcript named after the session ID. Sessions without an identifiable transcript show no cost rather than an estimate. In JSON as `sessions.sessions[].cost` with `session_id`.

## [1.1.40] - 2026-10-07

### Changed
- **Sessions header**: shows the session count only. It used to show `(sessions/processes)` when they differed, but the process figure counts every `claude` process on the host, so it could read `37/36 procs` and meant nothing. `running_processes` stays in `--once --json`; the README now says what it counts.
- **Legacy Codex prices removed**: `gpt-5-codex`, `gpt-5.2-codex` and `codex-mini-latest` are not on OpenAI's current pricing page and never appeared in this workspace's usage, so their prices could not be sourced. A request on one of them now shows as unpriced (`?`). `gpt-5.3-codex` is kept, verified against the page.

## [1.1.39] - 2026-10-07

### Fixed
- **`--once` reported every tmux session as WORKING**: a session's first sample had nothing to compare against and was counted as fresh activity, so one-shot snapshots (and the dashboard's first frame) showed idle sessions as WORKING. A first sample is now the baseline, and `--once` takes a second session sample one refresh interval (2s) later, so WORKING means the pane actually changed. `--once` takes about a second longer.

## [1.1.38] - 2026-10-07

### Changed
- Removed two layout renderers that no terminal size could reach (a two-over-one layout and an older stacked layout). They are why the README described a layout the dashboard never drew. No visible change.
- Releases are now built by a single-pod CI pipeline, and pushes that change only tracking or documentation files no longer publish an empty release. See v1.1.32 and v1.1.36 below.

## [1.1.37] - 2026-10-07

### Added
- Explicit task reports distinguish external waits, resumable work, completion, and pauses from idle turns. The attention command and debounced notifications show genuine decision reasons, and snapshots carry optional task continuity metadata.

### Fixed
- Claude idle notifications no longer request human attention. Native permission requests and questions remain visible through Stop and long idle periods and take precedence over task metadata and stale terminal output.
- Claude and Codex hooks share atomic, locked session updates; new work clears stale reports while task identity survives, and reports written by tools survive Stop and compaction.

## [1.1.36] - 2026-10-07

No changes to the binary. Published automatically for a push that only updated bead tracking files; the CI pipeline now skips such releases.

## [1.1.35] - 2026-10-07

### Fixed
- **Token models list pushed below the stats in short wide terminals**: at 214x18 a busy Sessions panel took the width the token panel needed for its two-column layout, so the models list stacked under the stats and most of it was cut off. When the terminal is too short to stack them, the token panel now keeps the width it needs for the side-by-side layout, and the Sessions panel gives way first by truncating names and re-flowing its columns. Tall terminals are unchanged.
- **120-139 column terminals cramped the token panel**: three panels side by side left it 23-33 columns wide with its header wrapping. Terminals narrower than 140 columns now use the stacked layout. The README and `--help` described a two-over-one layout for 120-239 columns that the dashboard never used; they now describe the two layouts that exist.

## [1.1.34] - 2026-10-07

### Fixed
- **Codex long-context requests were priced at the short-context rate**: OpenAI bills a request whose prompt exceeds 272K input tokens (cached input included) at roughly twice the input and 1.5 times the output rate. Codex logs each request's prompt size, so ccdash now prices such requests from OpenAI's published long-context table and shows them as their own row (`gpt-6-sol >272K`; `gpt-6-sol:long-context` in JSON and exports). A model with no published long-context rate falls back to its short-context price, marked `?`. None of this workspace's 4,078 most recent Codex requests crossed the threshold, so current totals are unchanged.

## [1.1.33] - 2026-10-07

### Fixed
- **Codex costs were wrong in both directions**: the OpenAI price table had `gpt-5.6-luna` at five times its published rate ($1.00/$6.00 per 1M tokens instead of $0.20/$1.20), with `gpt-5.6-sol` and `gpt-5.6-terra` also overstated, while `gpt-6-luna`, `gpt-6-sol`, `gpt-6.1-sol` and `gpt-6-astra` had no entry and counted as $0. All rows now match OpenAI's published standard-tier, short-context rates (checked 2026-10-07). On this workspace's last 7 days the Codex total moved from $1,597 to $917. Requests above 272K input tokens are billed at a higher long-context rate that ccdash does not model, so those are slightly understated.

## [1.1.32] - 2026-10-07

No changes to the binary. Published automatically for a push that only updated the plan and bead tracking files; the CI pipeline now skips such releases.

## [1.1.31] - 2026-10-06

### Fixed
- **Current Claude models were priced by guesswork**: `claude-opus-5-5` was costed at Opus 5's $5/$25 instead of $4/$20, `claude-fable-5-1` cache reads at $1.00 instead of $0.25, and `claude-sonnet-5-5` and `claude-haiku-4-5-20251001` fell back to family prices. All four now have exact entries.
- **Unknown Codex and OpenCode models looked free**: a model missing from the price table (such as `gpt-6-luna` or `gpt-6-sol`) was counted at $0 with no marker. It is still counted at $0, since no price is invented, but is now marked `?` in the dashboard and `pricing_estimated: true` in JSON. Free OpenCode Zen models stay unmarked.
- **Stacked layouts hid the token cost and wasted session rows**: the Sessions panel budgeted two rows for borders that are drawn outside its height, so it always left rows blank and showed `... +N more` with room to spare; the compact layout also reserved half the screen for it regardless of content. At 80x24 the token panel lost its Total and Cost lines. The Sessions panel now uses every row it is given, the compact layout hands unused rows to the token and system panels, and when the token panel is short it leads with Cost and Total.

### Added
- **`pricing_estimated` in `--once --json`**: each `tokens.model_usages` entry reports whether its cost came from an exact price. This is an additive field; `schema_version` stays 1. The README documents `pricing.models` overrides in `~/.ccdash/config.yaml`.

## [1.1.30] - 2026-10-06

### Added
- **`--since` and `--attention` for scripts**: `--since=monday|today|24h|7d|<RFC3339>` sets the token window for `--once` and every `--export` format, so a script can line its totals up with the dashboard or its own window. `--attention` lists sessions in `ASKING` and exits 1 when any need a human; on its own it collects only session state, and with `--once` the snapshot is still printed.

## [1.1.29] - 2026-10-02

### Changed
- **Human-attention sessions are now prominent**: ASKING sessions sort first and use bright purple indicators, READY sessions use a neutral indicator, and the sessions header reports how many sessions need you.

## [1.1.27] - 2026-10-02

### Fixed
- **Development builds nagged about updates and duplicated the update prompt**: skip update checks for non-release versions and keep the update actions in one status-bar notice.

## [1.1.26] - 2026-10-02

### Fixed
- **Dashboard help listed panel-focus keys with no handler and omitted active controls and ASKING**: derive documented shortcuts from the dashboard key bindings and render session indicators from the canonical status list.

## [1.1.24] - 2026-10-02

### Fixed
- **Token totals varied with the launch directory**: token collectors now use the documented `~/.ccdash/tokens.db` cache from every working directory, so followers and `--once` read the same token history. A missing cache is reported as unavailable instead of as zero usage.

## [1.1.23] - 2026-10-02

### Fixed
- **Dashboard panels overflowing terminal dimensions**: budget panel frames and compact layouts against available terminal space, preserve aligned borders, and bound the final view by terminal width and height.

## [1.1.21] - 2026-10-02

### Fixed
- **Sessions panel listed every tmux session as an agent**: a tmux session running a shell, a build, a log tail or ccdash itself was shown as an agent session, and as `WORKING` whenever its output had scrolled in the last 30 seconds. A tmux session is now listed only if it is hook-tracked, is a worker, has a `claude`, `codex`, `opencode` or NEEDLE process in one of its panes' process trees, or shows agent prompt markers in its pane (an agent reached over `ssh`). Where the process table cannot be read (macOS) the previous behaviour is kept.

## [1.1.20] - 2026-09-29

### Added
- **NEEDLE worker context and bead status**: worker rows show the workspace path, agent/provider/model, active bead and worker state, or an empty queue; unavailable heartbeat metadata degrades to explicit unavailable labels.
- **Grouped worker and interactive sessions**: classifies Claude Code and OpenCode worker names, with a per-user worker-log fallback; displays interactive sessions first, adds section headers and type icons, and uses abbreviated single-line entries to fit constrained terminal heights.

## [1.1.19] - 2026-09-24

### Added
- **OpenCode token usage**: read OpenCode's SQLite session store (`~/.local/share/opencode/opencode.db`, override with `OPENCODE_DB`) read-only, so the token panel, rate, sparkline and per-model breakdown include OpenCode sessions such as NEEDLE's `opencode/space-bunny-free` workers. Only sessions updated since the last cycle are read, through the message index, so the multi-GB message table is never scanned. Each completed assistant message is one event; reasoning counts as output; messages still streaming wait until complete. Free Zen models (`-free`) are priced at zero, known OpenAI/Claude/GLM families at list price, and unknown models at zero.

## [1.1.17] - 2026-09-21

### Fixed
- **Self-update checks failing with GitHub API status 403**: use GitHub's documented latest-release redirect instead of the shared-IP unauthenticated REST API quota, then construct the exact platform download URL from the resolved tag.

## [1.1.16] - 2026-09-20

### Fixed
- **Codex hook failures under concurrent status updates**: serialize session updates, write session and hook configuration files atomically, and record hook errors without logging payloads.
- **Codex cached input double-counting**: exclude cache-write input as well as cached input from the uncached-input counter.

## [1.1.15] - 2026-09-16

### Fixed
- **Token trend sparkline overflowing the token panel**: it was appended inline to the `Rate:` line inside a fixed 22-column left column with no width truncation, so on sustained activity it could grow past 30 characters and wrap the whole panel, dragging the `Models:` column down with it on narrow terminals (e.g. 223x19). Now rendered as its own full-width "Trend (30m):" row below both columns, clipped to the panel's content width.
- **Models column collapsing new model versions to a bare family name**: `shortenModelName` only recognized the `-4-5`/`4.5` generation via hardcoded per-version checks, so anything newer (`claude-opus-4-7`, `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-sonnet-5`, `claude-fable-5`) fell through to the generic `contains("opus")`/`contains("sonnet")` branches and rendered as an undifferentiated `"Opus"`/`"Sonnet"` — two different models showing the identical label. `glm-4.7` was worse: it matched the `contains("glm-4")` fallback and was mislabeled `"GLM 4"`. Replaced the hardcoded table with a generic family+version parser (`modelVersionNear`) that reads the version number adjacent to the family token directly out of the model ID, so new versions display correctly without a code change; a small `legacyModelNames` map still covers the handful of IDs that don't fit the pattern (GLM's qualitative variant names, Claude 3.x's `<version>-<family>` ordering).
- **Token panel's ideal width hardcoded at 60, with the extra space from a wide terminal always donated to the Tmux panel instead of Models**: `calculateRequiredTokenWidth()` computed the width actually needed for the longest current model name but was never called — `renderUltraWide` used a flat `idealTokenWidth := 60` instead, so a longer model name in the Models column had no way to claim more room even on a very wide terminal. `calculateRequiredTokenWidth` now shares the same column-layout constants as `renderTokenPanel` (previously two independently hand-maintained formulas) and is wired into `renderUltraWide`'s sizing. Also trimmed the System Resources panel's wide-terminal width from a flat 60 to its actual content floor (52 content columns, set by the fixed-format Disk I/O / Net I/O lines) so a wide terminal doesn't reserve width there that Models/Tmux could use.
- **New model IDs silently priced at GLM-4.5's rate**: `getPricingForModel` only recognized the `-4-5` Claude generation and the bare `glm-5`/`glm-4.x` tiers, so `claude-opus-4-7`, `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-sonnet-5`, and `claude-fable-5` all fell through to `defaultPricing` (GLM-4.5's $0.6/$2.2 rate) instead of their real Claude pricing — understating cost by roughly an order of magnitude. Separately, `glm-5.1` matched the generic `contains(model, "glm-5")` check before a more specific one existed, so it silently billed at GLM-5's rate ($1/$3.2) instead of its own ($1.4/$4.4, confirmed against docs.z.ai). Added pricing for every current Claude tier (Opus 4.6/4.7/4.8/5, Sonnet 4.6/5, Fable 5) and GLM 5.1/5.2/5.3/5.3-Flash, and reordered the GLM-5.x fallback checks most-specific-first. `glm-5-turbo` (observed in live usage but absent from docs.z.ai's current pricing page — likely a reseller/proxy-specific tier) is priced from OpenRouter's provider-comparison table, whose "Z.ai" row matches these figures exactly; `glm-5-code` has the same absence from docs.z.ai but no corroborating source yet, and is left unchanged and still flagged unverified in code.

### Added
- **Automatic release versioning**: pushing to `main` now runs lint/vet/build and, if the push didn't already bump `VERSION` itself, auto-bumps the patch version and tags a release — no more manually computing and pushing a release tag by hand.

## [1.1.9] - 2026-08-28

### Fixed
- **Compiled release binaries removed from git history**: `releases/*`, a stray `ccdash-test` binary, `.ccdash/tokens.db`, and `.beads/traces/*` were tracked in git, ballooning the repo to 77MB and keeping the working tree permanently `-dirty`. History rewritten to remove them; `.gitignore` now blocks their recurrence. Compiled binaries are distributed exclusively via GitHub Releases going forward.
- **Automated release pipeline repaired**: the `ccdash-ci-sensor` (Argo Events) had a dead NATS JetStream subscription since 2026-08-24 and, independently, was unreachable behind a Traefik routing rule that always favored a separate Forgejo-based webhook path for `/ccdash`. Removed the dead GitHub-based trigger and consolidated on the working Forgejo-triggered pipeline (`ccdash-tag-trigger`), which builds all 4 platform binaries and publishes this release automatically on tag push.

## [1.1.2] - 2026-08-27

### Fixed
- **Hook scripts failing on NixOS (and any distro without `/bin/bash`)**: All 9 generated hook script templates (`session-start.sh`, `session-end.sh`, `stop.sh`, `pre-tool-use.sh`, `post-tool-use.sh`, `notification.sh`, `permission-request.sh`, `prompt-submit.sh`, and the shared Codex hook template) were shebanged `#!/bin/bash`, a path that does not exist on NixOS (which provides only `/bin/sh`). Claude Code reported this as a confusing "No such file or directory" against the hook script itself rather than the missing interpreter. Shebangs now use the portable `#!/usr/bin/env bash`, which resolves correctly on both NixOS and traditional FHS distros (Debian, Ubuntu, etc.) where `/bin/bash` does exist.

## [1.1.1] - 2026-08-23

### Fixed
- **NEEDLE worker session double-counting**: Fixed bug where NEEDLE workers with dotted adapter names (e.g., `claude-code-glm-4.7-g47-agentscribe`) were counted as multiple separate sessions. Session identifiers are now normalized across hook, tmux, and NEEDLE registry sources by removing the `needle-` prefix and converting dots to underscores (e.g., `glm-4.7` → `glm-4_7`), ensuring consistent deduplication during session merge.
- **Token rate sparkline removed**: Removed incomplete token rate sparkline from the dashboard UI.

### Added
- **Leader-only session escalation notifications**: Added leader election to notification system, ensuring only one ccdash instance sends notifications when multiple instances are monitoring the same fleet. Prevents duplicate alerts for sessions needing human input.

## [1.1.0] - 2026-08-17

### Added
- **Codex CLI token accounting**: ccdash now scans Codex rollout logs under
  `~/.codex/sessions/YYYY/MM/DD`, combines their input, output, cached-input,
  and cache-write tokens with Claude Code usage, and shows a provider-aware
  per-model cost breakdown. Existing SQLite caches migrate automatically and
  retain historical Claude rows.
- **Codex session hooks**: `ccdash --install-codex-hooks` installs status-only
  Codex lifecycle hooks; token data continues to come from local rollout logs.
- **NEEDLE worker visibility**: workers without tmux sessions or Claude hook
  records now appear in the sessions panel from the NEEDLE worker registry.
- **Configurable token-cost alerting**: the token panel can highlight spend
  after a configured threshold is crossed.
- **Notification diagnostics**: `ccdash --test-notify` tests the configured
  notification webhook without waiting for a session transition.
- **Startup update notice**: available releases are shown in a dismissible
  startup notice.

### Changed
- **Stale READY sessions**: long-idle READY sessions are flagged and sorted so
  sessions needing attention are easier to identify.
- **Multi-provider cache schema**: token events and completed-file aggregates
  now carry their source (`claude` or `codex`) for correct parsing and pricing.

## [1.0.3] - 2026-07-15

### Removed
- **Network I/O panel**: reverted the dedicated 4th panel added in v0.9.x-era commit `57fd448` (an autonomous NEEDLE worker pickup, bead bf-d0c, never approved as core scope). ccdash is back to the intended 3-panel layout (System Resources, Token Usage, Sessions) in every mode — the Net I/O summary line inside System Resources is unchanged. `docs/plan/plan.md` now documents the 3-panel layout as a locked decision so it isn't re-added autonomously.

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
