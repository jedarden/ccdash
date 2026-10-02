# ADDENDUM: Layout Analysis for 199x14 Display

## Critical Constraint Analysis

**Actual Display:** 199x14 (199 cols × 14 rows)

### Width Analysis (199 cols)

```
Total width: 199 cols
Panel padding: -6 cols
Available: 193 cols

Current 3-panel allocation:
- System: 60 cols
- Token: 60 cols
- Tmux: 73 cols (193 - 60 - 60)
  └─ Content: 69 cols (73 - 4 for borders/padding)
```

**vs. Previous 206x14 assumption:**
- Had 80 cols for Tmux panel → 76 cols content
- **Lost 7 cols of content width** (76 → 69)

### Height Analysis (14 rows)

```
Total height: 14 rows
Header/footer: -3 rows
Panel height: 11 rows

With multi-line worker cells (3 lines each):
- Header: 1 row ("Workers (N)")
- Separator: 1 row
- Worker 1: 3 rows
- Worker 2: 3 rows
- Worker 3: 3 rows
- Total: 11 rows → Maxes out with just 3 workers!
```

**Critical Issue:** Option A (multi-line worker cells) doesn't scale at 14 row height.

---

## Revised Space Calculation at 199x14

### Step-by-step Panel Width Distribution

From ccdash layout code:

```go
totalPanelWidth := 199 - 6 = 193 cols

systemWidth := 60 cols  // Fixed for displays >= 180 cols

availableWidth := 193 - 60 = 133 cols  // For Token + Tmux

// Minimum widths
minTokenWidth := 46 cols
minTmuxWidth := 50 cols  // With minCellWidth = 28
remainingAfterMins := 133 - 46 - 50 = 37 cols extra

// Ideal widths
idealTokenWidth := 60 cols
idealTmuxWidth := 39 cols (single column layout)

// Calculate wants
tokenWant := 60 - 46 = 14
tmuxWant := 39 - 50 = -11 → 0 (clamped)
totalWant := 14

// Proportional allocation
tokenExtra := 37 * 14 / 14 = 37
tmuxExtra := 0
tokenWidth := 46 + 37 = 83
tmuxWidth := 50 + 0 = 50

// Cap token panel
excess := 83 - 60 = 23
tokenWidth := 60
tmuxWidth := 50 + 23 = 73 cols
```

**Final Layout at 199x14:**
| Panel | Width |
|-------|-------|
| System | 60 cols |
| Token | 60 cols |
| Tmux | 73 cols |
| **Total** | **193 cols** |

**Tmux Content Width:** 73 - 4 = **69 cols**

---

## Impact on Worker Visualization

### Cell Width Constraints

**Single column layout (best case):**
- Cell width: 69 cols
- Fixed overhead: ~20 chars (status, windows, idle, emoji)
- Max name length: 69 - 20 = **49 chars** ✅ (sufficient)

**Multi-column layout (if many sessions):**
- 2 columns: cellWidth = (69 - 1) / 2 = 34 cols
- Max name length: 34 - 20 = **14 chars** ❌ (truncates worker names!)

**Problem:** With increased minCellWidth recommendation (28→40), 2 columns won't fit:
- Required width for 2 cols: 40 * 2 + 1 = 81 cols
- Available: 69 cols
- **2 columns not possible with minCellWidth=40**

---

## Revised Recommendations for 199x14

### Option A: Single-Line Compact (REVISED RECOMMENDATION)

Given the **11 row** constraint, multi-line worker cells don't scale. Pivot to single-line format:

