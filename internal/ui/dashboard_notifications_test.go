package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
	"github.com/jedarden/ccdash/internal/notify"
)

func TestDashboardDeliversHookNotificationsOnlyForLeaderSnapshots(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload notify.Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode webhook payload: %v", err)
			return
		}
		requests <- payload.SessionName
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	d := &Dashboard{
		notifyClient:        notify.NewClient(server.URL, true),
		notificationTracker: notify.NewTracker(time.Nanosecond),
	}
	session := metrics.HookSession{SessionID: "session-1", Status: "waiting"}
	snapshot := func(isLeader bool) {
		t.Helper()
		d.Update(metricsMsg{
			isLeader:              isLeader,
			hookSessionsCollected: true,
			hookSessions:          []metrics.HookSession{session},
		})
	}
	assertNoRequest := func(duration time.Duration) {
		t.Helper()
		select {
		case got := <-requests:
			t.Fatalf("webhook received session %q before leader delivery", got)
		case <-time.After(duration):
		}
	}

	// A nonleader seeing a waiting session must not advance the notification
	// state machine. Otherwise the first leader snapshot could deliver it at once.
	snapshot(false)
	time.Sleep(3 * time.Millisecond)
	snapshot(true)
	assertNoRequest(20 * time.Millisecond)

	time.Sleep(3 * time.Millisecond)
	snapshot(true)
	select {
	case got := <-requests:
		if got != "session-1" {
			t.Errorf("webhook session_name = %q, want session-1", got)
		}
	case <-time.After(time.Second):
		t.Fatal("leader snapshot did not deliver the debounced notification")
	}

	// Persistent waiting on the leader is one-shot; it does not duplicate sends.
	snapshot(true)
	assertNoRequest(20 * time.Millisecond)
}
