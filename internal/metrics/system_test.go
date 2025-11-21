package metrics

import (
	"testing"
	"time"
)

func TestNewSystemCollector(t *testing.T) {
	collector := NewSystemCollector()

	if collector == nil {
		t.Fatal("NewSystemCollector returned nil")
	}

	if collector.prevIOCounters == nil {
		t.Error("prevIOCounters map not initialized")
	}

	if collector.prevIOTime.IsZero() {
		t.Error("prevIOTime not initialized")
	}
}

func TestCollect(t *testing.T) {
	collector := NewSystemCollector()
	metrics := collector.Collect()

	// Check that LastUpdate is recent
	if time.Since(metrics.LastUpdate) > 5*time.Second {
		t.Errorf("LastUpdate is too old: %v", metrics.LastUpdate)
	}

	// Check CPU metrics (allow error on unsupported systems)
	if metrics.CPU.Error == nil {
		if metrics.CPU.TotalPercent < 0 || metrics.CPU.TotalPercent > 100 {
			t.Errorf("Invalid TotalPercent: %f", metrics.CPU.TotalPercent)
		}
		if len(metrics.CPU.PerCore) == 0 {
			t.Error("PerCore is empty")
		}
		for i, pct := range metrics.CPU.PerCore {
			if pct < 0 || pct > 100 {
				t.Errorf("Invalid per-core percentage at index %d: %f", i, pct)
			}
		}
	}

	// Check Load metrics (may not be available on all systems)
	if metrics.Load.Error == nil {
		if metrics.Load.Load1 < 0 {
			t.Errorf("Invalid Load1: %f", metrics.Load.Load1)
		}
		if metrics.Load.Load5 < 0 {
			t.Errorf("Invalid Load5: %f", metrics.Load.Load5)
		}
		if metrics.Load.Load15 < 0 {
			t.Errorf("Invalid Load15: %f", metrics.Load.Load15)
		}
	}

	// Check Memory metrics
	if metrics.Memory.Error == nil {
		if metrics.Memory.Total == 0 {
			t.Error("Memory total is 0")
		}
		if metrics.Memory.Used > metrics.Memory.Total {
			t.Errorf("Memory used (%d) exceeds total (%d)", metrics.Memory.Used, metrics.Memory.Total)
		}
		if metrics.Memory.Percentage < 0 || metrics.Memory.Percentage > 100 {
			t.Errorf("Invalid memory percentage: %f", metrics.Memory.Percentage)
		}
	} else {
		t.Logf("Memory collection error (may be expected): %v", metrics.Memory.Error)
	}

	// Check Swap metrics (swap may not be configured)
	if metrics.Swap.Error == nil {
		if metrics.Swap.Used > metrics.Swap.Total {
			t.Errorf("Swap used (%d) exceeds total (%d)", metrics.Swap.Used, metrics.Swap.Total)
		}
		if metrics.Swap.Total > 0 {
			if metrics.Swap.Percentage < 0 || metrics.Swap.Percentage > 100 {
				t.Errorf("Invalid swap percentage: %f", metrics.Swap.Percentage)
			}
		}
	}

	// Disk I/O metrics (first collection may be zero)
	if metrics.DiskIO.Error == nil {
		if metrics.DiskIO.ReadBytesPerSec < 0 {
			t.Errorf("Invalid ReadBytesPerSec: %f", metrics.DiskIO.ReadBytesPerSec)
		}
		if metrics.DiskIO.WriteBytesPerSec < 0 {
			t.Errorf("Invalid WriteBytesPerSec: %f", metrics.DiskIO.WriteBytesPerSec)
		}
	}
}

