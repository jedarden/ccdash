# ADR 0005: Support Concurrent Multi-User Access

## Status

Accepted (2026-02-07)

## Context

ccdash is deployed in multi-user environments where:
- Multiple developers work on the same cluster/server
- Each user runs ccdash in their own terminal
- Users have independent tmux sessions
- Users may spawn their own worker agents
- Multiple ccdash instances run concurrently

**Requirements:**
1. Each user should see their own tmux sessions
2. Worker detection should work per-user
3. No conflicts between concurrent ccdash instances
4. Each user's terminal dimensions may differ (adaptive layout)
5. Metadata should be user-scoped
6. No shared state coordination needed

**Example Scenario:**
```
Server: ardenone-cluster

User A (terminal: 199x14):
  - Running: ccdash in ~/terminal-a
  - Tmux sessions: alpha, bravo, c-glm-worker-a
  - Sees: Own sessions only

User B (terminal: 240x30):
  - Running: ccdash in ~/terminal-b
  - Tmux sessions: charlie, delta, c-glm-worker-b
  - Sees: Own sessions only

User C (terminal: 160x40):
  - Running: ccdash in ~/terminal-c
  - Tmux sessions: echo, foxtrot, o-glm-worker-c
  - Sees: Own sessions only
```

## Decision

We will design ccdash worker visualization to be **fully independent per-user** with no shared state or coordination between instances.

### 1. Session Visibility: User-Scoped

Each ccdash instance only sees the user's own tmux sessions.

**Implementation:**
```go
// tmux list-sessions shows only current user's sessions
func (tc *TmuxCollector) listSessions() ([]TmuxSession, error) {
    cmd := exec.Command("tmux", "list-sessions", "-F", "...")
    // This naturally scopes to current user's tmux server
    // No cross-user visibility
}
```

**Why this works:**
- `tmux list-sessions` connects to `$TMUX_TMPDIR/default` socket
- Each user has their own tmux server (unless explicitly shared)
- No additional scoping needed

**Verification:**
```bash
# As user A
tmux list-sessions  # Shows only user A's sessions

# As user B (different terminal)
tmux list-sessions  # Shows only user B's sessions
```

### 2. Worker Detection: Pattern-Based (No Shared State)

Worker detection uses session name pattern matching - fully stateless.

**Implementation:**
```go
func IsWorkerSession(sessionName string) bool {
    // Stateless pattern matching
    executors := []string{
        "claude-code-glm-47-",
        "opencode-glm-47-",
    }
    for _, prefix := range executors {
        if strings.HasPrefix(sessionName, prefix) {
            return true
        }
    }
    return false
}
```

**Why this works:**
- No database or shared state
- Pure function based on session name
- Each instance independently determines worker vs interactive
- Identical logic across all instances

### 3. Metadata Files: User-Scoped Filesystem

Worker metadata stored in user's home directory.

**File Locations:**
```
User A: /home/userA/.beads-workers/metadata/c-glm-worker-a.json
User B: /home/userB/.beads-workers/metadata/c-glm-worker-b.json
User C: /home/userC/.beads-workers/metadata/o-glm-worker-c.json
```

**Why this works:**
- Each user's `$HOME` is isolated
- No file conflicts between users
- No locking or coordination needed
- Reads are user-scoped automatically

**Implementation:**
```go
func GetWorkerMetadata(sessionName string) (*WorkerMetadata, error) {
    // Reads from current user's home directory
    metadataPath := filepath.Join(
        os.Getenv("HOME"),  // Current user's home
        ".beads-workers",
        "metadata",
        sessionName+".json",
    )
    data, err := os.ReadFile(metadataPath)
    // ...
}
```

### 4. Display Layout: Per-Terminal Adaptation

Each ccdash instance adapts to its own terminal dimensions.

**Implementation:**
```go
func (d *Dashboard) View() string {
    // Each instance has own terminal size
    width, height := d.width, d.height

    // Independent layout decision
    if width < 200 || height < 20 {
        return d.renderSingleLineLayout()
    }
    return d.renderMultiLineLayout()
}
```

