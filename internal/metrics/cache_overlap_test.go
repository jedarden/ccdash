package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func overlapEvent(file string, line int64, at time.Time, input int64) TokenEvent {
	return TokenEvent{Timestamp: at, Model: "claude-sonnet-5", Source: "claude", InputTokens: input, SourceFile: file, LineNumber: line}
}

func hybridInput(t *testing.T, cache *TokenCache) int64 {
	t.Helper()
	agg, err := cache.QueryTokensHybrid(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return agg.InputTokens
}

func TestRecompletingAFileKeepsItsEarlierLines(t *testing.T) {
	cache := newTestTokenCache(t)
	file := "/p/session.jsonl"
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if err := cache.InsertTokenEventBatch([]TokenEvent{overlapEvent(file, 1, base, 100), overlapEvent(file, 2, base, 200)}); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileComplete(file); err != nil {
		t.Fatal(err)
	}

	// The session is resumed: the file grows and is active again.
	if err := cache.MarkFileActive(file); err != nil {
		t.Fatal(err)
	}
	if err := cache.InsertTokenEventBatch([]TokenEvent{overlapEvent(file, 3, base.Add(time.Hour), 400)}); err != nil {
		t.Fatal(err)
	}
	if got := hybridInput(t, cache); got != 700 {
		t.Fatalf("while active, total input = %d, want 700 (earlier summary + new line)", got)
	}

	if err := cache.MarkFileComplete(file); err != nil {
		t.Fatal(err)
	}
	agg, ok := cache.GetFileAggregate(file)
	if !ok || agg.TotalInputTokens != 700 || agg.EventCount != 3 {
		t.Fatalf("re-completed summary = %+v, want input 700 over 3 events", agg)
	}
	if got := hybridInput(t, cache); got != 700 {
		t.Fatalf("after re-completion, total input = %d, want 700", got)
	}
}

func TestSummarisedLinesAreNotStoredAgain(t *testing.T) {
	cache := newTestTokenCache(t)
	file := "/p/session.jsonl"
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	lines := []TokenEvent{overlapEvent(file, 1, base, 100), overlapEvent(file, 2, base, 200)}
	if err := cache.InsertTokenEventBatch(lines); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileComplete(file); err != nil {
		t.Fatal(err)
	}

	// Another process that read the file before it was summarised inserts
	// the same lines late, plus one genuinely new line.
	if err := cache.InsertTokenEventBatch(append(lines, overlapEvent(file, 3, base, 400))); err != nil {
		t.Fatal(err)
	}
	if err := cache.InsertTokenEvent(base, "claude-sonnet-5", 100, 0, 0, 0, file, 1); err != nil {
		t.Fatal(err)
	}
	if got := hybridInput(t, cache); got != 700 {
		t.Fatalf("total input = %d, want 700: summarised lines were stored again", got)
	}
}

func TestInvalidatingARewrittenFileDropsItsSummary(t *testing.T) {
	cache := newTestTokenCache(t)
	file := "/p/session.jsonl"
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if err := cache.InsertTokenEventBatch([]TokenEvent{overlapEvent(file, 1, base, 100)}); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileComplete(file); err != nil {
		t.Fatal(err)
	}
	if err := cache.InvalidateFile(file); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.GetFileAggregate(file); ok {
		t.Fatal("summary survived invalidation; the re-read file would be counted on top of it")
	}
	// Re-reading the rewritten file from line 1 must be accepted.
	if err := cache.InsertTokenEventBatch([]TokenEvent{overlapEvent(file, 1, base, 150)}); err != nil {
		t.Fatal(err)
	}
	if got := hybridInput(t, cache); got != 150 {
		t.Fatalf("total input = %d, want 150 from the rewritten file only", got)
	}
}

func TestSchemaFiveRepairsOverlapFromOlderCaches(t *testing.T) {
	cache := newTestTokenCache(t)
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	existing := filepath.Join(t.TempDir(), "still-here.jsonl")
	if err := os.WriteFile(existing, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dup, gone := "/gone/duplicate.jsonl", "/gone/differs.jsonl"
	for _, f := range []string{dup, existing, gone} {
		if err := cache.InsertTokenEventBatch([]TokenEvent{overlapEvent(f, 1, base, 100), overlapEvent(f, 2, base, 200)}); err != nil {
			t.Fatal(err)
		}
		if err := cache.MarkFileComplete(f); err != nil {
			t.Fatal(err)
		}
	}
	// Recreate the pre-v5 damage directly: exact duplicates of a summary,
	// and differing overlap for a file that exists and one that does not.
	db := cache.GetDB()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec("UPDATE file_aggregates SET covered_lines = 0")
	for _, row := range []struct {
		file  string
		line  int64
		input int64
	}{{dup, 1, 100}, {dup, 2, 200}, {existing, 1, 999}, {gone, 1, 999}} {
		mustExec(`INSERT INTO token_events (timestamp, timestamp_unix, model, source, input_tokens, output_tokens,
			cache_read_tokens, cache_creation_tokens, source_file, line_number) VALUES (?, ?, 'claude-sonnet-5', 'claude', ?, 0, 0, 0, ?, ?)`,
			base.Format(time.RFC3339Nano), base.Unix(), row.input, row.file, row.line)
	}
	mustExec("UPDATE schema_version SET version = 4")

	if err := cache.initDB(); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM token_events WHERE source_file = ?", dup).Scan(&n); err != nil || n != 0 {
		t.Errorf("exact duplicates left: %d (err %v), want 0", n, err)
	}
	if _, ok := cache.GetFileAggregate(existing); ok {
		t.Error("an overlapping file that still exists should be reset for rebuilding")
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM token_events WHERE source_file = ?", gone).Scan(&n); err != nil || n != 1 {
		t.Errorf("unprovable overlap for a deleted file was changed: %d events (err %v), want 1 kept", n, err)
	}
	if _, ok := cache.GetFileAggregate(gone); !ok {
		t.Error("summary for a deleted file with unprovable overlap was removed")
	}
	var version int
	if err := db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil || version != schemaVersion {
		t.Errorf("schema version = %d (err %v), want %d", version, err, schemaVersion)
	}
}
