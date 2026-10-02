package metrics

import (
	"reflect"
	"testing"
)

func TestIsAgentCommand(t *testing.T) {
	for comm, want := range map[string]bool{
		"claude":          true,
		"codex":           true,
		"opencode":        true,
		"needle":          true,
		"needle-stable":   true,
		"needle-transfor": true,
		"bash":            false,
		"ccdash":          false,
		"node":            false,
		"claude-helper":   false,
		"":                false,
	} {
		if got := isAgentCommand(comm); got != want {
			t.Errorf("isAgentCommand(%q) = %v, want %v", comm, got, want)
		}
	}
}

func TestParsePanePIDs(t *testing.T) {
	got := parsePanePIDs("100 alpha\n101 alpha\n200 my session\nbogus line\n300\n\n")
	want := map[string][]int{
		"alpha":      {100, 101},
		"my session": {200},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePanePIDs = %v, want %v", got, want)
	}
}

func TestSessionsHostingAgent(t *testing.T) {
	panes := map[string][]int{
		"interactive": {100}, // bash -> claude
		"direct":      {200}, // pane root is codex itself
		"worker":      {300}, // bash -> needle-stable, no agent child yet
		"shell":       {400}, // bash -> sleep
		"dashboard":   {500}, // ccdash
		"multi":       {600, 610},
	}
	procs := map[int]procInfo{
		1:   {comm: "systemd", ppid: 0},
		100: {comm: "bash", ppid: 1},
		101: {comm: "claude", ppid: 100},
		200: {comm: "codex", ppid: 1},
		300: {comm: "bash", ppid: 1},
		301: {comm: "needle-stable", ppid: 300},
		400: {comm: "bash", ppid: 1},
		401: {comm: "sleep", ppid: 400},
		500: {comm: "ccdash", ppid: 1},
		600: {comm: "bash", ppid: 1},
		610: {comm: "bash", ppid: 1},
		611: {comm: "node", ppid: 610},
		612: {comm: "opencode", ppid: 611},
		// An agent outside tmux must not be attributed to any session.
		900: {comm: "claude", ppid: 1},
	}

	got := sessionsHostingAgent(panes, procs)
	want := map[string]bool{"interactive": true, "direct": true, "worker": true, "multi": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sessionsHostingAgent = %v, want %v", got, want)
	}
}

func TestSessionsHostingAgentSurvivesParentCycle(t *testing.T) {
	procs := map[int]procInfo{
		10: {comm: "claude", ppid: 11},
		11: {comm: "bash", ppid: 10},
	}
	if got := sessionsHostingAgent(map[string][]int{"s": {99}}, procs); len(got) != 0 {
		t.Fatalf("expected no sessions, got %v", got)
	}
}

func TestHostsAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	withProcess := map[string]bool{"alpha": true}

	cases := []struct {
		name    string
		session TmuxSession
		want    bool
	}{
		{"agent process in pane", TmuxSession{Name: "alpha"}, true},
		{"agent UI markers only (ssh)", TmuxSession{Name: "remote", agentUI: true}, true},
		{"executor-named worker", TmuxSession{Name: "claude-code-glm-alpha"}, true},
		{"plain shell with scrolling output", TmuxSession{Name: "build", Status: StatusWorking}, false},
	}
	for _, tc := range cases {
		if got := hostsAgent(tc.session, withProcess); got != tc.want {
			t.Errorf("%s: hostsAgent = %v, want %v", tc.name, got, tc.want)
		}
	}
}