```
╔═══════════════════════════════════════════════════════════════════════════╗
║ System (60)      │ Token (60)      │ Tmux Sessions (73)                  ║
╠═══════════════════════════════════════════════════════════════════════════╣
║ CPU:  45.2%      │ Requests: 1,247 │ Workers (3)                         ║
║ Mem:  76.8%      │ Input:    2.4M  │ 🤖 c-glm-alpha     🟢 WORK 2m ~/k  ║
║ Disk: 32.1%      │ Output:   890K  │ 🤖 c-glm-bravo     🔴 READY 5m ~/a ║
║ I/O:  R:12.3 W:8 │ Cost:    $24.56 │ 🤖 o-glm-charlie   🟡 ACT 1m ~/t   ║
║ Net:  Rx:2.1 Tx:1│                 │                                     ║
║ Load: 2.45       │ Rate Limits:    │ Interactive (2)                     ║
║ Temp: CPU:58°C   │  Req: 51.96%    │ 💻 alpha  🟡 ACTIVE 30s 📎          ║
║ Uptime: 3d 14h   │  In:  48.00%    │ 💻 delta  🔴 READY 10m 📎           ║
║                  │  Out: 17.80%    │                                     ║
╚═══════════════════════════════════════════════════════════════════════════╝
```

**Format per line:**
```
[icon] [abbrev-name] [status-emoji] [status] [idle] [abbrev-workspace]
🤖 c-glm-alpha  🟢 WORK 2m ~/kalshi
```

**Name Abbreviation Strategy:**
- `claude-code-glm-47-alpha` → `c-glm-alpha` (save 17 chars)
- `opencode-glm-47-bravo` → `o-glm-bravo` (save 13 chars)
- `claude-code-sonnet-charlie` → `c-sonnet-charlie` (save 12 chars)

**Status Abbreviation:**
- `WORKING` → `WORK`
- `READY` → `READY` (keep)
- `ACTIVE` → `ACT`
- `ERROR` → `ERR`

**Workspace Abbreviation:**
- Show just last path segment: `/home/coder/prompts/kalshi-improvement` → `~/kalshi`
- Or: First letter + ellipsis: `~/k...`

**Layout:**
- Line 1: Section header "Interactive (N)"
- Lines 2-3: Interactive sessions (single line each)
- Line 4: Empty separator
- Line 5: Section header "Workers (N)"
- Lines 6-8: Worker sessions (single line each)
- Lines 9-11: Empty/future use

**Pros:**
- ✅ Fits in 11 rows even with 3 workers + 2 interactive
- ✅ Still shows worker distinction (🤖 vs 💻)
- ✅ Includes abbreviated workspace context
- ✅ Clear grouping with section headers

**Cons:**
- ⚠️ Name abbreviation may be unclear initially
- ⚠️ Limited workspace context (just last dir)
- ⚠️ No bead status (not enough space)

---

### Option B: Tooltip/Expandable Detail (NEW)

**Compact display + on-demand expansion:**

```
╔═══════════════════════════════════════════════════════════════════════════╗
║ Tmux Sessions (73 cols)                                                   ║
╠═══════════════════════════════════════════════════════════════════════════╣
║ Workers (3): 🤖🤖🤖  [2 WORKING, 1 READY]  → Press 'w' for details       ║
║                                                                           ║
║ Interactive (2): 💻💻  [2 ACTIVE, both attached]                          ║
║                                                                           ║
║ Sessions: alpha*, delta*, c-glm-alpha, c-glm-bravo, o-glm-charlie        ║
║           (* = attached)                                                  ║
╚═══════════════════════════════════════════════════════════════════════════╝
```

**On 'w' key press → Full worker detail view:**

```
╔═══════════════════════════════════════════════════════════════════════════╗
║ Worker Details (Press 'q' to return)                                     ║
╠═══════════════════════════════════════════════════════════════════════════╣
║ 🤖 claude-code-glm-47-alpha                                               ║
║    Workspace: ~/prompts/kalshi-improvement                                ║
║    Status: 🟢 WORKING  1w  2m                                             ║
║    Beads: 3 ready / 2 blocked / 7 done (12 total)                        ║
║                                                                           ║
║ 🤖 claude-code-glm-47-bravo                                               ║
║    Workspace: ~/ardenone-cluster/botburrow                                ║
║    Status: 🔴 READY  2w  5m                                               ║
║                                                                           ║
║ 🤖 opencode-glm-47-charlie                                                ║
║    Workspace: ~/trading/backtest-engine                                   ║
║    Status: 🟡 ACTIVE  1w  1m                                              ║
║    Beads: 5 ready / 0 blocked / 8 done (13 total)                        ║
╚═══════════════════════════════════════════════════════════════════════════╝
```

