package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
)

func TestSendTestPostsJSONAndReportsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		var payload Payload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if payload.SessionName != "ccdash-test" {
			t.Errorf("session_name = %q, want ccdash-test", payload.SessionName)
		}
		if payload.IdleDuration != 30*time.Second {
			t.Errorf("idle_duration = %s, want 30s", payload.IdleDuration)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result := NewClient(server.URL, true).SendTest(&Payload{
		SessionName:  "ccdash-test",
		IdleDuration: 30 * time.Second,
		Timestamp:    time.Now(),
	})

	if !result.Success {
		t.Fatalf("SendTest() failed: %+v", result)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", result.StatusCode, http.StatusNoContent)
	}
}

func TestSendTestReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer server.Close()

	result := NewClient(server.URL, true).SendTest(&Payload{})

	if result.Success {
		t.Fatal("SendTest() succeeded for a 502 response")
	}
	if result.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", result.StatusCode, http.StatusBadGateway)
	}
	if !strings.Contains(result.Error, "502") {
		t.Errorf("error = %q, want HTTP status", result.Error)
	}
}

func TestSendTestReportsTransportFailure(t *testing.T) {
	result := NewClient("http://127.0.0.1:1", true).SendTest(&Payload{})

	if result.Success {
		t.Fatal("SendTest() succeeded for an unreachable endpoint")
	}
	if result.StatusCode != 0 {
		t.Errorf("status = %d, want 0 for transport failure", result.StatusCode)
	}
	if result.Error == "" {
		t.Error("transport failure returned no error")
	}
}

func TestSendWebhookFailureDoesNotStopLaterNotifications(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	client.Send(&Payload{SessionName: "first"})
	client.Send(&Payload{SessionName: "second"})

	if got := requests.Load(); got != 2 {
		t.Fatalf("webhook requests = %d, want 2 after a failed first delivery", got)
	}
}

func TestShouldNotifyDetectsOnlyEntryIntoWaitingState(t *testing.T) {
	client := NewClient("https://example.test/webhook", true)
	tests := []struct {
		name string
		old  string
		new  string
		want bool
	}{
		{name: "working to waiting", old: "working", new: "waiting", want: true},
		{name: "active to asking", old: "active", new: "asking", want: true},
		{name: "normalized states", old: " WORKING ", new: " ASKING ", want: true},
		{name: "waiting to asking", old: "waiting", new: "asking"},
		{name: "asking to waiting", old: "asking", new: "waiting"},
		{name: "waiting to working", old: "waiting", new: "working"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.ShouldNotify(&SessionTransition{OldStatus: tt.old, NewStatus: tt.new})
			if got != tt.want {
				t.Fatalf("ShouldNotify(%q -> %q) = %t, want %t", tt.old, tt.new, got, tt.want)
			}
		})
	}

	if got := NewClient("https://example.test/webhook", false).ShouldNotify(&SessionTransition{NewStatus: "waiting"}); got {
		t.Fatal("disabled client should not notify")
	}
	if got := client.ShouldNotify(nil); got {
		t.Fatal("nil transition should not notify")
	}
}

func TestPayloadContainsOnlyNotificationFields(t *testing.T) {
	payload, err := json.Marshal(Payload{
		SessionName:  "build",
		ProjectDir:   "/work/project",
		IdleDuration: 15 * time.Second,
		Timestamp:    time.Now(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("payload fields = %d, want 3: %s", len(fields), payload)
	}
	for _, field := range []string{"session_name", "project_dir", "idle_duration"} {
		if _, ok := fields[field]; !ok {
			t.Errorf("payload missing %q", field)
		}
	}
	if _, ok := fields["timestamp"]; ok {
		t.Error("payload unexpectedly contains timestamp")
	}
}

func TestDisabledClientDoesNotMakeNetworkCalls(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
	}))
	defer server.Close()

	NewClient(server.URL, false).Send(&Payload{SessionName: "disabled"})
	select {
	case <-requests:
		t.Fatal("disabled client made a network request")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTrackerDebouncesHookSessionEscalation(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	tracker := NewTracker(15 * time.Second)
	tracker.now = func() time.Time { return now }

	session := metrics.HookSession{
		SessionID:       "session-1",
		TmuxSessionName: "build",
		ProjectDir:      "/work/project",
		LastActivity:    now,
		Status:          "working",
	}
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("working snapshot produced %d notifications", len(got))
	}

	now = now.Add(1 * time.Second)
	session.Status = "waiting"
	session.LastActivity = now
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("new waiting snapshot produced %d notifications", len(got))
	}

	now = now.Add(15 * time.Second)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 1 {
		t.Fatalf("debounced waiting snapshot produced %d notifications, want 1", len(got))
	} else {
		if got[0].SessionName != "build" || got[0].ProjectDir != "/work/project" {
			t.Errorf("payload identity = %+v", got[0])
		}
		if got[0].IdleDuration != 15*time.Second {
			t.Errorf("idle duration = %s, want 15s", got[0].IdleDuration)
		}
	}

	now = now.Add(time.Minute)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("persistent waiting snapshot produced %d duplicate notifications", len(got))
	}
}

