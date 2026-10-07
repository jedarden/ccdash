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

## Token cache: events and summaries

`~/.ccdash/tokens.db` holds two kinds of rows per transcript file:

- `token_events`: one row per usage line of a file that is still being
  written, keyed by file and line number.
- `file_aggregates`: a summary of a file's lines, with per-model totals and
  `covered_lines`, the number of leading lines it holds.

When a file has been idle past the completion threshold, its events are
merged into its summary and deleted, in one write-locked transaction. If the
file grows again (a resumed session), the summary is flagged active and new
lines arrive as events; totals count the summary plus those events. Event
inserts skip any line at or below `covered_lines`, so a second ccdash process
reading the same file cannot store a summarised line twice. A rewritten file
(fewer lines than were read) drops its summary and events and is re-read from
line 1. Schema 5 repaired caches written before these rules (see
`CHANGELOG.md`, v1.2.2).

Token totals for a window are the summaries whose latest event is in the
window plus the events in it. A summary counts as a whole, so a long-running
transcript that started before the window contributes all of its lines.

## Costs

Prices are per model per million tokens, kept in tables per source:
`internal/metrics/tokens.go` (Claude and GLM) and
`internal/metrics/codex.go` (OpenAI), each row commented with where its price
came from. Codex requests whose prompt exceeds 272K tokens are recorded as
`<model>:long-context` and priced from OpenAI's long-context table.
`pricing.models` in `~/.ccdash/config.yaml` overrides any row. A model priced
by a family fallback, or with no known price, is marked estimated (`?` in the
panel, `pricing_estimated` in JSON); unknown models count as $0 rather than a
guess.

Two figures are derived from the same data:

- **Weekly projection** (`metrics.ProjectWeekCost`): in the default
  Monday-09:00 window, after the first day, cost x 7 days / elapsed.
- **Per-session cost** (`TokenCollector.SessionCosts`): a hook-tracked session's
  spend, found through the transcript named after its session ID
  (`<id>.jsonl` for Claude Code, `rollout-<ts>-<id>.jsonl` for Codex).
  Sessions without such a transcript get no figure.

## Headless modes

The CLI reuses the collectors without Bubble Tea (`cmd/ccdash`):

- `--once` takes one snapshot. Session status needs two pane samples, so it
  takes a baseline, runs the slower collectors, and samples sessions again at
  least 2s later. Rates that need two samples are `null`.
- `--watch --json` streams a snapshot per interval as JSON lines, with the
  rates measured.
- `--attention` collects sessions only and exits 1 if any is ASKING.
- `--doctor` checks data sources, the cache and lease, config, hooks (and
  `jq`, which they need) and tmux, without collecting metrics.

`--once` and `--watch` share one versioned JSON schema (`schema_version`,
golden-tested in `cmd/ccdash/testdata`). Fields are only ever added within a
version.

## UI code map

`internal/ui`: `dashboard.go` holds the model, `Update`, `View` and metrics
collection; `layout.go` allocates width and height and draws the compact and
wide layouts; `panel_system.go`, `panel_tokens.go` and `panel_sessions.go`
render the three panels; `keys.go` holds key bindings and handlers;
`overlays.go` (lookback picker, help), `worker_detail.go` and `statusbar.go`
render the rest; `text.go` and `styles.go` are shared helpers.

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

## Releases

Every push to `main` runs the checks (gofmt, vet, the test suite, build) on
Argo Workflows; a push that changes the binary's inputs is tagged and
released. See [`notes/release-pipeline.md`](notes/release-pipeline.md).