**Pros:**
- ✅ Fits in 11 rows easily
- ✅ Rich detail available on demand
- ✅ Scalable (handles 10+ workers in summary)
- ✅ Progressive disclosure pattern

**Cons:**
- ⚠️ Requires keyboard interaction
- ⚠️ Less info visible by default
- ⚠️ Higher implementation complexity

---

### Option C: Hybrid - Icons + Hover Line

**Single list with enhanced icons, selected item shows detail:**

```
╔═══════════════════════════════════════════════════════════════════════════╗
║ Tmux Sessions (5: 3 workers, 2 interactive)                              ║
╠═══════════════════════════════════════════════════════════════════════════╣
║ 🤖 c-glm-alpha      🟢 WORK  2m  ~/kalshi                                ║
║ 🤖 c-glm-bravo      🔴 READY 5m  ~/botburrow                             ║
║ 🤖 o-glm-charlie    🟡 ACT   1m  ~/backtest                              ║
║ 💻 alpha            🟡 ACT  30s  📎                                       ║
║ 💻 delta            🔴 READY 10m 📎                                       ║
║                                                                           ║
║ ─────────────────────────────────────────────────────────────────────────║
║ Hover: c-glm-alpha → claude-code-glm-47-alpha                            ║
║        /home/coder/prompts/kalshi-improvement                             ║
╚═══════════════════════════════════════════════════════════════════════════╝
```

**Pros:**
- ✅ Fits in 11 rows
- ✅ Shows all sessions at once
- ✅ Detail line shows full context for selected item
- ✅ Icons provide visual grouping

**Cons:**
- ⚠️ No explicit grouping (just icon distinction)
- ⚠️ Requires navigation/selection
- ⚠️ Detail line changes rapidly with arrow keys

---

## Updated Comparison Matrix for 199x14

| Criterion | Option A (Compact) | Option B (Expandable) | Option C (Hybrid) |
|-----------|--------------------|-----------------------|-------------------|
| **Fits in 11 rows** | ✅ Yes (up to 5-6 sessions) | ✅ Yes (unlimited) | ✅ Yes (up to 7 sessions) |
| **Worker distinction** | ✅ Clear (icon + section) | ⚠️ Moderate (summary) | ⚠️ Moderate (icon only) |
| **Workspace visibility** | ⚠️ Abbreviated only | ✅ Full (on demand) | ⚠️ Abbreviated + detail line |
| **Bead status** | ❌ No space | ✅ Yes (on demand) | ❌ No space |
| **Implementation** | ⭐⭐ Low | ⭐⭐⭐⭐ High | ⭐⭐⭐ Moderate |
| **User interaction** | None required | 'w' key to expand | Arrow keys to select |
| **Scalability** | ⚠️ 6 sessions max | ✅ Unlimited | ⚠️ 8 sessions max |
| **Info density** | ⭐⭐⭐⭐ Good | ⭐⭐⭐ Moderate | ⭐⭐⭐⭐ Good |

---

## REVISED RECOMMENDATION for 199x14

### Primary: **Option A (Single-Line Compact with Sections)**

**Rationale:**
1. **Height constraint is critical** - 11 rows can't fit multi-line cells
2. **Width is workable** - 69 cols sufficient for abbreviated display
3. **Low complexity** - Similar implementation to original Option A, just single-line cells
4. **Clear grouping** - Section headers still provide organization
5. **No interaction required** - All info visible at once

**Trade-offs accepted:**
- Abbreviated names (but still recognizable with pattern)
- Abbreviated workspace paths (last directory segment)
- No bead status (defer to Phase 4 with expandable view)

### Secondary: **Option B (Expandable Detail)** for 10+ workers

If users regularly run 10+ workers, Option B's summary mode scales better.

---

## Implementation Adjustments for 199x14

### Phase 1: Basic Detection (Unchanged)
- Add 🤖/💻 icons
- Detect worker vs interactive
- **Time:** 2-4 hours

