package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSessionTypeByExecutorName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name string
		want SessionType
	}{
		// These prefixes are the documented Claude Code and OpenCode worker
		// names. NEEDLE may add its own prefix to the full executor name.
		{name: "claude-code-glm-47-alpha", want: SessionTypeWorker},
		{name: "claude-code-sonnet-delta", want: SessionTypeWorker},
		{name: "opencode-glm-47-bravo", want: SessionTypeWorker},
		{name: "needle-claude-code-glm-4_7-alpha", want: SessionTypeWorker},
		{name: "needle-opencode-glm-47-charlie", want: SessionTypeWorker},
		{name: " CLAUDE-CODE-SONNET-ECHO ", want: SessionTypeWorker},

		// A worker prefix must start the name and include a suffix.
		{name: "alpha", want: SessionTypeInteractive},
		{name: "claude-code", want: SessionTypeInteractive},
		{name: "opencode", want: SessionTypeInteractive},
		{name: "needle-claude-code", want: SessionTypeInteractive},
		{name: "interactive-claude-code-sonnet", want: SessionTypeInteractive},
		{name: "my-opencode-glm-47-bravo", want: SessionTypeInteractive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectSessionType(tt.name); got != tt.want {
				t.Fatalf("DetectSessionType(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestHasWorkerLog(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, ".beads-workers")
	if err := os.Mkdir(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "custom-session.log")
	if err := os.WriteFile(logPath, []byte("worker log"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !hasWorkerLog(home, "custom-session") {
		t.Fatal("session with a worker log should be detected")
	}
	if hasWorkerLog(home, "interactive-session") {
		t.Fatal("session without a worker log should remain interactive")
	}
	if hasWorkerLog(home, "") {
		t.Fatal("empty session name must not match a worker log")
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(logPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if hasWorkerLog(home, "custom-session") {
		t.Fatal("a directory must not count as a worker log")
	}
}

func TestDetectSessionTypeFallsBackToWorkerLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	logDir := filepath.Join(home, ".beads-workers")
	if err := os.Mkdir(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "custom-session.log"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if got := DetectSessionType("custom-session"); got != SessionTypeWorker {
		t.Fatalf("DetectSessionType(custom-session) = %q, want worker", got)
	}
	if got := DetectSessionType("interactive-session"); got != SessionTypeInteractive {
		t.Fatalf("DetectSessionType(interactive-session) = %q, want interactive without a worker log", got)
	}
}
