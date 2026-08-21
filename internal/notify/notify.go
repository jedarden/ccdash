// Package notify sends optional notifications when a hook-tracked session
// needs human input.
package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
)

const (
	// DebounceWindow is how long a waiting/asking state must persist before a
	// notification is emitted.
	DebounceWindow = 15 * time.Second
	webhookTimeout = 5 * time.Second
)

// Payload is the complete webhook payload. Keep this deliberately small: it
// contains no transcript, token, or cost information.
type Payload struct {
	SessionName  string        `json:"session_name"`
	ProjectDir   string        `json:"project_dir"`
	IdleDuration time.Duration `json:"idle_duration"`
	// Timestamp is retained for source compatibility with the initial CLI
	// diagnostic payload, but is intentionally omitted from webhook JSON.
	Timestamp time.Time `json:"-"`
}

// SessionTransition describes a status change for callers that want to make
// the transition decision without using Tracker.
type SessionTransition struct {
	SessionName  string
	ProjectDir   string
	OldStatus    string
	NewStatus    string
	IdleDuration time.Duration
}

// Client posts notification payloads to a configured webhook.
type Client struct {
	webhookURL string
	httpClient *http.Client
	enabled    bool
}

// NewClient creates a notification client. A disabled client never makes an
// HTTP request, even when a webhook URL is present.
func NewClient(webhookURL string, enabled bool) *Client {
	return &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: webhookTimeout},
		enabled:    enabled,
	}
}

// Send posts a notification and logs one warning on failure. It intentionally
// does not return an error: notification delivery must never affect the
// dashboard. Callers that must not block their refresh loop should invoke it
// in a goroutine.
func (c *Client) Send(payload *Payload) {
	if c == nil || !c.enabled {
		return
	}

	status, err := c.post(payload)
	if err != nil {
		if status != 0 {
			c.logWarning("webhook returned HTTP status %d", status)
			return
		}
		c.logWarning("webhook request failed")
	}
}

// TestResult contains the result of a test notification.
type TestResult struct {
	Success    bool
	StatusCode int
	Error      string
}

// SendTest posts a notification and returns details for the explicit CLI
// diagnostic command. Unlike Send, this method is allowed to return errors.
func (c *Client) SendTest(payload *Payload) TestResult {
	if c == nil || !c.enabled {
		return TestResult{Error: "notifications are disabled or webhook_url is not configured"}
	}

	status, err := c.post(payload)
	if err != nil {
		return TestResult{
			StatusCode: status,
			Error:      err.Error(),
		}
	}

	return TestResult{Success: true, StatusCode: status}
}

// ShouldNotify reports whether a transition enters a waiting/asking state.
// Tracker is preferred because it also handles persistence and deduplication;
// this method remains useful to small integrations and preserves the client
// API used by earlier releases.
func (c *Client) ShouldNotify(transition *SessionTransition) bool {
	if c == nil || !c.enabled || transition == nil {
		return false
	}
	return isWaitingStatus(strings.ToLower(strings.TrimSpace(transition.NewStatus))) &&
		!isWaitingStatus(strings.ToLower(strings.TrimSpace(transition.OldStatus)))
}

var errInvalidWebhook = errors.New("invalid webhook configuration")

func (c *Client) post(payload *Payload) (int, error) {
	if strings.TrimSpace(c.webhookURL) == "" {
		return 0, errInvalidWebhook
	}
	if !validWebhookURL(c.webhookURL) {
		return 0, errInvalidWebhook
	}
	if payload == nil {
		return 0, errors.New("notification payload is nil")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return 0, errInvalidWebhook
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		// Do not include the URL in the returned error: webhook URLs commonly
		// contain bearer tokens or other credentials.
		return 0, errors.New("webhook request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.StatusCode, fmt.Errorf("webhook returned HTTP status %d", response.StatusCode)
	}

	return response.StatusCode, nil
}

func validWebhookURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func (c *Client) logWarning(format string, args ...interface{}) {
	log.Printf("[notify] warning: "+format, args...)
}

// Tracker turns raw hook-session snapshots into one-shot notification
// payloads. It tracks session IDs rather than display names so two sessions
// using the same project directory cannot collide.
type Tracker struct {
	previous     map[string]string
	waitingSince map[string]time.Time
	notified     map[string]bool
	debounce     time.Duration
	now          func() time.Time
}

// NewTracker creates a debouncer using the supplied duration. Non-positive
// durations use the ADR's default window.
func NewTracker(debounce time.Duration) *Tracker {
	if debounce <= 0 {
		debounce = DebounceWindow
	}
	return &Tracker{
		previous:     make(map[string]string),
		waitingSince: make(map[string]time.Time),
		notified:     make(map[string]bool),
		debounce:     debounce,
		now:          time.Now,
	}
}

// Update consumes the current raw HookSession snapshot and returns any
// notifications whose waiting/asking state has persisted through the debounce
// window. The first snapshot of an already-waiting session is treated as the
// beginning of observation unless LastActivity provides an earlier, usable
// status timestamp.
func (t *Tracker) Update(sessions []metrics.HookSession) []Payload {
	if t == nil {
		return nil
	}

	now := t.now()
	seen := make(map[string]struct{}, len(sessions))
	var payloads []Payload

	for _, session := range sessions {
		key := sessionKey(session)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}

		status := strings.ToLower(strings.TrimSpace(session.Status))
		previous, hadPrevious := t.previous[key]
		waiting := isWaitingStatus(status)

		if waiting {
			if !hadPrevious || !isWaitingStatus(previous) {
				t.waitingSince[key] = now
				t.notified[key] = false
				if !hadPrevious && !session.LastActivity.IsZero() && session.LastActivity.Before(now) {
					t.waitingSince[key] = session.LastActivity
				}
			} else if _, ok := t.waitingSince[key]; !ok {
				t.waitingSince[key] = now
			}

			if !t.notified[key] && !t.waitingSince[key].Add(t.debounce).After(now) {
				idleDuration := now.Sub(session.LastActivity)
				if session.LastActivity.IsZero() {
					idleDuration = now.Sub(t.waitingSince[key])
				}
				if idleDuration < 0 {
					idleDuration = 0
				}

				payloads = append(payloads, Payload{
					SessionName:  sessionName(session),
					ProjectDir:   session.ProjectDir,
					IdleDuration: idleDuration,
				})
				t.notified[key] = true
			}
		} else {
			delete(t.waitingSince, key)
			delete(t.notified, key)
		}

		t.previous[key] = status
	}

	for key := range t.previous {
		if _, ok := seen[key]; !ok {
			delete(t.previous, key)
			delete(t.waitingSince, key)
			delete(t.notified, key)
		}
	}

	return payloads
}

func isWaitingStatus(status string) bool {
	return status == "waiting" || status == "asking"
}

func sessionKey(session metrics.HookSession) string {
	if session.SessionID != "" {
		return session.SessionID
	}
	return sessionName(session)
}

func sessionName(session metrics.HookSession) string {
	if session.TmuxSessionName != "" {
		return session.TmuxSessionName
	}
	if session.ProjectDir != "" {
		name := filepath.Base(session.ProjectDir)
		if name != "." && name != string(filepath.Separator) {
			return name
		}
	}
	return session.SessionID
}
