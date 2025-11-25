package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero", 0, "0"},
		{"small", 42, "42"},
		{"hundreds", 999, "999"},
		{"thousands", 1000, "1,000"},
		{"tens of thousands", 12345, "12,345"},
		{"millions", 1234567, "1,234,567"},
		{"billions", 1234567890, "1,234,567,890"},
		{"negative", -12345, "-12,345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTokens(tt.input)
			if result != tt.expected {
				t.Errorf("FormatTokens(%d) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"zero", 0.0, "$0.00"},
		{"small", 0.001, "$0.0010"},
		{"very small", 0.0001, "$0.0001"},
		{"cents", 0.42, "$0.42"},
		{"dollars", 12.34, "$12.34"},
		{"large", 1234.56, "$1234.56"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCost(tt.input)
			if result != tt.expected {
				t.Errorf("FormatCost(%f) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatTokenRate(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"zero", 0.0, "0 tok/min"},
		{"small", 42.7, "43 tok/min"},
		{"hundreds", 999.2, "999 tok/min"},
		{"thousands", 1234.5, "1,234 tok/min"},
		{"large", 123456.0, "123,456 tok/min"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTokenRate(tt.input)
			if result != tt.expected {
				t.Errorf("FormatTokenRate(%f) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"one minute", 1 * time.Minute, "1.0m"},
		{"minutes", 5*time.Minute + 30*time.Second, "5.5m"},
		{"one hour", 1 * time.Hour, "1h0m"},
		{"hours and minutes", 2*time.Hour + 15*time.Minute, "2h15m"},
		{"long session", 5*time.Hour + 45*time.Minute, "5h45m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("FormatDuration(%v) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewTokenCollector(t *testing.T) {
	collector := NewTokenCollector()
	if collector == nil {
		t.Fatal("NewTokenCollector() returned nil")
	}

	// Should have a projects directory set
	if collector.projectsDir == "" {
		t.Error("projectsDir should not be empty")
	}
}

func TestNewTokenCollectorWithPath(t *testing.T) {
	testPath := "/test/path"
	collector := NewTokenCollectorWithPath(testPath)
	
	if collector == nil {
		t.Fatal("NewTokenCollectorWithPath() returned nil")
	}

	if collector.projectsDir != testPath {
		t.Errorf("projectsDir = %s; want %s", collector.projectsDir, testPath)
	}
}

func TestCollectWithNonexistentDir(t *testing.T) {
	collector := NewTokenCollectorWithPath("/nonexistent/directory")
	metrics, err := collector.Collect()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if metrics.Available {
		t.Error("Expected Available to be false for nonexistent directory")
	}

	if metrics.Error == "" {
		t.Error("Expected error message for nonexistent directory")
	}
}

func TestCollectWithEmptyDir(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "ccdash-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a project subdirectory
	projectDir := filepath.Join(tmpDir, "-test-project")
	if err := os.Mkdir(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create collector pointing to temp directory
	collector := NewTokenCollectorWithPath(tmpDir)

	// Override working directory for this test
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir("/test/project")

	metrics, err := collector.Collect()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if metrics.Available {
		t.Error("Expected Available to be false for empty directory")
	}

	if metrics.Error == "" {
		t.Error("Expected error message for empty directory")
	}
}

func TestParseJSONLFile(t *testing.T) {
	// Create temporary directory and test file
	tmpDir, err := os.MkdirTemp("", "ccdash-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.jsonl")

	// Create test JSONL data
	testData := []claudeMessage{
		{
			Type:      "assistant",
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Message: messageData{
				Model: "claude-sonnet-4-5-20250929",
				Usage: usageData{
					InputTokens:              100,
					OutputTokens:             50,
					CacheReadInputTokens:     200,
					CacheCreationInputTokens: 300,
				},
			},
		},
		{
			Type:      "assistant",
			Timestamp: time.Now().Add(1 * time.Minute).Format(time.RFC3339Nano),
			Message: messageData{
				Model: "claude-sonnet-4-5-20250929",
				Usage: usageData{
					InputTokens:          150,
					OutputTokens:         75,
					CacheReadInputTokens: 100,
					CacheCreation: cacheCreation{
						Ephemeral5mInputTokens: 250,
					},
				},
			},
		},
		{
			Type:      "user",
			Timestamp: time.Now().Add(2 * time.Minute).Format(time.RFC3339Nano),
			Message: messageData{
				Model: "claude-sonnet-4-5-20250929",
				Usage: usageData{
					InputTokens:  999,
					OutputTokens: 999,
				},
			},
		},
	}

	// Write test data to file
	file, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}

	for _, msg := range testData {
		data, _ := json.Marshal(msg)
		file.Write(data)
		file.Write([]byte("\n"))
	}
	file.Close()

	// Test parsing
	collector := NewTokenCollectorWithPath(tmpDir)
	metrics, timestamps := collector.parseJSONLFile(testFile)

	// Verify metrics (should only count assistant messages with output)
	expectedInput := int64(100 + 150)
	expectedOutput := int64(50 + 75)
	expectedCacheRead := int64(200 + 100)
	expectedCacheCreate := int64(300 + 250)

	if metrics.InputTokens != expectedInput {
		t.Errorf("InputTokens = %d; want %d", metrics.InputTokens, expectedInput)
	}

	if metrics.OutputTokens != expectedOutput {
		t.Errorf("OutputTokens = %d; want %d", metrics.OutputTokens, expectedOutput)
	}

	if metrics.CacheReadTokens != expectedCacheRead {
		t.Errorf("CacheReadTokens = %d; want %d", metrics.CacheReadTokens, expectedCacheRead)
	}

	if metrics.CacheCreationTokens != expectedCacheCreate {
		t.Errorf("CacheCreationTokens = %d; want %d", metrics.CacheCreationTokens, expectedCacheCreate)
	}

	// Should have 2 timestamps (only assistant messages with output)
	if len(timestamps) != 2 {
		t.Errorf("timestamps length = %d; want 2", len(timestamps))
	}

	// Verify model tracking
	if len(metrics.Models) != 1 {
		t.Errorf("Models length = %d; want 1", len(metrics.Models))
	}
}

func TestCalculateCost(t *testing.T) {
	collector := NewTokenCollector()

	tests := []struct {
		name     string
		metrics  TokenMetrics
		expected float64
	}{
		{
			name: "all zeros",
			metrics: TokenMetrics{
				InputTokens:         0,
				OutputTokens:        0,
				CacheReadTokens:     0,
				CacheCreationTokens: 0,
			},
			expected: 0.0,
		},
		{
			name: "input only",
			metrics: TokenMetrics{
				InputTokens:         1_000_000,
				OutputTokens:        0,
				CacheReadTokens:     0,
				CacheCreationTokens: 0,
			},
			expected: 3.0, // $3 per million input tokens
		},
		{
			name: "output only",
			metrics: TokenMetrics{
				InputTokens:         0,
				OutputTokens:        1_000_000,
				CacheReadTokens:     0,
				CacheCreationTokens: 0,
			},
			expected: 15.0, // $15 per million output tokens
		},
		{
			name: "mixed tokens",
			metrics: TokenMetrics{
				InputTokens:         100_000,  // $0.30
				OutputTokens:        50_000,   // $0.75
				CacheReadTokens:     200_000,  // $0.06
				CacheCreationTokens: 150_000,  // $0.5625
			},
			expected: 1.6725,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.calculateCost(tt.metrics)
			// Use approximate comparison for floats
			if !floatEquals(result, tt.expected, 0.0001) {
				t.Errorf("calculateCost() = %f; want %f", result, tt.expected)
			}
		})
	}
}

func TestCalculate60sRate(t *testing.T) {
	collector := NewTokenCollector()

	now := time.Now()
	tests := []struct {
		name       string
		timestamps []timestampedTokens
		minRate    float64
		maxRate    float64
	}{
		{
			name:       "empty",
			timestamps: []timestampedTokens{},
			minRate:    0,
			maxRate:    0,
		},
		{
			name: "single point",
			timestamps: []timestampedTokens{
				{tokens: 100, timestamp: now},
			},
			minRate: 0,
			maxRate: 0,
		},
		{
			name: "two points in 30 seconds",
			timestamps: []timestampedTokens{
				{tokens: 100, timestamp: now.Add(-30 * time.Second)},
				{tokens: 100, timestamp: now},
			},
			minRate: 300, // ~400 tokens/min (200 tokens in 30s = 400/min)
			maxRate: 500,
		},
		{
			name: "multiple points over 2 minutes",
			timestamps: []timestampedTokens{
				{tokens: 100, timestamp: now.Add(-120 * time.Second)},
				{tokens: 100, timestamp: now.Add(-90 * time.Second)},
				{tokens: 100, timestamp: now.Add(-30 * time.Second)},
				{tokens: 100, timestamp: now},
			},
			minRate: 300, // Only last 60s counted (last 2 entries = 200 tokens in 30s)
			maxRate: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.calculate60sRate(tt.timestamps)
			if result < tt.minRate || result > tt.maxRate {
				t.Errorf("calculate60sRate() = %f; want between %f and %f", 
					result, tt.minRate, tt.maxRate)
			}
		})
	}
}

func TestFindProjectDir(t *testing.T) {
	// Create temporary directory structure
	tmpDir, err := os.MkdirTemp("", "ccdash-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a project directory
	testCwd := "/workspaces/test-project"
	projectName := strings.ReplaceAll(testCwd, "/", "-")
	projectDir := filepath.Join(tmpDir, projectName)
	if err := os.Mkdir(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	collector := NewTokenCollectorWithPath(tmpDir)
	result := collector.findProjectDir(testCwd)

	if result != projectDir {
		t.Errorf("findProjectDir() = %s; want %s", result, projectDir)
	}
}

func TestFindProjectDirNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccdash-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	collector := NewTokenCollectorWithPath(tmpDir)
	result := collector.findProjectDir("/nonexistent/project")

	if result != "" {
		t.Errorf("findProjectDir() = %s; want empty string", result)
	}
}

// Helper function to compare floats with tolerance
func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}

func TestGetPricingForModel(t *testing.T) {
	tests := []struct {
		name              string
		model             string
		expectedInput     float64
		expectedOutput    float64
		expectedCacheRead float64
	}{
		{
			name:              "Opus 4.5 exact match",
			model:             "claude-opus-4-5-20251101",
			expectedInput:     5.0,
			expectedOutput:    25.0,
			expectedCacheRead: 0.50,
		},
		{
			name:              "Opus 4.5 prefix match",
			model:             "claude-opus-4-5-20251115",
			expectedInput:     5.0,
			expectedOutput:    25.0,
			expectedCacheRead: 0.50,
		},
		{
			name:              "Sonnet 4.5 exact match",
			model:             "claude-sonnet-4-5-20250929",
			expectedInput:     3.0,
			expectedOutput:    15.0,
			expectedCacheRead: 0.30,
		},
		{
			name:              "Haiku 4.5 exact match",
			model:             "claude-haiku-4-5-20250929",
			expectedInput:     1.0,
			expectedOutput:    5.0,
			expectedCacheRead: 0.10,
		},
		{
			name:              "Unknown model uses default (Sonnet 4.5)",
			model:             "claude-unknown-model",
			expectedInput:     3.0,
			expectedOutput:    15.0,
			expectedCacheRead: 0.30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pricing := getPricingForModel(tt.model)
			if pricing.InputPerMillion != tt.expectedInput {
				t.Errorf("InputPerMillion = %f; want %f", pricing.InputPerMillion, tt.expectedInput)
			}
			if pricing.OutputPerMillion != tt.expectedOutput {
				t.Errorf("OutputPerMillion = %f; want %f", pricing.OutputPerMillion, tt.expectedOutput)
			}
			if pricing.CacheReadPerMillion != tt.expectedCacheRead {
				t.Errorf("CacheReadPerMillion = %f; want %f", pricing.CacheReadPerMillion, tt.expectedCacheRead)
			}
		})
	}
}

func TestCalculateCostWithModels(t *testing.T) {
	collector := NewTokenCollector()

	tests := []struct {
		name     string
		metrics  TokenMetrics
		expected float64
	}{
		{
			name: "Opus 4.5 pricing",
			metrics: TokenMetrics{
				InputTokens:         1_000_000,
				OutputTokens:        1_000_000,
				CacheReadTokens:     0,
				CacheCreationTokens: 0,
				Models:              []string{"claude-opus-4-5-20251101"},
			},
			expected: 30.0, // $5 input + $25 output
		},
		{
			name: "Sonnet 4.5 pricing",
			metrics: TokenMetrics{
				InputTokens:         1_000_000,
				OutputTokens:        1_000_000,
				CacheReadTokens:     0,
				CacheCreationTokens: 0,
				Models:              []string{"claude-sonnet-4-5-20250929"},
			},
			expected: 18.0, // $3 input + $15 output
		},
		{
			name: "Haiku 4.5 pricing",
			metrics: TokenMetrics{
				InputTokens:         1_000_000,
				OutputTokens:        1_000_000,
				CacheReadTokens:     0,
				CacheCreationTokens: 0,
				Models:              []string{"claude-haiku-4-5-20250929"},
			},
			expected: 6.0, // $1 input + $5 output
		},
		{
			name: "Opus 4.5 with cache",
			metrics: TokenMetrics{
				InputTokens:         100_000,  // $0.50
				OutputTokens:        50_000,   // $1.25
				CacheReadTokens:     200_000,  // $0.10
				CacheCreationTokens: 100_000,  // $0.625
				Models:              []string{"claude-opus-4-5-20251101"},
			},
			expected: 2.475,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.calculateCost(tt.metrics)
			if !floatEquals(result, tt.expected, 0.0001) {
				t.Errorf("calculateCost() = %f; want %f", result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkFormatTokens(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FormatTokens(1234567890)
	}
}

func BenchmarkParseJSONLFile(b *testing.B) {
	// Create test file
	tmpDir, _ := os.MkdirTemp("", "ccdash-bench-*")
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.jsonl")
	file, _ := os.Create(testFile)

	// Write 100 test messages
	now := time.Now()
	for i := 0; i < 100; i++ {
		msg := claudeMessage{
			Type:      "assistant",
			Timestamp: now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			Message: messageData{
				Model: "claude-sonnet-4-5-20250929",
				Usage: usageData{
					InputTokens:              100,
					OutputTokens:             50,
					CacheReadInputTokens:     200,
					CacheCreationInputTokens: 300,
				},
			},
		}
		data, _ := json.Marshal(msg)
		file.Write(data)
		file.Write([]byte("\n"))
	}
	file.Close()

	collector := NewTokenCollectorWithPath(tmpDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.parseJSONLFile(testFile)
	}
}