**Why this works:**
- Terminal dimensions read from local terminal
- No assumption of uniform display sizes
- User A (199x14) gets single-line format
- User B (240x30) gets multi-line format
- Both work concurrently without conflict

### 5. Refresh Cycle: Independent Per-Instance

Each ccdash instance has its own refresh timer.

**Implementation:**
```go
func tickEvery(duration time.Duration) tea.Cmd {
    return tea.Tick(duration, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

**Why this works:**
- No global timer or coordinator
- Each instance refreshes every 2 seconds independently
- No synchronization needed
- No race conditions

## Consequences

### Positive

1. **Zero Coordination Overhead**
   - No inter-process communication
   - No shared state management
   - No locking or synchronization
   - Simple, robust design

2. **Fully Isolated Operation**
   - Users cannot interfere with each other
   - One user's ccdash crash doesn't affect others
   - No cascading failures
   - Independent upgrade/restart per user

3. **Scalable Architecture**
   - Supports unlimited concurrent users
   - No performance degradation with more users
   - No central bottleneck
   - Linear resource usage

4. **Flexible Display Adaptation**
   - Each user gets optimal layout for their terminal
   - Mixed display sizes supported simultaneously
   - No "lowest common denominator" compromise

5. **Privacy and Security**
   - Users only see their own sessions
   - No cross-user information leakage
   - Metadata isolated per user
   - Follows principle of least privilege

### Negative

1. **No Cross-User Visibility**
   - Users cannot see other users' workers
   - No cluster-wide worker dashboard
   - **Mitigation:** Not a requirement for current use case
   - **Alternative:** Separate tool for cluster-wide view if needed

2. **Potential Duplicate Worker Names**
   - Multiple users could spawn `c-glm-alpha`
   - Each sees only their own, but names not globally unique
   - **Mitigation:** User-scoped tmux sessions prevent actual conflicts
   - **Note:** Session names are `user@host:session`, unique per user

3. **No Shared Worker Pool**
   - Cannot implement shared worker queue across users
   - Each user manages their own workers
   - **Mitigation:** Not a requirement; workers are user-owned

### Neutral

- **Deployment simplicity**: Each user runs their own binary, no daemon needed
- **Configuration**: User-specific settings in `~/.config/ccdash/` (future)

## Multi-User Scenarios

### Scenario 1: Shared Development Server

**Setup:**
```
Server: dev.example.com
Users: alice, bob, charlie
Each user: SSH into server, run tmux, spawn workers, run ccdash
```

**User Sessions:**
```
alice@dev:
  tmux sessions: alice-alpha, alice-bravo, c-glm-worker-1
  ccdash sees: 3 sessions (2 interactive, 1 worker)

bob@dev:
  tmux sessions: bob-alpha, bob-charlie, c-glm-worker-2
  ccdash sees: 3 sessions (2 interactive, 1 worker)

charlie@dev:
  tmux sessions: charlie-delta, o-glm-worker-1, o-glm-worker-2
  ccdash sees: 3 sessions (1 interactive, 2 workers)
```

**Result:** Each user's ccdash shows only their sessions, no conflicts.

### Scenario 2: Different Terminal Sizes

**Setup:**
```
alice: MacBook Pro (240x60 terminal)
bob: iPad SSH client (120x30 terminal)
charlie: DevPod in browser (199x14 terminal)
```

**Layouts:**
```
alice: Multi-line format with full paths (has space)
bob: Multi-line format, narrow layout (adaptive)
charlie: Single-line format with abbreviations (constrained)
```

**Result:** Each user gets optimal layout for their display, all work simultaneously.

### Scenario 3: Rolling Updates

**Setup:**
```
ccdash v0.4.0 released with worker visualization
Users upgrade at different times
```

**Upgrade Path:**
```
Day 1: alice upgrades to v0.4.0
  - alice sees new worker visualization
  - bob/charlie still on v0.3.x (old unified list)
  - No compatibility issues

Day 2: bob upgrades to v0.4.0
  - bob sees new worker visualization
  - charlie still on v0.3.x
  - No conflicts

Day 3: charlie upgrades to v0.4.0
  - All users on v0.4.0
  - All see worker visualization
