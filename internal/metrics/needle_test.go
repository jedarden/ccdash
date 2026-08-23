package metrics

import (
	"testing"
	"time"
)

// TestNormalizeForTmux verifies that worker IDs are correctly normalized to match
// NEEDLE's tmux naming convention.
func TestNormalizeForTmux(t *testing.T) {
	tests := []struct {
		name     string
		workerID string
		want     string
	}{
		{
			name:     "dotted adapter version",
			workerID: "claude-code-glm-4.7-alpha",
			want:     "claude-code-glm-4_7-alpha",
		},
		{
			name:     "multiple dots in adapter",
			workerID: "claude-opus-4.8.0-beta",
			want:     "claude-opus-4_8_0-beta",
		},
		{
			name:     "no dots - unchanged",
			workerID: "claude-sonnet-worker",
			want:     "claude-sonnet-worker",
		},
		{
			name:     "dots at beginning and end",
			workerID: ".worker.",
			want:     "_worker_",
		},
		{
			name:     "consecutive dots",
			workerID: "adapter..1.2",
			want:     "adapter__1_2",
		},
		{
			name:     "empty string",
			workerID: "",
			want:     "",
		},
		{
			name:     "single dot",
			workerID: "a.b",
			want:     "a_b",
		},
		{
			name:     "real NEEDLE worker ID with glm-4.7",
			workerID: "claude-code-glm-4.7-delta",
			want:     "claude-code-glm-4_7-delta",
		},
		{
			name:     "print worker with dotted version",
			workerID: "claude-print-opus-4.8.0-alpha",
			want:     "claude-print-opus-4_8_0-alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeForTmux(tt.workerID)
			if got != tt.want {
				t.Errorf("normalizeForTmux(%q) = %q, want %q", tt.workerID, got, tt.want)
			}
		})
	}
}

// TestHasTmuxSessionFor verifies that worker IDs are correctly matched against
// tmux session names using NEEDLE's sanitized naming convention.
func TestHasTmuxSessionFor(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		workerID string
		sessions []TmuxSession
		want     bool
	}{
		{
			name:     "exact match with dotted adapter",
			workerID: "claude-code-glm-4.7-alpha",
			sessions: []TmuxSession{
				{Name: "needle-claude-code-glm-4_7-alpha", Created: now},
			},
			want: true,
		},
		{
			name:     "no match - different worker",
			workerID: "claude-code-glm-4.7-alpha",
			sessions: []TmuxSession{
				{Name: "needle-claude-sonnet-beta", Created: now},
			},
			want: false,
		},
		{
			name:     "match with multiple dots",
			workerID: "claude-opus-4.8.0-beta",
			sessions: []TmuxSession{
				{Name: "needle-claude-opus-4_8_0-beta", Created: now},
			},
			want: true,
		},
		{
			name:     "partial session name contains worker ID",
			workerID: "claude-code-glm-4.7-lab-roam-1",
			sessions: []TmuxSession{
				{Name: "needle-claude-code-glm-4_7-lab-roam-1-session", Created: now},
			},
			want: true,
		},
		{
			name:     "worker ID without dots matches tmux with underscores",
			workerID: "claude-sonnet-worker",
			sessions: []TmuxSession{
				{Name: "needle-claude-sonnet-worker", Created: now},
			},
			want: true,
		},
		{
			name:     "empty worker ID returns false",
			workerID: "",
			sessions: []TmuxSession{
				{Name: "needle-test-worker", Created: now},
			},
			want: false,
		},
		{
			name:     "empty sessions returns false",
			workerID: "claude-code-glm-4.7-alpha",
			sessions: []TmuxSession{},
			want:     false,
		},
		{
			name:     "dotted version does not match non-dotted session",
			workerID: "claude-glm-4.7",
			sessions: []TmuxSession{
				{Name: "needle-claude-glm-4-7", Created: now},
			},
			want: false,
		},
		{
			name:     "real-world case from the bug report",
			workerID: "claude-code-glm-4.7-delta",
			sessions: []TmuxSession{
				{Name: "needle-claude-code-glm-4_7-delta", Created: now},
				{Name: "some-other-session", Created: now},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTmuxSessionFor(tt.workerID, tt.sessions)
			if got != tt.want {
				t.Errorf("hasTmuxSessionFor(%q, sessions) = %v, want %v", tt.workerID, got, tt.want)
			}
		})
	}
}

