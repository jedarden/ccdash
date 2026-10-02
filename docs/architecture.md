# Architecture

ccdash is a Bubble Tea terminal dashboard. The `Dashboard` model owns three
collectors, combines their snapshots, and renders the System Resources,
Token Usage, and Sessions panels.

## Collectors and data sources

- `SystemCollector` reads CPU, load, memory, swap, disk, and network counters
  through gopsutil. It keeps the previous disk/network counters so each
  refresh can report rates.
- `TokenCollector` uses the `Source` interface for three providers. Claude
  Code JSONL is read from `~/.claude/projects` plus configured extra roots;
  Codex rollout JSONL is read from `~/.codex/sessions/YYYY/MM/DD`; and
  OpenCode is read read-only from `~/.local/share/opencode/opencode.db` (or
  `OPENCODE_DB`). JSONL ingestion runs in a background cycle and writes token
  events to `~/.ccdash/tokens.db`; the UI-facing query reads the SQLite cache
  and applies the lookback and provider-specific pricing.
- `TmuxCollector` builds the Sessions panel. It combines Claude/Codex hook
  session files, `tmux list-sessions` and pane inspection, and NEEDLE's worker
  registry/heartbeat data. Process-tree and pane markers prevent ordinary
  shells, builds, and log tails from appearing as agent sessions. Worker
  metadata can add workspace, provider/model, state, and bead details.

Hooks are status sources, not token sources. Token counts always come from
transcripts or the OpenCode database.

## Hook-over-tmux precedence

Each refresh first reads available hook records and then reads live tmux
sessions. A hook record is displayed only when its corresponding tmux session
still exists, which removes stale hook files. For a matching session, the hook
status is authoritative for lifecycle state: tmux pane activity may confirm
`WORKING`, but it cannot downgrade a hook-reported working state. Tmux supplies
the attached flag and fills in sessions that predate hook installation.

NEEDLE workers without tmux or hook records are added from the worker registry;
workers already represented by tmux are skipped. The merged sessions are
classified as worker or interactive, sorted with `ASKING` first, and then
passed to the UI. The resulting source label is `hooks`, `tmux`, `needle`, or
`hybrid`.

## Leader/follower cache

All dashboard instances open the same SQLite cache. On each refresh an instance
tries to acquire or renew a short collector lease. The leader collects fresh
system and session metrics and writes them to the `metrics_cache` table. A
follower reads those snapshots while they are fresh, avoiding duplicate
collection; a cache miss falls back to local collection, and lease/database
errors fail safe by allowing local collection. Only the leader forwards raw
hook snapshots to the notification tracker, so multiple dashboards do not
send duplicate alerts.

Token metrics are different: every instance performs the fast shared-cache
query, while background ingestion is protected by the SQLite/cache locking
model. This keeps slow transcript I/O out of the render path and lets all
instances see the same token history.

## Render loop

`Dashboard.Init` starts an immediate collection and a two-second Bubble Tea
tick. Each tick launches system, token, and session work concurrently. The
collection command waits up to three seconds, returns whichever snapshots are
available, and sends a `metricsMsg` to `Update`. `Update` stores the snapshot,
processes leader-only notifications, and schedules the next tick.

`View` selects the responsive layout, renders the three panels and status bar,
and bounds the final frame to the terminal dimensions. The Sessions panel
groups interactive sessions before workers; the other panels render the latest
collector snapshots. A one-shot CLI mode uses the same collectors without
starting Bubble Tea.
