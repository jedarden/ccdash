# Multi-Directory JSONL Scanning Implementation (bf-3gc)

## Summary
Implemented multi-directory JSONL scanning support with glob pattern expansion for ccdash, allowing users to aggregate token usage from multiple Claude Code project roots.

## Changes Made

### 1. Core Implementation (`internal/metrics/tokens.go`)
- Added `ExpandGlobPatterns()` function that:
  - Expands glob patterns like `/home/*/projects` or `/path/to/project*`
  - Handles literal paths (no glob characters)
  - Filters out non-existent directories
  - Deduplicates results to prevent duplicate scans
  
- Updated `buildDefaultProjectsDirs()` to call `ExpandGlobPatterns()` on directory lists
- Updated documentation to reflect glob pattern support in CCDASH_EXTRA_DIRS

### 2. CLI Integration (`cmd/ccdash/main.go`)
- Updated `--extra-dirs` flag handler to call `metrics.ExpandGlobPatterns()`
- Glob patterns now work for both CLI flag (comma-separated) and environment variable (colon-separated)

### 3. Test Coverage (`internal/metrics/tokens_test.go`)
- Tests already existed for `expandGlobPatterns()` but function was missing
- Updated test calls to use exported function name `ExpandGlobPatterns()`
- All tests pass:
  - Glob pattern matching (`/tmp/project*` matches multiple directories)
  - Mixed literal and glob paths
  - Non-existent path filtering
  - Glob patterns with no matches
  - Deduplication of duplicate paths

## Usage Examples

### CLI Flag
```bash
ccdash --extra-dirs='/home/user/projects/*,/home/user/work/*/sessions'
```

### Environment Variable
```bash
CCDASH_EXTRA_DIRS='/home/*/projects' ccdash
```

### Both Methods
```bash
CCDASH_EXTRA_DIRS='/alt/projects' ccdash --extra-dirs='/home/user/*'
```

## Technical Details

**Glob Pattern Support**: Uses Go's `filepath.Glob()` function which supports:
- `*` - matches any sequence of characters (except path separator)
- `?` - matches any single character
- `[...]` - matches any character in the brackets

**Filtering**: Only directories are included in results (files are skipped)
**Deduplication**: Uses map to track seen paths, preventing duplicates
**Error Handling**: Invalid glob patterns are silently skipped

## Benefits
1. **Multiple Project Roots**: Users with projects in different locations can now scan them all
2. **Flexible Patterns**: Glob patterns make it easy to scan whole directory trees
3. **Backwards Compatible**: Existing literal paths continue to work unchanged
4. **Environment Variable Support**: Easy configuration without changing launch commands

## Notes
- Build errors in `dashboard.go` (network I/O panel) are pre-existing and unrelated to this change
- All token collection tests pass successfully
- The implementation was already partially in place (tests existed) but the function was missing
