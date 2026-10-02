package metrics

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// procInfo is the slice of /proc/<pid>/stat needed to walk a process tree.
type procInfo struct {
	comm string
	ppid int
}

// isAgentCommand reports whether a process name belongs to an agent harness or
// to the NEEDLE supervisor that dispatches one. comm is truncated to 15 bytes
// by the kernel, so NEEDLE's versioned binaries ("needle-stable") are matched
// by prefix.
func isAgentCommand(comm string) bool {
	switch comm {
	case "claude", "codex", "opencode", "needle":
		return true
	}
	return strings.HasPrefix(comm, "needle-")
}

// readProcTable snapshots comm and ppid for every process. ok is false when
// /proc is unavailable (macOS), which callers must treat as "cannot tell"
// rather than "no agents running".
func readProcTable() (procs map[int]procInfo, ok bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	procs = make(map[int]procInfo, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, ppid, ok := readProcStat(pid)
		if !ok {
			continue
		}
		procs[pid] = procInfo{comm: comm, ppid: ppid}
	}
	return procs, len(procs) > 0
}

// parsePanePIDs parses `tmux list-panes -a -F '#{pane_pid} #{session_name}'`
// output into session name -> pane root PIDs. The PID comes first because a
// session name may contain spaces.
func parsePanePIDs(output string) map[string][]int {
	panes := make(map[string][]int)
	for _, line := range strings.Split(output, "\n") {
		pidStr, name, found := strings.Cut(strings.TrimRight(line, "\r"), " ")
		if !found || name == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		panes[name] = append(panes[name], pid)
	}
	return panes
}

// sessionsHostingAgent returns the sessions that have an agent process at or
// below one of their pane root PIDs.
func sessionsHostingAgent(panes map[string][]int, procs map[int]procInfo) map[string]bool {
	owner := make(map[int]string)
	for name, pids := range panes {
		for _, pid := range pids {
			owner[pid] = name
		}
	}

	hosting := make(map[string]bool)
	for pid, info := range procs {
		if !isAgentCommand(info.comm) {
			continue
		}
		// Bounded walk: the cap guards against a cycle from PID reuse mid-scan.
		for cur, depth := pid, 0; cur > 1 && depth < 32; cur, depth = procs[cur].ppid, depth+1 {
			if name, ok := owner[cur]; ok {
				hosting[name] = true
				break
			}
		}
	}
	return hosting
}

// agentSessions returns the tmux sessions with an agent process in one of
// their panes. ok is false when that cannot be determined; the caller must
// then keep every session instead of hiding them all.
func (tc *TmuxCollector) agentSessions() (hosting map[string]bool, ok bool) {
	procs, ok := readProcTable()
	if !ok {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "list-panes", "-a", "-F", "#{pane_pid} #{session_name}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, false
	}

	return sessionsHostingAgent(parsePanePIDs(stdout.String()), procs), true
}

// hostsAgent decides whether a tmux session not tracked by hooks belongs in
// the Sessions panel. Without it every tmux session was listed, and any shell
// or build whose output scrolled was reported as a WORKING agent.
//
// Agent UI markers are kept as a signal because an agent reached over ssh
// inside tmux has no local process to find.
func hostsAgent(session TmuxSession, withAgentProcess map[string]bool) bool {
	return withAgentProcess[session.Name] ||
		session.agentUI ||
		DetectSessionType(session.Name) == SessionTypeWorker
}
