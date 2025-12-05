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
	Rate                float64       `json:"rate"`              // tokens/min over 60s window
	SessionAvgRate      float64       `json:"session_avg_rate"`  // average tokens/min for entire session
	TimeSpan            time.Duration `json:"time_span"`
	EarliestTimestamp   time.Time     `json:"earliest_timestamp"`
	LatestTimestamp     time.Time     `json:"latest_timestamp"`
	LookbackFrom        time.Time     `json:"lookback_from"`     // Start of measurement period
	Models              []string      `json:"models"`
	ModelUsages         []ModelUsage  `json:"model_usages"`      // Per-model breakdown
	Available           bool          `json:"available"`
	Error               string        `json:"error,omitempty"`
	LastUpdate          time.Time     `json:"last_update"`
}

// TokenCollector collects and aggregates token usage from Claude Code sessions
type TokenCollector struct {
	projectsDir  string
	lookbackFrom time.Time // Only include data from this time onwards
	cache        *TokenCache
	// Track file line counts for incremental processing
	fileLineCache map[string]int64
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
			projectsDir:   "",
			lookbackFrom:  GetMondayNineAM(),
			cache:         NewTokenCache(),
			fileLineCache: make(map[string]int64),
		}
	}
	return &TokenCollector{
		projectsDir:   filepath.Join(home, ".claude", "projects"),
		lookbackFrom:  GetMondayNineAM(),
		cache:         NewTokenCache(),
		fileLineCache: make(map[string]int64),
	}
}

// NewTokenCollectorWithLookback creates a TokenCollector with a custom lookback time
func NewTokenCollectorWithLookback(lookbackFrom time.Time) *TokenCollector {
	home, err := os.UserHomeDir()
	if err != nil {
		return &TokenCollector{
			projectsDir:   "",
			lookbackFrom:  lookbackFrom,
			cache:         NewTokenCache(),
			fileLineCache: make(map[string]int64),
		}
	}
	return &TokenCollector{
		projectsDir:   filepath.Join(home, ".claude", "projects"),
		lookbackFrom:  lookbackFrom,
		cache:         NewTokenCache(),
		fileLineCache: make(map[string]int64),
	}
}

