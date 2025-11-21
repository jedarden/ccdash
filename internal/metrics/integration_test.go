package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIntegration_AllCollectors tests all collectors together
func TestIntegration_AllCollectors(t *testing.T) {
	systemCollector := NewSystemCollector()
	tokenCollector := NewTokenCollector()
	tmuxCollector := NewTmuxCollector()

	// Collect system metrics
	systemMetrics, err := systemCollector.Collect()
	if err != nil {
		t.Fatalf("System collection failed: %v", err)
	}
	if systemMetrics == nil {
		t.Fatal("System metrics is nil")
	}
	t.Logf("System: CPU=%.1f%%, Memory=%.1f%%",
		systemMetrics.CPUTotal, systemMetrics.MemoryPercent)

	// Collect token metrics
	tokenMetrics, err := tokenCollector.Collect()
	if err != nil {
		t.Fatalf("Token collection failed: %v", err)
	}
	if tokenMetrics == nil {
		t.Fatal("Token metrics is nil")
	}
	if tokenMetrics.Available {
		t.Logf("Tokens: Total=%s, Cost=%s",
			FormatTokens(tokenMetrics.TotalTokens),
			FormatCost(tokenMetrics.TotalCost))
	} else {
		t.Logf("Tokens: Not available - %s", tokenMetrics.Error)
	}

	// Collect tmux metrics
	tmuxMetrics := tmuxCollector.Collect()
	if tmuxMetrics == nil {
		t.Fatal("Tmux metrics is nil")
	}
	if tmuxMetrics.Available {
		t.Logf("Tmux: %d sessions", tmuxMetrics.Total)
	} else {
		t.Logf("Tmux: Not available - %s", tmuxMetrics.Error)
	}
}

// TestIntegration_SystemMetricsOverTime tests system metrics collection over time
func TestIntegration_SystemMetricsOverTime(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping time-based test in short mode")
	}

	collector := NewSystemCollector()

	// Collect multiple samples
	samples := 3
	for i := 0; i < samples; i++ {
		metrics, err := collector.Collect()
		if err != nil {
			t.Fatalf("Collection %d failed: %v", i, err)
		}

		t.Logf("Sample %d: CPU=%.1f%%, Mem=%.1f%%, DiskRead=%s, DiskWrite=%s",
			i,
			metrics.CPUTotal,
			metrics.MemoryPercent,
			FormatRate(metrics.DiskReadRate),
			FormatRate(metrics.DiskWriteRate))

		if i < samples-1 {
			time.Sleep(1 * time.Second)
		}
	}

	// Disk rates should be calculable after first sample
	metrics, _ := collector.Collect()
	if collector.lastDiskIO == nil {
		t.Error("Disk IO tracking should be initialized")
	}

	t.Logf("Final metrics: %+v", metrics)
}

// TestIntegration_TokenRateTracking tests token rate calculation over time
func TestIntegration_TokenRateTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping time-based test in short mode")
	}

	collector := NewTokenCollector()

	// Simulate token usage over time
	baseTime := time.Now()
	for i := 0; i < 5; i++ {
		collector.addToHistory(TokenSnapshot{
			Timestamp:   baseTime.Add(time.Duration(i*10) * time.Second),
			TotalTokens: int64((i + 1) * 1000),
		})
	}

	rate := collector.calculateRate()
	t.Logf("Token rate: %s", FormatTokenRate(rate))

	// Rate should be positive if there's token growth
	if len(collector.history) >= 2 {
		oldest := collector.history[0]
		newest := collector.history[len(collector.history)-1]
		if newest.TotalTokens > oldest.TotalTokens && rate <= 0 {
			t.Error("Rate should be positive when tokens are increasing")
		}
	}
}

