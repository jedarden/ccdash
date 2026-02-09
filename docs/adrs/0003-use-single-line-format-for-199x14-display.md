# ADR 0003: Use Single-Line Format for 199x14 Display

## Status

Accepted (2026-02-07)

## Context

Initial research assumed a display size of **206x30** (206 cols × 30 rows), which allows multi-line worker cells showing:
- Line 1: Icon + full session name
- Line 2: Full workspace path
- Line 3: Status + windows + idle + bead status

**Actual display size: 199x14** (199 cols × 14 rows)

### Constraint Analysis

```
Total height: 14 rows
Panel header/footer: -3 rows
Available content height: 11 rows

Multi-line format capacity:
- Interactive (2 sessions × 2 lines): 4 rows
- Workers (3 sessions × 3 lines): 9 rows
- Section headers: 2 rows
Total: 15 rows → EXCEEDS 11 row limit!
```

**Critical finding:** The **14-row height** is the binding constraint, not width.

### Space Allocation at 199x14

```
Width: 199 cols
Panel padding: -6 cols
Total: 193 cols

Distribution:
- System: 60 cols
- Token: 60 cols
- Tmux: 73 cols (content: 69 cols)
```

**Width is sufficient** for abbreviated single-line display.

## Decision

We will use **single-line cell format** for worker and interactive sessions at 199x14 displays.

### Single-Line Format

```
[icon] [abbreviated-name] [status-emoji] [status] [idle] [workspace] [attached]
🤖 c-glm-alpha  🟢 WORK  2m  ~/kalshi
```

### Abbreviation Strategy

#### Session Names
```
claude-code-glm-47-alpha → c-glm-alpha (saves 17 chars, 65% reduction)
opencode-glm-47-bravo    → o-glm-bravo (saves 13 chars, 62% reduction)
claude-code-sonnet-delta → c-sonnet-delta (saves 12 chars, 52% reduction)
```

**Approach:**
- `claude-code-` → `c-`
- `opencode-` → `o-`
- `-47` removed (version number omitted)
- NATO callsign preserved (essential identifier)

#### Status Text
```
WORKING → WORK  (saves 3 chars, 43% reduction)
ACTIVE  → ACT   (saves 3 chars, 50% reduction)
READY   → READY (no change, already short)
ERROR   → ERR   (saves 2 chars, 40% reduction)
```

#### Workspace Paths
```
/home/coder/prompts/kalshi-improvement → ~/kalshi (saves ~30 chars, 80% reduction)
/home/coder/ardenone-cluster/botburrow → ~/botburrow (saves ~25 chars, 75% reduction)
```

**Approach:**
- Replace `$HOME` with `~`
- Show last directory segment only
- Truncate if > 12 chars: `kalshi-improvement` → `kalshi-imp...`

### Capacity with Single-Line Format

```
Interactive (2 lines) + spacing (1 line): 3 rows
Workers (3 lines) + spacing (1 line): 4 rows
Section headers: 2 rows
Total: 9 rows → Fits in 11 rows with room to spare!
```

**Advantage:** Can show 6-7 sessions comfortably, vs 3-4 with multi-line format.

## Consequences

### Positive

1. **Fits Display Constraint**
   - 9 rows for 5 sessions leaves 2 rows buffer
   - Can accommodate 1-2 additional sessions if needed

2. **More Sessions Visible**
   - Single-line format: 6-7 sessions visible
   - Multi-line format: 3-4 sessions visible
   - 50% increase in capacity

3. **Faster Scanning**
   - Users can scan 6 sessions in single view
   - No need to scroll for typical 3 workers + 2 interactive

4. **Less Overwhelming**
   - Compact display reduces visual clutter
   - Essential information preserved

### Negative

1. **Abbreviated Names Less Clear**
   - `c-glm-alpha` may be ambiguous initially
   - Users must learn abbreviation pattern
   - **Mitigation:** Consistent pattern, expandable detail view (Phase 5)

2. **Limited Workspace Context**
   - Only last directory shown (`~/kalshi` vs full path)
   - May be ambiguous for nested projects
   - **Mitigation:** Expandable detail view shows full paths

3. **No Inline Bead Status**
   - Bead queue status not visible in compact view
   - Users must use expandable view ('w' key) for details
   - **Mitigation:** Acceptable trade-off for space savings

### Neutral

