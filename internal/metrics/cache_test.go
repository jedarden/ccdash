package metrics

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCollectorLeaseIsExclusiveRenewableAndReleasedByOwner(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokens.db")
	newCache := func() *TokenCache {
		t.Helper()
		cache := &TokenCache{dbPath: dbPath, cacheDir: filepath.Dir(dbPath)}
		if err := cache.initDB(); err != nil {
			t.Fatalf("initialize token cache: %v", err)
		}
		t.Cleanup(func() {
			if err := cache.Close(); err != nil {
				t.Errorf("close token cache: %v", err)
			}
		})
		return cache
	}
	first := newCache()
	second := newCache()

	if !first.TryAcquireLease("instance-a") {
		t.Fatal("first instance should acquire an unheld lease")
	}
	if second.TryAcquireLease("instance-b") {
		t.Fatal("second instance acquired a lease held by the first")
	}
	if !first.TryAcquireLease("instance-a") {
		t.Fatal("lease holder should be able to renew its own lease")
	}

	first.ReleaseLease("instance-b")
	if second.TryAcquireLease("instance-b") {
		t.Fatal("releasing another instance's lease should not transfer ownership")
	}
	first.ReleaseLease("instance-a")
	if !second.TryAcquireLease("instance-b") {
		t.Fatal("second instance should acquire the lease after its owner releases it")
	}
	if first.TryAcquireLease("instance-a") {
		t.Fatal("first instance reacquired a lease held by the second")
	}
}

