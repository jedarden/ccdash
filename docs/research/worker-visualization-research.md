# Worker Visualization in ccdash - Research Document

## Executive Summary

`ccdash` currently displays all tmux sessions uniformly without distinguishing between **worker sessions** (autonomous bead agents) and **interactive CLI sessions** (user-attached terminals). This research explores how to enhance ccdash to visually separate and better represent these two distinct session types.

## Current State Analysis

### Session Types in the Environment

#### 1. Interactive CLI Sessions
- **Naming pattern**: Simple NATO callsigns (`alpha`, `bravo`, `charlie`, `delta`)
- **Purpose**: User-attached terminals running Claude Code interactively
- **Characteristics**:
  - Often attached (user is actively in the session)
  - Manual command execution
  - Respond to user input directly
  - No automated task processing

#### 2. Worker Sessions (Bead Agents)
- **Naming pattern**: `<executor>-<nato-callsign>`
  - Examples: `claude-code-glm-47-alpha`, `opencode-glm-47-bravo`, `claude-code-sonnet-charlie`
- **Purpose**: Autonomous agents processing beads (tasks) from a workspace queue
- **Characteristics**:
  - Usually detached (running in background)
  - Execute tasks from `.beads/` workspace autonomously
  - Log output to `~/.beads-workers/<session-name>.log`
  - Run via `/home/coder/claude-config/scripts/bead-worker.sh`
  - Tied to a specific workspace path
  - Process tasks until queue is empty

### Current ccdash Implementation

**File**: `/home/coder/ccdash/internal/metrics/tmux.go`

**Session Detection**:
```go
type TmuxSession struct {
    Name              string        // Session name (e.g., "alpha", "claude-code-glm-47-bravo")
    Windows           int           // Number of windows
    Attached          bool          // Whether user is attached
    Status            SessionStatus // WORKING, READY, ACTIVE, ERROR
    Created           time.Time     // Creation timestamp
    LastContentChange time.Time     // Last time content changed
    IdleDuration      time.Duration // Time since last content change
    LastLines         []string      // Last few lines of output
    Source            string        // "tmux", "hooks", or "hybrid"
}
```

**Status Detection Logic**:
1. **WORKING**: Detects interrupt hints (`"esc to interrupt"`, `"ctrl+c to interrupt"`) or `"(running)"`
2. **READY**: Detects Claude prompt waiting for input (idle >30s)
3. **ACTIVE**: User attached or recent content changes
4. **ERROR**: API errors, rate limits, connection errors

**Display Format** (line 1404 of `dashboard.go`):
```
[emoji] [session-name] [status] [windows]w [idle-time] [attached-indicator]
```

Example:
```
🟢 alpha         WORKING 1w 15s  📎
🔴 bravo         READY   2w 5m
🟡 charlie       ACTIVE  1w 30s  📎
🟢 claude-code-glm-47-alpha WORKING 1w 2m
```

### Limitations of Current Approach

1. **No Visual Distinction**: Workers and interactive CLIs look identical
2. **No Grouping**: All sessions displayed in alphabetical order
3. **Missing Worker Context**:
   - No indication of workspace path
   - No bead status (ready/blocked/completed count)
   - No worker-specific metadata
4. **Name Truncation**: Long worker names (`claude-code-glm-47-alpha` = 26 chars) may truncate in narrow layouts
5. **No Worker Summary**: Can't see at a glance how many workers are running vs interactive CLIs

---

## Research: How to Distinguish Workers

### Detection Strategy

#### Method 1: Session Name Pattern Matching (Recommended)

**Implementation**:
```go
func IsWorkerSession(sessionName string) bool {
    // Workers follow pattern: <executor>-<callsign>
    // Interactive CLIs are just: <callsign>

    // List of known executor prefixes
    executors := []string{
        "claude-code-glm-47-",
        "claude-code-sonnet-",
        "opencode-glm-47-",
        // Add more as executors are added
    }

    for _, prefix := range executors {
        if strings.HasPrefix(sessionName, prefix) {
            return true
        }
    }

    return false
}
```

**Pros**:
- Simple and reliable
- No external dependencies
- Pattern is enforced by spawn-workers.sh

**Cons**:
- Hardcoded executor list needs maintenance
- Won't catch workers with unexpected naming patterns

#### Method 2: Check for Worker Log File

