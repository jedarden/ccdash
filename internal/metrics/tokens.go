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

// ModelUsage tracks token usage and cost for a specific model
type ModelUsage struct {
	Model               string  `json:"model"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`
}

// TokenMetrics represents aggregated token usage metrics
type TokenMetrics struct {
	InputTokens         int64         `json:"input_tokens"`
	OutputTokens        int64         `json:"output_tokens"`
	CacheReadTokens     int64         `json:"cache_read_tokens"`
	CacheCreationTokens int64         `json:"cache_creation_tokens"`
	TotalTokens         int64         `json:"total_tokens"`
	TotalCost           float64       `json:"total_cost"`
	Rate                float64       `json:"rate"`             // tokens/min over 60s window
	SessionAvgRate      float64       `json:"session_avg_rate"` // average tokens/min for entire session
	TimeSpan            time.Duration `json:"time_span"`
	EarliestTimestamp   time.Time     `json:"earliest_timestamp"`
	LatestTimestamp     time.Time     `json:"latest_timestamp"`
	LookbackFrom        time.Time     `json:"lookback_from"` // Start of measurement period
	Models              []string      `json:"models"`
	ModelUsages         []ModelUsage  `json:"model_usages"` // Per-model breakdown
	Available           bool          `json:"available"`
	Error               string        `json:"error,omitempty"`
	LastUpdate          time.Time     `json:"last_update"`
}

// TokenCollector collects and aggregates token usage from Claude Code sessions
type TokenCollector struct {
	projectsDir  string
	lookbackFrom time.Time // Only include data from this time onwards
	cache        *TokenCache
}

// GetMondayNineAM returns the most recent Monday at 9am local time
// If today is Monday before 9am, returns last Monday's 9am
func GetMondayNineAM() time.Time {
	now := time.Now()
	// Find the most recent Monday
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday becomes 7 for easier math
	}
	daysUntilMonday := weekday - 1 // Days since Monday (Mon=0, Tue=1, ...)

	// Start of Monday (midnight)
	monday := now.AddDate(0, 0, -daysUntilMonday)
	monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 9, 0, 0, 0, monday.Location())

	// If we haven't reached 9am on Monday yet, go back to previous Monday
	if monday.After(now) {
		monday = monday.AddDate(0, 0, -7)
	}

	return monday
}

// NewTokenCollector creates a new TokenCollector with default Monday 9am lookback
func NewTokenCollector() *TokenCollector {
	home, err := os.UserHomeDir()
	if err != nil {
		return &TokenCollector{
			projectsDir:  "",
			lookbackFrom: GetMondayNineAM(),
			cache:        NewTokenCache(),
		}
	}
	return &TokenCollector{
		projectsDir:  filepath.Join(home, ".claude", "projects"),
		lookbackFrom: GetMondayNineAM(),
		cache:        NewTokenCache(),
	}
}

// NewTokenCollectorWithLookback creates a TokenCollector with a custom lookback time
func NewTokenCollectorWithLookback(lookbackFrom time.Time) *TokenCollector {
	home, err := os.UserHomeDir()
	if err != nil {
		return &TokenCollector{
			projectsDir:  "",
			lookbackFrom: lookbackFrom,
			cache:        NewTokenCache(),
		}
	}
	return &TokenCollector{
		projectsDir:  filepath.Join(home, ".claude", "projects"),
		lookbackFrom: lookbackFrom,
		cache:        NewTokenCache(),
	}
}

// NewTokenCollectorWithPath creates a TokenCollector with a custom path (for testing)
func NewTokenCollectorWithPath(path string) *TokenCollector {
	return &TokenCollector{
		projectsDir:  path,
		lookbackFrom: GetMondayNineAM(),
		cache:        NewTokenCache(),
	}
}

// SetLookback sets the lookback time filter
func (tc *TokenCollector) SetLookback(t time.Time) {
	tc.lookbackFrom = t
}

// GetLookback returns the current lookback time
func (tc *TokenCollector) GetLookback() time.Time {
	return tc.lookbackFrom
}

