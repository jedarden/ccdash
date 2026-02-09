# ADR 0001: Distinguish Worker from Interactive Sessions

## Status

Accepted (2026-02-07)

## Context

ccdash displays all tmux sessions uniformly without distinguishing between:

1. **Worker sessions**: Autonomous bead agents processing tasks from workspace queues
   - Naming pattern: `claude-code-glm-47-alpha`, `opencode-glm-47-bravo`
   - Purpose: Background task processing
   - Typical count: 3-10 sessions

2. **Interactive sessions**: User-attached terminals running Claude Code interactively
   - Naming pattern: Simple NATO callsigns (`alpha`, `bravo`, `charlie`)
   - Purpose: Direct user interaction
   - Typical count: 2-4 sessions

**Problem:**
- Users cannot quickly identify which sessions are their personal terminals vs background workers
- No context about what workers are doing (workspace, task queue status)
- Mixed alphabetical sorting makes assessment difficult
- Scales poorly with 10+ total sessions

## Decision

We will **visually distinguish worker sessions from interactive sessions** using:

### 1. Icon-Based Identification
- Workers: 🤖 robot icon
- Interactive: 💻 computer icon

### 2. Section-Based Grouping
- Interactive sessions displayed in "Interactive (N)" section
- Worker sessions displayed in "Workers (N)" section
- Sections separated visually with headers and spacing

### 3. Detection Strategy
Primary method: Pattern matching on session names (stateless, multi-user safe)
```go
func IsWorkerSession(sessionName string) bool {
    executors := []string{
        "claude-code-glm-47-",
        "claude-code-sonnet-",
        "opencode-glm-47-",
    }
    for _, prefix := range executors {
        if strings.HasPrefix(sessionName, prefix) {
            return true
        }
    }
    return false
}
```

Fallback method: Check for worker log file at `~/.beads-workers/<session-name>.log`

**Multi-user consideration:** Pattern matching is stateless and operates on session names visible to the current user only. Each ccdash instance independently detects workers without coordination (see ADR 0005).

### 4. Extended Metadata (Future)
Workers will display additional context:
- Workspace path
- Bead queue status (ready/blocked/completed counts)
- Executor type

## Consequences

### Positive

- **Clear visual distinction**: Users immediately recognize session types
- **Better organization**: Grouped display prevents cognitive overload
- **Enhanced context**: Workers show relevant metadata (workspace, beads)
- **Scalability**: Handles 10+ workers without confusion
- **Actionable information**: Users can quickly assess worker health
- **Multi-user safe**: Pattern matching is stateless, works independently per user (see ADR 0005)

### Negative

- **Implementation complexity**: Requires session type detection and dual rendering paths
- **Maintenance burden**: Executor name patterns must be kept in sync with worker naming conventions
- **Space consumption**: Section headers and metadata reduce available space for sessions
- **Pattern brittleness**: Non-standard worker names won't be detected (mitigated by log file fallback)

### Neutral

- **Performance impact**: Minimal (<10ms per refresh for metadata reads)
- **Backward compatibility**: Display-only change, no breaking API changes
- **Concurrent access**: Each ccdash instance operates independently (ADR 0005)

## Implementation Notes

- **Phase 1** (2-4 hours): Basic detection and icon display
- **Phase 2** (4-6 hours): Section-based grouping
- **Phase 3** (4-8 hours): Worker metadata integration
- **Total effort**: 10-18 hours

See `docs/notes/QUICKSTART-199x14.md` for implementation guide.

## Alternatives Considered

### Alternative 1: Color-Based Distinction Only
Use background colors instead of icons and sections.

**Rejected because:**
- Less explicit than icons
- Relies on color perception
- Doesn't provide organizational grouping

### Alternative 2: Separate TUI Tool (`beads-dash`)
Create dedicated worker dashboard separate from ccdash.

**Rejected because:**
- Duplicates tmux session tracking logic
- Forces users to switch between tools
- Loses unified view of system + tokens + sessions

### Alternative 3: No Distinction
Leave all sessions in mixed list.

**Rejected because:**
- Doesn't solve the problem
- User feedback indicates need for clarity
- Scales poorly with many workers

## References

- Research: `docs/notes/worker-visualization-research.md`
- Implementation guide: `docs/notes/QUICKSTART-199x14.md`
- Display analysis: `docs/notes/ADDENDUM-199x14-analysis.md`