// TestNeedleWorkerDisplayName verifies that display names strip the agent prefix.
func TestNeedleWorkerDisplayName(t *testing.T) {
	tests := []struct {
		name   string
		worker NeedleWorker
		want   string
	}{
		{
			name: "strips agent prefix from ID",
			worker: NeedleWorker{
				ID:        "claude-code-glm-4.7-alpha",
				Agent:     "claude-code",
				Workspace: "/home/coding/test",
			},
			want: "glm-4.7-alpha",
		},
		{
			name: "returns ID when agent doesn't match",
			worker: NeedleWorker{
				ID:        "some-other-worker",
				Agent:     "claude-code",
				Workspace: "/home/coding/test",
			},
			want: "some-other-worker",
		},
		{
			name: "returns ID when agent is empty",
			worker: NeedleWorker{
				ID:        "claude-code-worker",
				Agent:     "",
				Workspace: "/home/coding/test",
			},
			want: "claude-code-worker",
		},
		{
			name: "returns ID when trimmed result is empty",
			worker: NeedleWorker{
				ID:        "claude-code",
				Agent:     "claude-code",
				Workspace: "/home/coding/test",
			},
			want: "claude-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.worker.DisplayName()
			if got != tt.want {
				t.Errorf("NeedleWorker.DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNormalizeForNeedle verifies that names are normalized consistently for
// deduplication across hook, tmux, and NEEDLE registry sources.
func TestNormalizeForNeedle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tmux session name with needle prefix",
			input: "needle-claude-code-glm-4_7-alpha",
			want:  "claude-code-glm-4_7-alpha",
		},
		{
			name:  "worker ID with dots",
			input: "claude-code-glm-4.7-alpha",
			want:  "claude-code-glm-4_7-alpha",
		},
		{
			name:  "display name with dots",
			input: "glm-4.7-alpha",
			want:  "glm-4_7-alpha",
		},
		{
			name:  "multiple dots in adapter",
			input: "claude-opus-4.8.0-beta",
			want:  "claude-opus-4_8_0-beta",
		},
		{
			name:  "tmux session with multiple dots normalized",
			input: "needle-claude-opus-4_8_0-beta",
			want:  "claude-opus-4_8_0-beta",
		},
		{
			name:  "no dots - unchanged except needle prefix",
			input: "needle-claude-sonnet-worker",
			want:  "claude-sonnet-worker",
		},
		{
			name:  "worker ID without dots or prefix",
			input: "claude-sonnet-worker",
			want:  "claude-sonnet-worker",
		},
		{
			name:  "needle prefix without dots",
			input: "needle-alpha",
			want:  "alpha",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "just needle prefix",
			input: "needle-",
			want:  "",
		},
		{
			name:  "dots at beginning and end",
			input: ".worker.",
			want:  "_worker_",
		},
		{
			name:  "consecutive dots",
			input: "adapter..1.2",
			want:  "adapter__1_2",
		},
		{
			name:  "needle prefix with consecutive dots",
			input: "needle-adapter..1.2",
			want:  "adapter__1_2",
		},
		{
			name:  "real-world dotted adapter matching",
			input: "glm-4.7-alpha",
			want:  "glm-4_7-alpha",
		},
		{
			name:  "real-world tmux session name",
			input: "needle-glm-4_7-alpha",
			want:  "glm-4_7-alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeForNeedle(tt.input)
			if got != tt.want {
				t.Errorf("normalizeForNeedle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSessionDeduplicationWithDottedNames verifies that sessions with dotted
// adapter names are correctly deduplicated when merging from multiple sources.
func TestSessionDeduplicationWithDottedNames(t *testing.T) {
	now := time.Now()

	// Simulate the three data sources:
	// 1. Hook sessions use display names (may have dots)
	// 2. Tmux sessions use sanitized names (underscores, needle- prefix)
	// 3. NEEDLE workers use display names (may have dots)
	hookSessions := []TmuxSession{
		{Name: "glm-4.7-alpha", Source: "hooks", Created: now},
	}
	tmuxSessions := []TmuxSession{
		{Name: "needle-glm-4_7-alpha", Source: "tmux", Created: now},
	}
	needleSessions := []TmuxSession{
		{Name: "glm-4.7-alpha", Source: "needle", Created: now},
	}

	// Build the seenNames map as the merge logic does
	seenNames := make(map[string]bool)

	// First pass: hook sessions that have corresponding tmux sessions
	for _, session := range hookSessions {
		_, exists := map[string]TmuxSession{
			"glm-4.7-alpha": {Name: "needle-glm-4_7-alpha", Created: now},
		}[session.Name]
		if !exists {
			continue
		}
		normalizedKey := normalizeForNeedle(session.Name)
		seenNames[normalizedKey] = true
	}

	// Second pass: tmux sessions not tracked by hooks
	for _, session := range tmuxSessions {
		normalizedKey := normalizeForNeedle(session.Name)
		if !seenNames[normalizedKey] {
			seenNames[normalizedKey] = true
		}
	}

	// Third pass: NEEDLE workers
	needledSessionAdded := false
	for _, session := range needleSessions {
		normalizedKey := normalizeForNeedle(session.Name)
		if !seenNames[normalizedKey] {
			needledSessionAdded = true
			seenNames[normalizedKey] = true
		}
	}

	// Verify that all three sources resolve to the same normalized key
	hookKey := normalizeForNeedle("glm-4.7-alpha")
	tmuxKey := normalizeForNeedle("needle-glm-4_7-alpha")
	needleKey := normalizeForNeedle("glm-4.7-alpha")

	if hookKey != tmuxKey {
		t.Errorf("Hook key %q != tmux key %q", hookKey, tmuxKey)
	}
	if tmuxKey != needleKey {
		t.Errorf("Tmux key %q != needle key %q", tmuxKey, needleKey)
	}

	// The key should exist in seenNames
	if !seenNames[hookKey] {
		t.Errorf("Expected key %q not found in seenNames", hookKey)
	}

	// NEEDLE session should not have been added (already seen via hook/tmux)
	if needledSessionAdded {
		t.Errorf("NEEDLE session was added despite existing hook/tmux session")
	}

	// Should only have one unique session
	if len(seenNames) != 1 {
		t.Errorf("Expected 1 unique session, got %d", len(seenNames))
	}
}
