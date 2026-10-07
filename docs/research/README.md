# Research archive

This directory contains historical research and design material for features
that are now implemented. It is useful for understanding the constraints and
trade-offs behind the current behavior, but it is not an implementation
backlog. For the maintained description of the running system, read
[`../architecture.md`](../architecture.md).

## Worker and session visualization

The worker-visualization feature shipped in v1.1.20. These documents preserve
the research that led to worker/interactive classification, interactive-first
ordering, section layout, and compact rows:

- [`worker-visualization-research.md`](worker-visualization-research.md) — full
  investigation and alternatives.
- [`comparison-table.md`](comparison-table.md) — layout alternatives.
- [`ADDENDUM-199x14-analysis.md`](ADDENDUM-199x14-analysis.md) — constrained
  terminal analysis.
- [`QUICKSTART-199x14.md`](QUICKSTART-199x14.md) and [`QUICKSTART.md`](QUICKSTART.md)
  — historical implementation guides.
- [`SUMMARY.md`](SUMMARY.md) — short historical summary.
- [`layout-mockups-199x14.txt`](layout-mockups-199x14.txt),
  [`layout-mockups.txt`](layout-mockups.txt), and
  [`display-size-comparison.txt`](display-size-comparison.txt) — visual
  exploration.

## Other shipped-feature research

- [`codex-rollout-schema.md`](codex-rollout-schema.md) — captured Codex
  rollout usage shape used by the Codex source.
- [`ci-migration-2026-08.md`](ci-migration-2026-08.md) — the original move from
  GitHub Actions to Argo Workflows (superseded by
  [`../notes/release-pipeline.md`](../notes/release-pipeline.md)).

Research files may describe earlier names, dimensions, or proposed phases.
When they disagree with the implementation, the source code and
[`architecture.md`](../architecture.md) are authoritative.