```

**Result:** No forced coordinated upgrade, no version conflicts.

## Implementation Considerations

### 1. Tmux Server Scoping

**Default behavior (already correct):**
```bash
# tmux creates per-user socket
/tmp/tmux-$(id -u)/default

# Each user connects to their own socket
alice connects to /tmp/tmux-1001/default
bob connects to /tmp/tmux-1002/default
charlie connects to /tmp/tmux-1003/default
```

**No changes needed** - tmux naturally scopes per-user.

### 2. Metadata File Permissions

**Set restrictive permissions:**
```bash
# In worker spawn script
mkdir -p ~/.beads-workers/metadata
chmod 700 ~/.beads-workers/metadata  # Only owner can access

# Metadata files
chmod 600 ~/.beads-workers/metadata/*.json  # Only owner can read/write
```

**Why:** Prevents cross-user access, follows security best practices.

### 3. No Locking Needed

**Read operations (ccdash):**
- Read tmux sessions: No locking needed (tmux handles)
- Read metadata files: No locking needed (read-only, single writer)

**Write operations (worker spawn):**
- Write metadata on spawn: Single writer (spawning process)
- Delete metadata on exit: Single writer (exiting process)
- No concurrent writes to same file

### 4. Error Handling for Missing Metadata

**Graceful degradation:**
```go
func GetWorkerMetadata(sessionName string) (*WorkerMetadata, error) {
    // If file doesn't exist, return nil (not error)
    // Allows worker to display without metadata
    _, err := os.Stat(metadataPath)
    if os.IsNotExist(err) {
        return nil, nil  // Not an error, just no metadata
    }
    // ...
}
```

**Why:** Other user's workers may not have metadata visible, don't fail.

## Testing Requirements

### Multi-User Tests

**Test 1: Concurrent ccdash Instances**
```bash
# Terminal 1 (user A)
ccdash &

# Terminal 2 (user B, different SSH session)
ccdash &

# Verify: No conflicts, each sees own sessions
```

**Test 2: Different Terminal Sizes**
```bash
# User A: 199x14 terminal
COLUMNS=199 LINES=14 ccdash
# Should use single-line format

# User B: 240x30 terminal
COLUMNS=240 LINES=30 ccdash
# Should use multi-line format
```

**Test 3: Shared Worker Names**
```bash
# User A spawns: c-glm-alpha
# User B spawns: c-glm-alpha
# Verify: No conflicts, each sees only their own
```

**Test 4: Metadata Isolation**
```bash
# User A creates metadata for worker
ls -la ~/.beads-workers/metadata/
# Verify: 700 permissions, not readable by other users

# User B tries to read User A's metadata
cat /home/userA/.beads-workers/metadata/worker.json
# Should fail with permission denied
```

## Alternative Approaches Considered

### Alternative 1: Shared Database with User Column

Use SQLite database with user column to track all sessions globally.

**Rejected because:**
- Adds unnecessary complexity
- Requires coordination/locking
- No benefit over independent operation
- Single point of failure
- Cross-user visibility not needed

### Alternative 2: Central Daemon with RPC

Run central ccdash daemon, users connect via RPC.

**Rejected because:**
- Much more complex (daemon management, RPC protocol)
- Single point of failure (daemon crash affects all)
- Requires coordination for upgrades
- Overkill for simple dashboard needs
- No requirement for central coordination

### Alternative 3: Shared Tmux Server

All users connect to shared tmux server.

**Rejected because:**
- Security risk (users can attach to each other's sessions)
- Breaks tmux isolation model
- No benefit for ccdash
- Goes against best practices

## Future Considerations

### Optional: Cluster-Wide Worker View

If future requirement emerges for seeing all users' workers:

**Approach:**
- Separate tool: `ccdash-cluster` or `ccdash --cluster-view`
- Requires elevated permissions or shared metadata location
- Opt-in, not default
- Does not replace per-user ccdash

**Not implemented now** - no current requirement.

## References

- Related ADR: ADR 0001 (Worker detection)
- Related ADR: ADR 0003 (Adaptive layout)
- tmux documentation: https://github.com/tmux/tmux/wiki
- File permissions best practices: OWASP guidelines
