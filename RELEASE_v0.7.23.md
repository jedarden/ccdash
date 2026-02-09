# Release v0.7.23

## Bug Fixes

### Fixed PID Tracking for Claude Code Sessions

**Problem:**
- Session hooks were incorrectly storing the hook script's PID (`$$`) instead of the actual Claude Code process PID
- Hook scripts exit immediately after execution, leaving dead PIDs in session files
- When Claude Code restarts in the same tmux window, old session files with stale PIDs remained
- ccdash displayed all sessions as "ready" despite them actively running, because the tracked PIDs were dead

**Solution:**
- Session hooks now walk up the process tree from `$PPID` to find the actual "claude" process
- Store the real Claude Code process PID instead of the ephemeral hook script PID
- Automatically clean up old session files for the same tmux session when a new session starts
- `prompt-submit` hook now updates the PID to handle cases where the process restarted

**Impact:**
- Sessions now correctly show as "working" or "active" when Claude Code is running
- No more false "ready" status for active sessions
- Accurate process tracking even when Claude Code restarts in the same tmux window

**Testing:**
- Verified hook captures running Claude process PID correctly
- Confirmed PID is valid and points to actual "claude" process
- Old stale session files are properly cleaned up on new session start

## Installation

### Linux AMD64
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.7.23/ccdash-linux-amd64
chmod +x ccdash-linux-amd64
sudo mv ccdash-linux-amd64 /usr/local/bin/ccdash
```

### Linux ARM64
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.7.23/ccdash-linux-arm64
chmod +x ccdash-linux-arm64
sudo mv ccdash-linux-arm64 /usr/local/bin/ccdash
```

### macOS AMD64
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.7.23/ccdash-darwin-amd64
chmod +x ccdash-darwin-amd64
sudo mv ccdash-darwin-amd64 /usr/local/bin/ccdash
```

### macOS ARM64 (Apple Silicon)
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.7.23/ccdash-darwin-arm64
chmod +x ccdash-darwin-arm64
sudo mv ccdash-darwin-arm64 /usr/local/bin/ccdash
```

### Update Hooks

After installing the new binary, reinstall the hooks to get the fixes:

```bash
ccdash --install-hooks
```

This will update the hook scripts in `~/.ccdash/hooks/` with the corrected PID tracking logic.

## Checksums

```
be3d1f5269732cea4a6a639684f763ccef7970de4b503a80b56f8a210fd3d56d  ccdash-linux-amd64
12faf2ada3e7ea193de8d420a04d6cec46830d8259000ba173784c3f49fe16bf  ccdash-linux-arm64
0591568869344623e5754b573f8506d198fd443756810805bc53f1c79e3650a2  ccdash-darwin-amd64
fa4139c0d5aadd01f8fc0064b541cdf2175ff40d0c4d85b2aaa3aa7aa52ecf39  ccdash-darwin-arm64
```

## Full Changelog

See [CHANGELOG.md](CHANGELOG.md) for complete version history.
