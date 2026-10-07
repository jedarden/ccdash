package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestWithMeasuredRatesFillsOneShotNulls(t *testing.T) {
	system := metrics.SystemMetrics{
		DiskIO: metrics.DiskIOMetrics{ReadBytesPerSec: 10, WriteBytesPerSec: 20},
		NetIO: metrics.NetIOMetrics{RecvBytesPerSec: 30, SentBytesPerSec: 40,
			Interfaces: []metrics.NetInterface{{Name: "eth0", RecvBytesPerSec: 3, SentBytesPerSec: 4}}},
	}
	tokens := &metrics.TokenMetrics{Available: true, Rate: 1234}

	oneShot := makeSnapshot(time.Now(), "test", system, tokens, nil, 0)
	if oneShot.System.DiskIO.ReadBytesPerSec != nil || oneShot.Tokens.Rate != nil {
		t.Fatal("a one-shot snapshot must leave rates null")
	}

	s := withMeasuredRates(oneShot, system, tokens)
	if s.System.DiskIO.ReadBytesPerSec == nil || *s.System.DiskIO.ReadBytesPerSec != 10 ||
		*s.System.DiskIO.WriteBytesPerSec != 20 || *s.System.NetIO.RecvBytesPerSec != 30 ||
		*s.System.NetIO.Interfaces[0].SentBytesPerSec != 4 || *s.Tokens.Rate != 1234 {
		t.Fatalf("rates not filled: %+v %+v", s.System, s.Tokens)
	}
}

func TestRunWatchStreamsJSONLinesUntilStopped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out := &lockedBuffer{}
	stop := make(chan os.Signal, 1)
	done := make(chan int, 1)
	go func() { done <- runWatch("", time.Time{}, 100*time.Millisecond, out, stop) }()

	deadline := time.Now().Add(15 * time.Second)
	for bytes.Count([]byte(out.String()), []byte("\n")) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("watch produced fewer than 2 lines in 15s: %q", out.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop <- os.Interrupt
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch did not stop after the signal")
	}

	scanner := bufio.NewScanner(bytes.NewReader([]byte(out.String())))
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		var line struct {
			SchemaVersion int `json:"schema_version"`
			System        struct {
				DiskIO struct {
					ReadBytesPerSec *float64 `json:"read_bytes_per_sec"`
				} `json:"disk_io"`
			} `json:"system"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("line is not JSON: %v: %q", err, scanner.Text())
		}
		if line.SchemaVersion != snapshotSchemaVersion {
			t.Errorf("schema_version = %d", line.SchemaVersion)
		}
		if line.System.DiskIO.ReadBytesPerSec == nil {
			t.Error("watch lines should carry measured disk I/O rates, got null")
		}
	}
}