**Implementation**:
```go
func IsWorkerSession(sessionName string) bool {
    logPath := filepath.Join(os.Getenv("HOME"), ".beads-workers", sessionName + ".log")
    _, err := os.Stat(logPath)
    return err == nil
}
```

**Pros**:
- Works for any worker regardless of naming
- Leverages existing worker infrastructure

**Cons**:
- Filesystem check on every refresh (slower)
- Log files may persist after session is killed

#### Method 3: Parse Running Process Command Line

**Implementation**:
```bash
# Check if session is running bead-worker.sh
tmux display-message -t <session-name> -p '#{pane_pid}'
ps -p <pid> -o args=
# Look for "bead-worker.sh" in command
```

**Pros**:
- Most accurate detection
- Works even if naming pattern changes

**Cons**:
- Complex (requires process tree traversal)
- Performance overhead
- May not catch workers that have replaced the shell process

#### Method 4: Worker Metadata File (Future Enhancement)

**Implementation**:
Workers could write metadata on startup:
```bash
# In bead-worker.sh, create metadata file:
mkdir -p ~/.beads-workers/metadata
cat > ~/.beads-workers/metadata/$SESSION_NAME.json <<EOF
{
  "session_name": "$SESSION_NAME",
  "executor": "$EXECUTOR",
  "workspace": "$WORKSPACE",
  "pid": $$,
  "started_at": "$(date -Iseconds)"
}
EOF
```

ccdash reads metadata:
```go
func GetWorkerMetadata(sessionName string) (*WorkerMetadata, error) {
    metadataPath := filepath.Join(os.Getenv("HOME"), ".beads-workers", "metadata", sessionName + ".json")
    // Parse JSON
}
```

**Pros**:
- Rich metadata (workspace, executor, PID)
- Enables deep integration with beads status
- Clean separation of concerns

**Cons**:
- Requires changes to worker spawn scripts
- More moving parts
- Metadata may become stale

**Recommendation**: Start with **Method 1 (Pattern Matching)** for immediate implementation, then add **Method 4 (Metadata)** as a future enhancement.

---

## Proposed UI Enhancements

### 1. Visual Grouping: Separate Worker Panel

**Current Layout** (ultra-wide mode at 206+ cols):
```
┌──────────────┬──────────────┬──────────────┐
│ System       │ Tokens       │ Tmux         │
│ Metrics      │              │ Sessions     │
│              │              │ (all mixed)  │
└──────────────┴──────────────┴──────────────┘
```

**Proposed Layout Option A**: Unified Tmux Panel with Sections
```
┌──────────────┬──────────────┬──────────────────────────┐
│ System       │ Tokens       │ Tmux Sessions            │
│ Metrics      │              │ ┌──────────────────────┐ │
│              │              │ │ Workers (3)          │ │
│              │              │ │ 🤖 claude-glm-alpha  │ │
│              │              │ │ 🤖 claude-glm-bravo  │ │
│              │              │ │ 🤖 opencode-charlie  │ │
│              │              │ └──────────────────────┘ │
│              │              │ ┌──────────────────────┐ │
│              │              │ │ Interactive (2)      │ │
│              │              │ │ 💻 alpha             │ │
│              │              │ │ 💻 delta             │ │
│              │              │ └──────────────────────┘ │
└──────────────┴──────────────┴──────────────────────────┘
```

**Proposed Layout Option B**: Separate Worker Panel (ultra-wide only)
```
┌──────────────┬──────────────┬──────────────┬──────────────┐
│ System       │ Tokens       │ Workers      │ Interactive  │
│ Metrics      │              │ 🤖 claude... │ 💻 alpha     │
│              │              │ 🤖 opencode  │ 💻 delta     │
└──────────────┴──────────────┴──────────────┴──────────────┘
```

**Proposed Layout Option C**: Compact Worker Summary Row
```
┌──────────────┬──────────────┬──────────────┐
│ System       │ Tokens       │ Tmux         │
│ Metrics      │              │ Sessions     │
│              │              │              │
│              │              │ Workers: 3   │
│              │              │ 🤖🤖🤖         │
│              │              │ (click h for │
│              │              │  details)    │
└──────────────┴──────────────┴──────────────┘
```

**Recommendation**: **Option A (Unified with Sections)** provides the best balance:
- Preserves existing layout structure
- Clear visual separation
- Works well in existing space
- Scalable (sections can collapse/expand)

