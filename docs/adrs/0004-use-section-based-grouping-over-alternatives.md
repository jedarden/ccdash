# ADR 0004: Use Section-Based Grouping Over Alternatives

## Status

Accepted (2026-02-07)

## Context

Having decided to distinguish workers from interactive sessions (ADR 0001), we need to determine **how to visually organize** the two session types in the tmux panel.

**Three main approaches were evaluated:**

### Option A: Unified Panel with Sections
Display both session types in the tmux panel, grouped by section headers.
```
┌─ Tmux Sessions ──────────┐
│ Interactive (2)          │
│ 💻 alpha  ...            │
│ 💻 delta  ...            │
│                          │
│ Workers (3)              │
│ 🤖 c-glm-alpha  ...      │
│ 🤖 c-glm-bravo  ...      │
│ 🤖 o-glm-charlie  ...    │
└──────────────────────────┘
```

### Option B: Separate Panels
Dedicated panel for workers, dedicated panel for interactive.
```
┌─ System ──┬─ Token ──┬─ Workers ──┬─ Interactive ─┐
│ ...       │ ...      │ 🤖 ...     │ 💻 ...        │
└───────────┴──────────┴────────────┴───────────────┘
```

### Option C: Compact Summary with Expandable Detail
Collapsed summary by default, expandable on demand.
```
┌─ Tmux Sessions ──────────┐
│ Workers (3): 🤖🤖🤖       │
│ 2 WORKING, 1 READY       │
│ Press 'w' for details    │
│                          │
│ Interactive (2): 💻💻    │
│ 2 ACTIVE, both attached  │
└──────────────────────────┘
```

## Decision

We will use **Option A: Unified Panel with Sections** for worker/interactive grouping.

### Implementation

```go
func (d *Dashboard) renderTmuxPanel(width, height int) string {
    // Group sessions by type
    workers, interactive := d.groupSessions(sessions)

    var sections []string

    // Render interactive section (first)
    if len(interactive) > 0 {
        sections = append(sections, d.renderInteractiveSection(interactive, width-4))
    }

    // Render workers section (second)
    if len(workers) > 0 {
        sections = append(sections, d.renderWorkerSection(workers, width-4))
    }

    return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
```

### Section Headers

```go
func (d *Dashboard) renderInteractiveSection(sessions []TmuxSession, width int) string {
    header := lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("214")). // Orange
        Render(fmt.Sprintf("Interactive (%d)", len(sessions)))

    // ... render cells ...
}

func (d *Dashboard) renderWorkerSection(workers []TmuxSession, width int) string {
    header := lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("45")). // Cyan
        Render(fmt.Sprintf("Workers (%d)", len(workers)))

    // ... render cells ...
}
```

## Consequences

### Positive

1. **Clear Visual Separation**
   - Section headers explicitly label each group
   - Consistent with existing ccdash panel structure
   - No ambiguity about session types

2. **Works at 199x14 Display**
   - Fits within existing tmux panel
   - No additional horizontal space required
   - Single-line format + sections = 9 rows (fits in 11)

3. **Low Implementation Complexity**
   - Extends existing `renderTmuxPanel()` function
   - No major layout refactoring required
   - ~50 lines of new code
   - **Effort:** 4-6 hours

4. **Scalable Design**
   - Sections can accommodate 10+ workers
   - Empty sections automatically omitted
   - Future: Sections can collapse/expand (Phase 5)

5. **Rich Information Display**
   - All session details visible by default
   - No interaction required for basic info
   - Section counts provide quick summary

6. **Consistent UX**
   - Matches existing panel-based layout
   - Users familiar with system/token panels
   - Natural extension of current design

### Negative

1. **Vertical Space Usage**
   - Section headers consume 2 rows
   - Less space for sessions vs no sections
   - **Mitigation:** Single-line format compensates

2. **Not Optimal for 10+ Workers**
   - Many workers may require scrolling (future)
   - Section can become long
   - **Mitigation:** Option C can be added later for many-worker case