- **Abbreviation functions add complexity**: Minimal, ~30 lines of code
- **Different from wider displays**: Acceptable, adaptive design for constraints
- **Per-user adaptation**: Each user's ccdash adapts independently to their terminal size (ADR 0005)

## Implementation

### Core Functions

```go
// Abbreviate worker name
func abbreviateWorkerName(name string) string {
    name = strings.Replace(name, "claude-code-", "c-", 1)
    name = strings.Replace(name, "opencode-", "o-", 1)
    name = strings.Replace(name, "-47", "", 1)
    return name
}

// Abbreviate status
func abbreviateStatus(status metrics.SessionStatus) string {
    switch status {
    case metrics.StatusWorking: return "WORK"
    case metrics.StatusActive: return "ACT"
    case metrics.StatusReady: return "READY"
    case metrics.StatusError: return "ERR"
    }
    return string(status)
}

// Abbreviate workspace path
func abbreviateWorkspace(path string) string {
    // Replace home with ~
    if strings.HasPrefix(path, os.Getenv("HOME")) {
        path = "~" + strings.TrimPrefix(path, os.Getenv("HOME"))
    }

    // Extract last segment
    parts := strings.Split(path, "/")
    if len(parts) > 0 {
        last := parts[len(parts)-1]
        if len(last) > 12 {
            return last[:9] + "..."
        }
        return last
    }
    return path
}
```

### Cell Renderer

```go
func (d *Dashboard) renderWorkerCell(worker metrics.TmuxSession, width int) string {
    icon := "🤖"
    name := abbreviateWorkerName(worker.Name)
    statusEmoji := worker.Status.GetEmoji()
    status := abbreviateStatus(worker.Status)
    idle := formatDuration(worker.IdleDuration)
    workspace := ""
    if worker.WorkerMetadata != nil {
        workspace = abbreviateWorkspace(worker.WorkerMetadata.Workspace)
    }

    return fmt.Sprintf("%s %-15s %s %-5s %4s  ~/%s",
        icon, name, statusEmoji, status, idle, workspace)
}
```

## Alternatives Considered

### Alternative 1: Multi-Line Format (Original Plan)
Use 3-line worker cells with full names and paths.

**Rejected because:**
- Doesn't fit 11-row height constraint
- Only shows 3-4 sessions max
- Wastes vertical space for minimal additional context

### Alternative 2: Horizontal Compression Only
Keep multi-line format, compress width instead.

**Rejected because:**
- Height is the binding constraint, not width
- Width compression wouldn't solve the problem
- 69 cols content width already tight

### Alternative 3: Scrollable View
Implement scrolling for multi-line format.

**Rejected because:**
- Adds interaction complexity
- Users want at-a-glance view without scrolling
- Scrolling defeats purpose of dashboard

### Alternative 4: Hide Some Sessions
Only show top N sessions, hide rest.

**Rejected because:**
- Users need full session list visibility
- Hidden sessions may be critical
- Arbitrary cutoff (which N?) creates confusion

## Display-Size Adaptive Strategy

**For displays >= 200 cols and >= 20 rows:**
Use multi-line format (original QUICKSTART.md)

**For displays < 200 cols or < 20 rows:**
Use single-line format (QUICKSTART-199x14.md)

**Detection:**
```go
func shouldUseSingleLineFormat(width, height int) bool {
    return width < 200 || height < 20
}
```

This provides optimal layout for each display constraint.

**Multi-user support:** Each ccdash instance independently reads its own terminal dimensions and selects the appropriate layout. Users with different terminal sizes can run ccdash concurrently, each getting the optimal format for their display (see ADR 0005).

## Testing Requirements

- [ ] Verify 5 sessions fit in 11 rows (2 interactive + 3 workers)
- [ ] Verify abbreviations render correctly
- [ ] Verify workspace paths show last directory
- [ ] Verify names truncate gracefully if still too long
- [ ] Test with 7 sessions (should still fit)
- [ ] Test with 0 interactive or 0 workers (empty sections)
- [ ] Verify line wrapping doesn't occur

## References

- Display analysis: `docs/notes/ADDENDUM-199x14-analysis.md`
- Implementation guide: `docs/notes/QUICKSTART-199x14.md`
- Visual mockups: `docs/notes/layout-mockups-199x14.txt`
- Space calculations: `docs/notes/display-size-comparison.txt`