---

### 2. Visual Styling: Icons and Colors

#### Worker-Specific Indicators

**Icon Mappings**:
```go
func GetSessionIcon(session TmuxSession) string {
    if IsWorkerSession(session.Name) {
        return "🤖" // Robot for workers
    }
    return "💻" // Computer for interactive
}
```

**Color Schemes**:

**Current Status Colors**:
- 🟢 Green: WORKING
- 🔴 Red: READY (waiting for input)
- 🟡 Yellow: ACTIVE (user attached)
- ❌ Red X: ERROR

**Proposed Worker Colors** (secondary color to distinguish type):
```
Workers:
  🤖 [cyan background] claude-code-glm-47-alpha
  🤖 [cyan background] opencode-glm-47-bravo

Interactive:
  💻 [default background] alpha
  💻 [default background] delta
```

Or use styled borders:
```
╔═════════════════════════╗  ← Double border for workers
║ 🤖 claude-glm-alpha     ║
║ 🟢 WORKING 1w 2m        ║
╚═════════════════════════╝

┌─────────────────────────┐  ← Single border for interactive
│ 💻 alpha                │
│ 🟡 ACTIVE 1w 30s 📎     │
└─────────────────────────┘
```

**Recommendation**: Use **distinct icons (🤖 vs 💻)** plus **optional background tint** for workers. Avoid complex borders (increases space usage).

---

### 3. Worker-Specific Metadata Display

#### Enhanced Worker Cell Format

**Current Format** (all sessions):
```
[emoji] [name] [status] [windows]w [idle] [attached]
🟢 alpha WORKING 1w 15s 📎
```

**Proposed Worker Format**:
```
[icon] [name]
    [workspace-path]
    [status] [windows]w [idle] [beads-status]

🤖 claude-glm-alpha
   /home/coder/ardenone-cluster/prompts/kalshi
   🟢 WORKING 1w 2m ⏳ 3/12 beads
```

**Metadata to Display**:
- **Workspace path**: Where the worker is processing beads
- **Bead status**: Ready/blocked/completed task counts
- **Executor type**: `glm-4.7` vs `sonnet` (extracted from name)
- **Worker uptime**: How long the worker has been running

#### Fetching Bead Metadata

**Integration with `br` CLI**:
```go
func GetBeadStats(workspacePath string) (*BeadStats, error) {
    // Run: br stats --workspace=<path> --json
    cmd := exec.Command("br", "stats", "--workspace=" + workspacePath, "--json")
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }

    var stats BeadStats
    json.Unmarshal(output, &stats)
    return &stats, nil
}

type BeadStats struct {
    Ready      int `json:"ready"`
    Blocked    int `json:"blocked"`
    InProgress int `json:"in_progress"`
    Completed  int `json:"completed"`
    Total      int `json:"total"`
}
```

**Display Format**:
```
⏳ 3 ready / 2 blocked / 7 done (12 total)
```

**Challenge**: Need to extract workspace path from worker session
- Option 1: Read from worker log file (first line typically logs workspace)
- Option 2: Parse worker metadata file (requires worker script changes)
- Option 3: Query tmux environment variable if set by worker

---

### 4. Interactive Features

#### Keyboard Shortcuts

Extend existing help mode (`h` key):

**Current Shortcuts**:
- `h`: Cycle through help modes (system, tokens, tmux)
- `q`: Quit
- `l`: Open lookback picker
- `u`: Check for updates

**Proposed New Shortcuts**:
- `w`: Toggle "workers only" view
- `i`: Toggle "interactive only" view
- `a`: Show all sessions (default)
- `W`: Focus worker panel (option B layout)
- `t`: Toggle worker metadata detail level

#### Click-to-Inspect (Future)

If mouse support is added:
- Click worker name → show full workspace path and bead list
- Click status → show recent log lines
- Click "Workers (3)" header → collapse/expand section

---

## Implementation Roadmap

### Phase 1: Basic Worker Detection (Immediate)
**Goal**: Distinguish workers from interactive sessions

**Changes**:
1. Add `IsWorkerSession()` helper function to `tmux.go`
2. Add `SessionType` field to `TmuxSession` struct:
   ```go
   type SessionType string
   const (
       SessionTypeWorker     SessionType = "worker"
       SessionTypeInteractive SessionType = "interactive"
   )
   ```
