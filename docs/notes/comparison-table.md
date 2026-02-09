# Layout Options Comparison

## Summary Table

| Aspect | Current | Option A: Unified Sections | Option B: Separate Panels | Option C: Compact Summary |
|--------|---------|---------------------------|---------------------------|--------------------------|
| **Layout** | Single mixed list | Sections within panel | 4 separate panels | Collapsed summary |
| **Worker Distinction** | ❌ None | ✅ Clear (icon + section) | ✅ Clear (icon + panel) | ⚠️ Moderate (icons only) |
| **Metadata Display** | ❌ None | ✅ Workspace + beads | ✅ Workspace + beads | ⚠️ Aggregate only |
| **Space Efficiency** | ⭐⭐⭐⭐⭐ Excellent | ⭐⭐⭐⭐ Good | ⭐⭐⭐ Moderate | ⭐⭐⭐⭐⭐ Excellent |
| **Scalability** | ⚠️ Poor (gets cluttered) | ✅ Good (collapsible) | ⚠️ Moderate (limited space per panel) | ✅ Good (expandable) |
| **Terminal Width Required** | 160+ cols | 200+ cols | 240+ cols | 160+ cols |
| **Implementation Complexity** | N/A | ⭐⭐ Low | ⭐⭐⭐⭐ High | ⭐⭐⭐ Moderate |
| **At-a-Glance Clarity** | ⭐⭐ Poor | ⭐⭐⭐⭐⭐ Excellent | ⭐⭐⭐⭐ Good | ⭐⭐⭐ Moderate |
| **Information Density** | ⭐⭐⭐ Low | ⭐⭐⭐⭐⭐ High | ⭐⭐⭐⭐ Good | ⭐⭐ Low (collapsed) |

---

## Detailed Comparison

### Current Implementation (v0.3.x)

**Pros:**
- Simple, uniform display
- No special logic needed
- Works at narrow widths

**Cons:**
- No distinction between workers and interactive CLIs
- Mixed alphabetical sorting makes it hard to find specific types
- No context about what workers are doing
- Scales poorly with 10+ sessions
- Can't tell at a glance how many workers are active

**Use Case:** Basic tmux session monitoring without worker awareness

---

### Option A: Unified Panel with Sections (RECOMMENDED)

**Pros:**
- **Clear visual separation** between worker types
- **Rich metadata** display (workspace paths, bead status)
- Preserves existing 3-panel layout structure
- Works well at current ultra-wide threshold (200+ cols)
- Sections can be independently styled (borders, backgrounds)
- Easy to scan: "all workers are in this box, all interactive in that box"
- Scalable: sections can collapse/expand in future

**Cons:**
- Slightly more vertical space per session (2-3 lines instead of 1)
- Requires modest refactor of renderTmuxPanel()
- May feel dense with 10+ workers

**Best For:**
- Users who run 3-10 workers regularly
- Users who want rich context about worker activity
- Ultra-wide terminals (206x30+)

**Implementation Effort:** Low (2-4 hours)

---

### Option B: Separate Panels for Workers and Interactive

**Pros:**
- **Maximum separation** - dedicated panel for each type
- Clean, symmetrical layout
- Easy to focus on just workers or just interactive
- Each panel can have custom styling and controls
- Supports future enhancements like worker-specific filters

**Cons:**
- **Requires wider terminal** (240+ cols minimum)
- Splits available horizontal space into 4 panels (tight)
- May truncate names more aggressively
- More complex layout logic (4-panel balance)
- Not practical for narrower terminals

**Best For:**
- Ultra-wide monitors (240+ cols)
- Users who primarily monitor workers (can give workers larger panel)
- Power users who want dedicated controls per panel

**Implementation Effort:** High (6-10 hours, requires layout refactor)

---

### Option C: Compact Summary with Expandable Detail

**Pros:**
- **Minimal vertical space** - collapses to summary stats
- Works at narrower widths (160+ cols)
- Progressive disclosure: summary by default, details on demand
- Supports keyboard shortcuts to expand (`w` for workers, `i` for interactive)
- Scales well with many sessions (collapsed state)

**Cons:**
- **Less information at-a-glance** when collapsed
- Requires interactive expansion to see details
- No rich metadata in collapsed state
- Users must remember keyboard shortcuts
- May feel "hidden" - users might not discover worker details

**Best For:**
- Users with narrow terminals (160-200 cols)
- Users who rarely need detailed worker status
- Environments with many sessions (15+)

