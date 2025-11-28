package metrics

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemMetrics holds all collected system metrics with timestamp
type SystemMetrics struct {
	CPU        CPUMetrics
	Load       LoadMetrics
	Memory     MemoryMetrics
	Swap       SwapMetrics
	DiskIO     DiskIOMetrics
	NetIO      NetIOMetrics
	LastUpdate time.Time
}

// CPUMetrics holds CPU usage information
type CPUMetrics struct {
	TotalPercent float64
	PerCore      []float64
	Error        error
}

// LoadMetrics holds system load averages
type LoadMetrics struct {
	Load1  float64
	Load5  float64
	Load15 float64
	Error  error
}

// MemoryMetrics holds memory usage information
type MemoryMetrics struct {
	Used       uint64
	Total      uint64
	Percentage float64
	Error      error
}

// SwapMetrics holds swap usage information
type SwapMetrics struct {
	Used       uint64
	Total      uint64
	Percentage float64
	Error      error
}

// DiskIOMetrics holds disk I/O rate information
type DiskIOMetrics struct {
	ReadBytesPerSec  float64
	WriteBytesPerSec float64
	Error            error
}

// NetIOMetrics holds network I/O rate information
type NetIOMetrics struct {
	RecvBytesPerSec float64
	SentBytesPerSec float64
	Error           error
}

// SystemCollector collects system metrics
type SystemCollector struct {
	// Previous disk I/O counters for rate calculation
	prevIOCounters map[string]disk.IOCountersStat
	prevIOTime     time.Time
	// Previous network I/O counters for rate calculation
	prevNetCounters []net.IOCountersStat
	prevNetTime     time.Time
}

// NewSystemCollector creates a new SystemCollector instance
func NewSystemCollector() *SystemCollector {
	return &SystemCollector{
		prevIOCounters: make(map[string]disk.IOCountersStat),
		prevIOTime:     time.Now(),
	}
}

// Collect gathers all system metrics
func (sc *SystemCollector) Collect() SystemMetrics {
	now := time.Now()

	metrics := SystemMetrics{
		LastUpdate: now,
	}

	// Collect CPU metrics
	metrics.CPU = sc.collectCPU()

	// Collect load averages
	metrics.Load = sc.collectLoad()

	// Collect memory metrics
	metrics.Memory = sc.collectMemory()

	// Collect swap metrics
	metrics.Swap = sc.collectSwap()

	// Collect disk I/O metrics
	metrics.DiskIO = sc.collectDiskIO()

	// Collect network I/O metrics
	metrics.NetIO = sc.collectNetIO()

	return metrics
}

// collectCPU collects CPU usage metrics
func (sc *SystemCollector) collectCPU() CPUMetrics {
	cpuMetrics := CPUMetrics{}

	// Get per-core CPU percentages (1 second interval)
	perCore, err := cpu.Percent(time.Second, true)
	if err != nil {
		cpuMetrics.Error = fmt.Errorf("failed to collect per-core CPU: %w", err)
		return cpuMetrics
	}
	cpuMetrics.PerCore = perCore

	// Calculate total CPU percentage as average of all cores
	if len(perCore) > 0 {
		var sum float64
		for _, pct := range perCore {
			sum += pct
		}
		cpuMetrics.TotalPercent = sum / float64(len(perCore))
	}

	return cpuMetrics
}

// collectLoad collects system load averages
func (sc *SystemCollector) collectLoad() LoadMetrics {
	loadMetrics := LoadMetrics{}

	loadAvg, err := load.Avg()
	if err != nil {
		loadMetrics.Error = fmt.Errorf("failed to collect load averages: %w", err)
		return loadMetrics
	}

	loadMetrics.Load1 = loadAvg.Load1
	loadMetrics.Load5 = loadAvg.Load5
	loadMetrics.Load15 = loadAvg.Load15

	return loadMetrics
}