3. Populate `SessionType` during session parsing
4. Update `GetSessionIcon()` to return 🤖 for workers, 💻 for interactive

**File Changes**:
- `/home/coder/ccdash/internal/metrics/tmux.go`: Add detection logic
- `/home/coder/ccdash/internal/ui/dashboard.go`: Update icon rendering

**Testing**:
```bash
# Verify detection works
cd /home/coder/ccdash
go run cmd/ccdash/main.go

# Should see:
# 🤖 claude-code-glm-47-alpha
# 💻 alpha
```

---

### Phase 2: Visual Grouping (Short-term)
**Goal**: Separate workers and interactive sessions in UI

**Changes**:
1. Sort sessions by type (workers first, then interactive)
2. Add section headers: "Workers (N)" and "Interactive (N)"
3. Apply background tint or border styling to worker cells
4. Add worker count to panel title: "Tmux (5 sessions: 3 workers)"

**Layout Changes** (Option A - Unified Panel with Sections):
```
┌─ Tmux Sessions (5: 3 workers, 2 interactive) ─┐
│                                                │
│ ┌─ Workers (3) ─────────────────────────────┐ │
│ │ 🤖 claude-code-glm-47-alpha               │ │
│ │    🟢 WORKING 1w 2m                        │ │
│ │                                            │ │
│ │ 🤖 claude-code-sonnet-bravo               │ │
│ │    🔴 READY 2w 5m                          │ │
│ │                                            │ │
│ │ 🤖 opencode-glm-47-charlie                │ │
│ │    🟡 ACTIVE 1w 1m                         │ │
│ └────────────────────────────────────────────┘ │
│                                                │
│ ┌─ Interactive (2) ─────────────────────────┐ │
│ │ 💻 alpha                                   │ │
│ │    🟡 ACTIVE 1w 30s 📎                     │ │
│ │                                            │ │
│ │ 💻 delta                                   │ │
│ │    🔴 READY 3w 10m 📎                      │ │
│ └────────────────────────────────────────────┘ │
└────────────────────────────────────────────────┘
```

**File Changes**:
- `/home/coder/ccdash/internal/ui/dashboard.go`: Update `renderTmuxPanel()` to group sessions
- Add `renderWorkerSection()` and `renderInteractiveSection()` helpers

---

### Phase 3: Worker Metadata Integration (Medium-term)
**Goal**: Display workspace path and bead statistics for workers

**Changes**:
1. Update worker spawn script to create metadata files:
   ```bash
   # In /home/coder/claude-config/scripts/bead-worker.sh
   mkdir -p ~/.beads-workers/metadata
   cat > ~/.beads-workers/metadata/$SESSION_NAME.json <<EOF
   {
     "session_name": "$SESSION_NAME",
     "executor": "$EXECUTOR",
     "workspace": "$WORKSPACE",
     "started_at": "$(date -Iseconds)"
   }
   EOF
   ```

2. Add metadata reading in ccdash:
   ```go
   type WorkerMetadata struct {
       SessionName string    `json:"session_name"`
       Executor    string    `json:"executor"`
       Workspace   string    `json:"workspace"`
       StartedAt   time.Time `json:"started_at"`
   }

   func GetWorkerMetadata(sessionName string) (*WorkerMetadata, error) {
       // Read ~/.beads-workers/metadata/<session>.json
   }
   ```

3. Extend `TmuxSession` struct:
   ```go
   type TmuxSession struct {
       // ... existing fields ...
       WorkerMetadata *WorkerMetadata `json:"worker_metadata,omitempty"`
   }
   ```

4. Display in worker cells:
   ```
   🤖 claude-glm-alpha
      /home/coder/prompts/kalshi-improvement
      🟢 WORKING 1w 2m
   ```

**File Changes**:
- `/home/coder/claude-config/scripts/bead-worker.sh`: Write metadata
- `/home/coder/ccdash/internal/metrics/tmux.go`: Read metadata
- `/home/coder/ccdash/internal/ui/dashboard.go`: Display workspace path

---

### Phase 4: Bead Status Integration (Long-term)
**Goal**: Show real-time bead queue status for each worker

**Changes**:
1. Query bead stats for each worker workspace:
   ```bash
   br stats --workspace=/path/to/workspace --json
   ```

