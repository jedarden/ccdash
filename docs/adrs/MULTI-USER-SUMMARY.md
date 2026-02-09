# Multi-User Support Summary

## Quick Overview

✅ **ccdash fully supports concurrent multi-user access out of the box**

Each user runs their own ccdash instance with:
- Independent session visibility (user's tmux sessions only)
- User-scoped metadata (`~/.beads-workers/metadata/`)
- Adaptive display (each terminal size handled independently)
- No coordination or shared state required

## How It Works

### User Isolation

```
┌─────────────────────────────────────────────────────────────┐
│ Server: dev.example.com                                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  alice@dev (terminal 199x14)                                │
│  ├─ tmux sessions: alice-alpha, c-glm-worker-1              │
│  ├─ ccdash sees: 2 sessions (1 interactive, 1 worker)       │
│  └─ metadata: ~/.beads-workers/metadata/                    │
│                                                             │
│  bob@dev (terminal 240x30)                                  │
│  ├─ tmux sessions: bob-alpha, bob-bravo, o-glm-worker-2     │
│  ├─ ccdash sees: 3 sessions (2 interactive, 1 worker)       │
│  └─ metadata: ~/.beads-workers/metadata/                    │
│                                                             │
│  charlie@dev (terminal 120x30)                              │
│  ├─ tmux sessions: charlie-delta, c-glm-worker-3            │
│  ├─ ccdash sees: 2 sessions (1 interactive, 1 worker)       │
│  └─ metadata: ~/.beads-workers/metadata/                    │
│                                                             │
└─────────────────────────────────────────────────────────────┘

Result: No conflicts, each user sees only their own sessions
```

### Architecture Principles

1. **Stateless Detection**
   - Pattern matching on session names
   - No database or coordination
   - Each instance independently determines worker vs interactive

2. **User-Scoped Data**
   - Tmux sessions: Per-user tmux server
   - Metadata files: `$HOME/.beads-workers/`
   - No cross-user visibility

3. **Independent Adaptation**
   - Each terminal reads its own dimensions
   - Adaptive layout per user (single-line vs multi-line)
   - No assumption of uniform displays

4. **Zero Coordination**
   - No inter-process communication
   - No locking or synchronization
   - No shared state

## Scenarios

### Scenario 1: Different Terminal Sizes

```
Alice (240x60): Multi-line format with full paths
Bob (199x14): Single-line format with abbreviations
Charlie (120x30): Multi-line format, narrow layout

✅ All work simultaneously, each gets optimal layout
```

### Scenario 2: Same Worker Names

```
Alice spawns: c-glm-alpha (in alice's tmux)
Bob spawns: c-glm-alpha (in bob's tmux)
Charlie spawns: c-glm-alpha (in charlie's tmux)

✅ No conflicts - each sees only their own c-glm-alpha
```

### Scenario 3: Rolling Updates

```
Day 1: Alice upgrades to v0.4.0 (worker visualization)
Day 2: Bob upgrades to v0.4.0
Day 3: Charlie still on v0.3.x

✅ No version conflicts, no forced coordination
```

## Security & Privacy

- ✅ Users only see their own sessions
- ✅ Metadata files: 600 permissions (user-readable only)
- ✅ Metadata directory: 700 permissions (user-accessible only)
- ✅ No cross-user information leakage
- ✅ Follows principle of least privilege

## Testing Multi-User Support

### Test 1: Concurrent Instances

```bash
# Terminal 1 (user A)
ssh userA@server
ccdash

# Terminal 2 (user B)
ssh userB@server
ccdash

# Verify: No conflicts, each sees own sessions
```

### Test 2: Different Displays

```bash
# User A: Small terminal
COLUMNS=199 LINES=14 ccdash
# Should use single-line format

# User B: Large terminal (concurrent)
COLUMNS=240 LINES=30 ccdash
# Should use multi-line format
```

### Test 3: Metadata Isolation

```bash
# User A
ls -la ~/.beads-workers/metadata/
# drwx------ (700) - only user A can access

# User B (tries to read user A's metadata)
cat /home/userA/.beads-workers/metadata/worker.json
# Permission denied ✅
```

## What Multi-User Does NOT Mean

❌ **Not a shared worker pool**: Each user manages their own workers
❌ **Not cluster-wide visibility**: Users don't see other users' workers
❌ **Not centralized management**: No daemon or coordinator
❌ **Not synchronized views**: Each user sees independent data

## Future: Optional Cluster-Wide View

If future requirement emerges for seeing all users' workers:

**Approach:**
- Separate tool: `ccdash-cluster` or `ccdash --cluster-view`
- Requires elevated permissions
- Opt-in, not default
- Does not replace per-user ccdash

**Not currently implemented** - no requirement yet.

## Related Documentation

- **ADR 0005**: Full architectural details on multi-user support
- **ADR 0001**: Worker detection (stateless, multi-user safe)
- **ADR 0003**: Adaptive layout (per-terminal adaptation)

## Summary

✅ **Multi-user support is built-in and requires no configuration**

Just run ccdash - it automatically:
- Sees only your sessions
- Adapts to your terminal
- Stores metadata in your home directory
- Works independently of other users

**Zero coordination overhead, maximum simplicity.**
