# ADR 0006: Push Notifications for Sessions Needing Human Input

## Status

Proposed (2026-07-20)

## Context

ccdash's core value — knowing when a Claude Code session needs a human — only reaches the human if they happen to be looking at the terminal. In practice this workspace runs many concurrent sessions: at review time this host alone had 15 attached tmux sessions (`alpha` through `oscar`), on top of NEEDLE worker fleets on the lab server. The recurring, documented cost of this gap is workers idling unnoticed on a blocked/asking state — re-dispatch loops and roamers churning CPU while waiting for something that never gets answered because nobody was watching.

v1.0.2 (2026-07-15, see `CHANGELOG.md`) already did the hard part of this problem: it wired the `Notification` and `PermissionRequest` Claude Code hooks so ccdash can distinguish "genuinely idle" from "actively blocked on a human." But today that signal only feeds the TUI's `READY` status color — it does nothing if the dashboard isn't on-screen. The dashboard is a pull-based tool for a push-shaped problem.

Separately, `jedarden/telegram-claude-bridge` already exists in this fleet (it has its own Argo Workflows build template, `telegram-claude-bridge-build`, and a declarative-config integration) as a live outbound channel from this workspace to the user's phone. It is the natural delivery mechanism — no new channel needs to be invented.

ccdash also already solves the "many concurrent instances, one source of truth" problem for its SQLite token cache via a lease-based leader election (`TryAcquireLease` / `collectorLeaseDuration` in `internal/metrics/cache.go`). The same mechanism is directly reusable to guarantee exactly one ccdash instance sends a notification for a given session, even when several people/terminals are watching the same fleet.

## Decision

Add an optional, opt-in outbound notifier to ccdash:

1. **New config file**, `~/.ccdash/config.yaml` (none exists today — ccdash is currently flag-only). Fields include `notify.enabled` (bool, default `false`) and `notify.webhook_url` (string, user-supplied — never hardcoded, since ccdash is a public OSS repo with users who have no `telegram-claude-bridge`).
2. **New package**, `internal/notify/`, with a small client that POSTs a compact JSON payload (session name, project dir, elapsed idle time — never token/cost content) to the configured webhook.
3. **Detection lives in the existing refresh loop of the lease-holding leader instance only.** Each refresh cycle, the leader diffs the previous vs. current status of every tracked session (from `HookSessionCollector`). A transition *into* `waiting`/`asking` that persists past a short debounce window (~15s, to skip transient blips already visible in the hook data) triggers one notification. The reverse transition (back to `working`) is silent — only escalations page the human, to keep the channel low-noise.
4. **Fails silently, never fatally.** An unreachable or misconfigured webhook logs a one-line warning to ccdash's existing log path and otherwise does not affect the dashboard; the feature is fully inert (zero behavior change, zero network calls) until a user opts in via config.

## Alternatives Considered

- **Fire the notification from the hook shell scripts directly** (`notification.sh` / `permission-request.sh` doing the `curl`). Rejected: those scripts are stateless per-invocation and have no visibility into debounce, dedup, or whether another ccdash instance already notified — this is exactly the class of problem the existing leader-election lease was built to solve, and reusing it beats reimplementing the same coordination in bash.
- **OS-native desktop notification** (`notify-send`, terminal bell) instead of/in addition to a webhook. Rejected as the *primary* channel: this is a headless Hetzner server reached over Tailscale — the user is routinely away from any terminal entirely, not merely unfocused on one. Kept as a candidate follow-up bead (cheap, local-only, no config needed) rather than folded into this ADR.
- **Do nothing / status quo (TUI-only signal).** Rejected: given the fleet's scale and the multiple existing memory-documented incidents of sessions silently stalling on human input, the latency between "session needs a human" and "a human notices" is a real, recurring cost, and the hook infrastructure to detect the transition precisely already shipped in 1.0.2 — only the delivery half is missing.
- **Central daemon that all ccdash instances report through**, rather than leader-election-gated notification from within existing instances. Rejected: mirrors the "Central Daemon with RPC" rejection in ADR 0005 — adds a new single point of failure and deployment unit for no benefit the existing per-instance-with-lease model doesn't already provide.

## Consequences

### Positive

- Turns ccdash from "must be watching" into a proactive alert on exactly the fleet's most common failure mode (stalled-on-human sessions).
- Reuses two things ccdash already shipped and already tested — the 1.0.2 hook wiring and the token-cache leader-election lease — rather than inventing new coordination.
- Strictly opt-in and additive: zero effect on any user who doesn't configure a webhook, so this doesn't change ccdash's behavior for the broader public GitHub audience.

### Negative

- Introduces ccdash's first outbound network dependency and first config file — new failure surface (silent-fail is a mitigation, but a misconfigured URL now means silent *non*-delivery too; a `ccdash --test-notify` command is the natural follow-up to make that debuggable).
- Slight scope creep from "pure dashboard" toward "alerting agent" — worth being explicit about in review since it's a one-way door for the project's identity as a lightweight, dependency-free TUI.
- Couples the public ccdash repo conceptually to a private-infra sibling project (`telegram-claude-bridge`); mitigated by keeping the endpoint fully user-supplied config with no bridge-specific assumptions baked into the payload shape (a generic JSON POST works for any webhook receiver, not just this fleet's bridge).

### Neutral

- This is the first ccdash feature that sends any data off-host. The payload is deliberately minimal (session/project name, idle duration) and excludes token counts, cost figures, and file contents — worth stating plainly in the README once implemented, since ccdash has external users who will reasonably ask "does this phone home."

## References

- Related ADR: ADR 0005 (per-instance independence, leader-election precedent)
- `docs/plan.md` — canonical copy of this ADR under "ADR-0006"
- `CHANGELOG.md` [1.0.2] — the Notification/PermissionRequest hook wiring this decision builds on