// TestIntegration_MockCCUsageEndToEnd tests full ccusage integration with mock
func TestIntegration_MockCCUsageEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "ccusage")

	// Create realistic mock ccusage output
	mockData := CCUsageResponse{
		Daily: []struct {
			Date                string   `json:"date"`
			InputTokens         int64    `json:"inputTokens"`
			OutputTokens        int64    `json:"outputTokens"`
			CacheCreationTokens int64    `json:"cacheCreationTokens"`
			CacheReadTokens     int64    `json:"cacheReadTokens"`
			TotalTokens         int64    `json:"totalTokens"`
			TotalCost           float64  `json:"totalCost"`
			ModelsUsed          []string `json:"modelsUsed"`
		}{
			{
				Date:                "2025-11-20",
				InputTokens:         45000,
				OutputTokens:        23000,
				CacheCreationTokens: 8000,
				CacheReadTokens:     4000,
				TotalTokens:         80000,
				TotalCost:           1.2500,
				ModelsUsed:          []string{"claude-sonnet-4", "claude-opus-4"},
			},
		},
		Totals: struct {
			InputTokens         int64   `json:"inputTokens"`
			OutputTokens        int64   `json:"outputTokens"`
			CacheCreationTokens int64   `json:"cacheCreationTokens"`
			CacheReadTokens     int64   `json:"cacheReadTokens"`
			TotalCost           float64 `json:"totalCost"`
			TotalTokens         int64   `json:"totalTokens"`
		}{
			InputTokens:         145000,
			OutputTokens:        73000,
			CacheCreationTokens: 28000,
			CacheReadTokens:     14000,
			TotalTokens:         260000,
			TotalCost:           4.2500,
		},
	}

	jsonData, err := json.Marshal(mockData)
	if err != nil {
		t.Fatalf("Failed to marshal mock data: %v", err)
	}

	scriptContent := "#!/bin/bash\necho '" + string(jsonData) + "'\n"
	err = os.WriteFile(mockScript, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir+":"+originalPath)

	// Test collection
	collector := NewTokenCollector()
	metrics, err := collector.Collect()

	if err != nil {
		t.Fatalf("Collection failed: %v", err)
	}

	if !metrics.Available {
		t.Fatalf("Metrics should be available: %s", metrics.Error)
	}

	// Verify all fields
	if metrics.InputTokens != mockData.Totals.InputTokens {
		t.Errorf("InputTokens = %d; want %d",
			metrics.InputTokens, mockData.Totals.InputTokens)
	}
	if metrics.OutputTokens != mockData.Totals.OutputTokens {
		t.Errorf("OutputTokens = %d; want %d",
			metrics.OutputTokens, mockData.Totals.OutputTokens)
	}
	if metrics.TotalTokens != mockData.Totals.TotalTokens {
		t.Errorf("TotalTokens = %d; want %d",
			metrics.TotalTokens, mockData.Totals.TotalTokens)
	}
	if metrics.TotalCost != mockData.Totals.TotalCost {
		t.Errorf("TotalCost = %.4f; want %.4f",
			metrics.TotalCost, mockData.Totals.TotalCost)
	}

	if len(metrics.Models) != len(mockData.Daily[0].ModelsUsed) {
		t.Errorf("Models count = %d; want %d",
			len(metrics.Models), len(mockData.Daily[0].ModelsUsed))
	}

	t.Logf("Successfully parsed: %s tokens, %s, %d models",
		FormatTokens(metrics.TotalTokens),
		FormatCost(metrics.TotalCost),
		len(metrics.Models))
}

// TestIntegration_MockTmuxEndToEnd tests full tmux integration with mock
func TestIntegration_MockTmuxEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "tmux")

	// Create mock tmux that handles both list-sessions and capture-pane
	now := time.Now().Unix()
	scriptContent := fmt.Sprintf(`#!/bin/bash
if [ "$1" = "list-sessions" ]; then
	echo "webapp:4:%d:1"
	echo "backend:2:%d:0"
	echo "database:1:%d:0"
elif [ "$1" = "capture-pane" ]; then
	echo "Sample output from $4"
fi
`, now-15, now-400, now-45)
	err := os.WriteFile(mockScript, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	os.Setenv("PATH", tmpDir+":"+originalPath)

	// Test collection
	collector := NewTmuxCollector()
	metrics := collector.Collect()

	if !metrics.Available {
		t.Fatalf("Metrics should be available: %s", metrics.Error)
	}

	if metrics.Total != 3 {
		t.Errorf("Total sessions = %d; want 3", metrics.Total)
	}

	if len(metrics.Sessions) != 3 {
		t.Fatalf("Sessions count = %d; want 3", len(metrics.Sessions))
	}

	// Verify session details
	sessionMap := make(map[string]TmuxSession)
	for _, session := range metrics.Sessions {
		sessionMap[session.Name] = session
		age := time.Since(session.Created)
		t.Logf("Session: %s, Windows: %d, Status: %s, Attached: %v, Age: %s",
			session.Name,
			session.Windows,
			session.Status,
			session.Attached,
			age)
	}

	// Verify webapp session (recent activity, attached)
	if webapp, ok := sessionMap["webapp"]; ok {
		if !webapp.Attached {
			t.Error("webapp should be attached")
		}
		if webapp.Windows != 4 {
			t.Errorf("webapp windows = %d; want 4", webapp.Windows)
		}
		// Attached sessions should be WORKING
		if webapp.Status != StatusWorking {
			t.Errorf("webapp status = %s; want WORKING", webapp.Status)
		}
	} else {
		t.Error("webapp session not found")
	}

	// Verify backend session (detached)
	if backend, ok := sessionMap["backend"]; ok {
		if backend.Attached {
			t.Error("backend should not be attached")
		}
		// Status depends on session age and activity
	} else {
		t.Error("backend session not found")
	}
}

