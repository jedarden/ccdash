# ADR 0002: Display Interactive Sessions Before Workers

## Status

Accepted (2026-02-07)

## Context

After deciding to distinguish worker and interactive sessions (see ADR 0001), we must determine the **display order** for the two session types.

**Two options:**
1. Workers first, Interactive second (initial research assumption)
2. Interactive first, Workers second

**User feedback:** Explicit request to display interactive sessions first.

## Decision

We will **display Interactive sessions BEFORE Workers** in the tmux panel.

### Rendering Order

```go
// In renderTmuxPanel()
if len(interactive) > 0 {
    sections = append(sections, d.renderInteractiveSection(interactive, width-4))
}
if len(workers) > 0 {
    sections = append(sections, d.renderWorkerSection(workers, width-4))
}
```

### Visual Result

```
Interactive (2)        ← Top section
💻 alpha   🟡 ACT 30s 📎
💻 delta   🔴 READY 10m 📎

Workers (3)            ← Bottom section
🤖 c-glm-alpha  🟢 WORK 2m ~/kalshi
🤖 c-glm-bravo  🔴 READY 5m ~/botburrow
🤖 o-glm-charlie 🟡 ACT 1m ~/backtest
```

## Consequences

### Positive

1. **User-Focused Priority**
   - Interactive sessions are what users directly control
   - Users naturally check "what am I doing?" before "what are workers doing?"

2. **Immediate Actionability**
   - Attached indicator (📎) is most relevant for interactive sessions
   - Users can immediately see if they're attached to the right session

3. **Visual Scanning Efficiency**
   - Eye naturally scans top-to-bottom
   - Most important/actionable information appears first

4. **Scalability**
   - Workers can scale to 10+ sessions
   - Placing them last prevents pushing interactive sessions off-screen
   - Interactive sessions typically 2-4, always visible at top

5. **UX Consistency**
   - Follows terminal convention: user processes before system processes
   - Aligns with "user > system" priority principle

### Negative

- **Change from initial research**: All mockups and examples needed updating
- **Potential confusion**: If users expect workers first (unlikely given user request)

### Neutral

- **No performance impact**: Pure display order change
- **Implementation simplicity**: Swapping two lines of code

## Rationale Details

### Why Interactive First Matters

**User Mental Model:**
1. "Am I attached to the right session?" (Interactive)
2. "Is my work running?" (Interactive)
3. "How are background jobs doing?" (Workers)

**Action Hierarchy:**
- Direct user actions (attach, detach) → Interactive sessions
- Monitoring actions (check status) → Worker sessions

**Information Hierarchy:**
- Time-sensitive: Attached status, current work
- Periodic check: Worker health, bead progress

### Scenario Analysis

**Scenario 1: One Interactive, Seven Workers (Typical)**
```
Interactive (1)        ← User's session immediately visible
💻 alpha  🟡 ACT 30s 📎

Workers (7)            ← Background status below
🤖 c-glm-alpha  ...
🤖 c-glm-bravo  ...
... (5 more workers)
```
User sees their session first, workers don't clutter priority view.

**Scenario 2: Multiple Interactive, No Workers (Development)**
```
Interactive (4)        ← All user sessions listed
💻 alpha   ...
💻 bravo   ...
💻 charlie ...
💻 delta   ...

Workers (0)            ← Section omitted when empty
(no workers)
```
User sees all their sessions, empty worker section doesn't waste space.

## Alternatives Considered

### Alternative 1: Workers First
Display workers before interactive sessions.

**Rejected because:**
- User explicitly requested interactive first
- Doesn't align with user's mental model
- Workers can push interactive sessions off-screen in constrained displays

### Alternative 2: Side-by-Side (Left/Right)
Display interactive on left, workers on right.

**Rejected because:**
- Requires wider display (240+ cols minimum)
- Not feasible for 199x14 display constraint
- Increases implementation complexity

### Alternative 3: Configurable Order
Let users choose display order via config.

**Rejected because:**
- Adds unnecessary complexity
- No compelling use case for workers-first priority
- Strong consensus for interactive-first approach

## Implementation Impact

**Files Modified:**
- `internal/ui/dashboard.go`: Swap section rendering order
- All documentation/mockups updated

**Code Change:**
```diff
- // Render workers section
- if len(workers) > 0 {
-     sections = append(sections, d.renderWorkerSection(workers, width-4))
- }
  // Render interactive section
  if len(interactive) > 0 {
      sections = append(sections, d.renderInteractiveSection(interactive, width-4))
  }
+ // Render workers section
+ if len(workers) > 0 {
+     sections = append(sections, d.renderWorkerSection(workers, width-4))
+ }
```

**Testing:**
- Verify interactive appears at top
- Verify workers appear at bottom
- Verify empty sections are omitted
- Verify scrolling behavior with 10+ sessions

## References

- User request: Context conversation (2026-02-07)
- Related ADR: ADR 0001 (Session distinction)
- Updated research: `CHANGELOG.md` and the archived material in `docs/research/`
- Visual mockups: `docs/research/layout-mockups-199x14.txt`
