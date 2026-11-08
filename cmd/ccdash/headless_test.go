package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
)

// captureStdout redirects os.Stdout to a pipe for the duration of run and
// returns everything written to it.
//
// The drain goroutine is started before run: a host under pipe-memory
// pressure (fs.pipe-user-pages-soft exhausted by other processes' fds) gets
// new pipes with buffers as small as 8KiB, and a snapshot larger than that
// buffer blocks inside its single write until something reads. Waiting for
// run to return before reading deadlocked TestRunOnceAppliesSinceWindow on
// exactly such a host while CI stayed green, because the drain must race the
// write, not follow it.
func captureStdout(t *testing.T, run func()) []byte {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer

	type captured struct {
		output  []byte
		readErr error
	}
	// Buffered so the goroutine finishes without a receiver even if run
	// panicked or exited via t.Fatal.
	done := make(chan captured, 1)
	go func() {
		out, err := io.ReadAll(reader)
		_ = reader.Close()
		done <- captured{output: out, readErr: err}
	}()

	defer func() {
		os.Stdout = oldStdout
		// Ends the drain if run did not return normally; closing an
		// already-closed writer is a harmless no-op error.
		_ = writer.Close()
	}()

	run()

	if err := writer.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatalf("close captured stdout pipe: %v", err)
	}
	result := <-done
	if result.readErr != nil {
		t.Fatalf("read captured stdout: %v", result.readErr)
	}
	return result.output
}

func TestParseSince(t *testing.T) {
	location := time.FixedZone("EDT", -4*60*60)
	now := time.Date(2026, time.October, 2, 15, 4, 5, 0, location)
	tests := []struct {
		name  string
		value string
		want  time.Time
	}{
		{name: "monday uses dashboard boundary", value: "monday", want: time.Date(2026, time.September, 28, 9, 0, 0, 0, location)},
		{name: "today starts local day", value: "today", want: time.Date(2026, time.October, 2, 0, 0, 0, 0, location)},
		{name: "24 hours", value: "24h", want: now.Add(-24 * time.Hour)},
		{name: "7 days", value: "7d", want: now.Add(-7 * 24 * time.Hour)},
		{name: "RFC3339", value: "2026-10-01T09:00:00-04:00", want: time.Date(2026, time.October, 1, 9, 0, 0, 0, location)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseSince(test.value, now)
			if err != nil {
				t.Fatalf("parseSince(%q): %v", test.value, err)
			}
			if !got.Equal(test.want) {
				t.Errorf("parseSince(%q) = %s, want %s", test.value, got, test.want)
			}
		})
	}

	for _, value := range []string{"", "yesterday", "2026-10-01"} {
		t.Run("invalid "+value, func(t *testing.T) {
			if _, err := parseSince(value, now); err == nil {
				t.Errorf("parseSince(%q) succeeded, want an error", value)
			}
		})
	}
}

func TestParseSinceMondayBeforeNineUsesPreviousWeek(t *testing.T) {
	location := time.FixedZone("EDT", -4*60*60)
	now := time.Date(2026, time.September, 28, 8, 59, 0, 0, location)
	want := time.Date(2026, time.September, 21, 9, 0, 0, 0, location)
	got, err := parseSince("monday", now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Errorf("parseSince(monday) = %s, want %s", got, want)
	}
}

func TestExportFiltersHonorSince(t *testing.T) {
	since := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	events := []metrics.TokenEventExport{
		{ID: 1, Timestamp: since.Add(-time.Nanosecond)},
		{ID: 2, Timestamp: since},
		{ID: 3, Timestamp: since.Add(time.Second)},
	}
	filteredEvents := tokenEventsSince(events, since)
	if len(filteredEvents) != 2 || filteredEvents[0].ID != 2 || filteredEvents[1].ID != 3 {
		t.Fatalf("tokenEventsSince = %+v, want events 2 and 3", filteredEvents)
	}

	aggregates := []metrics.FileAggregateExport{
		{SourceFile: "old.jsonl", LatestTimestamp: since.Add(-time.Second)},
		{SourceFile: "current.jsonl", LatestTimestamp: since},
		{SourceFile: "recent.jsonl", LatestTimestamp: since.Add(time.Second)},
	}
	filteredAggregates := fileAggregatesSince(aggregates, since)
	if len(filteredAggregates) != 2 || filteredAggregates[0].SourceFile != "current.jsonl" || filteredAggregates[1].SourceFile != "recent.jsonl" {
		t.Fatalf("fileAggregatesSince = %+v, want current.jsonl and recent.jsonl", filteredAggregates)
	}
	if got := len(tokenEventsSince(events, time.Time{})); got != len(events) {
		t.Errorf("zero-boundary event filter returned %d events, want %d", got, len(events))
	}
}

