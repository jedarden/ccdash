package metrics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenCodeSource reads usage from OpenCode's SQLite store
// (~/.local/share/opencode/opencode.db) instead of JSONL transcripts. NEEDLE's
// opencode-* workers (e.g. opencode/space-bunny-free) record their usage here.
//
// The store can be tens of GB, so ingestion never scans the message table:
// the small session table says which sessions changed since the last cycle,
// and only those sessions' messages are read through the
// (session_id, time_created, id) index.
//
// Each session is treated like a transcript file keyed "<db>#<session id>",
// and each message is a "line" numbered by its position in the session. An
// assistant message becomes a TokenEvent only once it has completed: OpenCode
// fills token counts in place while a response streams, so ingestion stops at
// the first incomplete assistant message and resumes there next cycle.
type OpenCodeSource struct {
	dbPath string
}

// NewOpenCodeSource returns a source for the default OpenCode store. OPENCODE_DB
// overrides the path.
func NewOpenCodeSource() *OpenCodeSource {
	path := os.Getenv("OPENCODE_DB")
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".local", "share", "opencode", "opencode.db")
		}
	}
	return &OpenCodeSource{dbPath: path}
}

// NewOpenCodeSourceWithDB returns a source for a specific store (for testing).
func NewOpenCodeSourceWithDB(path string) *OpenCodeSource {
	return &OpenCodeSource{dbPath: path}
}

func (s *OpenCodeSource) Name() string { return "opencode" }

// ProjectDirs reports the store's directory only when the store exists, so the
// collector counts OpenCode as configured exactly when there is data to read.
func (s *OpenCodeSource) ProjectDirs(home string) []string {
	if s.dbPath == "" {
		return nil
	}
	if info, err := os.Stat(s.dbPath); err != nil || info.IsDir() {
		return nil
	}
	return []string{filepath.Dir(s.dbPath)}
}

// ParseUsageLine is unused: OpenCode is not a JSONL source. The collector
// calls IngestInto instead (see dbIngestSource).
func (s *OpenCodeSource) ParseUsageLine(raw []byte) (*TokenEvent, bool, error) {
	return nil, false, nil
}

func (s *OpenCodeSource) HookInstaller() HookInstaller { return nil }

// PricingForModel prices "<provider>/<model>" ids. Free Zen models ("-free"
// suffix) cost nothing. Known OpenAI and Claude/GLM families use their list
// price, matching the API-equivalent cost ccdash shows for other sources.
// Anything else is zero rather than borrowing an unrelated default rate.
func (s *OpenCodeSource) PricingForModel(model string) ModelPricing {
	id := model
	if i := strings.LastIndex(model, "/"); i >= 0 {
		id = model[i+1:]
	}
	if strings.HasSuffix(id, "-free") {
		return ModelPricing{}
	}
	if pricing := NewCodexSource().PricingForModel(id); pricing != (ModelPricing{}) {
		return pricing
	}
	for _, family := range []string{"claude", "sonnet", "opus", "haiku", "fable", "glm"} {
		if strings.Contains(id, family) {
			return getPricingForModel(id)
		}
	}
	return ModelPricing{}
}

// dbIngestSource is implemented by sources backed by a database rather than
// JSONL files. The collector hands them the cache instead of walking their
// directories for *.jsonl.
type dbIngestSource interface {
	IngestInto(cache *TokenCache, completeThreshold time.Duration) error
}

type openCodeMessage struct {
	Role       string `json:"role"`
	ModelID    string `json:"modelID"`
	ProviderID string `json:"providerID"`
	Time       struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
	Tokens struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
}

func (s *OpenCodeSource) sessionKey(sessionID string) string {
	return s.dbPath + "#" + sessionID
}

