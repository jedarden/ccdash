# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for ccdash, documenting significant architectural and design decisions.

## What is an ADR?

An Architecture Decision Record (ADR) is a document that captures an important architectural decision made along with its context and consequences.

## Format

Each ADR follows this structure:
- **Status**: Proposed, Accepted, Deprecated, Superseded
- **Context**: The issue motivating this decision
- **Decision**: The change being proposed or made
- **Consequences**: The resulting context after applying the decision

## Index of ADRs

### Active

| ADR | Title | Date | Status |
|-----|-------|------|--------|
| [0001](./0001-distinguish-worker-from-interactive-sessions.md) | Distinguish Worker from Interactive Sessions | 2026-02-07 | Accepted |
| [0002](./0002-display-interactive-sessions-before-workers.md) | Display Interactive Sessions Before Workers | 2026-02-07 | Accepted |
| [0003](./0003-use-single-line-format-for-199x14-display.md) | Use Single-Line Format for 199x14 Display | 2026-02-07 | Accepted |
| [0004](./0004-use-section-based-grouping-over-alternatives.md) | Use Section-Based Grouping Over Alternatives | 2026-02-07 | Accepted |
| [0005](./0005-support-concurrent-multi-user-access.md) | Support Concurrent Multi-User Access | 2026-02-07 | Accepted |

## ADR Summary

### Worker Visualization Feature (ADRs 0001-0005)

A comprehensive enhancement to distinguish and better visualize worker sessions (autonomous bead agents) from interactive CLI sessions, with full multi-user concurrent access support.

**Key Decisions:**
1. **Distinction Method**: Icon + section grouping (🤖 for workers, 💻 for interactive)
2. **Display Order**: Interactive first, workers second (user-focused priority)
3. **Layout Format**: Single-line compact format for 199x14 display constraints
4. **Grouping Strategy**: Section-based within unified tmux panel
5. **Multi-User Support**: Fully independent per-user operation, no shared state

**Constraint:** Actual display is 199x14 (not 206x30 as initially assumed)
- 14-row height is binding constraint
- Single-line format required to fit 5+ sessions

**Implementation Timeline:**
- Phase 1: Basic detection (2-4h)
- Phase 2: Section grouping (4-6h)
- Phase 3: Metadata integration (4-8h)
- **Total**: 10-18 hours

**Documentation:**
- Implementation guide: `docs/notes/QUICKSTART-199x14.md`
- Research: `docs/notes/worker-visualization-research.md`
- Display analysis: `docs/notes/ADDENDUM-199x14-analysis.md`

## Creating New ADRs

When making significant architectural decisions:

1. **Create new ADR file**: `NNNN-title-in-kebab-case.md`
2. **Use next number**: Increment from latest ADR
3. **Follow template**: Copy structure from existing ADRs
4. **Update this README**: Add to index table

### ADR Numbering

- ADRs are numbered sequentially: 0001, 0002, 0003, ...
- Numbers are never reused
- Deprecated ADRs remain in the index with updated status

### When to Create an ADR

Create an ADR for decisions that:
- Impact architecture or design significantly
- Are difficult or expensive to reverse
- Affect multiple components or subsystems
- Require justification or explanation
- Set precedent for future decisions

### When NOT to Create an ADR

Don't create ADRs for:
- Trivial implementation details
- Temporary workarounds
- Routine refactoring
- Bug fixes (unless they reveal architectural issues)

## References

- [ADR Template](https://github.com/joelparkerhenderson/architecture-decision-record)
- [When to Write ADRs](https://adr.github.io/)
