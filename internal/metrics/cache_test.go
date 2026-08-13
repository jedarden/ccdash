package metrics

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestTokenCacheMigratesSourceColumnsAndAggregatesBothHarnesses(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokens.db")
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := []string{
		`CREATE TABLE schema_version (version INTEGER PRIMARY KEY)`,
		`INSERT INTO schema_version VALUES (3)`,
		`CREATE TABLE token_events (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp TEXT NOT NULL, timestamp_unix INTEGER NOT NULL, model TEXT NOT NULL, input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0, cache_read_tokens INTEGER DEFAULT 0, cache_creation_tokens INTEGER DEFAULT 0, source_file TEXT NOT NULL, line_number INTEGER NOT NULL)`,
		`CREATE TABLE file_aggregates (source_file TEXT PRIMARY KEY, is_complete BOOLEAN DEFAULT 0, completed_at INTEGER DEFAULT 0, total_input_tokens INTEGER DEFAULT 0, total_output_tokens INTEGER DEFAULT 0, total_cache_read_tokens INTEGER DEFAULT 0, total_cache_creation_tokens INTEGER DEFAULT 0, event_count INTEGER DEFAULT 0, earliest_timestamp INTEGER DEFAULT 0, latest_timestamp INTEGER DEFAULT 0, model_breakdown TEXT DEFAULT '{}')`,
		`INSERT INTO token_events (timestamp, timestamp_unix, model, input_tokens, output_tokens, source_file, line_number) VALUES ('2026-08-13T20:00:00Z', 1786651200, 'claude-sonnet-4-5-20250929', 10, 2, 'claude.jsonl', 1)`,
	}
	for _, statement := range legacySchema {
		if _, err := legacy.Exec(statement); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	cache := &TokenCache{dbPath: dbPath, cacheDir: filepath.Dir(dbPath)}
	if err := cache.initDB(); err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	for _, table := range []string{"token_events", "file_aggregates"} {
		var sourceColumn string
		if err := cache.db.QueryRow("SELECT name FROM pragma_table_info(?) WHERE name = 'source'", table).Scan(&sourceColumn); err != nil {
			t.Fatalf("%s source column missing after migration: %v", table, err)
		}
	}

	when := time.Unix(1786651260, 0)
	if err := cache.InsertTokenEventBatch([]TokenEvent{{
		Timestamp: when, Model: "gpt-5.6-luna", Source: "codex",
		InputTokens: 8, CacheReadTokens: 4, OutputTokens: 3,
		SourceFile: "codex.jsonl", LineNumber: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileComplete("codex.jsonl"); err != nil {
		t.Fatal(err)
	}

	result, err := cache.QueryTokensHybrid(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if result.InputTokens != 18 || result.OutputTokens != 5 || result.CacheReadTokens != 4 || result.EventCount != 2 {
		t.Fatalf("unexpected cross-source totals: %+v", result)
	}
	if result.ModelMetrics["claude-sonnet-4-5-20250929"].Source != "claude" {
		t.Fatalf("legacy row did not receive Claude source: %+v", result.ModelMetrics)
	}
	if result.ModelMetrics["gpt-5.6-luna"].Source != "codex" {
		t.Fatalf("Codex row lost source: %+v", result.ModelMetrics)
	}
}
