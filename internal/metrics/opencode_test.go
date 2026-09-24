package metrics

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openCodeFixture is a minimal OpenCode store: the session and message
// columns OpenCodeSource reads, with the real schema's message index.
type openCodeFixture struct {
	t    *testing.T
	path string
	db   *sql.DB
	seq  int64
}

func newOpenCodeFixture(t *testing.T) *openCodeFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{
		`CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL)`,
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL)`,
		`CREATE INDEX message_session_time_created_id_idx ON message (session_id, time_created, id)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return &openCodeFixture{t: t, path: path, db: db, seq: time.Now().Add(-time.Hour).UnixMilli()}
}

func (f *openCodeFixture) touch(session string) {
	f.t.Helper()
	f.seq += 1000
	if _, err := f.db.Exec(`INSERT INTO session (id, directory, time_created, time_updated) VALUES (?, '/repo', ?, ?)
		ON CONFLICT(id) DO UPDATE SET time_updated = excluded.time_updated`, session, f.seq, f.seq); err != nil {
		f.t.Fatal(err)
	}
}

func (f *openCodeFixture) user(session, id string) {
	f.t.Helper()
	f.seq += 10
	f.insert(session, id, fmt.Sprintf(`{"role":"user","time":{"created":%d}}`, f.seq))
}

// assistant adds a response; completed=false leaves it mid-stream.
func (f *openCodeFixture) assistant(session, id string, completed bool, input, output, reasoning, cacheRead, cacheWrite int64) {
	f.t.Helper()
	f.seq += 10
	done := int64(0)
	if completed {
		done = f.seq + 5
	}
	f.insert(session, id, fmt.Sprintf(`{"role":"assistant","modelID":"space-bunny-free","providerID":"opencode","time":{"created":%d,"completed":%d},"tokens":{"input":%d,"output":%d,"reasoning":%d,"cache":{"read":%d,"write":%d}}}`,
		f.seq, done, input, output, reasoning, cacheRead, cacheWrite))
}

