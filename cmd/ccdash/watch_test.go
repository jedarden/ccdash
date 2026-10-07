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

func TestWatchLoopStreamsJSONLinesUntilStopped(t *testing.T) {
	out := &lockedBuffer{}
	stop := make(chan os.Signal, 1)
	done := make(chan int, 1)
	var calls int
	collect := func(now time.Time) Snapshot {
		calls++
		read := float64(calls)
		s := Snapshot{SchemaVersion: snapshotSchemaVersion, Timestamp: now}
		s.System.DiskIO.ReadBytesPerSec = &read
		return s
	}
	go func() { done <- watchLoop(20*time.Millisecond, stop, out, collect) }()

	deadline := time.Now().Add(5 * time.Second)
	for bytes.Count([]byte(out.String()), []byte("\n")) < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("watch produced fewer than 3 lines: %q", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop <- os.Interrupt
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not stop after the signal")
	}

	scanner := bufio.NewScanner(bytes.NewReader([]byte(out.String())))
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
		if line.SchemaVersion != snapshotSchemaVersion || line.System.DiskIO.ReadBytesPerSec == nil {
			t.Errorf("unexpected line: %q", scanner.Text())
		}
	}
}

func TestWatchLoopStopsWhenTheReaderGoesAway(t *testing.T) {
	stop := make(chan os.Signal)
	done := make(chan int, 1)
	go func() {
		done <- watchLoop(10*time.Millisecond, stop, failingWriter{}, func(now time.Time) Snapshot {
			return Snapshot{SchemaVersion: snapshotSchemaVersion}
		})
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch kept running after a failed write")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }
