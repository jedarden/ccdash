# Multi-Directory JSONL Support - Already Complete

## Task
Feature: Multi-directory JSONL support for token tracking

## Status: Already Implemented

This feature was **already implemented** in commit `c35b4a0` and released in **v0.9.7** (May 14, 2026).

## Implementation Details

The implementation includes:

1. **CLI Flag**: `--extra-dirs=<dirs>` - Accepts comma-separated list of additional root directories
2. **Environment Variable**: `CCDASH_EXTRA_DIRS` - Accepts colon-separated paths
3. **Both methods stack** on top of the default `~/.claude/projects` root

## Code Locations

- `internal/metrics/tokens.go`:
  - `buildDefaultProjectsDirs()` (lines 81-94): Reads `CCDASH_EXTRA_DIRS` env var
  - `AddProjectsDir()` (lines 208-210): Adds additional directories
  - `findAllProjectDirs()` (lines 491-511): Aggregates results across all configured directories

- `cmd/ccdash/main.go`:
  - `--extra-dirs` flag (line 28)
  - Help text and examples (lines 204-206, 257-259)

- `internal/ui/dashboard.go`:
  - `AddProjectsDirs()` (lines 151-157): Passes extra dirs to token collector

## Verification

```bash
# Using CLI flag
ccdash --extra-dirs=/alt/path
ccdash --extra-dirs=/path1,/path2

# Using environment variable
CCDASH_EXTRA_DIRS=/path1:/path2 ccdash
```

## Related Beads

- **bf-109**: Original bead for this feature (completed in v0.9.7)
- **bf-3xq**: This bead (duplicate/closed as already complete)