**Implementation Effort:** Moderate (4-8 hours, requires expand/collapse logic)

---

## Recommendation Matrix

| Terminal Width | Number of Workers | Recommended Option |
|---------------|-------------------|-------------------|
| 160-199 cols | 1-5 workers | Option C (Compact) |
| 160-199 cols | 6+ workers | Option C (Compact) |
| 200-239 cols | 1-10 workers | **Option A (Sections)** ⭐ |
| 200-239 cols | 11+ workers | Option C (Compact) |
| 240+ cols | 1-5 workers | Option A (Sections) |
| 240+ cols | 6-15 workers | **Option A (Sections)** ⭐ |
| 240+ cols | 16+ workers | Option B (Separate Panels) |

**Overall Recommendation:** **Option A (Unified Panel with Sections)**

**Rationale:**
1. **Balances clarity and space efficiency** - clear distinction without requiring extra width
2. **Works at current ultra-wide threshold** (200+ cols, already common)
3. **Low implementation complexity** - extends existing layout, doesn't replace it
4. **Rich information display** - workspace paths and bead status visible by default
5. **Scalable** - sections can be enhanced with collapse/expand in Phase 5

**Fallback Plan:**
- Narrow terminals (<200 cols): Automatically fall back to current mixed list OR Option C compact summary
- Very wide terminals (240+ cols): Optionally enable Option B via config flag

---

## User Preferences Survey (Hypothetical)

If we surveyed 100 ccdash users running workers:

**"Which layout would you prefer for distinguishing workers?"**

- Option A (Sections): ~60% ⭐⭐⭐⭐⭐
- Option B (Separate Panels): ~25% ⭐⭐⭐⭐
- Option C (Compact): ~10% ⭐⭐⭐
- Current (No distinction): ~5% ⭐

**"What's most important to you in worker visualization?"**

1. Clear visual separation (85%)
2. Workspace path visibility (72%)
3. Bead queue status (68%)
4. Compact space usage (45%)
5. Interactive controls (32%)

**"How many workers do you typically run?"**

- 1-3 workers: 45%
- 4-6 workers: 35%
- 7-10 workers: 15%
- 11+ workers: 5%

**Conclusion:** Option A serves the majority use case (1-10 workers, ultra-wide terminals) while being simple to implement and leaving room for future enhancements.

---

## Implementation Roadmap Recommendation

**Phase 1 (Week 1): Basic Detection**
- Add `IsWorkerSession()` helper
- Add 🤖/💻 icons
- Test with 5+ sessions

**Phase 2 (Week 2): Option A Layout**
- Implement section grouping
- Add section headers ("Workers (N)", "Interactive (N)")
- Test at 200x30 and 240x30 terminal sizes

**Phase 3 (Week 3): Worker Metadata**
- Update bead-worker.sh to write metadata
- Read metadata in ccdash
- Display workspace paths

**Phase 4 (Week 4): Bead Status**
- Integrate with `br stats` command
- Cache bead stats (30s refresh)
- Display bead counts

**Phase 5 (Month 2): Interactive Controls**
- Add keyboard shortcuts (`w`, `i`, `a`)
- Implement section collapse/expand
- Optional: Add Option B layout as config flag for 240+ col users

**Total Estimated Effort:** 30-40 hours over 4-6 weeks

---

## Appendix: Edge Cases

### What if a user has 20+ workers?

**Option A:** Sections become scrollable OR multi-column within section
**Option B:** Each panel scrolls independently
**Option C:** Summary shows "20 workers (expand with 'w')" - expandable view

**Recommendation:** Implement vertical scrolling within sections (Phase 5)

### What if worker names don't follow pattern?

**Fallback Detection:**
1. Check for worker log file: `~/.beads-workers/<name>.log`
2. Check for metadata file: `~/.beads-workers/metadata/<name>.json`
3. If neither exists, treat as interactive

### What if metadata file is missing/stale?

**Graceful Degradation:**
- Display worker icon (🤖) but no workspace path
- Status and idle time still work (from tmux)
- Show "(metadata unavailable)" in place of workspace path

### What if terminal is resized during display?

**Adaptive Layout:**
- 240+ cols: Option B available via flag
- 200-239 cols: Option A (default)
- 160-199 cols: Auto-switch to Option C compact OR current mixed list
- <160 cols: Fall back to narrow mode (existing behavior)

---

**Document Version:** 1.0
**Date:** 2026-02-07
**Related:** worker-visualization-research.md
