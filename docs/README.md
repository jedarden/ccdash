# ccdash Documentation

This directory contains comprehensive documentation for ccdash development and architecture.

## Directory Structure

```
docs/
├── adrs/           # Architecture Decision Records
│   ├── README.md
│   ├── 0001-distinguish-worker-from-interactive-sessions.md
│   ├── 0002-display-interactive-sessions-before-workers.md
│   ├── 0003-use-single-line-format-for-199x14-display.md
│   └── 0004-use-section-based-grouping-over-alternatives.md
│
└── notes/          # Research notes and implementation guides
    ├── README.md
    ├── SUMMARY.md
    ├── QUICKSTART-199x14.md
    ├── worker-visualization-research.md
    └── ... (additional research files)
```

## Quick Navigation

### For Developers

**Implementing worker visualization feature?**
→ Start here: [`notes/QUICKSTART-199x14.md`](./notes/QUICKSTART-199x14.md)

**Understanding architectural decisions?**
→ Read: [`adrs/README.md`](./adrs/README.md)

**Need quick overview?**
→ Read: [`notes/SUMMARY.md`](./notes/SUMMARY.md)

### For Decision Makers

**Want to understand design choices?**
→ Read: [`adrs/`](./adrs/) directory

**Need comprehensive research?**
→ Read: [`notes/worker-visualization-research.md`](./notes/worker-visualization-research.md)

**Comparing layout options?**
→ Read: [`notes/comparison-table.md`](./notes/comparison-table.md)

## Current Focus: Worker Visualization

### What is it?

An enhancement to ccdash that visually distinguishes **worker sessions** (autonomous bead agents) from **interactive CLI sessions** (user-attached terminals).

### Key Features

- 🤖 **Icon-based distinction**: Workers get robot icon, interactive get computer icon
- 📊 **Section grouping**: Interactive and workers in separate labeled sections
- ⚡ **Interactive first**: User sessions displayed before background workers
- 📏 **Adaptive layout**: Single-line format for constrained displays (199x14)
- 📍 **Rich metadata**: Workspace paths and context for workers

### Current Display

**Before:**
```
🟢 alpha                        ACTIVE   1w  30s  📎
🟢 claude-code-glm-47-alpha     WORKING  1w  2m
🔴 claude-code-glm-47-bravo     READY    2w  5m
🟡 delta                        ACTIVE   3w  10m  📎
```
*All sessions mixed, no distinction*

**After:**
```
Interactive (2)
💻 alpha   🟡 ACT   30s  📎
💻 delta   🟡 ACT   10m  📎

Workers (2)
🤖 c-glm-alpha  🟢 WORK  2m  ~/kalshi
🤖 c-glm-bravo  🔴 READY 5m  ~/botburrow
```
*Clear grouping, rich context*

## Documentation Overview

### Architecture Decision Records (ADRs)

Documents key architectural decisions with context and consequences.

**Current ADRs:**
- **ADR 0001**: Why distinguish workers from interactive
- **ADR 0002**: Why display interactive before workers
- **ADR 0003**: Why use single-line format for 199x14
- **ADR 0004**: Why use section-based grouping

**Read:** [`adrs/README.md`](./adrs/README.md)

### Research Notes

Comprehensive research, implementation guides, and analysis.

**Key Documents:**
- **QUICKSTART-199x14.md**: Step-by-step implementation guide
- **worker-visualization-research.md**: Original comprehensive research
- **ADDENDUM-199x14-analysis.md**: Display constraint analysis
- **comparison-table.md**: Layout options comparison
- **layout-mockups-199x14.txt**: Visual ASCII mockups

**Read:** [`notes/README.md`](./notes/README.md)

## Implementation Status

### Completed
- ✅ Research and analysis
- ✅ Architecture decisions documented
- ✅ Implementation guide written
- ✅ Visual mockups created

### In Progress
- 🔄 Phase 1: Basic worker detection
- 🔄 Phase 2: Section-based grouping
- 🔄 Phase 3: Metadata integration

### Planned
- 📋 Phase 4: Bead status integration
- 📋 Phase 5: Interactive controls and expandable views

## Getting Started

### For First-Time Contributors

1. **Read the summary**: [`notes/SUMMARY.md`](./notes/SUMMARY.md) (3 minutes)
2. **Review ADRs**: [`adrs/README.md`](./adrs/README.md) (10 minutes)
3. **Study implementation guide**: [`notes/QUICKSTART-199x14.md`](./notes/QUICKSTART-199x14.md) (30 minutes)
4. **Start coding**: Follow Phase 1 step-by-step

### For Reviewers

1. **Check ADRs**: Understand architectural rationale
2. **Review mockups**: [`notes/layout-mockups-199x14.txt`](./notes/layout-mockups-199x14.txt)
3. **Verify constraints**: [`notes/ADDENDUM-199x14-analysis.md`](./notes/ADDENDUM-199x14-analysis.md)

## Contributing to Documentation

### Adding Research Notes

Place new research documents in `docs/notes/`:
```bash
# Add research document
cp your-research.md /home/coder/ccdash/docs/notes/

# Update notes/README.md to reference it
```

### Adding ADRs

1. Create new ADR in `docs/adrs/`:
```bash
# Use next sequential number
vim /home/coder/ccdash/docs/adrs/0005-your-decision.md
```

2. Follow ADR template structure:
   - Status
   - Context
   - Decision
   - Consequences

3. Update `docs/adrs/README.md` index

### Documentation Standards

- **Clarity**: Write for future developers unfamiliar with context
- **Completeness**: Include rationale, alternatives, and consequences
- **Traceability**: Link related documents and ADRs
- **Maintenance**: Keep documentation in sync with code changes

## Tools and Resources

### Viewing Markdown

```bash
# In terminal
cat docs/adrs/0001-distinguish-worker-from-interactive-sessions.md

# With markdown viewer
glow docs/adrs/0001-distinguish-worker-from-interactive-sessions.md

# In browser (if using DevPod with port forwarding)
# Use markdown preview in VS Code or similar
```

### Searching Documentation

```bash
# Find ADRs mentioning "worker"
grep -r "worker" docs/adrs/

# Find implementation guides
find docs/notes/ -name "*QUICKSTART*"

# Search all docs
grep -r "single-line format" docs/
```

## Questions?

- **General questions**: Review [`notes/README.md`](./notes/README.md)
- **Architectural questions**: Review [`adrs/README.md`](./adrs/README.md)
- **Implementation questions**: Review [`notes/QUICKSTART-199x14.md`](./notes/QUICKSTART-199x14.md)

## Changelog

### 2026-02-07
- Created documentation structure
- Added ADRs 0001-0004 for worker visualization
- Copied research notes from `/home/coder/research/worker-tui/`
- Established documentation standards

---

**Last Updated:** 2026-02-07
