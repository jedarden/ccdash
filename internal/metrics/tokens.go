package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TokenMetrics represents aggregated token usage metrics
type TokenMetrics struct {
	InputTokens         int64         `json:"input_tokens"`
	OutputTokens        int64         `json:"output_tokens"`
	CacheReadTokens     int64         `json:"cache_read_tokens"`
	CacheCreationTokens int64         `json:"cache_creation_tokens"`
	TotalTokens         int64         `json:"total_tokens"`
	TotalCost           float64       `json:"total_cost"`
	Rate                float64       `json:"rate"`              // tokens/min over 60s window
	SessionAvgRate      float64       `json:"session_avg_rate"`  // average tokens/min for entire session
	TimeSpan            time.Duration `json:"time_span"`
	EarliestTimestamp   time.Time     `json:"earliest_timestamp"`
	LatestTimestamp     time.Time     `json:"latest_timestamp"`
	Models              []string      `json:"models"`
	Available           bool          `json:"available"`
	Error               string        `json:"error,omitempty"`
	LastUpdate          time.Time     `json:"last_update"`
}

// TokenCollector collects and aggregates token usage from Claude Code sessions
type TokenCollector struct {
	projectsDir string
}

// NewTokenCollector creates a new TokenCollector
func NewTokenCollector() *TokenCollector {
	home, err := os.UserHomeDir()
	if err != nil {
		return &TokenCollector{projectsDir: ""}
	}
	return &TokenCollector{
		projectsDir: filepath.Join(home, ".claude", "projects"),
	}
}

// NewTokenCollectorWithPath creates a TokenCollector with a custom path (for testing)
func NewTokenCollectorWithPath(path string) *TokenCollector {
	return &TokenCollector{projectsDir: path}
}

// claudeMessage represents the structure of Claude API messages in JSONL
type claudeMessage struct {
	Message   messageData `json:"message"`
	Timestamp string      `json:"timestamp"`
	Type      string      `json:"type"`
}

// messageData contains the actual API response
type messageData struct {
	Model string    `json:"model"`
	Usage usageData `json:"usage"`
}

