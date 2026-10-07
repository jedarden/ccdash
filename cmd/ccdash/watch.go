package main

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
)

// runWatch writes one --once-schema snapshot per interval as a JSON line
// until stop fires. Unlike --once it reports the rates that need two samples
// (disk/network I/O, tokens/min), because every line after the baseline has
// an earlier sample to measure against. A write error (the reader went away)
// ends the stream normally.
func runWatch(extraDirs string, since time.Time, interval time.Duration, out io.Writer, stop <-chan os.Signal) int {
	systemCollector, tokenCollector, tmuxCollector := newHeadlessCollectors(extraDirs, since)
	defer tokenCollector.StopBackgroundIngestion()
	budget := onceBudget()

	// Baselines for the I/O counters and pane contents.
	systemCollector.Collect()
	tmuxCollector.Collect()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	encoder := json.NewEncoder(out)
	for {
		select {
		case <-stop:
			return 0
		case <-ticker.C:
		}
		now := time.Now()
		system := systemCollector.Collect()
		tokens, err := tokenCollector.Collect()
		if err != nil {
			tokens = nil
		}
		sessions := tmuxCollector.Collect()
		snapshot := withMeasuredRates(makeSnapshot(now, version, system, tokens, sessions, budget), system, tokens)
		if err := encoder.Encode(snapshot); err != nil {
			return 0
		}
	}
}

// withMeasuredRates fills the rate fields makeSnapshot leaves null for a
// single sample. Only call it when the collectors have an earlier sample.
func withMeasuredRates(s Snapshot, system metrics.SystemMetrics, tokens *metrics.TokenMetrics) Snapshot {
	if system.DiskIO.Error == nil {
		read, write := system.DiskIO.ReadBytesPerSec, system.DiskIO.WriteBytesPerSec
		s.System.DiskIO.ReadBytesPerSec, s.System.DiskIO.WriteBytesPerSec = &read, &write
	}
	if system.NetIO.Error == nil {
		recv, sent := system.NetIO.RecvBytesPerSec, system.NetIO.SentBytesPerSec
		s.System.NetIO.RecvBytesPerSec, s.System.NetIO.SentBytesPerSec = &recv, &sent
		for i := range s.System.NetIO.Interfaces {
			if i >= len(system.NetIO.Interfaces) {
				break
			}
			r, w := system.NetIO.Interfaces[i].RecvBytesPerSec, system.NetIO.Interfaces[i].SentBytesPerSec
			s.System.NetIO.Interfaces[i].RecvBytesPerSec, s.System.NetIO.Interfaces[i].SentBytesPerSec = &r, &w
		}
	}
	if tokens != nil && s.Tokens != nil {
		rate := tokens.Rate
		s.Tokens.Rate = &rate
	}
	return s
}