func TestJSONExportAppliesSinceToCachedAggregates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cache := metrics.NewTokenCache()
	since := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := cache.InsertTokenEvent(since.Add(-time.Hour), "old-model", 100, 0, 0, 0, "old.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := cache.InsertTokenEvent(since.Add(time.Minute), "current-model", 7, 0, 0, 0, "current.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileComplete("old.jsonl"); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileComplete("current.jsonl"); err != nil {
		t.Fatal(err)
	}

	var exportErr error
	output := captureStdout(t, func() {
		exportErr = exportJSONSince(cache, since)
	})
	if exportErr != nil {
		t.Fatalf("exportJSONSince: %v", exportErr)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}

	var exported struct {
		Events     []metrics.TokenEventExport    `json:"events"`
		Aggregates []metrics.FileAggregateExport `json:"aggregates"`
	}
	if err := json.Unmarshal(output, &exported); err != nil {
		t.Fatalf("decode export: %v\n%s", err, output)
	}
	if len(exported.Events) != 0 {
		t.Errorf("export contains %d raw events, want none after both files were aggregated", len(exported.Events))
	}
	if len(exported.Aggregates) != 1 || exported.Aggregates[0].SourceFile != "current.jsonl" {
		t.Fatalf("export aggregates = %+v, want only current.jsonl", exported.Aggregates)
	}
}

func TestRunOnceAppliesSinceWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatal(err)
	}

	cache := metrics.NewTokenCache()
	since := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := cache.InsertTokenEvent(since.Add(-time.Second), "old-model", 100, 0, 0, 0, "old.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := cache.InsertTokenEvent(since.Add(time.Second), "current-model", 7, 0, 0, 0, "current.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}

	var code int
	output := captureStdout(t, func() {
		code = runOnceModeWithOptions(true, "", since, false)
	})
	if code != 0 {
		t.Fatalf("runOnceModeWithOptions exit code = %d, output: %s", code, output)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(output, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v\n%s", err, output)
	}
	if snapshot.Tokens == nil || snapshot.Tokens.TotalTokens != 7 || snapshot.Tokens.Prompts != 1 {
		t.Fatalf("snapshot tokens = %+v, want only the recent 7-token event", snapshot.Tokens)
	}
	if !snapshot.Tokens.LookbackFrom.Equal(since) {
		t.Errorf("snapshot lookback_from = %s, want %s", snapshot.Tokens.LookbackFrom, since)
	}
}

func TestAttentionListsAskingSessionsAndSetsExitCode(t *testing.T) {
	sessions := []metrics.TmuxSession{
		{Name: "ready", Status: metrics.StatusReady},
		{Name: "zeta", Status: metrics.StatusAsking},
		{Name: "working", Status: metrics.StatusWorking},
		{Name: "alpha", Status: metrics.StatusAsking},
	}
	asking := askingSessions(sessions)
	if got := attentionExitCode(sessions); got != 1 {
		t.Fatalf("attentionExitCode = %d, want 1 when ASKING sessions exist", got)
	}
	var output bytes.Buffer
	printAttentionSessions(&output, asking)
	if got, want := output.String(), "Sessions asking for human input:\n  - alpha\n  - zeta\n"; got != want {
		t.Fatalf("attention output = %q, want %q", got, want)
	}
	if got := attentionExitCode([]metrics.TmuxSession{{Name: "ready", Status: metrics.StatusReady}}); got != 0 {
		t.Errorf("attentionExitCode = %d with no ASKING sessions, want 0", got)
	}
}

func TestHelpDocumentsHeadlessFlags(t *testing.T) {
	output := string(captureStdout(t, printHelp))
	for _, want := range []string{"--since=<window>", "--attention", "monday, today, 24h, 7d"} {
		if !strings.Contains(output, want) {
			t.Errorf("help output does not include %q", want)
		}
	}
}

func TestAttentionShowsDecisionAndExcludesTaskLifecycle(t *testing.T) {
	sessions := []metrics.TmuxSession{
		{Name: "ci", Status: metrics.StatusWaitingExternal},
		{Name: "done", Status: metrics.StatusComplete},
		{Name: "continue", Status: metrics.StatusResumable},
		{Name: "pause", Status: metrics.StatusPaused},
		{Name: "choice", Status: metrics.StatusAsking, AttentionReason: "Which region?\nRecommended: iad"},
	}
	var output bytes.Buffer
	printAttentionSessions(&output, askingSessions(sessions))
	if got, want := output.String(), "Sessions asking for human input:\n  - choice: Which region? Recommended: iad\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
