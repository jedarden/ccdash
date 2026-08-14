# ADR 0007: Support Codex CLI Alongside Claude Code

## Status

Proposed (2026-08-12)

## Context

ccdash today is hard-coded to a single harness. Token tracking
(`internal/metrics/tokens.go`) only ever scans `~/.claude/projects`, parses
lines against an Anthropic-shaped `claudeMessage`/`usageData` struct
(`input_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`,
`output_tokens`), and prices the result against a `modelPricing` map keyed to
Claude model ids (tokens.go:627-645). Session status tracking
(`internal/metrics/hooks.go`) is equally single-harness: `--install-hooks`
writes a Claude Code hook block to `~/.claude/settings.json` for
`SessionStart`, `Stop`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, and
`Notification`.

Research earlier in this session (2026-08-11/12) found that Codex CLI
(OpenAI's coding agent) now has a structurally similar architecture:

- Session transcripts persist to `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`
  (date-tree, not Claude's flat-per-project layout) and contain per-turn token
  usage statistics.
- As of v0.124.0 (April 2026), Codex ships a stable, explicitly "Claude-style"
  hooks system (`hooks.json` or an inline `[hooks]` table in `config.toml`)
  with near 1:1 event-name parity to Claude Code's hook set: `SessionStart`,
  `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`,
  `PermissionRequest`, `PreCompact`, `PostCompact`, `SubagentStart`,
  `SubagentStop`, `Stop`.

This session's earlier discussion concluded that extending ccdash with a
pluggable source abstraction beats standing up a second, harness-agnostic TUI
from scratch — most of ccdash (Bubble Tea layout, SQLite cache, system panel,
rate/cost math) is already provider-agnostic; only the ingestion schema and
pricing table are Claude-specific.

**Correction to the design brief that motivated this ADR:** neither harness's
hooks carry token usage or cost. Verified against Codex's official hooks/notify
docs (2026-08-12) — `Stop`/`SessionEnd`/`PostToolUse` payloads carry only
session/turn/tool metadata (`session_id`, `transcript_path`, `tool_input`,
`tool_response`, etc.), and the separate `notify` mechanism's payload
(`type`, `thread-id`, `turn-id`, `cwd`, messages) is equally bare. This matches
ccdash's own existing behavior: `hooks.go` already uses Claude Code hooks
exclusively for session status, never for tokens — the JSONL transcript
ingester in `tokens.go` is the only source of token data today, and hook
payloads only ever carry a `transcript_path` pointer. Codex hooks have to play
the same narrow role — liveness/status signal, not a usage feed.

## Decision

1. **Introduce a `Source` abstraction** in `internal/metrics/` so
   `TokenCollector` iterates over `[]Source` instead of being hardwired to the
   Claude schema:

   ```go
   type Source interface {
       Name() string                     // "claude", "codex"
       ProjectDirs(home string) []string // ~/.claude/projects, ~/.codex/sessions
       ParseUsageLine(raw []byte) (*TokenEvent, bool, error)
       PricingForModel(model string) ModelPricing
       HookInstaller() HookInstaller     // per-harness hook file writer
   }
   ```

   The SQLite cache gains a `source` column (default `"claude"` via
   migration, so existing caches don't need a wipe) so the query layer, rate
   calculation, and per-model cost breakdown work unmodified across both
   sources in the same lookback window.

2. **Add `internal/metrics/codex.go`** — a Codex-specific JSONL reader that
   walks the `~/.codex/sessions/YYYY/MM/DD/` date-tree (recursive, unlike
   Claude's flat layout) and decodes Codex's own per-turn usage record. The
   exact field names are not yet confirmed from a live sample — Codex's usage
   shape is documented to exist but not documented field-by-field the way
   Anthropic's API is. **Capture a real rollout file from a live Codex session
   before writing the parser**; do not guess the schema from prose docs.

3. **Add a `codexPricing` table**, mirroring `modelPricing` (tokens.go:627),
   keyed to OpenAI model ids. Maintained independently — different vendor,
   different price-change cadence, no shared rows with the Claude table.

4. **Extend the hook installer** with a Codex variant that writes the
   equivalent `hooks.json`/`config.toml` `[hooks]` block. Map Codex's
   `PermissionRequest` to the same "waiting for human" state Claude's
   `Notification` hook drives today. Leave `PreCompact`/`PostCompact`/
   `SubagentStart`/`SubagentStop` unwired initially — ccdash doesn't use those
   transitions on the Claude side either, so this is no regression in scope,
   just headroom for a later bead.

5. **Tag session records with `source`** alongside the existing PID/tmux-pane
   identity so the sessions panel can badge Claude vs. Codex sessions,
   extending the 🤖/💻 icon convention from ADR-0001/0002 rather than
   replacing it.

## Alternatives Considered

- **Derive token counts from hook payloads instead of JSONL parsing.**
  Rejected — verified neither harness's hooks carry usage data; the
  transcript file is the only source of truth for tokens on either platform.
- **New standalone, harness-agnostic dashboard.** Rejected per this session's
  earlier discussion: duplicates UI/cache/system-panel work ccdash already
  has, for no benefit — the harness-specific work (ingestion schema +
  pricing) is a small fraction of the codebase, not a reason to fork the
  whole project.
- **Keep one hardcoded ingester and branch on file path** (`if source ==
  "codex"` sprinkled through `tokens.go`) instead of a `Source` interface.
  Rejected: the current code already conflates "find files," "parse a line,"
  and "price a model" into one hardcoded path; a third harness (plausible
  given how fast Codex converged on Claude's hook shape) would mean a third
  set of branches through the same functions instead of one new file
  implementing an interface.

## Consequences

### Positive

- Token totals and cost become cross-harness comparable in one dashboard.
- Reuses all existing UI/cache/system-panel infrastructure unchanged.
- The interface leaves room for a third harness later without another
  architectural decision — just a new file.

### Negative

- Requires a SQLite schema migration (`source` column) against every
  existing user's `~/.ccdash/` cache.
- Codex's usage-field shape is unconfirmed pending a live sample — the
  parser in item 2 is provisional until that capture happens.
- Two independently-versioned pricing tables to maintain instead of one.
- The hook installer roughly doubles its surface: two settings-file formats
  to write, detect (`--check-hooks`), and uninstall.

### Neutral

- This is ccdash's first non-Anthropic integration — the README's framing
  ("dashboard for Claude Code") needs updating once implemented, since the
  project stops being Claude-only.

## References

- Related ADR: none directly; builds on the hook-tracking foundation from
  the CHANGELOG [1.0.2] Notification/PermissionRequest wiring referenced in
  ADR-0006.
- `docs/plan.md` — canonical copy of this ADR under "ADR-0007", plus Phase 7
  in Implementation Phases.
- Codex hooks docs (2026-08-12 capture): `developers.openai.com/codex/hooks`
  (redirects to `learn.chatgpt.com/docs/hooks`) and
  `developers.openai.com/codex/config-advanced`.
