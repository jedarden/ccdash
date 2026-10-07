package metrics

import (
	"testing"
	"time"
)

func TestSessionStatusesPutAskingFirst(t *testing.T) {
	statuses := SessionStatuses()
	want := []SessionStatus{StatusAsking, StatusWorking, StatusWaitingExternal, StatusResumable, StatusComplete, StatusPaused, StatusReady, StatusActive, StatusError}
	if len(statuses) != len(want) {
		t.Fatalf("SessionStatuses() has %d entries, want %d", len(statuses), len(want))
	}
	for i := range want {
		if statuses[i] != want[i] {
			t.Errorf("SessionStatuses()[%d] = %s, want %s", i, statuses[i], want[i])
		}
	}
}

func TestSessionStatusAttentionPresentation(t *testing.T) {
	if got := StatusAsking.GetEmoji(); got != "🟣" {
		t.Errorf("ASKING emoji = %q, want purple circle", got)
	}
	if got := StatusAsking.GetColor(); got != "\033[95m" {
		t.Errorf("ASKING color = %q, want bright magenta", got)
	}
	if got := StatusReady.GetEmoji(); got == "🔴" {
		t.Errorf("READY still uses urgent red emoji: %q", got)
	}
	if got := StatusReady.GetColor(); got == "\033[31m" || got == "\033[91m" {
		t.Errorf("READY still uses urgent red color: %q", got)
	}
	if got := StatusReady.Description(); got != "Idle and waiting for the next prompt" {
		t.Errorf("READY description = %q", got)
	}
}

func TestSortSessionsByAttention(t *testing.T) {
	sessions := []TmuxSession{
		{Name: "zulu", Status: StatusReady},
		{Name: "bravo", Status: StatusWorking},
		{Name: "alpha", Status: StatusAsking},
		{Name: "charlie", Status: StatusActive},
		{Name: "echo", Status: StatusAsking},
	}

	sortSessionsByAttention(sessions)

	want := []string{"alpha", "echo", "bravo", "charlie", "zulu"}
	for i, name := range want {
		if sessions[i].Name != name {
			t.Errorf("sessions[%d] = %q, want %q", i, sessions[i].Name, name)
		}
	}
}

func TestFirstPaneSampleIsBaselineNotActivity(t *testing.T) {
	tc := &TmuxCollector{
		sessionActivityMap:  make(map[string]time.Time),
		sessionContentCache: make(map[string]string),
	}
	now := time.Now()
	session := TmuxSession{Name: "build", Created: now.Add(-time.Hour)}

	first := tc.statusFromContent(session, "make: compiling 1/10\n", now)
	if first.Status == StatusWorking {
		t.Fatalf("first sample status = %s, want not WORKING: there is nothing to compare it with", first.Status)
	}

	unchanged := tc.statusFromContent(session, "make: compiling 1/10\n", now.Add(2*time.Second))
	if unchanged.Status == StatusWorking {
		t.Fatalf("unchanged second sample status = %s, want not WORKING", unchanged.Status)
	}

	changed := tc.statusFromContent(session, "make: compiling 2/10\n", now.Add(4*time.Second))
	if changed.Status != StatusWorking {
		t.Fatalf("changed sample status = %s, want WORKING", changed.Status)
	}
}
