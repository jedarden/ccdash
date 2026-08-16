package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
