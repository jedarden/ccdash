# Worker Visualization Research for ccdash

This directory contains comprehensive research and implementation guidance for enhancing `ccdash` to better distinguish and visualize **worker sessions** (autonomous bead agents) from **interactive CLI sessions**.

## 📁 Directory Contents

| File | Description | When to Read |
|------|-------------|--------------|
| **[SUMMARY.md](./SUMMARY.md)** ⚡ | Quick summary (3-second takeaway) | **START HERE** for overview |
| **[QUICKSTART-199x14.md](./QUICKSTART-199x14.md)** ⭐ | Step-by-step guide for **199x14 displays** | **START HERE** for implementation |
| **[CHANGELOG.md](./CHANGELOG.md)** | What changed and why | Read for recent updates |
| **[ADDENDUM-199x14-analysis.md](./ADDENDUM-199x14-analysis.md)** | Analysis of 199x14 constraints | Read for why single-line format is needed |
| **[layout-mockups-199x14.txt](./layout-mockups-199x14.txt)** | ASCII mockups for 199x14 (interactive first) | Read for visual preview at current display |
| **[QUICKSTART.md](./QUICKSTART.md)** | Original guide (206x14+ displays) | Use for wider/taller displays |
| **[worker-visualization-research.md](./worker-visualization-research.md)** | Comprehensive research document | Read for full context and design rationale |
| **[comparison-table.md](./comparison-table.md)** | Detailed comparison of layout options | Read when deciding which layout to implement |
| **[layout-mockups.txt](./layout-mockups.txt)** | ASCII art visual mockups (original) | Read for visual preview of proposed layouts |
| **[display-size-comparison.txt](./display-size-comparison.txt)** | Side-by-side display comparison | Read to understand display constraints |
| **[implementation-sketch.go](./implementation-sketch.go)** | Conceptual code snippets | Reference during implementation |

---

## ⚡ Latest Update (2026-02-07)

**Interactive sessions now display FIRST, workers SECOND** (user requested)

**Why:** Interactive sessions are user-controlled and immediately actionable. Users naturally want to see "what am I doing?" before "what are workers doing?"

**See:** [CHANGELOG.md](./CHANGELOG.md) for details | [SUMMARY.md](./SUMMARY.md) for quick overview

---

## 🎯 Quick Overview

### The Problem

`ccdash` currently displays all tmux sessions uniformly:
```
🟢 alpha                        ACTIVE   1w  30s  📎
🟢 bravo                        WORKING  2w  45s  📎
🟢 claude-code-glm-47-alpha     WORKING  1w  2m
🔴 claude-code-glm-47-bravo     READY    2w  5m
🔴 delta                        READY    3w  10m  📎
```

**Issues:**
- ❌ Can't distinguish workers from interactive CLIs at a glance
- ❌ No context about what workers are doing (workspace, bead status)
- ❌ Mixed sorting makes it hard to assess worker health
- ❌ Scales poorly with 10+ workers

### The Solution

Enhanced visualization with grouping (Interactive first):

```
┌─ Interactive (2) ───────────────────────────────────┐
│                                                    │
│ 💻 alpha         🟡 ACTIVE    1w  30s  📎          │
│ 💻 delta         🔴 READY     3w  10m  📎          │
└────────────────────────────────────────────────────┘

┌─ Workers (3) ──────────────────────────────────────┐
│                                                    │
│ 🤖 claude-code-glm-47-alpha                        │
│    ~/prompts/kalshi-improvement                    │
│    🟢 WORKING  1w  2m  ⏳ 3/12 beads ready         │
│                                                    │
│ 🤖 claude-code-glm-47-bravo                        │
│    ~/ardenone-cluster/botburrow                    │
│    🔴 READY    2w  5m                              │
└────────────────────────────────────────────────────┘
```

**Benefits:**
- ✅ Clear visual distinction (🤖 vs 💻)
- ✅ Grouped by type for easy scanning
- ✅ Rich metadata (workspace paths, bead status)
- ✅ Scales well with many workers

---

## 🚀 Getting Started

### ⚠️ IMPORTANT: Check Your Display Size First

```bash
# Check your current display dimensions
tput cols  # Width in columns
tput lines # Height in rows
```

**Current ccdash display: 199x14** → Use **[QUICKSTART-199x14.md](./QUICKSTART-199x14.md)**

**If you have 200+ cols and 20+ rows** → Use **[QUICKSTART.md](./QUICKSTART.md)**

### For Implementers (Writing Code) at 199x14

1. **Read:** [ADDENDUM-199x14-analysis.md](./ADDENDUM-199x14-analysis.md) for constraint analysis
2. **Follow:** [QUICKSTART-199x14.md](./QUICKSTART-199x14.md) for step-by-step instructions
3. **Reference:** [implementation-sketch.go](./implementation-sketch.go) for code snippets
4. **Test:** Follow the testing checklist in QUICKSTART-199x14.md

