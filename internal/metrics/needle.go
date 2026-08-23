package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// NeedleWorker mirrors one entry in ~/.needle/state/workers.json.
//
// NEEDLE workers supervised by systemd run with NEEDLE_INNER=1, which keeps the
// worker loop in the foreground instead of detaching into tmux. They also
// dispatch via `claude --print`, and Claude Code does not fire SessionStart in
// print mode. So such workers are invisible to both the tmux collector and the
// hook collector, and the registry is the only source that sees them.
type NeedleWorker struct {
	ID             string    `json:"id"`
	PID            int       `json:"pid"`
	Workspace      string    `json:"workspace"`
	Agent          string    `json:"agent"`
	Provider       string    `json:"provider"`
	StartedAt      time.Time `json:"started_at"`
	BeadsProcessed int       `json:"beads_processed"`
}

type needleRegistry struct {
	Workers []NeedleWorker `json:"workers"`
}

// NeedleCollector reads the NEEDLE worker registry.
type NeedleCollector struct {
	registryPath string
}

// NewNeedleCollector creates a collector pointed at the default registry path.
func NewNeedleCollector() (*NeedleCollector, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &NeedleCollector{
		registryPath: filepath.Join(home, ".needle", "state", "workers.json"),
	}, nil
}

// IsAvailable reports whether a NEEDLE registry exists on this host.
func (nc *NeedleCollector) IsAvailable() bool {
	if nc == nil {
		return false
	}
	_, err := os.Stat(nc.registryPath)
	return err == nil
}

// CollectWorkers returns registry entries whose process is still alive.
//
// Dead entries are skipped rather than deleted: the registry belongs to NEEDLE,
// and reaping stale rows is the mend strand's job, not the dashboard's.
func (nc *NeedleCollector) CollectWorkers() ([]NeedleWorker, error) {
	if nc == nil {
		return nil, nil
	}

	data, err := os.ReadFile(nc.registryPath)
	if err != nil {
		return nil, err
	}

	var reg needleRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, err
	}

	live := make([]NeedleWorker, 0, len(reg.Workers))
	for _, w := range reg.Workers {
		if w.PID > 0 && isProcessRunning(w.PID) {
			live = append(live, w)
		}
	}
	return live, nil
}

// DisplayName strips the redundant agent prefix from the worker id, so
// "claude-code-glm-4.7-lab-roam-1" displays as "lab-roam-1". The agent is shown
// separately in the detail line.
func (w *NeedleWorker) DisplayName() string {
	if w.Agent != "" {
		if trimmed := strings.TrimPrefix(w.ID, w.Agent+"-"); trimmed != w.ID && trimmed != "" {
			return trimmed
		}
	}
	return w.ID
}

// ToTmuxSession converts a worker into the shape the sessions pane renders.
// busy reports whether the worker currently has a live `claude` descendant,
// which is what distinguishes dispatching from idling between beads.
func (w *NeedleWorker) ToTmuxSession(busy bool) TmuxSession {
	status := StatusReady
	if busy {
		status = StatusWorking
	}

	detail := w.Agent
	if w.Workspace != "" {
		detail += " · " + filepath.Base(w.Workspace)
	}
	detail += " · " + strconv.Itoa(w.BeadsProcessed) + " beads"

	return TmuxSession{
		Name:      w.DisplayName(),
		Windows:   1,
		Attached:  busy,
		Created:   w.StartedAt,
		Status:    status,
		LastLines: []string{detail},
		Source:    "needle",
	}
}

// normalizeForTmux converts a worker ID to the tmux session naming convention
// used by NEEDLE, which replaces dots with underscores (e.g., "glm-4.7" becomes
// "glm-4_7"). This ensures identifiers match when comparing against tmux session
// names.
func normalizeForTmux(workerID string) string {
	return strings.ReplaceAll(workerID, ".", "_")
}

// normalizeForNeedle removes the "needle-" prefix and normalizes dots to underscores
// to create a consistent identifier for deduplication. This handles both full tmux
// session names (e.g., "needle-claude-code-glm-4_7-alpha" → "claude-code-glm-4_7-alpha")
// and worker IDs with dots (e.g., "claude-code-glm-4.7-alpha" → "claude-code-glm-4_7-alpha").
func normalizeForNeedle(name string) string {
	// Remove the "needle-" prefix if present
	normalized := strings.TrimPrefix(name, "needle-")
	// Replace dots with underscores for consistency
	normalized = strings.ReplaceAll(normalized, ".", "_")
	return normalized
}

// hasTmuxSessionFor reports whether any tmux session already represents the
// given NEEDLE worker id. needle names its tmux sessions after the worker id
// with a prefix and sanitizes the name by replacing dots with underscores, so
// normalization is required before comparison.
func hasTmuxSessionFor(workerID string, sessions []TmuxSession) bool {
	if workerID == "" {
		return false
	}
	normalizedID := normalizeForTmux(workerID)
	for _, s := range sessions {
		if strings.Contains(s.Name, normalizedID) {
			return true
		}
	}
	return false
}

// claudeOwners returns the set of PIDs from want that have a live `claude`
// descendant. It walks /proc once and follows each claude process up its parent
// chain, which is intact for NEEDLE dispatches (claude -> bash -> needle) even
// though the dispatch runs inside a transient systemd scope.
func claudeOwners(want map[int]bool) map[int]bool {
	owners := make(map[int]bool)
	if len(want) == 0 {
		return owners
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return owners
	}

	parent := make(map[int]int, len(entries))
	claudePIDs := make([]int, 0, 8)

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, ppid, ok := readProcStat(pid)
		if !ok {
			continue
		}
		parent[pid] = ppid
		if comm == "claude" {
			claudePIDs = append(claudePIDs, pid)
		}
	}

	for _, pid := range claudePIDs {
		// Bounded walk: process trees here are shallow, and the cap guards
		// against a cycle from PID reuse mid-scan.
		for cur, depth := parent[pid], 0; cur > 1 && depth < 32; cur, depth = parent[cur], depth+1 {
			if want[cur] {
				owners[cur] = true
				break
			}
		}
	}
	return owners
}

// readProcStat returns the comm and ppid for a pid. The comm field is wrapped
// in parentheses and may itself contain spaces or parens, so it is parsed from
// the last ')' rather than by splitting on whitespace.
func readProcStat(pid int) (comm string, ppid int, ok bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", 0, false
	}
	s := string(data)

	open := strings.IndexByte(s, '(')
	close := strings.LastIndexByte(s, ')')
	if open < 0 || close < 0 || close < open {
		return "", 0, false
	}
	comm = s[open+1 : close]

	rest := strings.Fields(s[close+1:])
	// rest[0] is state, rest[1] is ppid
	if len(rest) < 2 {
		return "", 0, false
	}
	ppid, err = strconv.Atoi(rest[1])
	if err != nil {
		return "", 0, false
	}
	return comm, ppid, true
}
