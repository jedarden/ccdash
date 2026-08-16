package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Payload represents the notification payload sent to the webhook
// Contains only session metadata, never token/cost data
type Payload struct {
	SessionName    string        `json:"session_name"`
	ProjectDir     string        `json:"project_dir"`
	IdleDuration   time.Duration `json:"idle_duration"`
	Timestamp      time.Time     `json:"timestamp"`
}

// Client handles sending notifications to the configured webhook
type Client struct {
	webhookURL string
	httpClient *http.Client
	enabled    bool
}

// NewClient creates a new notification client
func NewClient(webhookURL string, enabled bool) *Client {
	return &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		enabled: enabled,
	}
}

// Send sends a notification payload to the webhook
// Silent-fail on errors: logs a warning but never returns an error
func (c *Client) Send(payload *Payload) {
	// If notifications are disabled, do nothing
	if !c.enabled || c.webhookURL == "" {
		return
	}

	// Marshal the payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		c.logWarning("Failed to marshal notification payload: %v", err)
		return
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", c.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		c.logWarning("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logWarning("Failed to send notification: %v", err)
		return
	}
	defer resp.Body.Close()

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logWarning("Notification webhook returned status %d", resp.StatusCode)
	}
}

// logWarning logs a warning message to ccdash's log
// This is a one-line log as specified in the requirements
func (c *Client) logWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf("[notify] "+format, args...)
	log.Println(msg)

	// Also log to a dedicated file for debugging
	// Use the same directory as the config
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	logFile := filepath.Join(homeDir, ".ccdash", "notify.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	f.WriteString(fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), msg))
}

// SessionTransition represents a session status change that may trigger a notification
type SessionTransition struct {
	SessionName  string
	ProjectDir   string
	OldStatus    string
	NewStatus    string
	IdleDuration time.Duration
}

// ShouldNotify determines if a transition should trigger a notification
// Only notify on transitions INTO waiting/asking states that persist
func (c *Client) ShouldNotify(transition *SessionTransition) bool {
	if !c.enabled {
		return false
	}

	// Only notify on transitions into waiting/asking
	if transition.NewStatus != "waiting" && transition.NewStatus != "asking" {
		return false
	}

	// Don't notify if already in waiting/asking (no transition)
	if transition.OldStatus == "waiting" || transition.OldStatus == "asking" {
		return false
	}

	// Don't notify on transitions from working (already handled by debounce)
	// The caller should handle debouncing before calling this

	return true
}

// TestResult contains the result of a test notification
type TestResult struct {
	Success    bool
	StatusCode int
	Error      string
}

// SendTest sends a test notification and returns detailed error information
// Unlike Send(), this method returns errors and status codes for debugging
func (c *Client) SendTest(payload *Payload) TestResult {
	// If notifications are disabled, return error
	if !c.enabled || c.webhookURL == "" {
		return TestResult{
			Success:    false,
			StatusCode: 0,
			Error:      "notifications are disabled or webhook_url is not configured",
		}
	}

	// Marshal the payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return TestResult{
			Success:    false,
			StatusCode: 0,
			Error:      fmt.Sprintf("failed to marshal payload: %v", err),
		}
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", c.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return TestResult{
			Success:    false,
			StatusCode: 0,
			Error:      fmt.Sprintf("failed to create request: %v", err),
		}
	}

	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TestResult{
			Success:    false,
			StatusCode: 0,
			Error:      fmt.Sprintf("HTTP request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	// Return the result
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	var resultError string
	if !success {
		resultError = fmt.Sprintf("webhook returned HTTP status %d", resp.StatusCode)
	}
	return TestResult{
		Success:    success,
		StatusCode: resp.StatusCode,
		Error:      resultError,
	}
}