### Neutral

- **Not as space-efficient as Option C**: Acceptable, information density preferred
- **Not as separated as Option B**: Acceptable, unified view preferred

## Comparison Matrix

| Criterion | Option A (Sections) | Option B (Separate Panels) | Option C (Compact) |
|-----------|---------------------|---------------------------|-------------------|
| **Fits 199x14** | ✅ Yes | ❌ No (needs 240+ cols) | ✅ Yes |
| **Implementation** | ⭐⭐ Low (4-6h) | ⭐⭐⭐⭐ High (8-12h) | ⭐⭐⭐ Moderate (6-8h) |
| **Info density** | ⭐⭐⭐⭐⭐ High | ⭐⭐⭐⭐ Good | ⭐⭐ Low (collapsed) |
| **Clarity** | ⭐⭐⭐⭐⭐ Excellent | ⭐⭐⭐⭐ Good | ⭐⭐⭐ Moderate |
| **Scalability** | ⭐⭐⭐⭐ Good (6-8 sessions) | ⭐⭐⭐ Moderate | ⭐⭐⭐⭐⭐ Excellent (unlimited) |
| **Interaction** | None required | None required | 'w' key required |

**Option A wins on:** Clarity, info density, implementation simplicity, display fit

## Alternatives Rejected

### Option B: Separate Panels

**Why rejected:**
1. **Display constraint**: Requires 240+ cols minimum (4 panels)
2. **Complexity**: Major layout refactor, 4-panel balancing logic
3. **Space waste**: Each panel has borders/padding (12 cols overhead)
4. **Not needed**: Users prefer unified session view

**When to reconsider:** If users request dedicated worker panel and have 240+ col displays.

### Option C: Compact Summary

**Why rejected:**
1. **Interaction required**: Users must press 'w' to see details
2. **Hidden information**: Basic session list not immediately visible
3. **Cognitive load**: Users must remember keyboard shortcuts
4. **Discovery problem**: New users may not find expandable view

**When to reconsider:** If users regularly run 15+ sessions and need space optimization.

**Future integration:** Option C can complement Option A as an **alternative view mode**, not replacement.

## Future Enhancements

### Phase 5: Collapsible Sections
Add ability to collapse/expand sections:
```
Interactive (2) [collapse]    ← Click to collapse
💻 alpha  ...
💻 delta  ...

Workers (3) [collapse]        ← Click to collapse
🤖 c-glm-alpha  ...
...
```

### Optional Compact Mode
Add keyboard shortcut to toggle compact mode (Option C):
```
Press 'c' → Compact view
Press 'c' again → Full view
```

This provides best of both worlds for different use cases.

## Implementation Notes

### Empty Section Handling

```go
// Empty sections are automatically omitted
if len(interactive) > 0 {
    sections = append(sections, d.renderInteractiveSection(interactive, width-4))
}
// If no interactive sessions, section not rendered
```

### Section Styling

- **Interactive**: Orange header (`lipgloss.Color("214")`)
- **Workers**: Cyan header (`lipgloss.Color("45")`)
- Consistent with existing ccdash color scheme

### Header Format

```
Interactive (2)    ← Count in parentheses
Workers (3)        ← Bold, colored text
```

## Testing Requirements

- [ ] Verify sections render with correct headers
- [ ] Verify section headers show correct counts
- [ ] Verify empty sections are omitted
- [ ] Verify section colors render correctly (orange/cyan)
- [ ] Test with 0 interactive sessions
- [ ] Test with 0 worker sessions
- [ ] Test with 5+ sessions in each section
- [ ] Verify spacing between sections

## References

- Related ADR: ADR 0001 (Session distinction)
- Related ADR: ADR 0002 (Display order)
- Related ADR: ADR 0003 (Single-line format)
- Comparison table: `docs/notes/comparison-table.md`
- Visual mockups: `docs/notes/layout-mockups-199x14.txt`