// collectMemory collects memory usage metrics
func (sc *SystemCollector) collectMemory() MemoryMetrics {
	memMetrics := MemoryMetrics{}

	vmem, err := mem.VirtualMemory()
	if err != nil {
		memMetrics.Error = fmt.Errorf("failed to collect memory metrics: %w", err)
		return memMetrics
	}

	memMetrics.Used = vmem.Used
	memMetrics.Total = vmem.Total
	memMetrics.Percentage = vmem.UsedPercent

	return memMetrics
}

// collectSwap collects swap usage metrics
func (sc *SystemCollector) collectSwap() SwapMetrics {
	swapMetrics := SwapMetrics{}

	swap, err := mem.SwapMemory()
	if err != nil {
		swapMetrics.Error = fmt.Errorf("failed to collect swap metrics: %w", err)
		return swapMetrics
	}

	swapMetrics.Used = swap.Used
	swapMetrics.Total = swap.Total
	swapMetrics.Percentage = swap.UsedPercent

	return swapMetrics
}

// collectDiskIO collects disk I/O rate metrics
func (sc *SystemCollector) collectDiskIO() DiskIOMetrics {
	ioMetrics := DiskIOMetrics{}

	// Get current I/O counters
	ioCounters, err := disk.IOCounters()
	if err != nil {
		ioMetrics.Error = fmt.Errorf("failed to collect disk I/O: %w", err)
		return ioMetrics
	}

	now := time.Now()
	duration := now.Sub(sc.prevIOTime).Seconds()

	// Calculate rates if we have previous data
	if len(sc.prevIOCounters) > 0 && duration > 0 {
		var totalReadBytes, totalWriteBytes uint64

		for name, current := range ioCounters {
			if prev, exists := sc.prevIOCounters[name]; exists {
				totalReadBytes += current.ReadBytes - prev.ReadBytes
				totalWriteBytes += current.WriteBytes - prev.WriteBytes
			}
		}

		ioMetrics.ReadBytesPerSec = float64(totalReadBytes) / duration
		ioMetrics.WriteBytesPerSec = float64(totalWriteBytes) / duration
	}

	// Store current counters for next collection
	sc.prevIOCounters = ioCounters
	sc.prevIOTime = now

	return ioMetrics
}

// collectNetIO collects network I/O rate metrics
func (sc *SystemCollector) collectNetIO() NetIOMetrics {
	netMetrics := NetIOMetrics{}

	// Get current network I/O counters
	netCounters, err := net.IOCounters(false) // false = aggregate all interfaces
	if err != nil {
		netMetrics.Error = fmt.Errorf("failed to collect network I/O: %w", err)
		return netMetrics
	}

	if len(netCounters) == 0 {
		netMetrics.Error = fmt.Errorf("no network interfaces found")
		return netMetrics
	}

	now := time.Now()
	duration := now.Sub(sc.prevNetTime).Seconds()

	// Calculate rates if we have previous data
	if len(sc.prevNetCounters) > 0 && duration > 0 {
		// Use aggregate stats (first element when pernic=false)
		current := netCounters[0]
		prev := sc.prevNetCounters[0]

		recvBytes := current.BytesRecv - prev.BytesRecv
		sentBytes := current.BytesSent - prev.BytesSent

		netMetrics.RecvBytesPerSec = float64(recvBytes) / duration
		netMetrics.SentBytesPerSec = float64(sentBytes) / duration
	}

	// Store current counters for next collection
	sc.prevNetCounters = netCounters
	sc.prevNetTime = now

	return netMetrics
}

// FormatBytes formats bytes as human-readable string (KB/MB/GB/TB)
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.2f %s", float64(bytes)/float64(div), units[exp])
}

// FormatRate formats bytes per second as human-readable rate (KB/s, MB/s, GB/s)
func FormatRate(bytesPerSec float64) string {
	const unit = 1024.0
	if bytesPerSec < unit {
		return fmt.Sprintf("%.2f B/s", bytesPerSec)
	}

	div, exp := unit, 0
	for n := bytesPerSec / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB/s", "MB/s", "GB/s", "TB/s", "PB/s"}
	return fmt.Sprintf("%.2f %s", bytesPerSec/div, units[exp])
}