### Phase 2: Single-Line Grouped Layout (REVISED)

**Changes from original Phase 2:**

1. **Single-line cell format:**
```go
func (d *Dashboard) renderWorkerCell(worker metrics.TmuxSession, width int) string {
    icon := "🤖"
    statusEmoji := worker.Status.GetEmoji()

    // Abbreviate executor name
    name := abbreviateWorkerName(worker.Name)

    // Abbreviate status
    status := abbreviateStatus(worker.Status)

    // Format idle
    idle := formatDuration(worker.IdleDuration)

    // Abbreviate workspace (if metadata available)
    workspace := ""
    if worker.WorkerMetadata != nil {
        workspace = abbreviateWorkspace(worker.WorkerMetadata.Workspace)
    }

    // Single line format
    line := fmt.Sprintf("%s %s %s %-5s %4s %s",
        icon, name, statusEmoji, status, idle, workspace)

    return line
}
```

2. **Abbreviation helpers:**
```go
func abbreviateWorkerName(name string) string {
    // claude-code-glm-47-alpha → c-glm-alpha
    name = strings.Replace(name, "claude-code-", "c-", 1)
    name = strings.Replace(name, "opencode-", "o-", 1)
    name = strings.Replace(name, "-47", "", 1) // Remove version
    return name
}

func abbreviateStatus(status metrics.SessionStatus) string {
    switch status {
    case metrics.StatusWorking:
        return "WORK"
    case metrics.StatusActive:
        return "ACT"
    case metrics.StatusReady:
        return "READY"
    case metrics.StatusError:
        return "ERR"
    }
    return string(status)
}

func abbreviateWorkspace(path string) string {
    // /home/coder/prompts/kalshi-improvement → ~/kalshi-i...
    if strings.HasPrefix(path, os.Getenv("HOME")) {
        path = "~" + strings.TrimPrefix(path, os.Getenv("HOME"))
    }

    // Take last segment
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

3. **Section headers remain:**
```go
func (d *Dashboard) renderWorkerSection(workers []metrics.TmuxSession, width int) string {
    header := fmt.Sprintf("Workers (%d)", len(workers))
    // ... render each worker as single line ...
}
```

**Time:** 4-6 hours (reduced from 4-8 due to simpler single-line format)

### Phase 3: Metadata Integration (Simplified)

- Still create metadata files
- Still read workspace paths
- Display abbreviated in main view
- **Future:** Full path in expandable detail mode (Phase 5)

**Time:** 4-8 hours (reduced from 6-10)

### Phase 4: Bead Status (Deferred to Expandable Mode)

Skip bead status in default view. Implement in Phase 5 expandable detail view only.

### Phase 5: Expandable Detail View (NEW)

Add keyboard shortcut to show full detail:
- Press 'w' → Switch to worker detail view
- Press 'q' → Return to summary view
- Full names, full paths, bead status visible in detail view

**Time:** 8-12 hours

---

## Updated Space Allocation Recommendations

### Current minCellWidth Analysis

At 199x14 with 69 cols content:

**Single column:**
- Cell width: 69 cols ✅
- Can show: `🤖 c-glm-alpha  🟢 WORK  2m  ~/kalshi-imp` (47 chars)

**Two columns (if needed for many sessions):**
- With minCellWidth=28: 28*2+1 = 57 cols ✅ Fits
- With minCellWidth=40: 40*2+1 = 81 cols ❌ Doesn't fit

**Recommendation:** Keep minCellWidth=28 for 199x14 displays, use abbreviations.

**For wider displays (240+):** Increase minCellWidth to 40 as originally planned.

---

## Visual Mockup: Revised Option A at 199x14

```
╔═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╗
║ Claude Code Dashboard                                                                                    v0.4.0 │ Last update: 2s ago │ [h] help [w] workers [q] quit         ║
╠═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╣
║  ┌──────────────────────────────────────────────────┬──────────────────────────────────────────────────┬───────────────────────────────────────────────────────────────────────────┐   ║
║  │ System Metrics                                   │ Token Usage (Monday 9am)                        │ Tmux Sessions (5: 3 workers, 2 interactive)                               │   ║
║  ├──────────────────────────────────────────────────┼──────────────────────────────────────────────────┼───────────────────────────────────────────────────────────────────────────┤   ║
║  │                                                  │                                                  │                                                                           │   ║
║  │ CPU:  ████████████░░░░  45.2%                   │ Requests: 1,247                                  │ Workers (3)                                                               │   ║
║  │ Mem:  ███████████████████░  76.8%               │ Input:    2.4M                                   │ 🤖 c-glm-alpha    🟢 WORK   2m  ~/kalshi                                  │   ║
║  │ Disk: ████████░░░░░░░░░░  32.1%                 │ Output:   890K                                   │ 🤖 c-glm-bravo    🔴 READY  5m  ~/botburrow                               │   ║
║  │ I/O:  R:12.3 W:8.7                              │ Cache:    8.7M/1.2M                              │ 🤖 o-glm-charlie  🟡 ACT    1m  ~/backtest                                │   ║
║  │ Net:  Rx:2.1 Tx:1.8                             │ Cost:    $24.56                                  │                                                                           │   ║
║  │ Load: 2.45  2.12  1.98                          │                                                  │ Interactive (2)                                                           │   ║
║  │ Temp: CPU:58°C GPU:N/A                          │ Rate Limits (5hr):                               │ 💻 alpha          🟡 ACT   30s  📎                                        │   ║
║  │ Uptime: 3d 14h 22m                              │  Req: ██████████████░░  51.96%                  │ 💻 delta          🔴 READY 10m  📎                                        │   ║
║  │                                                  │  In:  ██████████░░░░░░  48.00%                  │                                                                           │   ║
║  └──────────────────────────────────────────────────┴──────────────────────────────────────────────────┴───────────────────────────────────────────────────────────────────────────┘   ║
╚═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╝
```

**Key changes for 199x14:**
- ✅ Single-line worker entries (fits in 11 rows)
- ✅ Abbreviated names: `c-glm-alpha` instead of `claude-code-glm-47-alpha`
- ✅ Abbreviated status: `WORK`, `ACT`, `READY`
- ✅ Abbreviated workspace: `~/kalshi` instead of full path
- ✅ Still grouped by type (workers/interactive)
- ✅ Section headers with counts
- ✅ Icons distinguish types (🤖 vs 💻)

---

## Updated Testing for 199x14

### Terminal Size Verification

```bash
# Verify current size
tput cols  # Should show 199
tput lines # Should show 14