func TestCollectMultipleTimes(t *testing.T) {
	collector := NewSystemCollector()

	// First collection
	metrics1 := collector.Collect()
	if metrics1.LastUpdate.IsZero() {
		t.Error("First collection LastUpdate is zero")
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Second collection
	metrics2 := collector.Collect()
	if metrics2.LastUpdate.IsZero() {
		t.Error("Second collection LastUpdate is zero")
	}

	// Verify timestamps are different
	if !metrics2.LastUpdate.After(metrics1.LastUpdate) {
		t.Error("Second collection timestamp should be after first")
	}

	// Disk I/O rates should potentially be calculated now
	if metrics2.DiskIO.Error == nil {
		t.Logf("Disk I/O - Read: %.2f B/s, Write: %.2f B/s",
			metrics2.DiskIO.ReadBytesPerSec,
			metrics2.DiskIO.WriteBytesPerSec)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    uint64
		expected string
	}{
		{"Zero bytes", 0, "0 B"},
		{"Bytes", 512, "512 B"},
		{"Kilobytes", 1024, "1.00 KB"},
		{"Kilobytes with fraction", 1536, "1.50 KB"},
		{"Megabytes", 1048576, "1.00 MB"},
		{"Megabytes with fraction", 1572864, "1.50 MB"},
		{"Gigabytes", 1073741824, "1.00 GB"},
		{"Gigabytes with fraction", 1610612736, "1.50 GB"},
		{"Terabytes", 1099511627776, "1.00 TB"},
		{"Large value", 5497558138880, "5.00 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("FormatBytes(%d) = %s; want %s", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestFormatRate(t *testing.T) {
	tests := []struct {
		name         string
		bytesPerSec  float64
		expected     string
	}{
		{"Zero rate", 0, "0.00 B/s"},
		{"Bytes per second", 512.5, "512.50 B/s"},
		{"Kilobytes per second", 1024, "1.00 KB/s"},
		{"KB/s with fraction", 1536, "1.50 KB/s"},
		{"Megabytes per second", 1048576, "1.00 MB/s"},
		{"MB/s with fraction", 1572864, "1.50 MB/s"},
		{"Gigabytes per second", 1073741824, "1.00 GB/s"},
		{"GB/s with fraction", 1610612736, "1.50 GB/s"},
		{"Terabytes per second", 1099511627776, "1.00 TB/s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRate(tt.bytesPerSec)
			if result != tt.expected {
				t.Errorf("FormatRate(%f) = %s; want %s", tt.bytesPerSec, result, tt.expected)
			}
		})
	}
}

func TestCollectCPU(t *testing.T) {
	collector := NewSystemCollector()
	cpuMetrics := collector.collectCPU()

	// CPU collection may take time due to 1-second interval
	if cpuMetrics.Error != nil {
		t.Logf("CPU collection error (may be expected on some systems): %v", cpuMetrics.Error)
		return
	}

	if cpuMetrics.TotalPercent < 0 || cpuMetrics.TotalPercent > 100 {
		t.Errorf("Invalid TotalPercent: %f", cpuMetrics.TotalPercent)
	}

	if len(cpuMetrics.PerCore) == 0 {
		t.Error("PerCore slice is empty")
	}

	// Verify average calculation
	var sum float64
	for _, pct := range cpuMetrics.PerCore {
		sum += pct
	}
	expectedAvg := sum / float64(len(cpuMetrics.PerCore))
	
	// Allow small floating point difference
	diff := cpuMetrics.TotalPercent - expectedAvg
	if diff < -0.01 || diff > 0.01 {
		t.Errorf("TotalPercent (%f) doesn't match average of PerCore (%f)",
			cpuMetrics.TotalPercent, expectedAvg)
	}
}

func TestCollectLoad(t *testing.T) {
	collector := NewSystemCollector()
	loadMetrics := collector.collectLoad()

	// Load averages may not be available on all platforms
	if loadMetrics.Error != nil {
		t.Logf("Load collection error (may be expected on Windows): %v", loadMetrics.Error)
		return
	}

	if loadMetrics.Load1 < 0 {
		t.Errorf("Invalid Load1: %f", loadMetrics.Load1)
	}
	if loadMetrics.Load5 < 0 {
		t.Errorf("Invalid Load5: %f", loadMetrics.Load5)
	}
	if loadMetrics.Load15 < 0 {
		t.Errorf("Invalid Load15: %f", loadMetrics.Load15)
	}
}

func TestCollectMemory(t *testing.T) {
	collector := NewSystemCollector()
	memMetrics := collector.collectMemory()

	if memMetrics.Error != nil {
		t.Fatalf("Memory collection failed: %v", memMetrics.Error)
	}

	if memMetrics.Total == 0 {
		t.Error("Total memory is 0")
	}

	if memMetrics.Used > memMetrics.Total {
		t.Errorf("Used memory (%d) exceeds total (%d)", memMetrics.Used, memMetrics.Total)
	}

	if memMetrics.Percentage < 0 || memMetrics.Percentage > 100 {
		t.Errorf("Invalid percentage: %f", memMetrics.Percentage)
	}

	// Log for informational purposes
	t.Logf("Memory: %s / %s (%.2f%%)",
		FormatBytes(memMetrics.Used),
		FormatBytes(memMetrics.Total),
		memMetrics.Percentage)
}

func TestCollectSwap(t *testing.T) {
	collector := NewSystemCollector()
	swapMetrics := collector.collectSwap()

	if swapMetrics.Error != nil {
		t.Logf("Swap collection error (may be expected): %v", swapMetrics.Error)
		return
	}

	if swapMetrics.Used > swapMetrics.Total {
		t.Errorf("Used swap (%d) exceeds total (%d)", swapMetrics.Used, swapMetrics.Total)
	}

	if swapMetrics.Total > 0 {
		if swapMetrics.Percentage < 0 || swapMetrics.Percentage > 100 {
			t.Errorf("Invalid percentage: %f", swapMetrics.Percentage)
		}
		t.Logf("Swap: %s / %s (%.2f%%)",
			FormatBytes(swapMetrics.Used),
			FormatBytes(swapMetrics.Total),
			swapMetrics.Percentage)
	} else {
		t.Log("No swap space configured")
	}
}

func TestCollectDiskIO(t *testing.T) {
	collector := NewSystemCollector()

	// First collection
	ioMetrics1 := collector.collectDiskIO()
	if ioMetrics1.Error != nil {
		t.Fatalf("First disk I/O collection failed: %v", ioMetrics1.Error)
	}

	// First collection should have zero rates (no previous data)
	if ioMetrics1.ReadBytesPerSec != 0 || ioMetrics1.WriteBytesPerSec != 0 {
		t.Log("First collection has non-zero rates (previous data may exist)")
	}

	// Wait and collect again
	time.Sleep(500 * time.Millisecond)

	ioMetrics2 := collector.collectDiskIO()
	if ioMetrics2.Error != nil {
		t.Fatalf("Second disk I/O collection failed: %v", ioMetrics2.Error)
	}

	// Rates should be calculated now (may still be zero if no I/O occurred)
	if ioMetrics2.ReadBytesPerSec < 0 {
		t.Errorf("Invalid ReadBytesPerSec: %f", ioMetrics2.ReadBytesPerSec)
	}
	if ioMetrics2.WriteBytesPerSec < 0 {
		t.Errorf("Invalid WriteBytesPerSec: %f", ioMetrics2.WriteBytesPerSec)
	}

	t.Logf("Disk I/O - Read: %s, Write: %s",
		FormatRate(ioMetrics2.ReadBytesPerSec),
		FormatRate(ioMetrics2.WriteBytesPerSec))
}

func BenchmarkCollect(b *testing.B) {
	collector := NewSystemCollector()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = collector.Collect()
	}
}

func BenchmarkFormatBytes(b *testing.B) {
	testValue := uint64(1572864) // 1.5 MB

	for i := 0; i < b.N; i++ {
		_ = FormatBytes(testValue)
	}
}

func BenchmarkFormatRate(b *testing.B) {
	testValue := 1572864.0 // 1.5 MB/s

	for i := 0; i < b.N; i++ {
		_ = FormatRate(testValue)
	}
}