// usageData contains token usage information
type usageData struct {
	InputTokens              int64         `json:"input_tokens"`
	CacheCreationInputTokens int64         `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64         `json:"cache_read_input_tokens"`
	OutputTokens             int64         `json:"output_tokens"`
	CacheCreation            cacheCreation `json:"cache_creation"`
}

// cacheCreation contains detailed cache creation token breakdown
type cacheCreation struct {
	Ephemeral5mInputTokens  int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens  int64 `json:"ephemeral_1h_input_tokens"`
}

// timestampedTokens represents tokens with their timestamp for rate calculation
type timestampedTokens struct {
	tokens    int64
	timestamp time.Time
}

// Collect gathers token metrics from all JSONL files in the projects directory
func (tc *TokenCollector) Collect() (*TokenMetrics, error) {
	metrics := &TokenMetrics{
		Available:  false,
		LastUpdate: time.Now(),
		Models:     []string{},
	}

	// Check if projects directory exists
	if tc.projectsDir == "" {
		metrics.Error = "Could not determine home directory"
		return metrics, nil
	}

	if _, err := os.Stat(tc.projectsDir); os.IsNotExist(err) {
		metrics.Error = "Claude projects directory not found"
		return metrics, nil
	}

	// Find the current project directory (based on cwd)
	cwd, err := os.Getwd()
	if err != nil {
		metrics.Error = fmt.Sprintf("Failed to get current directory: %v", err)
		return metrics, nil
	}

	projectDir := tc.findProjectDir(cwd)
	if projectDir == "" {
		metrics.Error = "No Claude project found for current directory"
		return metrics, nil
	}

	// Read all JSONL files in the project directory
	files, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil {
		metrics.Error = fmt.Sprintf("Failed to read project files: %v", err)
		return metrics, nil
	}

	if len(files) == 0 {
		metrics.Error = "No JSONL files found in project"
		return metrics, nil
	}

	// Aggregate data from all files
	var allTimestamps []timestampedTokens
	modelSet := make(map[string]bool)

	for _, file := range files {
		fileMetrics, timestamps := tc.parseJSONLFile(file)
		metrics.InputTokens += fileMetrics.InputTokens
		metrics.OutputTokens += fileMetrics.OutputTokens
		metrics.CacheReadTokens += fileMetrics.CacheReadTokens
		metrics.CacheCreationTokens += fileMetrics.CacheCreationTokens
		allTimestamps = append(allTimestamps, timestamps...)

		for _, model := range fileMetrics.Models {
			modelSet[model] = true
		}
	}

	// Calculate total tokens
	metrics.TotalTokens = metrics.InputTokens + metrics.OutputTokens +
		metrics.CacheReadTokens + metrics.CacheCreationTokens

	// Calculate cost (using Claude Sonnet 4.5 pricing as baseline)
	metrics.TotalCost = tc.calculateCost(*metrics)

	// Sort timestamps for rate calculations
	sort.Slice(allTimestamps, func(i, j int) bool {
		return allTimestamps[i].timestamp.Before(allTimestamps[j].timestamp)
	})

	// Calculate time span and timestamp tracking
	if len(allTimestamps) > 0 {
		metrics.EarliestTimestamp = allTimestamps[0].timestamp
		metrics.LatestTimestamp = allTimestamps[len(allTimestamps)-1].timestamp
		metrics.TimeSpan = metrics.LatestTimestamp.Sub(metrics.EarliestTimestamp)

		// Calculate session average rate (tokens per minute)
		if metrics.TimeSpan > 0 {
			minutes := metrics.TimeSpan.Minutes()
			if minutes > 0 {
				metrics.SessionAvgRate = float64(metrics.TotalTokens) / minutes
			}
		}

		// Calculate 60-second window rate
		metrics.Rate = tc.calculate60sRate(allTimestamps)
	}

	// Convert model set to sorted slice
	for model := range modelSet {
		metrics.Models = append(metrics.Models, model)
	}
	sort.Strings(metrics.Models)

	metrics.Available = true
	return metrics, nil
}

// findProjectDir finds the Claude project directory for the given working directory
func (tc *TokenCollector) findProjectDir(cwd string) string {
	// Convert path to project directory name format
	// e.g., /workspaces/test-agor -> -workspaces-test-agor
	projectName := strings.ReplaceAll(cwd, "/", "-")
	projectPath := filepath.Join(tc.projectsDir, projectName)

	if _, err := os.Stat(projectPath); err == nil {
		return projectPath
	}

	return ""
}

// parseJSONLFile parses a single JSONL file and extracts token metrics
func (tc *TokenCollector) parseJSONLFile(filename string) (TokenMetrics, []timestampedTokens) {
	metrics := TokenMetrics{}
	var timestamps []timestampedTokens
	modelSet := make(map[string]bool)

	file, err := os.Open(filename)
	if err != nil {
		return metrics, timestamps
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 1024*1024) // 1MB buffer
	scanner.Buffer(buf, 10*1024*1024) // 10MB max

	for scanner.Scan() {
		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // Skip malformed lines
		}

		// Only process assistant messages with usage data
		if msg.Type != "assistant" || msg.Message.Usage.OutputTokens == 0 {
			continue
		}

		// Parse timestamp
		timestamp, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
		if err != nil {
			timestamp = time.Now()
		}

		// Aggregate token counts
		usage := msg.Message.Usage
		metrics.InputTokens += usage.InputTokens
		metrics.OutputTokens += usage.OutputTokens
		metrics.CacheReadTokens += usage.CacheReadInputTokens

		// Cache creation tokens can come from multiple fields
		cacheCreation := usage.CacheCreationInputTokens
		if cacheCreation == 0 {
			cacheCreation = usage.CacheCreation.Ephemeral5mInputTokens +
				usage.CacheCreation.Ephemeral1hInputTokens
		}
		metrics.CacheCreationTokens += cacheCreation

		// Track total tokens for this message with timestamp
		msgTotalTokens := usage.InputTokens + usage.OutputTokens +
			usage.CacheReadInputTokens + cacheCreation
		timestamps = append(timestamps, timestampedTokens{
			tokens:    msgTotalTokens,
			timestamp: timestamp,
		})

		// Track model
		if msg.Message.Model != "" {
			modelSet[msg.Message.Model] = true
		}
	}

	// Convert model set to slice
	for model := range modelSet {
		metrics.Models = append(metrics.Models, model)
	}

	return metrics, timestamps
}

// calculate60sRate calculates the token rate over the last 60 seconds
func (tc *TokenCollector) calculate60sRate(timestamps []timestampedTokens) float64 {
	if len(timestamps) == 0 {
		return 0
	}

	// Get the most recent timestamp
	lastTime := timestamps[len(timestamps)-1].timestamp
	cutoffTime := lastTime.Add(-60 * time.Second)

	// Sum tokens in the last 60 seconds
	var totalTokens int64
	var windowStart time.Time

	for i := len(timestamps) - 1; i >= 0; i-- {
		if timestamps[i].timestamp.Before(cutoffTime) {
			break
		}
		totalTokens += timestamps[i].tokens
		windowStart = timestamps[i].timestamp
	}

	// Calculate rate (tokens per minute)
	if totalTokens == 0 {
		return 0
	}

	duration := lastTime.Sub(windowStart)
	if duration <= 0 {
		return 0
	}

	minutes := duration.Minutes()
	if minutes == 0 {
		return 0
	}

	return float64(totalTokens) / minutes
}

// calculateCost estimates the cost based on Claude Sonnet 4.5 pricing
func (tc *TokenCollector) calculateCost(metrics TokenMetrics) float64 {
	// Claude Sonnet 4.5 pricing (as of 2025):
	// Input: $3 per million tokens
	// Output: $15 per million tokens
	// Cache reads: $0.30 per million tokens
	// Cache creation: $3.75 per million tokens (1.25x input price)

	const (
		inputCostPerMillion    = 3.0
		outputCostPerMillion   = 15.0
		cacheReadCostPerMillion = 0.30
		cacheCreateCostPerMillion = 3.75
	)

	inputCost := float64(metrics.InputTokens) * inputCostPerMillion / 1_000_000
	outputCost := float64(metrics.OutputTokens) * outputCostPerMillion / 1_000_000
	cacheReadCost := float64(metrics.CacheReadTokens) * cacheReadCostPerMillion / 1_000_000
	cacheCreateCost := float64(metrics.CacheCreationTokens) * cacheCreateCostPerMillion / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheCreateCost
}

// FormatTokens formats a token count with thousands separators
func FormatTokens(count int64) string {
	if count == 0 {
		return "0"
	}

	// Handle negative numbers
	negative := count < 0
	if negative {
		count = -count
	}

	// Convert to string and add commas
	s := fmt.Sprintf("%d", count)
	var result []rune

	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, digit)
	}

	if negative {
		return "-" + string(result)
	}
	return string(result)
}

// FormatCost formats a cost value as currency
func FormatCost(cost float64) string {
	if cost == 0 {
		return "$0.00"
	}
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

// FormatTokenRate formats a token rate as tokens/min
func FormatTokenRate(rate float64) string {
	if rate == 0 {
		return "0 tok/min"
	}
	if rate < 1000 {
		return fmt.Sprintf("%.0f tok/min", rate)
	}
	// For larger rates, format with thousands separator
	return fmt.Sprintf("%s tok/min", FormatTokens(int64(rate)))
}

// FormatDuration formats a duration in a human-readable format
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