**Estimated Time (199x14):**
- Phase 1 (Basic detection): 2-4 hours
- Phase 2 (Single-line grouped): 4-6 hours
- Phase 3 (Metadata): 4-8 hours
- **Total:** 10-18 hours

### For Decision Makers (Choosing an Approach)

1. **Read:** [worker-visualization-research.md](./worker-visualization-research.md) - Executive Summary section
2. **Review:** [comparison-table.md](./comparison-table.md) for pros/cons of each layout option
3. **Visualize:** [layout-mockups.txt](./layout-mockups.txt) for ASCII mockups
4. **Decide:** Recommendation matrix in comparison-table.md

**TL;DR:** **Option A (Unified Panel with Sections)** is recommended for most use cases.

### For Researchers (Understanding the Design)

1. **Start:** [worker-visualization-research.md](./worker-visualization-research.md) - full context
2. **Deep Dive:** All sections cover detection strategies, UI proposals, edge cases
3. **Alternatives:** "Alternative Approaches" section explores other solutions

---

## 📊 Implementation Phases (Revised for 199x14)

### Phase 1: Basic Worker Detection ⭐ **Start Here**
- Add `IsWorkerSession()` function
- Add 🤖/💻 icons to distinguish session types
- **Result:** Immediate visual distinction
- **Time:** 2-4 hours

### Phase 2: Single-Line Grouped Layout (REVISED for 199x14)
- Implement section-based layout (workers/interactive)
- Add section headers with counts
- **Single-line cells** with abbreviated names/status/workspace
- **Result:** Clear separation, fits in 11 rows
- **Time:** 4-6 hours *(reduced from 4-8)*

### Phase 3: Worker Metadata Integration (Simplified for 199x14)
- Update worker spawn script to write metadata files
- Read metadata in ccdash (workspace path, executor)
- Display **abbreviated** workspace paths in worker cells
- **Result:** Context about worker activity (abbreviated)
- **Time:** 4-8 hours *(reduced from 6-10)*

### Phase 4: Bead Status Integration (Future)
- Query `br stats` for each workspace
- Display ready/blocked/completed bead counts
- Cache results for performance
- **Result:** Real-time bead queue visibility
- **Time:** 8-12 hours

### Phase 5: Interactive Controls (Future)
- Keyboard shortcuts (`w`, `i`, `a` for filtering)
- Section collapse/expand
- Sorting options
- **Result:** Power-user workflow enhancements
- **Time:** 12-20 hours

---

## 🎨 Layout Options Summary

Three main approaches were evaluated:

### Option A: Unified Panel with Sections ⭐ **RECOMMENDED**
- Sections within existing tmux panel
- Works at 200+ cols terminal width
- Rich metadata display
- **Best for:** Most users (3-10 workers, ultra-wide terminals)

### Option B: Separate Panels
- Dedicated panel for workers, separate for interactive
- Requires 240+ cols
- Maximum separation
- **Best for:** Power users with ultra-wide monitors (240+ cols)

### Option C: Compact Summary
- Collapsed by default, expandable on demand
- Works at 160+ cols
- Progressive disclosure
- **Best for:** Narrow terminals or many workers (15+)

**See [comparison-table.md](./comparison-table.md) for detailed analysis.**

---

## 🧪 Testing

### Quick Smoke Test

```bash
# 1. Spawn test workers
cd /home/coder/claude-config
./scripts/spawn-workers.sh --workspace=/tmp/test-workspace --workers=2 --executor=claude-code-glm-47

# 2. Build and run ccdash
cd /home/coder/ccdash
go build -o ccdash cmd/ccdash/main.go
./ccdash

# 3. Verify:
# - Workers show 🤖 icon
# - Interactive sessions show 💻 icon
# - Sections are grouped
# - Workspace paths display (if Phase 3 complete)
```

### Full Testing Checklist

See [QUICKSTART.md](./QUICKSTART.md) - "Testing Checklist" section.

---

## 📖 Key Concepts

### What is a Worker Session?

A **worker session** is a tmux session running an autonomous bead agent:
- **Purpose:** Process tasks (beads) from a workspace queue
- **Naming:** Follows pattern `<executor>-<nato-callsign>`
  - Example: `claude-code-glm-47-alpha`, `opencode-glm-47-bravo`
- **Behavior:** Runs in background (usually detached), logs to `~/.beads-workers/`
- **Launched via:** `/home/coder/claude-config/scripts/spawn-workers.sh`

### What is an Interactive Session?

An **interactive session** is a tmux session with user-attached Claude CLI:
- **Purpose:** Interactive terminal for user commands
- **Naming:** Simple NATO callsigns
  - Example: `alpha`, `bravo`, `charlie`, `delta`
- **Behavior:** User-attached, manual command execution
- **Launched via:** Direct tmux or `./agents/*/launch.sh` scripts

### How Detection Works

**Primary Method:** Pattern matching on session name
```go
executors := []string{
    "claude-code-glm-47-",
    "claude-code-sonnet-",
    "opencode-glm-47-",
}
// If name starts with any executor prefix → worker
```

