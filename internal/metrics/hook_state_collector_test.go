package metrics

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHookSessionCollectorMapsClaudeAndCodexEvents(t *testing.T) {
	harnesses := []struct {
		name         string
		start        string
		prompt       string
		postTool     string
		permission   string
		notification string
		stop         string
		end          string
	}{
		{
			name: "claude", start: "session-start.sh", prompt: "prompt-submit.sh",
			postTool: "post-tool-use.sh", permission: "permission-request.sh",
			notification: "notification.sh", stop: "stop.sh", end: "session-end.sh",
		},
		{
			name: "codex", start: "codex-session-start.sh", prompt: "codex-prompt-submit.sh",
			postTool: "codex-post-tool-use.sh", permission: "codex-permission-request.sh",
			stop: "codex-stop.sh", end: "codex-session-end.sh",
		},
	}

	for _, harness := range harnesses {
		t.Run(harness.name, func(t *testing.T) {
			home := t.TempDir()
			collector := newTestHookSessionCollector(t, home)
			if harness.name == "codex" {
				installer := newCodexHookInstaller(home)
				if err := installer.InstallHooks(); err != nil {
					t.Fatalf("install Codex hooks: %v", err)
				}
			}

			input := `{"session_id":"hook-state-test","cwd":"/tmp/project"}`
			runHarnessHook(t, harness.name, home, harness.start, input)
			assertCollectedHookStatus(t, collector, StatusActive, harness.name)

			runHarnessHook(t, harness.name, home, harness.prompt, input)
			assertCollectedHookStatus(t, collector, StatusWorking, harness.name)

			runHarnessHook(t, harness.name, home, harness.permission, input)
			assertCollectedHookStatus(t, collector, StatusAsking, harness.name)

			// PreToolUse only refreshes activity; the request remains visible until
			// the harness reports that a tool completed and work resumed.
			preTool := "pre-tool-use.sh"
			if harness.name == "codex" {
				preTool = "codex-pre-tool-use.sh"
			}
			runHarnessHook(t, harness.name, home, preTool, input)
			assertCollectedHookStatus(t, collector, StatusAsking, harness.name)

			runHarnessHook(t, harness.name, home, harness.postTool, input)
			assertCollectedHookStatus(t, collector, StatusWorking, harness.name)

			if harness.notification != "" {
				runHarnessHook(t, harness.name, home, harness.notification, input)
				assertCollectedHookStatus(t, collector, StatusAsking, harness.name)
			}

			runHarnessHook(t, harness.name, home, harness.stop, input)
			assertCollectedHookStatus(t, collector, StatusReady, harness.name)

			runHarnessHook(t, harness.name, home, harness.end, input)
			sessions, err := collector.CollectSessions()
			if err != nil {
				t.Fatalf("collect after SessionEnd: %v", err)
			}
			if len(sessions) != 0 {
				t.Fatalf("SessionEnd left sessions in collector: %+v", sessions)
			}
		})
	}
}

func TestHookSessionCollectorHandlesStaleAndMalformedFiles(t *testing.T) {
	collector := newTestHookSessionCollector(t, t.TempDir())
	old := time.Now().Add(-StaleSessionThreshold - time.Minute)
	fixtures := []HookSession{
		{SessionID: "old-working", Source: "claude", ProjectDir: "/tmp/working", LastActivity: old, Status: "working"},
		{SessionID: "old-stopped", Source: "claude", ProjectDir: "/tmp/stopped", LastActivity: old, Status: "stopped"},
		{SessionID: "old-waiting", Source: "codex", ProjectDir: "/tmp/waiting", LastActivity: old, Status: "waiting"},
	}
	for _, fixture := range fixtures {
		writeHookSessionFixture(t, collector, fixture)
	}
	if err := os.WriteFile(filepath.Join(collector.sessionsDir, "malformed.json"), []byte(`{"session_id":`), 0600); err != nil {
		t.Fatal(err)
	}

	sessions, err := collector.CollectSessions()
	if err != nil {
		t.Fatalf("collect sessions with malformed file: %v", err)
	}
	if len(sessions) != len(fixtures) {
		t.Fatalf("collector returned %d sessions, want %d (malformed file skipped): %+v", len(sessions), len(fixtures), sessions)
	}
	got := make(map[string]SessionStatus, len(sessions))
	for i := range sessions {
		got[sessions[i].SessionID] = sessions[i].ToTmuxSession().Status
	}
	want := map[string]SessionStatus{
		"old-working": StatusWorking,
		"old-stopped": StatusReady,
		"old-waiting": StatusReady,
	}
	for id, status := range want {
		if got[id] != status {
			t.Errorf("%s status = %s, want %s", id, got[id], status)
		}
	}
}