func TestTokenCacheMigratesLegacyDataAndAggregatesMixedSourcesExactlyOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokens.db")
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Schema version 3 predates source attribution. Include both an active
	// event and a completed file aggregate so migration has to preserve data
	// from each cache path, not just add columns to empty tables.
	legacySchema := []string{
		`CREATE TABLE schema_version (version INTEGER PRIMARY KEY)`,
		`INSERT INTO schema_version VALUES (3)`,
		`CREATE TABLE token_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			timestamp_unix INTEGER NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_read_tokens INTEGER DEFAULT 0,
			cache_creation_tokens INTEGER DEFAULT 0,
			source_file TEXT NOT NULL,
			line_number INTEGER NOT NULL
		)`,
		`CREATE TABLE file_aggregates (
			source_file TEXT PRIMARY KEY,
			is_complete BOOLEAN DEFAULT 0,
			completed_at INTEGER DEFAULT 0,
			total_input_tokens INTEGER DEFAULT 0,
			total_output_tokens INTEGER DEFAULT 0,
			total_cache_read_tokens INTEGER DEFAULT 0,
			total_cache_creation_tokens INTEGER DEFAULT 0,
			event_count INTEGER DEFAULT 0,
			earliest_timestamp INTEGER DEFAULT 0,
			latest_timestamp INTEGER DEFAULT 0,
			model_breakdown TEXT DEFAULT '{}'
		)`,
		`INSERT INTO token_events
			(timestamp, timestamp_unix, model, input_tokens, output_tokens,
			 cache_read_tokens, cache_creation_tokens, source_file, line_number)
			VALUES ('2026-08-13T20:00:00Z', 1786651200, 'claude-sonnet-4-5-20250929', 10, 2, 3, 1, 'claude-active.jsonl', 1)`,
		`INSERT INTO file_aggregates
			(source_file, is_complete, completed_at, total_input_tokens, total_output_tokens,
			 total_cache_read_tokens, total_cache_creation_tokens, event_count,
			 earliest_timestamp, latest_timestamp, model_breakdown)
			VALUES ('claude-complete.jsonl', 1, 1786651300, 100, 20, 30, 4, 2,
			        1786651100, 1786651250,
			        '{"claude-sonnet-4-5-20250929":{"InputTokens":100,"OutputTokens":20,"CacheReadTokens":30,"CacheCreationTokens":4}}')`,
	}
	for _, statement := range legacySchema {
		if _, err := legacy.Exec(statement); err != nil {
			_ = legacy.Close()
			t.Fatalf("create legacy cache fixture: %v", err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	cache := &TokenCache{dbPath: dbPath, cacheDir: filepath.Dir(dbPath)}
	if err := cache.initDB(); err != nil {
		t.Fatalf("migrate legacy cache: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	var version int
	if err := cache.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("read migrated schema version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}

	for _, table := range []string{"token_events", "file_aggregates"} {
		var sourceColumn string
		if err := cache.db.QueryRow("SELECT name FROM pragma_table_info(?) WHERE name = 'source'", table).Scan(&sourceColumn); err != nil {
			t.Fatalf("%s source column missing after migration: %v", table, err)
		}
	}

	legacyAggregate, ok := cache.GetFileAggregate("claude-complete.jsonl")
	if !ok {
		t.Fatal("legacy completed-file aggregate was lost during migration")
	}
	if !legacyAggregate.IsComplete || legacyAggregate.Source != "claude" {
		t.Fatalf("legacy aggregate status/source = complete:%t source:%q, want complete:true source:claude", legacyAggregate.IsComplete, legacyAggregate.Source)
	}
	if got := legacyAggregate.ModelBreakdown["claude-sonnet-4-5-20250929"].Source; got != "claude" {
		t.Fatalf("legacy aggregate model source = %q, want claude", got)
	}

	codexTime := time.Unix(1786651260, 0)
	codexEvent := TokenEvent{
		Timestamp: codexTime, Model: "gpt-5.6-luna", Source: "codex",
		InputTokens: 8, OutputTokens: 5, CacheReadTokens: 7, CacheCreationTokens: 2,
		SourceFile: "codex.jsonl", LineNumber: 1,
	}
	// Simulate a replay of the same source record. The source/file/line key
	// must keep it from inflating either event counts or token totals.
	if err := cache.InsertTokenEventBatch([]TokenEvent{codexEvent, codexEvent}); err != nil {
		t.Fatalf("insert Codex event and replay: %v", err)
	}

	want := AggregatedTokens{
		InputTokens: 118, OutputTokens: 27, CacheReadTokens: 40, CacheCreationTokens: 7,
		EventCount: 4,
	}
	assertMixedSourceTotals(t, cache, want)

	if err := cache.MarkFileComplete("codex.jsonl"); err != nil {
		t.Fatalf("aggregate Codex file: %v", err)
	}
	codexAggregate, ok := cache.GetFileAggregate("codex.jsonl")
	if !ok || !codexAggregate.IsComplete || codexAggregate.Source != "codex" {
		t.Fatalf("Codex aggregate missing or misattributed: aggregate=%+v present=%t", codexAggregate, ok)
	}
	if got := codexAggregate.ModelBreakdown["gpt-5.6-luna"].Source; got != "codex" {
		t.Fatalf("Codex aggregate model source = %q, want codex", got)
	}

	// MarkFileComplete moves rows from token_events into file_aggregates. The
	// hybrid query must count the event once across that transition.
	assertMixedSourceTotals(t, cache, want)
	var remainingCodexEvents int
	if err := cache.db.QueryRow("SELECT COUNT(*) FROM token_events WHERE source_file = 'codex.jsonl'").Scan(&remainingCodexEvents); err != nil {
		t.Fatal(err)
	}
	if remainingCodexEvents != 0 {
		t.Fatalf("completed Codex file retained %d individual events, want 0", remainingCodexEvents)
	}
}

func assertMixedSourceTotals(t *testing.T, cache *TokenCache, want AggregatedTokens) {
	t.Helper()
	got, err := cache.QueryTokensHybrid(time.Time{})
	if err != nil {
		t.Fatalf("query mixed-source totals: %v", err)
	}
	if got.InputTokens != want.InputTokens || got.OutputTokens != want.OutputTokens ||
		got.CacheReadTokens != want.CacheReadTokens || got.CacheCreationTokens != want.CacheCreationTokens ||
		got.EventCount != want.EventCount {
		t.Fatalf("mixed-source totals = input:%d output:%d cache-read:%d cache-create:%d events:%d, want input:%d output:%d cache-read:%d cache-create:%d events:%d",
			got.InputTokens, got.OutputTokens, got.CacheReadTokens, got.CacheCreationTokens, got.EventCount,
			want.InputTokens, want.OutputTokens, want.CacheReadTokens, want.CacheCreationTokens, want.EventCount)
	}

	claude, ok := got.ModelMetrics["claude-sonnet-4-5-20250929"]
	if !ok || claude.Source != "claude" {
		t.Fatalf("Claude model attribution missing: %+v", got.ModelMetrics)
	}
	codex, ok := got.ModelMetrics["gpt-5.6-luna"]
	if !ok || codex.Source != "codex" {
		t.Fatalf("Codex model attribution missing: %+v", got.ModelMetrics)
	}
	if claude.InputTokens != 110 || claude.OutputTokens != 22 || claude.CacheReadTokens != 33 || claude.CacheCreationTokens != 5 {
		t.Errorf("combined Claude model totals = %+v, want input:110 output:22 cache-read:33 cache-create:5", claude)
	}
	if codex.InputTokens != 8 || codex.OutputTokens != 5 || codex.CacheReadTokens != 7 || codex.CacheCreationTokens != 2 {
		t.Errorf("Codex model totals = %+v, want input:8 output:5 cache-read:7 cache-create:2", codex)
	}
}