**Fallback Method:** Check for worker log file
```go
logPath := "~/.beads-workers/<session-name>.log"
// If log file exists → worker
```

---

## 🔗 Related Resources

### ccdash Source Code
- Main repo: `/home/coder/ccdash/`
- Metrics: `/home/coder/ccdash/internal/metrics/tmux.go`
- UI: `/home/coder/ccdash/internal/ui/dashboard.go`

### Worker Infrastructure
- Spawn script: `/home/coder/claude-config/scripts/spawn-workers.sh`
- Worker script: `/home/coder/claude-config/scripts/bead-worker.sh`
- Logs: `/home/coder/.beads-workers/*.log`
- Metadata (future): `/home/coder/.beads-workers/metadata/*.json`

### Previous Research
- Layout analysis: `/home/coder/research/beads/ccdash-layout-analysis.md`
  - Background: Analysis of ccdash layout spacing and truncation issues
  - Recommended increasing `minCellWidth` from 28 to 40

---

## 💡 Design Rationale

### Why Separate Workers from Interactive?

1. **Different Use Cases:**
   - Workers: Autonomous background agents (monitor health, bead progress)
   - Interactive: User-driven terminals (check status, attached indicator)

2. **Different Information Needs:**
   - Workers: Need context (workspace, bead queue)
   - Interactive: Need minimal info (status, attached state)

3. **Scale Differently:**
   - Workers: Can scale to 10+ (need efficient layout)
   - Interactive: Typically 2-5 (simple display works)

4. **Different Visual Priority:**
   - Workers: Need at-a-glance health assessment (are agents stuck?)
   - Interactive: Less critical (user is often attached)

### Why Option A (Sections) is Recommended

- **Balance:** Clarity without sacrificing space
- **Familiarity:** Preserves existing 3-panel layout
- **Scalability:** Sections can expand/collapse in future
- **Accessibility:** Works at common ultra-wide width (200+ cols)
- **Information Density:** Rich metadata visible by default

---

## 🐛 Known Issues & Edge Cases

### Edge Case: Non-Standard Worker Names

**Issue:** Workers launched manually without standard naming won't be detected.

**Solution:** Fallback detection via log file check.

### Edge Case: Metadata Missing/Stale

**Issue:** Metadata file may not exist for old workers or if spawn script not updated.

**Solution:** Graceful degradation - display worker icon but no workspace path.

### Edge Case: 20+ Workers

**Issue:** Section becomes very long, hard to scan.

**Solutions:**
- Multi-column layout within section
- Scrollable section (Phase 5)
- Auto-switch to Option C (compact summary)

### Performance: Metadata File Reads

**Issue:** Reading 10 metadata files every 2s refresh might be slow.

**Mitigation:**
- Metadata reads are <1ms per file
- Total overhead: <10ms for 10 workers (negligible)
- If issue arises, add in-memory caching

---

## 📝 Future Enhancements

Beyond the 5 phases outlined:

1. **Worker Action Commands**
   - Press `k` to kill selected worker
   - Press `r` to restart worker
   - Press `l` to tail worker logs

2. **Bead Queue Visualization**
   - Show dependency graph for blocked beads
   - Click bead ID to see full description
   - Color-code beads by priority

3. **Multi-Workspace Dashboard**
   - Group workers by workspace
   - Show aggregate stats per workspace
   - Navigate between workspaces

4. **Historical Metrics**
   - Track worker uptime and throughput
   - Show beads completed per hour
   - Alert on stalled workers (no progress in 1hr)

5. **Integration with Other Tools**
   - Export metrics to Prometheus
   - Send alerts to Slack when worker errors
   - API endpoint for external dashboards

---

## 🤝 Contributing

If you implement enhancements or find issues:

1. **Document Changes:**
   - Update this README with new findings
   - Add notes to relevant research files

2. **Share Feedback:**
   - What worked well?
   - What was confusing?
   - What would you do differently?

3. **Extend Research:**
   - Add new layout mockups
   - Benchmark performance
   - Test on different terminal sizes

---

## 📅 Research Timeline

- **Date Created:** 2026-02-07
- **Research Scope:** Worker visualization in ccdash
- **Estimated Implementation:** 2-4 weeks (Phases 1-3)
- **Status:** Research complete, implementation pending

---

## 🏁 Conclusion

This research provides a clear path to enhance ccdash with worker-aware visualization. Starting with basic detection (Phase 1) and progressing through visual grouping (Phase 2) and metadata integration (Phase 3) will significantly improve the user experience for teams running multiple worker agents.

**Next Steps:**
1. Review QUICKSTART.md
2. Start with Phase 1 (basic detection)
3. Test with real worker sessions
4. Iterate and gather feedback

---

**For Questions or Clarifications:**
- Re-read the comprehensive research: [worker-visualization-research.md](./worker-visualization-research.md)
- Check the implementation sketch: [implementation-sketch.go](./implementation-sketch.go)
- Review layout options: [comparison-table.md](./comparison-table.md)

**Document Version:** 1.0
**Last Updated:** 2026-02-07