func TestTrackerDoesNotNotifyBeforeDebounceBoundary(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	const debounce = 15 * time.Second
	tracker := NewTracker(debounce)
	tracker.now = func() time.Time { return now }
	session := metrics.HookSession{
		SessionID:    "session-boundary",
		ProjectDir:   "/work/project",
		LastActivity: now,
		Status:       "waiting",
	}

	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("initial waiting snapshot produced %d notifications", len(got))
	}
	now = now.Add(debounce - time.Nanosecond)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("snapshot just before debounce produced %d notifications", len(got))
	}
	now = now.Add(time.Nanosecond)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 1 {
		t.Fatalf("snapshot at debounce boundary produced %d notifications, want 1", len(got))
	}
}

func TestTrackerTreatsRecoveryAsSilentAndStartsANewDebounce(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	const debounce = 10 * time.Second
	tracker := NewTracker(debounce)
	tracker.now = func() time.Time { return now }
	session := metrics.HookSession{SessionID: "session-recovery", Status: "waiting", LastActivity: now}

	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("initial waiting snapshot produced %d notifications", len(got))
	}
	now = now.Add(9 * time.Second)
	session.Status = "working"
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("recovery transition produced %d notifications", len(got))
	}

	// A fresh wait after recovery must start a fresh debounce interval.
	now = now.Add(time.Second)
	session.Status = "waiting"
	session.LastActivity = now
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("new waiting transition produced %d notifications", len(got))
	}
	now = now.Add(debounce - time.Nanosecond)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("new wait just before debounce produced %d notifications", len(got))
	}
	now = now.Add(time.Nanosecond)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 1 {
		t.Fatalf("new wait at debounce boundary produced %d notifications, want 1", len(got))
	}

	// Recovery after delivery is silent, and a subsequent wait is a new event.
	now = now.Add(time.Second)
	session.Status = "active"
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("post-notification recovery produced %d notifications", len(got))
	}
	now = now.Add(time.Second)
	session.Status = "asking"
	session.LastActivity = now
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 0 {
		t.Fatalf("re-escalation produced %d notifications before debounce", len(got))
	}
	now = now.Add(debounce)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 1 {
		t.Fatalf("re-escalation produced %d notifications at debounce boundary, want 1", len(got))
	}
}

func TestTrackerDeduplicatesBySessionIDWithoutCollidingOnProject(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	tracker := NewTracker(time.Second)
	tracker.now = func() time.Time { return now }
	sessions := []metrics.HookSession{
		{SessionID: "session-one", TmuxSessionName: "build", ProjectDir: "/work/project", Status: "waiting", LastActivity: now},
		{SessionID: "session-two", TmuxSessionName: "build", ProjectDir: "/work/project", Status: "waiting", LastActivity: now},
	}
	if got := tracker.Update(sessions); len(got) != 0 {
		t.Fatalf("initial snapshot produced %d notifications", len(got))
	}
	now = now.Add(time.Second)
	if got := tracker.Update(sessions); len(got) != 2 {
		t.Fatalf("two distinct session IDs in one project produced %d notifications, want 2", len(got))
	}
	now = now.Add(time.Minute)
	if got := tracker.Update(sessions); len(got) != 0 {
		t.Fatalf("persistent waiting sessions produced %d duplicate notifications", len(got))
	}
}

func TestTrackerResetsAfterWaitingStateEnds(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	tracker := NewTracker(time.Second)
	tracker.now = func() time.Time { return now }

	session := metrics.HookSession{SessionID: "session-1", ProjectDir: "/work/project", Status: "working", LastActivity: now}
	tracker.Update([]metrics.HookSession{session})
	session.Status = "asking"
	session.LastActivity = now
	tracker.Update([]metrics.HookSession{session})
	now = now.Add(time.Second)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 1 {
		t.Fatalf("asking snapshot produced %d notifications, want 1", len(got))
	}

	session.Status = "working"
	now = now.Add(time.Second)
	tracker.Update([]metrics.HookSession{session})
	session.Status = "waiting"
	session.LastActivity = now
	tracker.Update([]metrics.HookSession{session})
	now = now.Add(time.Second)
	if got := tracker.Update([]metrics.HookSession{session}); len(got) != 1 {
		t.Fatalf("re-escalated session produced %d notifications, want 1", len(got))
	}
}
