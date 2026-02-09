# Research Changelog - Worker TUI Visualization

## 2026-02-07 - Session Order Update

### Change Summary
**Interactive sessions now displayed BEFORE workers** (previously: workers first)

### Rationale
1. **User-Focused Priority**: Interactive sessions are what users directly control
2. **Immediate Actionability**: Users check "am I attached?" before checking worker status
3. **Visual Scanning**: Eye naturally scans top-to-bottom - most important info first
4. **UX Consistency**: Follows terminal convention of user processes before system processes
5. **Scalability**: Workers can scale to 10+ sessions - placing them last prevents pushing interactive off-screen

### Updated Order

**NEW (Interactive First):**
```
Interactive (2)
💻 alpha          🟡 ACT   30s  📎
💻 delta          🔴 READY 10m  📎

Workers (3)
🤖 c-glm-alpha    🟢 WORK   2m  ~/kalshi
🤖 c-glm-bravo    🔴 READY  5m  ~/botburrow
🤖 o-glm-charlie  🟡 ACT    1m  ~/backtest
```

**OLD (Workers First):**
```
Workers (3)
🤖 c-glm-alpha    🟢 WORK   2m  ~/kalshi
🤖 c-glm-bravo    🔴 READY  5m  ~/botburrow
🤖 o-glm-charlie  🟡 ACT    1m  ~/backtest

Interactive (2)
💻 alpha          🟡 ACT   30s  📎
💻 delta          🔴 READY 10m  📎
```

### Implementation Change

**File:** `/home/coder/ccdash/internal/ui/dashboard.go`
**Function:** `renderTmuxPanel()`

```go
// NEW ORDER (Interactive first)
if len(interactive) > 0 {
    sections = append(sections, d.renderInteractiveSection(interactive, width-4))
}
if len(workers) > 0 {
    sections = append(sections, d.renderWorkerSection(workers, width-4))
}
```

### Files Updated
- ✅ `QUICKSTART-199x14.md` - Code examples and expected results
- ✅ `ADDENDUM-199x14-analysis.md` - Layout capacity calculations
- ✅ `display-size-comparison.txt` - Visual mockups
- ✅ `layout-mockups-199x14.txt` - NEW: 199x14-specific mockups
- ✅ Implementation sketch references

### Backwards Compatibility
**No breaking changes** - this is a display order change only. Existing code structure remains the same.

---

## Initial Research - 2026-02-07

### Display Constraint Discovery
- **Actual display:** 199x14 (not 206x14 as initially assumed)
- **Critical constraint:** 14-row height (11 rows for content)
- **Impact:** Multi-line worker cells not feasible → single-line format required

### Key Decisions
1. Single-line cell format for 199x14 displays
2. Abbreviated names: `claude-code-glm-47-alpha` → `c-glm-alpha`
3. Abbreviated status: `WORKING` → `WORK`
4. Abbreviated workspace: full path → `~/last-dir`
5. Section grouping preserved (Interactive/Workers)

### Research Documents Created
- `worker-visualization-research.md` - Comprehensive analysis (original 206x14+ assumption)
- `ADDENDUM-199x14-analysis.md` - Revised for actual 199x14 constraints
- `QUICKSTART.md` - Original implementation guide (206x14+)
- `QUICKSTART-199x14.md` - Revised implementation guide for 199x14
- `comparison-table.md` - Layout options comparison
- `layout-mockups.txt` - ASCII art mockups (original)
- `layout-mockups-199x14.txt` - ASCII art mockups (199x14)
- `display-size-comparison.txt` - Side-by-side display size comparison
- `implementation-sketch.go` - Conceptual code examples
- `README.md` - Navigation and overview

### Total Research Size
**256KB** across **10 files**

---

**Document Version:** 1.0
**Last Updated:** 2026-02-07