# Test ccdash
cd /home/coder/ccdash
./ccdash
```

**What to verify:**
- [ ] All 3 panels visible (System, Token, Tmux)
- [ ] Tmux panel shows ~73 cols width
- [ ] Worker names abbreviated correctly
- [ ] All content fits in 11 rows (no scrolling)
- [ ] Section headers visible
- [ ] Can see 3 workers + 2 interactive without truncation

---

## Conclusion for 199x14

**The 14-row height is the binding constraint**, not the width. This eliminates multi-line worker cells from consideration.

**Revised implementation path:**
1. **Phase 1:** Basic detection with icons (unchanged)
2. **Phase 2:** Single-line grouped layout with abbreviations (4-6h)
3. **Phase 3:** Metadata integration with abbreviated display (4-8h)
4. **Phase 5:** Expandable detail view for full info (8-12h, optional)

**Total:** 10-18 hours for Phases 1-3 (down from 12-22h)

**Key Abbreviations:**
- Names: `claude-code-glm-47-alpha` → `c-glm-alpha`
- Status: `WORKING` → `WORK`, `ACTIVE` → `ACT`
- Workspace: `/home/coder/prompts/kalshi-improvement` → `~/kalshi`

This approach provides clear worker distinction while respecting the tight vertical space constraint.

---

**Document Version:** 1.0 (Addendum)
**Date:** 2026-02-07
**Display:** 199x14 (199 cols × 14 rows)
**Supersedes:** Original Option A (multi-line) recommendation