// NewTokenCollectorWithPath creates a TokenCollector with a custom path (for testing)
func NewTokenCollectorWithPath(path string) *TokenCollector {
	return &TokenCollector{
		projectsDir:   path,
		lookbackFrom:  GetMondayNineAM(),
		cache:         NewTokenCache(),
		fileLineCache: make(map[string]int64),
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
// Uses two-tier processing: files within lookback window are processed first (real-time),
// then historical data is loaded from cache for entries outside the lookback window.
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

	// TWO-TIER PROCESSING:
	// Tier 1: Process entries within lookback window (real-time, fresh data)
	// Tier 2: Use cached data for historical entries (outside lookback window)

	var allTimestamps []timestampedTokens
	modelSet := make(map[string]bool)
	aggregatedModelMetrics := make(map[string]*perModelMetrics)

	// Tier 1: Process files for entries within lookback window
	for _, file := range files {
		fileMetrics, timestamps, fileModelMetrics := tc.parseJSONLFile(file)
		metrics.InputTokens += fileMetrics.InputTokens
		metrics.OutputTokens += fileMetrics.OutputTokens
		metrics.CacheReadTokens += fileMetrics.CacheReadTokens
		metrics.CacheCreationTokens += fileMetrics.CacheCreationTokens
		allTimestamps = append(allTimestamps, timestamps...)

		for _, model := range fileMetrics.Models {
			modelSet[model] = true
		}

		// Aggregate per-model metrics across files
		for model, mm := range fileModelMetrics {
			if _, exists := aggregatedModelMetrics[model]; !exists {
				aggregatedModelMetrics[model] = &perModelMetrics{}
			}
			aggregatedModelMetrics[model].inputTokens += mm.inputTokens
			aggregatedModelMetrics[model].outputTokens += mm.outputTokens
			aggregatedModelMetrics[model].cacheReadTokens += mm.cacheReadTokens
			aggregatedModelMetrics[model].cacheCreationTokens += mm.cacheCreationTokens
		}

		// Tier 2: Cache historical data from this file for future use
		// This processes entries BEFORE the lookback window and caches them
		tc.cacheHistoricalData(file)
	}

	// Save cache periodically (every collection cycle)
	if tc.cache != nil {
		tc.cache.Save()
	}

	// Calculate total tokens
	metrics.TotalTokens = metrics.InputTokens + metrics.OutputTokens +
		metrics.CacheReadTokens + metrics.CacheCreationTokens

	// Convert model set to sorted slice (needed for cost calculation)
	for model := range modelSet {
		metrics.Models = append(metrics.Models, model)
	}
	sort.Strings(metrics.Models)

	// Build per-model usage with costs
	metrics.ModelUsages = make([]ModelUsage, 0, len(aggregatedModelMetrics))
	var totalCost float64
	for model, mm := range aggregatedModelMetrics {
		pricing := getPricingForModel(model)
		inputCost := float64(mm.inputTokens) * pricing.InputPerMillion / 1_000_000
		outputCost := float64(mm.outputTokens) * pricing.OutputPerMillion / 1_000_000
		cacheReadCost := float64(mm.cacheReadTokens) * pricing.CacheReadPerMillion / 1_000_000
		cacheCreateCost := float64(mm.cacheCreationTokens) * pricing.CacheCreatePerMillion / 1_000_000
		modelCost := inputCost + outputCost + cacheReadCost + cacheCreateCost

		usage := ModelUsage{
			Model:               model,
			InputTokens:         mm.inputTokens,
			OutputTokens:        mm.outputTokens,
			CacheReadTokens:     mm.cacheReadTokens,
			CacheCreationTokens: mm.cacheCreationTokens,
			TotalTokens:         mm.inputTokens + mm.outputTokens + mm.cacheReadTokens + mm.cacheCreationTokens,
			Cost:                modelCost,
		}
		metrics.ModelUsages = append(metrics.ModelUsages, usage)
		totalCost += modelCost
	}

	// Sort model usages by cost (highest first)
	sort.Slice(metrics.ModelUsages, func(i, j int) bool {
		return metrics.ModelUsages[i].Cost > metrics.ModelUsages[j].Cost
	})

	// Set total cost from per-model calculations
	metrics.TotalCost = totalCost

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

// perModelMetrics tracks metrics for a single model during parsing
type perModelMetrics struct {
	inputTokens         int64
	outputTokens        int64
	cacheReadTokens     int64
	cacheCreationTokens int64
}

// parseJSONLFile parses a single JSONL file and extracts token metrics
func (tc *TokenCollector) parseJSONLFile(filename string) (TokenMetrics, []timestampedTokens, map[string]*perModelMetrics) {
	metrics := TokenMetrics{}
	var timestamps []timestampedTokens
	modelSet := make(map[string]bool)
	modelMetrics := make(map[string]*perModelMetrics)

	file, err := os.Open(filename)
	if err != nil {
		return metrics, timestamps, modelMetrics
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

		// Skip entries before lookback time
		if !tc.lookbackFrom.IsZero() && timestamp.Before(tc.lookbackFrom) {
			continue
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

		// Track model and per-model metrics
		if msg.Message.Model != "" {
			modelSet[msg.Message.Model] = true

			// Initialize per-model metrics if needed
			if _, exists := modelMetrics[msg.Message.Model]; !exists {
				modelMetrics[msg.Message.Model] = &perModelMetrics{}
			}

			// Accumulate per-model tokens
			mm := modelMetrics[msg.Message.Model]
			mm.inputTokens += usage.InputTokens
			mm.outputTokens += usage.OutputTokens
			mm.cacheReadTokens += usage.CacheReadInputTokens
			mm.cacheCreationTokens += cacheCreation
		}
	}

	// Convert model set to slice
	for model := range modelSet {
		metrics.Models = append(metrics.Models, model)
	}

	return metrics, timestamps, modelMetrics
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

// cacheHistoricalData processes and caches entries before the lookback window
// This implements Tier 2 of the two-tier processing system
func (tc *TokenCollector) cacheHistoricalData(filename string) {
	if tc.cache == nil || tc.lookbackFrom.IsZero() {
		return // No cache or no lookback filter = nothing to cache
	}

	// Check file modification time
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return
	}

	// Skip if cache is fresh
	if !tc.cache.IsStale(filename, fileInfo.ModTime()) {
		return
	}

	// Parse file for historical entries (before lookback)
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	cached := &CachedTokenData{
		Models:       make(map[string]int64),
		ModelCosts:   make(map[string]float64),
		LastModified: fileInfo.ModTime(),
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var lineCount int64
	for scanner.Scan() {
		lineCount++

		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		if msg.Type != "assistant" || msg.Message.Usage.OutputTokens == 0 {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
		if err != nil {
			continue
		}

		// Only cache entries BEFORE the lookback window
		if timestamp.After(tc.lookbackFrom) || timestamp.Equal(tc.lookbackFrom) {
			continue
		}

		usage := msg.Message.Usage
		cached.InputTokens += usage.InputTokens
		cached.OutputTokens += usage.OutputTokens
		cached.CacheReadTokens += usage.CacheReadInputTokens

		cacheCreation := usage.CacheCreationInputTokens
		if cacheCreation == 0 {
			cacheCreation = usage.CacheCreation.Ephemeral5mInputTokens +
				usage.CacheCreation.Ephemeral1hInputTokens
		}
		cached.CacheCreationTokens += cacheCreation

		// Track per-model data
		if msg.Message.Model != "" {
			totalTokens := usage.InputTokens + usage.OutputTokens + usage.CacheReadInputTokens + cacheCreation
			cached.Models[msg.Message.Model] += totalTokens

			// Calculate cost for this model
			pricing := getPricingForModel(msg.Message.Model)
			inputCost := float64(usage.InputTokens) * pricing.InputPerMillion / 1_000_000
			outputCost := float64(usage.OutputTokens) * pricing.OutputPerMillion / 1_000_000
			cacheReadCost := float64(usage.CacheReadInputTokens) * pricing.CacheReadPerMillion / 1_000_000
			cacheCreateCost := float64(cacheCreation) * pricing.CacheCreatePerMillion / 1_000_000
			cached.ModelCosts[msg.Message.Model] += inputCost + outputCost + cacheReadCost + cacheCreateCost
		}
	}

	cached.LastProcessedLine = lineCount
	tc.cache.Set(filename, cached)
}

// GetCache returns the token cache (useful for accessing historical data)
func (tc *TokenCollector) GetCache() *TokenCache {
	return tc.cache
}

// calculateCost estimates the cost based on model-specific pricing
func (tc *TokenCollector) calculateCost(metrics TokenMetrics) float64 {
	// Determine pricing based on model used
	pricing := defaultPricing
	if len(metrics.Models) > 0 {
		// Use the first model's pricing (typically there's only one model per session)
		pricing = getPricingForModel(metrics.Models[0])
	}

	inputCost := float64(metrics.InputTokens) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(metrics.OutputTokens) * pricing.OutputPerMillion / 1_000_000
	cacheReadCost := float64(metrics.CacheReadTokens) * pricing.CacheReadPerMillion / 1_000_000
	cacheCreateCost := float64(metrics.CacheCreationTokens) * pricing.CacheCreatePerMillion / 1_000_000

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