func (f *openCodeFixture) insert(session, id, data string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`,
		id, session, f.seq, f.seq, data); err != nil {
		f.t.Fatal(err)
	}
	f.touch(session)
}

func (f *openCodeFixture) complete(id string) {
	f.t.Helper()
	f.seq += 10
	if _, err := f.db.Exec(`UPDATE message SET data = json_set(data, '$.time.completed', ?) WHERE id = ?`, f.seq, id); err != nil {
		f.t.Fatal(err)
	}
	var session string
	if err := f.db.QueryRow(`SELECT session_id FROM message WHERE id = ?`, id).Scan(&session); err != nil {
		f.t.Fatal(err)
	}
	f.touch(session)
}

func newTestTokenCache(t *testing.T) *TokenCache {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tokens.db")
	cache := &TokenCache{dbPath: dbPath, cacheDir: filepath.Dir(dbPath)}
	if err := cache.initDB(); err != nil {
		t.Fatal(err)
	}
	return cache
}

func ingestOpenCode(t *testing.T, source *OpenCodeSource, cache *TokenCache) *AggregatedTokens {
	t.Helper()
	if err := source.IngestInto(cache, 24*time.Hour); err != nil {
		t.Fatalf("IngestInto: %v", err)
	}
	agg, err := cache.QueryTokensSince(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return agg
}

func TestOpenCodeIngestsCompletedAssistantMessages(t *testing.T) {
	f := newOpenCodeFixture(t)
	f.user("s1", "m1")
	f.assistant("s1", "m2", true, 472, 66, 132, 100355, 0)
	f.user("s1", "m3")
	f.assistant("s1", "m4", true, 100, 10, 0, 50, 7)

	cache := newTestTokenCache(t)
	agg := ingestOpenCode(t, NewOpenCodeSourceWithDB(f.path), cache)

	if agg.EventCount != 2 {
		t.Fatalf("events = %d, want 2", agg.EventCount)
	}
	if agg.InputTokens != 572 || agg.OutputTokens != 66+132+10 || agg.CacheReadTokens != 100405 || agg.CacheCreationTokens != 7 {
		t.Fatalf("unexpected totals: %+v", agg)
	}
	mm, ok := agg.ModelMetrics["opencode/space-bunny-free"]
	if !ok || mm.Source != "opencode" {
		t.Fatalf("expected opencode/space-bunny-free from source opencode, got %+v", agg.ModelMetrics)
	}
}

func TestOpenCodeIngestsOnlyNewMessagesAndSkipsUnchangedSessions(t *testing.T) {
	f := newOpenCodeFixture(t)
	f.user("s1", "m1")
	f.assistant("s1", "m2", true, 10, 1, 0, 0, 0)
	source := NewOpenCodeSourceWithDB(f.path)
	cache := newTestTokenCache(t)

	if agg := ingestOpenCode(t, source, cache); agg.EventCount != 1 {
		t.Fatalf("first pass events = %d, want 1", agg.EventCount)
	}
	if agg := ingestOpenCode(t, source, cache); agg.EventCount != 1 || agg.InputTokens != 10 {
		t.Fatalf("unchanged session was re-ingested: %+v", agg)
	}

	f.user("s1", "m3")
	f.assistant("s1", "m4", true, 20, 2, 0, 0, 0)
	agg := ingestOpenCode(t, source, cache)
	if agg.EventCount != 2 || agg.InputTokens != 30 || agg.OutputTokens != 3 {
		t.Fatalf("growing session: want 2 events / 30 in / 3 out, got %+v", agg)
	}
}

func TestOpenCodeWaitsForStreamingMessageToComplete(t *testing.T) {
	f := newOpenCodeFixture(t)
	f.assistant("s1", "m1", true, 10, 1, 0, 0, 0)
	f.assistant("s1", "m2", false, 0, 0, 0, 0, 0) // counts not final yet
	f.assistant("s1", "m3", true, 30, 3, 0, 0, 0)
	source := NewOpenCodeSourceWithDB(f.path)
	cache := newTestTokenCache(t)

	if agg := ingestOpenCode(t, source, cache); agg.EventCount != 1 || agg.InputTokens != 10 {
		t.Fatalf("must stop at the streaming message: %+v", agg)
	}

	// The final counts land in place, then the message completes.
	if _, err := f.db.Exec(`UPDATE message SET data = json_set(data, '$.tokens.input', 20, '$.tokens.output', 2) WHERE id = 'm2'`); err != nil {
		t.Fatal(err)
	}
	f.complete("m2")
	agg := ingestOpenCode(t, source, cache)
	if agg.EventCount != 3 || agg.InputTokens != 60 || agg.OutputTokens != 6 {
		t.Fatalf("after completion want 3 events / 60 in / 6 out, got %+v", agg)
	}
}

func TestOpenCodeReingestsRevertedSession(t *testing.T) {
	f := newOpenCodeFixture(t)
	f.assistant("s1", "m1", true, 10, 1, 0, 0, 0)
	f.assistant("s1", "m2", true, 20, 2, 0, 0, 0)
	source := NewOpenCodeSourceWithDB(f.path)
	cache := newTestTokenCache(t)
	if agg := ingestOpenCode(t, source, cache); agg.EventCount != 2 {
		t.Fatalf("events = %d, want 2", agg.EventCount)
	}

	if _, err := f.db.Exec(`DELETE FROM message WHERE id = 'm2'`); err != nil {
		t.Fatal(err)
	}
	f.touch("s1")
	if agg := ingestOpenCode(t, source, cache); agg.EventCount != 1 || agg.InputTokens != 10 {
		t.Fatalf("reverted session: want 1 event / 10 in, got %+v", agg)
	}
}

func TestOpenCodeMissingStoreIsSilentlyAbsent(t *testing.T) {
	source := NewOpenCodeSourceWithDB(filepath.Join(t.TempDir(), "absent.db"))
	if dirs := source.ProjectDirs("/home/x"); len(dirs) != 0 {
		t.Fatalf("absent store must not register a directory, got %v", dirs)
	}
	if err := source.IngestInto(newTestTokenCache(t), time.Hour); err != nil {
		t.Fatalf("absent store must not error: %v", err)
	}
}

func TestOpenCodePricing(t *testing.T) {
	source := NewOpenCodeSourceWithDB("")
	if p := source.PricingForModel("opencode/space-bunny-free"); p != (ModelPricing{}) {
		t.Fatalf("free Zen model must be zero-priced, got %+v", p)
	}
	if p := source.PricingForModel("ardenone-openai/gpt-5.3-codex"); p != codexPricing["gpt-5.3-codex"] {
		t.Fatalf("known OpenAI model must use its list price, got %+v", p)
	}
	if p := source.PricingForModel("anthropic/claude-haiku-4-5-20251001"); p == (ModelPricing{}) {
		t.Fatal("known Claude model must be priced")
	}
	if p := source.PricingForModel("opencode/big-pickle"); p != (ModelPricing{}) {
		t.Fatalf("unknown model must be zero rather than a borrowed default, got %+v", p)
	}
}