func TestHookSessionStateTakesPrecedenceOverTmuxInspection(t *testing.T) {
	binDir := t.TempDir()
	fakeTmux := "#!/bin/sh\ncase \"$1\" in\n" +
		"  list-sessions) printf 'project:1:0:1780000000\\n' ;;\n" +
		"  capture-pane) printf 'Claude Code\\n❯\\n' ;;\n" +
		"  *) exit 2 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(fakeTmux), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	hookCollector := newTestHookSessionCollector(t, t.TempDir())
	writeHookSessionFixture(t, hookCollector, HookSession{
		SessionID: "asking-session", Source: "codex", ProjectDir: "/tmp/project",
		TmuxSessionName: "project", StartedAt: time.Now().Add(-time.Minute),
		LastActivity: time.Now(), Status: "waiting",
	})
	tmuxCollector := &TmuxCollector{
		hookCollector:       hookCollector,
		needleCollector:     &NeedleCollector{registryPath: filepath.Join(t.TempDir(), "missing-workers.json")},
		sessionActivityMap:  make(map[string]time.Time),
		sessionContentCache: make(map[string]string),
	}

	tmuxSessions, err := tmuxCollector.listSessions()
	if err != nil {
		t.Fatalf("inspect fake tmux session: %v", err)
	}
	if len(tmuxSessions) != 1 || tmuxSessions[0].Status != StatusActive {
		t.Fatalf("fake tmux should report ACTIVE, got %+v", tmuxSessions)
	}

	metrics := tmuxCollector.Collect()
	if len(metrics.Sessions) != 1 {
		t.Fatalf("Collect returned %d sessions, want 1: %+v", len(metrics.Sessions), metrics.Sessions)
	}
	if metrics.Sessions[0].Status != StatusAsking || metrics.Sessions[0].Harness != "codex" {
		t.Fatalf("hook state was replaced by tmux inspection: got %+v, want Codex ASKING", metrics.Sessions[0])
	}
}

func newTestHookSessionCollector(t *testing.T, home string) *HookSessionCollector {
	t.Helper()
	collector := &HookSessionCollector{
		baseDir:     filepath.Join(home, HooksDir),
		sessionsDir: filepath.Join(home, HooksDir, SessionsSubdir),
	}
	if err := collector.EnsureDirectories(); err != nil {
		t.Fatalf("create hook collector directories: %v", err)
	}
	return collector
}

func runHarnessHook(t *testing.T, harness, home, name, input string) {
	t.Helper()
	if harness == "codex" {
		installer := newCodexHookInstaller(home)
		runCodexStatusHook(t, installer, name, input, "")
		return
	}
	script, ok := HookScripts[name]
	if !ok {
		t.Fatalf("unknown Claude hook %q", name)
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "HOME="+home, "TMUX=")
	cmd.Stdin = strings.NewReader(input)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Claude hook %s failed: %v: %s", name, err, output)
	}
}

func assertCollectedHookStatus(t *testing.T, collector *HookSessionCollector, want SessionStatus, harness string) {
	t.Helper()
	sessions, err := collector.CollectSessions()
	if err != nil {
		t.Fatalf("collect hook sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("collector returned %d sessions, want 1: %+v", len(sessions), sessions)
	}
	session := sessions[0]
	if session.Source != harness {
		t.Errorf("session source = %q, want %q", session.Source, harness)
	}
	if got := session.ToTmuxSession().Status; got != want {
		t.Errorf("%s hook status %q mapped to %s, want %s", harness, session.Status, got, want)
	}
}

func writeHookSessionFixture(t *testing.T, collector *HookSessionCollector, session HookSession) {
	t.Helper()
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collector.sessionsDir, session.SessionID+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