// TestIntegration_FormattingConsistency tests that all formatting functions work together
func TestIntegration_FormattingConsistency(t *testing.T) {
	// System formatting
	t.Run("System", func(t *testing.T) {
		bytes := []uint64{0, 1024, 1048576, 1073741824}
		for _, b := range bytes {
			formatted := FormatBytes(b)
			rate := FormatRate(b)
			t.Logf("%d bytes = %s, rate = %s", b, formatted, rate)
		}

		colors := []struct {
			percent  float64
			expected string
		}{
			{50.0, "#00ff00"},
			{85.0, "#ffaa00"},
			{96.0, "#ff0000"},
		}
		for _, c := range colors {
			color := GetStatusColor(c.percent, 80.0, 95.0)
			if color != c.expected {
				t.Errorf("Color for %.1f%% = %s; want %s",
					c.percent, color, c.expected)
			}
		}
	})

	// Token formatting
	t.Run("Tokens", func(t *testing.T) {
		tokens := []int64{0, 1000, 1234567}
		for _, tok := range tokens {
			formatted := FormatTokens(tok)
			t.Logf("%d tokens = %s", tok, formatted)
		}

		costs := []float64{0.0, 1.2345, 99.9999}
		for _, cost := range costs {
			formatted := FormatCost(cost)
			t.Logf("%.4f cost = %s", cost, formatted)
		}

		rates := []float64{0.0, 1234.5, 9999.9}
		for _, rate := range rates {
			formatted := FormatTokenRate(rate)
			t.Logf("%.1f rate = %s", rate, formatted)
		}
	})

	// Tmux formatting
	t.Run("Tmux", func(t *testing.T) {
		statuses := []SessionStatus{
			StatusReady, StatusWorking, StatusStalled, StatusIdle,
		}
		for _, status := range statuses {
			emoji := status.GetEmoji()
			color := status.GetColor()
			t.Logf("%s: %s %s", status, emoji, color)
		}
	})
}

// TestIntegration_ConcurrentCollection tests collecting all metrics concurrently
func TestIntegration_ConcurrentCollection(t *testing.T) {
	systemCollector := NewSystemCollector()
	tokenCollector := NewTokenCollector()
	tmuxCollector := NewTmuxCollector()

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 3)

	// Collect all metrics concurrently
	go func() {
		_, err := systemCollector.Collect()
		results <- result{"system", err}
	}()

	go func() {
		_, err := tokenCollector.Collect()
		results <- result{"token", err}
	}()

	go func() {
		tmuxCollector.Collect()
		results <- result{"tmux", nil}
	}()

	// Wait for all results
	for i := 0; i < 3; i++ {
		res := <-results
		if res.err != nil {
			t.Errorf("%s collection failed: %v", res.name, res.err)
		} else {
			t.Logf("%s collection succeeded", res.name)
		}
	}
}

// TestIntegration_RepeatedCollection tests that collectors work correctly when called multiple times
func TestIntegration_RepeatedCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping repeated collection test in short mode")
	}

	systemCollector := NewSystemCollector()
	iterations := 5

	for i := 0; i < iterations; i++ {
		metrics, err := systemCollector.Collect()
		if err != nil {
			t.Fatalf("Iteration %d failed: %v", i, err)
		}

		if metrics == nil {
			t.Fatalf("Iteration %d returned nil metrics", i)
		}

		// Verify metrics are reasonable
		if metrics.CPUTotal < 0 || metrics.CPUTotal > 100 {
			t.Errorf("Iteration %d: invalid CPU total: %.2f", i, metrics.CPUTotal)
		}

		if metrics.MemoryPercent < 0 || metrics.MemoryPercent > 100 {
			t.Errorf("Iteration %d: invalid memory percent: %.2f", i, metrics.MemoryPercent)
		}

		t.Logf("Iteration %d: CPU=%.1f%%, Memory=%s/%s",
			i,
			metrics.CPUTotal,
			FormatBytes(metrics.MemoryUsed),
			FormatBytes(metrics.MemoryTotal))

		if i < iterations-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}