2. Add `BeadStats` to `WorkerMetadata`:
   ```go
   type BeadStats struct {
       Ready      int `json:"ready"`
       Blocked    int `json:"blocked"`
       InProgress int `json:"in_progress"`
       Completed  int `json:"completed"`
       Total      int `json:"total"`
   }

   type WorkerMetadata struct {
       // ... existing fields ...
       BeadStats *BeadStats `json:"bead_stats,omitempty"`
   }
   ```

3. Display bead status:
   ```
   🤖 claude-glm-alpha
      /home/coder/prompts/kalshi
      🟢 WORKING 1w 2m
      ⏳ 3 ready / 2 blocked / 7 done (12 total)
   ```

**Performance Considerations**:
- Cache bead stats (update every 30s, not every 2s)
- Only query for visible workers
- Skip if `br` CLI not available

**File Changes**:
- `/home/coder/ccdash/internal/metrics/beads.go`: New file for bead metrics
- `/home/coder/ccdash/internal/ui/dashboard.go`: Display bead status

---

### Phase 5: Interactive Controls (Future)
**Goal**: Allow user to filter, sort, and inspect sessions

**Features**:
- `w`: Show only workers
- `i`: Show only interactive sessions
- `a`: Show all (default)
- `s`: Sort by (name, status, idle time, uptime)
- Arrow keys: Navigate between sessions
- Enter: Attach to selected session (exec `tmux attach -t <name>`)

**Challenges**:
- Requires keyboard event handling in Bubble Tea model
- Attaching breaks ccdash TUI (need to suspend/resume)

---

## Space Allocation Adjustments

### Current Layout at 206x14 (Ultra-Wide)

From previous analysis (`ccdash-layout-analysis.md`):
```
| Panel   | Width  |
|---------|--------|
| System  | 60 cols|
| Token   | 60 cols|
| Tmux    | 80 cols|
```

**Tmux content width**: 80 - 4 (borders/padding) = **76 cols**

**Cell width** (single column): **76 cols**

**Name truncation**: With fixed overhead of ~20 chars, max name length = 56 chars

**Problem**: Worker names like `claude-code-glm-47-alpha` (26 chars) fit comfortably, but in multi-column layouts (when many sessions exist), names may truncate.

### Proposed Adjustments for Worker Display

#### Option 1: Two-Line Worker Names
```
🤖 claude-code-glm-47-alpha
   🟢 WORKING 1w 2m
```
- **Pro**: No truncation
- **Con**: Uses 2x vertical space per session

#### Option 2: Smart Name Abbreviation
```
🤖 cc-glm47-alpha
   🟢 WORKING 1w 2m
```
Abbreviate executor names:
- `claude-code-glm-47` → `cc-glm47`
- `opencode-glm-47` → `oc-glm47`
- `claude-code-sonnet` → `cc-sonnet`

**Pro**: Saves 10-15 chars per name
**Con**: Less clear what executor is running

#### Option 3: Increase minCellWidth (Already Proposed)

From `ccdash-layout-analysis.md`:
- Current `minCellWidth = 28` → Increase to `minCellWidth = 40`
- This accommodates full worker names even in multi-column layouts

**Recommendation**: **Implement Option 3** (increase minCellWidth) from the previous layout analysis. This solves truncation for all sessions, not just workers.

---

## Technical Considerations

### Performance Impact

**Metadata File Reads**:
- Workers: ~10 max concurrent (GLM worker limit)
- Metadata read: ~1ms per file
- **Total overhead**: <10ms per refresh (negligible)

**Bead Stats Queries** (Phase 4):
- `br stats` CLI call: ~50-200ms per workspace
- **Mitigation**: Cache results for 30s, only query on-demand

**Session Sorting/Grouping**:
- Sorting ~10 sessions: <1ms
- **Impact**: Negligible

### Compatibility

**Backward Compatibility**:
- Changes to `TmuxSession` struct are additive (new fields)
- Existing status detection logic unchanged
- No breaking changes to metrics API

**Terminal Compatibility**:
- Emoji icons (🤖, 💻) work in all modern terminals
- Background tints require 256-color support (widely available)
- Fallback: Use text labels `[W]` and `[I]` if emoji support missing

### Testability

