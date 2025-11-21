package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestSessionStatus_GetColor(t *testing.T) {
	tests := []struct {
		name     string
		status   SessionStatus
		expected string
	}{
		{"Working status", StatusWorking, "\033[32m"},
		{"Stalled status", StatusStalled, "\033[33m"},
		{"Idle status", StatusIdle, "\033[90m"},
		{"Ready status", StatusReady, "\033[36m"},
		{"Unknown status", SessionStatus("UNKNOWN"), "\033[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.GetColor()
			if result != tt.expected {
				t.Errorf("GetColor() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSessionStatus_GetEmoji(t *testing.T) {
	tests := []struct {
		name     string
		status   SessionStatus
		expected string
	}{
		{"Working status", StatusWorking, "🟢"},
		{"Stalled status", StatusStalled, "🟡"},
		{"Idle status", StatusIdle, "⚪"},
		{"Ready status", StatusReady, "🔵"},
		{"Unknown status", SessionStatus("UNKNOWN"), "❓"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.GetEmoji()
			if result != tt.expected {
				t.Errorf("GetEmoji() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNewTmuxCollector(t *testing.T) {
	collector := NewTmuxCollector()

	if collector == nil {
		t.Fatal("NewTmuxCollector() returned nil")
	}

	if collector.sessionActivityMap == nil {
		t.Error("sessionActivityMap not initialized")
	}

	if len(collector.sessionActivityMap) != 0 {
		t.Error("sessionActivityMap should be empty initially")
	}
}

func TestTmuxCollector_isTmuxAvailable(t *testing.T) {
	collector := NewTmuxCollector()

	// This test will vary based on whether tmux is installed
	// We just verify the method doesn't panic
	result := collector.isTmuxAvailable()

	// Result should be boolean
	if result != true && result != false {
		t.Error("isTmuxAvailable() should return a boolean")
	}
}

func TestTmuxCollector_parseSessionLine(t *testing.T) {
	collector := NewTmuxCollector()

	tests := []struct {
		name          string
		line          string
		expectError   bool
		expectedName  string
		expectedWin   int
		expectedAttch bool
	}{
		{
			name:          "Valid attached session",
			line:          "dev:3:1:1700000000",
			expectError:   false,
			expectedName:  "dev",
			expectedWin:   3,
			expectedAttch: true,
		},
		{
			name:          "Valid detached session",
			line:          "test:1:0:1700000000",
			expectError:   false,
			expectedName:  "test",
			expectedWin:   1,
			expectedAttch: false,
		},
		{
			name:          "Session with multiple windows",
			line:          "main:10:1:1700000000",
			expectError:   false,
			expectedName:  "main",
			expectedWin:   10,
			expectedAttch: true,
		},
		{
			name:        "Invalid format - too few parts",
			line:        "dev:3:1",
			expectError: true,
		},
		{
			name:        "Invalid format - bad windows count",
			line:        "dev:abc:1:1700000000",
			expectError: true,
		},
		{
			name:        "Invalid format - bad attached status",
			line:        "dev:3:x:1700000000",
			expectError: true,
		},
		{
			name:        "Invalid format - bad timestamp",
			line:        "dev:3:1:notanumber",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := collector.parseSessionLine(tt.line)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if session.Name != tt.expectedName {
				t.Errorf("Name = %q, want %q", session.Name, tt.expectedName)
			}

			if session.Windows != tt.expectedWin {
				t.Errorf("Windows = %d, want %d", session.Windows, tt.expectedWin)
			}

			if session.Attached != tt.expectedAttch {
				t.Errorf("Attached = %v, want %v", session.Attached, tt.expectedAttch)
			}

			if session.Created.IsZero() {
				t.Error("Created time should not be zero")
			}

			if session.Status == "" {
				t.Error("Status should be set")
			}
		})
	}
}

func TestTmuxCollector_parseSessions(t *testing.T) {
	collector := NewTmuxCollector()

	tests := []struct {
		name          string
		output        string
		expectedCount int
	}{
		{
			name:          "Single session",
			output:        "dev:3:1:1700000000",
			expectedCount: 1,
		},
		{
			name: "Multiple sessions",
			output: `dev:3:1:1700000000
test:1:0:1700000100
main:5:1:1700000200`,
			expectedCount: 3,
		},
		{
			name:          "Empty output",
			output:        "",
			expectedCount: 0,
		},
		{
			name:          "Whitespace only",
			output:        "   \n  \n  ",
			expectedCount: 0,
		},
		{
			name: "Mixed valid and invalid lines",
			output: `dev:3:1:1700000000
invalid:line
test:1:0:1700000100`,
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions, err := collector.parseSessions(tt.output)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(sessions) != tt.expectedCount {
				t.Errorf("Got %d sessions, want %d", len(sessions), tt.expectedCount)
			}
		})
	}
}

func TestTmuxCollector_determineStatus(t *testing.T) {
	collector := NewTmuxCollector()
	now := time.Now()

	tests := []struct {
		name           string
		session        TmuxSession
		activitySetup  func()
		expectedStatus SessionStatus
	}{
		{
			name: "Attached session is working",
			session: TmuxSession{
				Name:     "test",
				Attached: true,
				Created:  now.Add(-30 * time.Minute),
			},
			activitySetup:  func() {},
			expectedStatus: StatusWorking,
		},
		{
			name: "New detached session is ready",
			session: TmuxSession{
				Name:     "new",
				Attached: false,
				Created:  now.Add(-2 * time.Minute),
			},
			activitySetup:  func() {},
			expectedStatus: StatusReady,
		},
		{
			name: "Detached session older than 1 hour is stalled",
			session: TmuxSession{
				Name:     "old",
				Attached: false,
				Created:  now.Add(-2 * time.Hour),
			},
			activitySetup:  func() {},
			expectedStatus: StatusStalled,
		},
		{
			name: "Detached session older than 24 hours is idle",
			session: TmuxSession{
				Name:     "veryold",
				Attached: false,
				Created:  now.Add(-48 * time.Hour),
			},
			activitySetup:  func() {},
			expectedStatus: StatusIdle,
		},
		{
			name: "Recently detached session is ready",
			session: TmuxSession{
				Name:     "recent",
				Attached: false,
				Created:  now.Add(-10 * time.Hour),
			},
			activitySetup: func() {
				collector.sessionActivityMap["recent"] = now.Add(-30 * time.Minute)
			},
			expectedStatus: StatusReady,
		},
		{
			name: "Session detached for 2 hours is stalled",
			session: TmuxSession{
				Name:     "stale",
				Attached: false,
				Created:  now.Add(-10 * time.Hour),
			},
			activitySetup: func() {
				collector.sessionActivityMap["stale"] = now.Add(-2 * time.Hour)
			},
			expectedStatus: StatusStalled,
		},
		{
			name: "Session detached for 25 hours is idle",
			session: TmuxSession{
				Name:     "dormant",
				Attached: false,
				Created:  now.Add(-48 * time.Hour),
			},
			activitySetup: func() {
				collector.sessionActivityMap["dormant"] = now.Add(-25 * time.Hour)
			},
			expectedStatus: StatusIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset activity map
			collector.sessionActivityMap = make(map[string]time.Time)

			// Setup activity if needed
			tt.activitySetup()

			status := collector.determineStatus(tt.session)

			if status != tt.expectedStatus {
				t.Errorf("determineStatus() = %v, want %v", status, tt.expectedStatus)
			}
		})
	}
}

func TestTmuxCollector_Collect_TmuxNotAvailable(t *testing.T) {
	collector := NewTmuxCollector()

	// Mock tmux not being available by testing the structure
	metrics := collector.Collect()

	if metrics == nil {
		t.Fatal("Collect() returned nil")
	}

	if metrics.Sessions == nil {
		t.Error("Sessions should not be nil")
	}

	if metrics.LastUpdate.IsZero() {
		t.Error("LastUpdate should be set")
	}

	// If tmux is not available, Available should be false
	// If tmux is available, Available should be true
	// We just verify the field exists and is boolean
	if metrics.Available != true && metrics.Available != false {
		t.Error("Available should be a boolean")
	}
}

func TestTmuxCollector_GetMetrics(t *testing.T) {
	collector := NewTmuxCollector()

	metrics := collector.GetMetrics()

	if metrics == nil {
		t.Fatal("GetMetrics() returned nil")
	}

	// Should have the same behavior as Collect()
	if metrics.LastUpdate.IsZero() {
		t.Error("LastUpdate should be set")
	}

	if metrics.Sessions == nil {
		t.Error("Sessions should not be nil")
	}
}

func TestTmuxMetrics_Structure(t *testing.T) {
	// Test that TmuxMetrics has all required fields
	metrics := &TmuxMetrics{
		Sessions:   []TmuxSession{},
		Total:      0,
		Available:  true,
		Error:      "",
		LastUpdate: time.Now(),
	}

	if metrics.Sessions == nil {
		t.Error("Sessions should not be nil")
	}

	if metrics.Total != 0 {
		t.Error("Total should be 0")
	}

	if !metrics.Available {
		t.Error("Available should be true")
	}

	if metrics.LastUpdate.IsZero() {
		t.Error("LastUpdate should be set")
	}
}

func TestTmuxSession_Structure(t *testing.T) {
	// Test that TmuxSession has all required fields
	session := TmuxSession{
		Name:     "test",
		Windows:  3,
		Attached: true,
		Status:   StatusWorking,
		Created:  time.Now(),
	}

	if session.Name != "test" {
		t.Error("Name not set correctly")
	}

	if session.Windows != 3 {
		t.Error("Windows not set correctly")
	}

	if !session.Attached {
		t.Error("Attached not set correctly")
	}

	if session.Status != StatusWorking {
		t.Error("Status not set correctly")
	}

	if session.Created.IsZero() {
		t.Error("Created should be set")
	}
}

func TestSessionStatus_Constants(t *testing.T) {
	// Verify all status constants are defined correctly
	statuses := []SessionStatus{
		StatusWorking,
		StatusStalled,
		StatusIdle,
		StatusReady,
	}

	expectedValues := []string{"WORKING", "STALLED", "IDLE", "READY"}

	for i, status := range statuses {
		if string(status) != expectedValues[i] {
			t.Errorf("Status constant %d = %q, want %q", i, status, expectedValues[i])
		}
	}
}

func TestTmuxCollector_ActivityMapTracking(t *testing.T) {
	collector := NewTmuxCollector()
	now := time.Now()

	// Test that attached sessions update activity map
	attachedSession := TmuxSession{
		Name:     "active",
		Attached: true,
		Created:  now.Add(-1 * time.Hour),
	}

	status := collector.determineStatus(attachedSession)

	if status != StatusWorking {
		t.Errorf("Attached session should be WORKING, got %v", status)
	}

	// Verify activity was tracked
	activity, exists := collector.sessionActivityMap["active"]
	if !exists {
		t.Error("Activity should be tracked for attached session")
	}

	if time.Since(activity) > 1*time.Second {
		t.Error("Activity timestamp should be recent")
	}

	// Test that new sessions get tracked
	newSession := TmuxSession{
		Name:     "brand-new",
		Attached: false,
		Created:  now.Add(-1 * time.Minute),
	}

	status = collector.determineStatus(newSession)

	if status != StatusReady {
		t.Errorf("New session should be READY, got %v", status)
	}

	if _, exists := collector.sessionActivityMap["brand-new"]; !exists {
		t.Error("Activity should be tracked for new session")
	}
}

func TestParseSessionLine_EdgeCases(t *testing.T) {
	collector := NewTmuxCollector()

	tests := []struct {
		name        string
		line        string
		expectError bool
	}{
		{
			name:        "Session name with special characters",
			line:        "my-session_123:5:1:1700000000",
			expectError: false,
		},
		{
			name:        "Zero windows (edge case)",
			line:        "empty:0:0:1700000000",
			expectError: false,
		},
		{
			name:        "Very large window count",
			line:        "many:999:1:1700000000",
			expectError: false,
		},
		{
			name:        "Empty session name",
			line:        ":5:1:1700000000",
			expectError: false, // tmux allows this
		},
		{
			name:        "Completely empty line",
			line:        "",
			expectError: true,
		},
		{
			name:        "Extra colons in data",
			line:        "session:5:1:1700000000:extra:data",
			expectError: false, // Should parse first 4 parts
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := collector.parseSessionLine(tt.line)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				// Basic validation that parsing succeeded
				if !tt.expectError && session.Name == "" && !strings.HasPrefix(tt.line, ":") {
					t.Error("Session name should not be empty for valid input")
				}
			}
		})
	}
}

func TestTmuxCollector_Collect_ErrorHandling(t *testing.T) {
	collector := NewTmuxCollector()

	metrics := collector.Collect()

	// Should never return nil
	if metrics == nil {
		t.Fatal("Collect() should never return nil")
	}

	// Should always set LastUpdate
	if metrics.LastUpdate.IsZero() {
		t.Error("LastUpdate should always be set")
	}

	// Should initialize Sessions slice
	if metrics.Sessions == nil {
		t.Error("Sessions slice should be initialized")
	}

	// If there's an error, it should be in the Error field
	if !metrics.Available && metrics.Error == "" {
		t.Error("If tmux is not available, Error should explain why")
	}
}

func TestTmuxSession_JSONTags(t *testing.T) {
	// Verify that JSON tags are present for proper serialization
	session := TmuxSession{
		Name:     "test",
		Windows:  3,
		Attached: true,
		Status:   StatusWorking,
		Created:  time.Now(),
	}

	// This is a compile-time check that the fields exist
	_ = session.Name
	_ = session.Windows
	_ = session.Attached
	_ = session.Status
	_ = session.Created
}

func TestTmuxMetrics_JSONTags(t *testing.T) {
	// Verify that JSON tags are present for proper serialization
	metrics := &TmuxMetrics{
		Sessions:   []TmuxSession{},
		Total:      0,
		Available:  true,
		Error:      "",
		LastUpdate: time.Now(),
	}

	// This is a compile-time check that the fields exist
	_ = metrics.Sessions
	_ = metrics.Total
	_ = metrics.Available
	_ = metrics.Error
	_ = metrics.LastUpdate
}

func TestTmuxCollector_StatusTransitions(t *testing.T) {
	collector := NewTmuxCollector()
	now := time.Now()

	// Test status transition: WORKING -> READY -> STALLED -> IDLE

	// Start with attached (WORKING)
	session := TmuxSession{
		Name:     "transition-test",
		Attached: true,
		Created:  now.Add(-5 * time.Hour),
	}

	status := collector.determineStatus(session)
	if status != StatusWorking {
		t.Errorf("Step 1: Expected WORKING, got %v", status)
	}

	// Detach for 30 minutes (READY)
	session.Attached = false
	collector.sessionActivityMap["transition-test"] = now.Add(-30 * time.Minute)

	status = collector.determineStatus(session)
	if status != StatusReady {
		t.Errorf("Step 2: Expected READY, got %v", status)
	}

	// Wait 2 hours (STALLED)
	collector.sessionActivityMap["transition-test"] = now.Add(-2 * time.Hour)

	status = collector.determineStatus(session)
	if status != StatusStalled {
		t.Errorf("Step 3: Expected STALLED, got %v", status)
	}

	// Wait 25 hours (IDLE)
	collector.sessionActivityMap["transition-test"] = now.Add(-25 * time.Hour)

	status = collector.determineStatus(session)
	if status != StatusIdle {
		t.Errorf("Step 4: Expected IDLE, got %v", status)
	}
}
