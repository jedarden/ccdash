package metrics

import "testing"

func TestSessionStatusesPutAskingFirst(t *testing.T) {
	statuses := SessionStatuses()
	want := []SessionStatus{StatusAsking, StatusWorking, StatusReady, StatusActive, StatusError}
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