**Unit Tests**:
```go
func TestIsWorkerSession(t *testing.T) {
    tests := []struct {
        name     string
        session  string
        expected bool
    }{
        {"worker glm", "claude-code-glm-47-alpha", true},
        {"worker sonnet", "claude-code-sonnet-bravo", true},
        {"worker opencode", "opencode-glm-47-charlie", true},
        {"interactive", "alpha", false},
        {"interactive callsign", "delta", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := IsWorkerSession(tt.session)
            if result != tt.expected {
                t.Errorf("expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

**Integration Tests**:
1. Spawn test worker: `spawn-workers.sh --workspace=/tmp/test --workers=1`
2. Launch ccdash
3. Verify worker appears with 🤖 icon
4. Verify grouping and metadata display

---

## Alternative Approaches

### A. Separate TUI for Workers (`beads-dash`)

Instead of integrating into ccdash, create a dedicated worker dashboard:

**Pros**:
- Focused on worker/bead-specific metrics
- Can show deeper bead details (task tree, dependencies)
- Doesn't complicate ccdash codebase

**Cons**:
- Two separate tools to manage
- Duplication of tmux session tracking logic
- Users need to switch between dashboards

**Verdict**: Not recommended. Integration into ccdash provides unified view.

### B. Extend `br` CLI with Worker Dashboard

Add `br workers` command to show worker status:

```bash
$ br workers --workspace=/home/coder/prompts/kalshi

Workers (3):
  🤖 claude-glm-alpha    WORKING  2m idle  3/12 beads
  🤖 claude-glm-bravo    READY    5m idle  0/12 beads
  🤖 opencode-charlie    ERROR    1m idle  1/12 beads

Beads:
  ⏳ bd-abc  Fetch order history        [ready]
  ⏳ bd-def  Analyze execution failures [ready]
  🔒 bd-ghi  Generate report           [blocked by bd-abc]
  ...
```

**Pros**:
- Tightly integrated with beads workflow
- Can leverage `br` internal bead state
- No tmux dependency

**Cons**:
- Not real-time (requires manual refresh)
- Doesn't integrate with system/token metrics
- Duplicates session tracking

**Verdict**: Complementary approach. Both can coexist:
- `ccdash`: Real-time overview of all sessions + system metrics
- `br workers`: Deep dive into worker/bead status

---

## Conclusion

### Recommended Implementation Plan

**Priority 1: Phase 1 (Basic Worker Detection)**
- **Effort**: 2-4 hours
- **Impact**: High (immediate visual distinction)
- **Files**: `tmux.go`, `dashboard.go`

**Priority 2: Phase 2 (Visual Grouping)**
- **Effort**: 4-8 hours
- **Impact**: High (clear separation, better UX)
- **Files**: `dashboard.go` (layout refactor)

**Priority 3: Phase 3 (Worker Metadata)**
- **Effort**: 6-10 hours (includes worker script changes)
- **Impact**: Medium (useful but not critical)
- **Files**: `bead-worker.sh`, `tmux.go`, `dashboard.go`

**Priority 4: Phase 4 (Bead Status Integration)**
- **Effort**: 8-12 hours
- **Impact**: Medium (nice-to-have for workflow visibility)
- **Files**: New `beads.go`, `dashboard.go`

**Priority 5: Phase 5 (Interactive Controls)**
- **Effort**: 12-20 hours
- **Impact**: Low (advanced feature)
- **Files**: `dashboard.go` (event handling)

### Key Benefits

1. **Clarity**: Immediately distinguish worker agents from interactive terminals
2. **Efficiency**: Quickly assess worker status and bead queue health
3. **Context**: See workspace paths and task progress at a glance
4. **Scalability**: Supports 10+ workers without overwhelming UI
5. **Integration**: Unified view of system, tokens, and agent activity

### Next Steps

1. Create issue/bead in ccdash repo for Phase 1 implementation
2. Prototype icon-based detection in local ccdash fork
3. Test with 5+ workers + 3+ interactive sessions
4. Gather feedback on layout options (A, B, or C)
5. Iterate on metadata display format

---

**Document Version**: 1.0
**Date**: 2026-02-07
**Author**: Claude Sonnet 4.5
**Related Files**:
- `/home/coder/ccdash/internal/metrics/tmux.go`
- `/home/coder/ccdash/internal/ui/dashboard.go`
- `/home/coder/claude-config/scripts/spawn-workers.sh`
- `/home/coder/research/beads/ccdash-layout-analysis.md`