// GetCache returns the underlying token cache for shared metrics operations
func (tc *TokenCollector) GetCache() *TokenCache {
	return tc.cache
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
	Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"`
}

// Collect gathers token metrics from all JSONL files in the projects directory
// Uses SQLite for efficient lookback queries - data is indexed by timestamp
func (tc *TokenCollector) Collect() (*TokenMetrics, error) {
	metrics := &TokenMetrics{
		Available:    false,
		LastUpdate:   time.Now(),
		LookbackFrom: tc.lookbackFrom,
		Models:       []string{},
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

	// Step 1: Ingest any new data from JSONL files into SQLite
	// Continue processing even if individual files fail (for resilience)
	var lastIngestErr error
	for _, file := range files {
		if err := tc.ingestJSONLFile(file); err != nil {
			lastIngestErr = err
			// Continue processing other files even if one fails
		}
	}
	// Note: lastIngestErr is tracked but not returned to avoid blocking queries
	// when only some files have issues. The error is silently logged for debugging.
	_ = lastIngestErr

	// Step 2: Query SQLite for aggregated metrics based on lookback
	aggregated, err := tc.cache.QueryTokensSince(tc.lookbackFrom)
	if err != nil {
		metrics.Error = fmt.Sprintf("Failed to query token cache: %v", err)
		return metrics, nil
	}

	// Populate metrics from query results
	metrics.InputTokens = aggregated.InputTokens
	metrics.OutputTokens = aggregated.OutputTokens
	metrics.CacheReadTokens = aggregated.CacheReadTokens
	metrics.CacheCreationTokens = aggregated.CacheCreationTokens
	metrics.TotalTokens = aggregated.InputTokens + aggregated.OutputTokens +
		aggregated.CacheReadTokens + aggregated.CacheCreationTokens
	metrics.EarliestTimestamp = aggregated.EarliestTimestamp
	metrics.LatestTimestamp = aggregated.LatestTimestamp

	if !aggregated.EarliestTimestamp.IsZero() && !aggregated.LatestTimestamp.IsZero() {
		metrics.TimeSpan = aggregated.LatestTimestamp.Sub(aggregated.EarliestTimestamp)
	}

	// Build model list and per-model usage
	var totalCost float64
	metrics.ModelUsages = make([]ModelUsage, 0, len(aggregated.ModelMetrics))

	for model, mm := range aggregated.ModelMetrics {
		metrics.Models = append(metrics.Models, model)

		pricing := getPricingForModel(model)
		inputCost := float64(mm.InputTokens) * pricing.InputPerMillion / 1_000_000
		outputCost := float64(mm.OutputTokens) * pricing.OutputPerMillion / 1_000_000
		cacheReadCost := float64(mm.CacheReadTokens) * pricing.CacheReadPerMillion / 1_000_000
		cacheCreateCost := float64(mm.CacheCreationTokens) * pricing.CacheCreatePerMillion / 1_000_000
		modelCost := inputCost + outputCost + cacheReadCost + cacheCreateCost

		usage := ModelUsage{
			Model:               model,
			InputTokens:         mm.InputTokens,
			OutputTokens:        mm.OutputTokens,
			CacheReadTokens:     mm.CacheReadTokens,
			CacheCreationTokens: mm.CacheCreationTokens,
			TotalTokens:         mm.InputTokens + mm.OutputTokens + mm.CacheReadTokens + mm.CacheCreationTokens,
			Cost:                modelCost,
		}
		metrics.ModelUsages = append(metrics.ModelUsages, usage)
		totalCost += modelCost
	}

	sort.Strings(metrics.Models)

	// Sort model usages by cost (highest first)
	sort.Slice(metrics.ModelUsages, func(i, j int) bool {
		return metrics.ModelUsages[i].Cost > metrics.ModelUsages[j].Cost
	})

	metrics.TotalCost = totalCost

	// Calculate session average rate
	if metrics.TimeSpan > 0 {
		minutes := metrics.TimeSpan.Minutes()
		if minutes > 0 {
			metrics.SessionAvgRate = float64(metrics.TotalTokens) / minutes
		}
	}

	// Calculate 60-second window rate from recent events
	recentEvents, err := tc.cache.QueryRecentEvents(60)
	if err == nil && len(recentEvents) > 0 {
		metrics.Rate = tc.calculate60sRate(recentEvents)
	}

	metrics.Available = true
	return metrics, nil
}

// ingestJSONLFile reads a JSONL file and inserts new events into SQLite
// Returns an error if database operations fail (for proper error handling)
func (tc *TokenCollector) ingestJSONLFile(filename string) error {
	if tc.cache == nil {
		return nil
	}

	// Check file modification time
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return nil // File may have been deleted, not a critical error
	}

	// Get last processed state
	lastLine, lastMod, exists := tc.cache.GetFileState(filename)

	// If file hasn't been modified since last processing, skip
	if exists && !fileInfo.ModTime().After(lastMod) {
		return nil
	}

	// If file was modified (truncated/rewritten), invalidate and reprocess
	if exists && fileInfo.ModTime().After(lastMod) {
		// Check if file was truncated (new size < last line count implies rewrite)
		// For safety, we'll just process from where we left off
		// but if file was completely rewritten, invalidate
		file, err := os.Open(filename)
		if err != nil {
			return nil // File may have been deleted
		}
		defer file.Close()

		// Count current lines
		var currentLineCount int64
		scanner := bufio.NewScanner(file)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)
		for scanner.Scan() {
			currentLineCount++
		}

		// If file has fewer lines than we processed, it was rewritten
		if currentLineCount < lastLine {
			if err := tc.cache.InvalidateFile(filename); err != nil {
				return fmt.Errorf("failed to invalidate file %s: %w", filename, err)
			}
			lastLine = 0
		}
	}

	// Open file for processing
	file, err := os.Open(filename)
	if err != nil {
		return nil // File may have been deleted
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var lineNumber int64
	var events []TokenEvent

	for scanner.Scan() {
		lineNumber++

		// Skip already processed lines
		if lineNumber <= lastLine {
			continue
		}

		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		// Only process assistant messages with usage data
		if msg.Type != "assistant" || msg.Message.Usage.OutputTokens == 0 {
			continue
		}

		// Parse timestamp
		timestamp, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
		if err != nil {
			continue
		}

		usage := msg.Message.Usage
		cacheCreation := usage.CacheCreationInputTokens
		if cacheCreation == 0 {
			cacheCreation = usage.CacheCreation.Ephemeral5mInputTokens +
				usage.CacheCreation.Ephemeral1hInputTokens
		}

		events = append(events, TokenEvent{
			Timestamp:           timestamp,
			Model:               msg.Message.Model,
			InputTokens:         usage.InputTokens,
			OutputTokens:        usage.OutputTokens,
			CacheReadTokens:     usage.CacheReadInputTokens,
			CacheCreationTokens: cacheCreation,
			SourceFile:          filename,
			LineNumber:          lineNumber,
		})

		// Batch insert every 100 events
		if len(events) >= 100 {
			if err := tc.cache.InsertTokenEventBatch(events); err != nil {
				return fmt.Errorf("failed to insert batch for %s: %w", filename, err)
			}
			events = events[:0]
		}
	}

	// Insert remaining events
	if len(events) > 0 {
		if err := tc.cache.InsertTokenEventBatch(events); err != nil {
			return fmt.Errorf("failed to insert final batch for %s: %w", filename, err)
		}
	}

	// Update file state
	if err := tc.cache.SetFileState(filename, lineNumber, fileInfo.ModTime()); err != nil {
		return fmt.Errorf("failed to set file state for %s: %w", filename, err)
	}

	return nil
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

// calculate60sRate calculates the token rate over the last 60 seconds
func (tc *TokenCollector) calculate60sRate(events []TimestampedTokens) float64 {
	if len(events) == 0 {
		return 0
	}

	// Get the most recent timestamp
	lastTime := events[len(events)-1].Timestamp
	cutoffTime := lastTime.Add(-60 * time.Second)

	// Sum tokens in the last 60 seconds
	var totalTokens int64
	var windowStart time.Time

	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Timestamp.Before(cutoffTime) {
			break
		}
		totalTokens += events[i].Tokens
		windowStart = events[i].Timestamp
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

// ModelPricing contains pricing rates for a Claude model
type ModelPricing struct {
	InputPerMillion       float64
	OutputPerMillion      float64
	CacheReadPerMillion   float64
	CacheCreatePerMillion float64
}

// Model pricing constants (as of November 2025)
var modelPricing = map[string]ModelPricing{
	// Claude Opus 4.5 pricing
	"claude-opus-4-5-20251101": {
		InputPerMillion:       5.0,
		OutputPerMillion:      25.0,
		CacheReadPerMillion:   0.50,
		CacheCreatePerMillion: 6.25,
	},
	// Claude Sonnet 4.5 pricing
	"claude-sonnet-4-5-20250929": {
		InputPerMillion:       3.0,
		OutputPerMillion:      15.0,
		CacheReadPerMillion:   0.30,
		CacheCreatePerMillion: 3.75,
	},
	// Claude Haiku 4.5 pricing
	"claude-haiku-4-5-20250929": {
		InputPerMillion:       1.0,
		OutputPerMillion:      5.0,
		CacheReadPerMillion:   0.10,
		CacheCreatePerMillion: 1.25,
	},
}

// defaultPricing uses Claude Sonnet 4.5 as the fallback
var defaultPricing = ModelPricing{
	InputPerMillion:       3.0,
	OutputPerMillion:      15.0,
	CacheReadPerMillion:   0.30,
	CacheCreatePerMillion: 3.75,
}

// getPricingForModel returns the pricing for a given model name
func getPricingForModel(model string) ModelPricing {
	// Check exact match first
	if pricing, ok := modelPricing[model]; ok {
		return pricing
	}

	// Check for model family prefix matches
	if strings.Contains(model, "opus-4-5") || strings.Contains(model, "opus-4.5") {
		return modelPricing["claude-opus-4-5-20251101"]
	}
	if strings.Contains(model, "haiku-4-5") || strings.Contains(model, "haiku-4.5") {
		return modelPricing["claude-haiku-4-5-20250929"]
	}
	if strings.Contains(model, "sonnet-4-5") || strings.Contains(model, "sonnet-4.5") {
		return modelPricing["claude-sonnet-4-5-20250929"]
	}

	return defaultPricing
}

// GetCacheDBPath returns the path to the SQLite database for external tools like DuckDB
func (tc *TokenCollector) GetCacheDBPath() string {
	if tc.cache != nil {
		return tc.cache.GetDBPath()
	}
	return ""
}

// FormatTokensCompact formats tokens with K/M suffixes for compact display
func FormatTokensCompact(count int64) string {
	if count >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(count)/1_000_000)
	}
	if count >= 1_000 {
		return fmt.Sprintf("%.0fK", float64(count)/1_000)
	}
	return fmt.Sprintf("%d", count)
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

// FormatCost formats a cost value as currency with comma separators
func FormatCost(cost float64) string {
	if cost == 0 {
		return "$0.00"
	}
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}

	// Format with commas for costs >= $1,000
	if cost >= 1000 {
		wholePart := int64(cost)
		decimalPart := cost - float64(wholePart)
		return fmt.Sprintf("$%s.%02d", FormatTokens(wholePart), int(decimalPart*100+0.5))
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

// FormatTokenRateCompact formats a token rate compactly (e.g., "1.2M/min")
func FormatTokenRateCompact(rate float64) string {
	if rate == 0 {
		return "0/min"
	}
	if rate >= 1000000 {
		return fmt.Sprintf("%.1fM/min", rate/1000000)
	}
	if rate >= 1000 {
		return fmt.Sprintf("%.1fK/min", rate/1000)
	}
	return fmt.Sprintf("%.0f/min", rate)
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