func (s *OpenCodeSource) open() (*sql.DB, error) {
	// Read-only, without immutable=1: OpenCode writes the store concurrently
	// (WAL), and immutable would let us read a torn snapshot.
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", (&url.URL{Path: s.dbPath}).EscapedPath())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// IngestInto reads sessions changed since their recorded state and inserts
// their newly completed assistant messages into cache.
func (s *OpenCodeSource) IngestInto(cache *TokenCache, completeThreshold time.Duration) error {
	if cache == nil || s.dbPath == "" {
		return nil
	}
	if _, err := os.Stat(s.dbPath); err != nil {
		return nil // OpenCode not installed or store absent: nothing to do
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()

	type sessionRow struct {
		id      string
		updated time.Time
	}
	rows, err := db.Query(`SELECT id, time_updated FROM session`)
	if err != nil {
		return err
	}
	var sessions []sessionRow
	for rows.Next() {
		var id string
		var updatedMs int64
		if err := rows.Scan(&id, &updatedMs); err != nil {
			rows.Close()
			return err
		}
		sessions = append(sessions, sessionRow{id: id, updated: time.UnixMilli(updatedMs)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, session := range sessions {
		key := s.sessionKey(session.id)
		if agg, ok := cache.GetFileAggregate(key); ok && agg.IsComplete {
			if !session.updated.After(agg.CompletedAt) {
				continue
			}
			cache.MarkFileActive(key)
		}
		if err := s.ingestSession(db, cache, session.id, session.updated); err != nil {
			continue // one unreadable session must not starve the rest
		}
		if time.Since(session.updated) > completeThreshold {
			cache.MarkFileComplete(key)
		}
	}
	return nil
}

func (s *OpenCodeSource) ingestSession(db *sql.DB, cache *TokenCache, sessionID string, updated time.Time) error {
	key := s.sessionKey(sessionID)
	lastLine, lastMod, exists := cache.GetFileState(key)
	if exists && !updated.After(lastMod) {
		return nil
	}

	// Messages are only appended, but a reverted session loses its tail; a
	// session shorter than what was processed is re-ingested from scratch.
	var count int64
	if err := db.QueryRow(`SELECT count(*) FROM message WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
		return err
	}
	if count < lastLine {
		if err := cache.InvalidateFile(key); err != nil {
			return err
		}
		lastLine = 0
	}

	rows, err := db.Query(`SELECT data FROM message WHERE session_id = ? ORDER BY time_created, id LIMIT -1 OFFSET ?`, sessionID, lastLine)
	if err != nil {
		return err
	}
	defer rows.Close()

	line := lastLine
	var events []TokenEvent
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return err
		}
		var msg openCodeMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			line++ // malformed message: skip it permanently, as a JSONL line would be
			continue
		}
		if msg.Role == "assistant" && msg.Time.Completed == 0 {
			break // still streaming; resume here next cycle
		}
		line++
		if event, ok := openCodeEvent(msg); ok {
			event.Source = s.Name()
			event.SourceFile = key
			event.LineNumber = line
			events = append(events, event)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := cache.InsertTokenEventBatch(events); err != nil {
		return err
	}
	// Record progress with the session's own update time only when every
	// message was consumed; otherwise leave lastMod behind so the next cycle
	// revisits the session even if OpenCode has not touched it since.
	stateMod := updated
	if line < count {
		stateMod = lastMod
	}
	return cache.SetFileState(key, line, stateMod)
}

// openCodeEvent converts one completed assistant message. OpenCode's input
// excludes cache reads/writes, so the counters stay mutually exclusive as for
// the other sources; reasoning is billed as output, so it is added there.
func openCodeEvent(msg openCodeMessage) (TokenEvent, bool) {
	if msg.Role != "assistant" || msg.ModelID == "" {
		return TokenEvent{}, false
	}
	output := msg.Tokens.Output + msg.Tokens.Reasoning
	if msg.Tokens.Input == 0 && output == 0 && msg.Tokens.Cache.Read == 0 && msg.Tokens.Cache.Write == 0 {
		return TokenEvent{}, false
	}
	ms := msg.Time.Completed
	if ms == 0 {
		ms = msg.Time.Created
	}
	model := msg.ModelID
	if msg.ProviderID != "" {
		model = msg.ProviderID + "/" + msg.ModelID
	}
	return TokenEvent{
		Timestamp:           time.UnixMilli(ms),
		Model:               model,
		InputTokens:         msg.Tokens.Input,
		OutputTokens:        output,
		CacheReadTokens:     msg.Tokens.Cache.Read,
		CacheCreationTokens: msg.Tokens.Cache.Write,
	}, true
}
