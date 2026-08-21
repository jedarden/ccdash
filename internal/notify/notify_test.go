package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
