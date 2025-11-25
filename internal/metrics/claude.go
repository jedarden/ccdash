package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeUsageEntry represents a single line from Claude Code's JSONL files
type ClaudeUsageEntry struct {
	Type      string    `json:"type"`
	Message   *Message  `json:"message,omitempty"`
	Timestamp string    `json:"timestamp"`
	SessionID string    `json:"sessionId"`
	UUID      string    `json:"uuid"`
}

// Message contains the API response with usage data
type Message struct {
	Model string       `json:"model"`
	Role  string       `json:"role"`
	Usage *UsageData   `json:"usage,omitempty"`
}

// UsageData contains token usage information
type UsageData struct {
	InputTokens              int64         `json:"input_tokens"`
	OutputTokens             int64         `json:"output_tokens"`
	CacheCreationInputTokens int64         `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64         `json:"cache_read_input_tokens"`
	CacheCreation            *CacheDetails `json:"cache_creation,omitempty"`
	ServiceTier              string        `json:"service_tier"`
}

// CacheDetails contains cache-specific token counts
type CacheDetails struct {
	Ephemeral5mInputTokens  int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens  int64 `json:"ephemeral_1h_input_tokens"`
}

// ClaudeUsageCollector collects usage directly from Claude Code's JSONL files
type ClaudeUsageCollector struct {
	claudeDir string
}

// NewClaudeUsageCollector creates a new collector for Claude Code usage
func NewClaudeUsageCollector() *ClaudeUsageCollector {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return &ClaudeUsageCollector{claudeDir: ""}
	}
	return &ClaudeUsageCollector{
		claudeDir: filepath.Join(homeDir, ".claude"),
	}
}

// CollectUsage reads all JSONL files and aggregates token usage
func (c *ClaudeUsageCollector) CollectUsage() (*TokenMetrics, error) {
	metrics := &TokenMetrics{
		LastUpdate: time.Now(),
		Available:  false,
	}

	var earliestTime time.Time
	var latestTime time.Time

	// Check if Claude directory exists
	if c.claudeDir == "" || !c.dirExists(c.claudeDir) {
		metrics.Error = "Claude Code directory not found (~/.claude)"
		return metrics, nil
	}

	// Find all JSONL files in projects directory
	projectsDir := filepath.Join(c.claudeDir, "projects")
	if !c.dirExists(projectsDir) {
		metrics.Error = "No projects directory found"
		return metrics, nil
	}

	var totalInput int64
	var totalOutput int64
	var totalCacheCreation int64
	var totalCacheRead int64
	modelsMap := make(map[string]bool)

	// Walk through all project directories
	err := filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		// Only process .jsonl files
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			// Skip agent files (they don't have usage data)
			if strings.Contains(filepath.Base(path), "agent-") {
				return nil
			}

			// Parse this JSONL file
			input, output, cacheCreate, cacheRead, models, earliest, latest := c.parseJSONL(path)
			totalInput += input
			totalOutput += output
			totalCacheCreation += cacheCreate
			totalCacheRead += cacheRead

			for _, model := range models {
				modelsMap[model] = true
			}

			// Track earliest and latest timestamps
			if !earliest.IsZero() {
				if earliestTime.IsZero() || earliest.Before(earliestTime) {
					earliestTime = earliest
				}
			}
			if !latest.IsZero() {
				if latestTime.IsZero() || latest.After(latestTime) {
					latestTime = latest
				}
			}
		}

		return nil
	})

	if err != nil {
		metrics.Error = fmt.Sprintf("error walking projects: %v", err)
		return metrics, nil
	}

	// No data found
	if totalInput == 0 && totalOutput == 0 {
		metrics.Error = "No usage data found in Claude Code projects"
		return metrics, nil
	}

	// Populate metrics
	metrics.InputTokens = totalInput
	metrics.OutputTokens = totalOutput
	metrics.CacheCreationTokens = totalCacheCreation
	metrics.CacheReadTokens = totalCacheRead
	metrics.TotalTokens = totalInput + totalOutput + totalCacheCreation + totalCacheRead
	metrics.Available = true

	// Convert models map to slice
	for model := range modelsMap {
		metrics.Models = append(metrics.Models, model)
	}

	// Estimate cost (simplified pricing)
	metrics.TotalCost = c.estimateCost(metrics)

	// Set time span
	if !earliestTime.IsZero() && !latestTime.IsZero() {
		metrics.EarliestTimestamp = earliestTime
		metrics.LatestTimestamp = latestTime
		metrics.TimeSpan = latestTime.Sub(earliestTime)
	}

	return metrics, nil
}

// parseJSONL parses a single JSONL file and extracts usage data
func (c *ClaudeUsageCollector) parseJSONL(path string) (int64, int64, int64, int64, []string, time.Time, time.Time) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, 0, nil, time.Time{}, time.Time{}
	}
	defer file.Close()

	var totalInput int64
	var totalOutput int64
	var totalCacheCreation int64
	var totalCacheRead int64
	var models []string
	modelsMap := make(map[string]bool)
	var earliestTime time.Time
	var latestTime time.Time

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry ClaudeUsageEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed lines
		}

		// Only process assistant messages with usage data
		if entry.Message != nil && entry.Message.Role == "assistant" && entry.Message.Usage != nil {
			usage := entry.Message.Usage
			totalInput += usage.InputTokens
			totalOutput += usage.OutputTokens
			totalCacheCreation += usage.CacheCreationInputTokens
			totalCacheRead += usage.CacheReadInputTokens

			// Track models
			if entry.Message.Model != "" {
				modelsMap[entry.Message.Model] = true
			}

			// Track timestamps
			if entry.Timestamp != "" {
				if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
					if earliestTime.IsZero() || t.Before(earliestTime) {
						earliestTime = t
					}
					if latestTime.IsZero() || t.After(latestTime) {
						latestTime = t
					}
				}
			}
		}
	}

	// Convert models map to slice
	for model := range modelsMap {
		models = append(models, model)
	}

	return totalInput, totalOutput, totalCacheCreation, totalCacheRead, models, earliestTime, latestTime
}

// dirExists checks if a directory exists
func (c *ClaudeUsageCollector) dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// estimateCost provides a cost estimate based on model-specific pricing
func (c *ClaudeUsageCollector) estimateCost(metrics *TokenMetrics) float64 {
	// Determine pricing based on model used
	pricing := defaultPricing
	if len(metrics.Models) > 0 {
		pricing = getPricingForModel(metrics.Models[0])
	}

	cost := float64(metrics.InputTokens) * pricing.InputPerMillion / 1_000_000
	cost += float64(metrics.OutputTokens) * pricing.OutputPerMillion / 1_000_000
	cost += float64(metrics.CacheCreationTokens) * pricing.CacheCreatePerMillion / 1_000_000
	cost += float64(metrics.CacheReadTokens) * pricing.CacheReadPerMillion / 1_000_000

	return cost
}
