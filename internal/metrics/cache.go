package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	// Database configuration for concurrent access
	maxOpenConns    = 1  // SQLite only supports one writer at a time
	maxIdleConns    = 1  // Keep one connection ready
	connMaxLifetime = 0  // Don't expire connections
	connMaxIdleTime = 0  // Don't expire idle connections

	// Retry configuration for database locks
	// Keep retries minimal to avoid blocking the UI
	maxRetries     = 3
	baseRetryDelay = 50 * time.Millisecond
	maxRetryDelay  = 200 * time.Millisecond

	// Operation timeout - keep short to avoid UI hangs
	dbOperationTimeout = 5 * time.Second
)

// TokenCache manages persistent SQLite-based caching of token metrics
// The database is queryable by external tools like DuckDB for advanced analytics
type TokenCache struct {
	dbPath    string
	db        *sql.DB
	mu        sync.RWMutex
	cacheDir  string
}

const (
	cacheDirName  = ".ccdash"
	cacheDBName   = "tokens.db"
	schemaVersion = 1
)

// withRetry executes a database operation with exponential backoff retry on lock errors
func withRetry[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	var result T
	var lastErr error
	delay := baseRetryDelay

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		result, lastErr = operation()
		if lastErr == nil {
			return result, nil
		}

		// Check if it's a database lock error
		errStr := lastErr.Error()
		if !isLockError(errStr) {
			return result, lastErr
		}

		// Wait before retry with exponential backoff
		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
		}
	}

	return result, fmt.Errorf("database operation failed after %d retries: %w", maxRetries, lastErr)
}

// withRetryNoResult executes a database operation that returns only an error
func withRetryNoResult(ctx context.Context, operation func() error) error {
	_, err := withRetry(ctx, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

// isLockError checks if the error is a database lock error
func isLockError(errStr string) bool {
	lockPhrases := []string{
		"database is locked",
		"SQLITE_BUSY",
		"SQLITE_LOCKED",
		"database table is locked",
	}
	for _, phrase := range lockPhrases {
		if contains(errStr, phrase) {
			return true
		}
	}
	return false
}

// contains checks if s contains substr (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// NewTokenCache creates a new SQLite-based token cache in the .ccdash directory
func NewTokenCache() *TokenCache {
	// Get directory where binary is invoked (current working directory)
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	cacheDir := filepath.Join(cwd, cacheDirName)
	dbPath := filepath.Join(cacheDir, cacheDBName)

	tc := &TokenCache{
		cacheDir: cacheDir,
		dbPath:   dbPath,
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return tc
	}

	// Initialize database
	if err := tc.initDB(); err != nil {
		return tc
	}

	return tc
}

// initDB initializes the SQLite database with the required schema
func (tc *TokenCache) initDB() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Enhanced connection string for multi-instance/multi-process support:
	// _journal_mode=WAL: Write-Ahead Logging for better concurrent read/write support
	// _synchronous=NORMAL: Balance between safety and performance
	// _busy_timeout=30000: Wait up to 30 seconds for locks (increased from 5s)
	// _txlock=immediate: Acquire write lock at transaction start to avoid deadlocks
	// _cache_size=-64000: Use 64MB of cache (negative = KB)
	// _mmap_size=268435456: Memory-map up to 256MB for faster reads
	connStr := tc.dbPath + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=30000&_txlock=immediate&_cache_size=-64000&_mmap_size=268435456"
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return err
	}

	// Configure connection pool for concurrent access
	// SQLite with WAL mode supports concurrent readers but only one writer
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	tc.db = db

	// Apply additional PRAGMA settings for better concurrent performance
	pragmas := []string{
		"PRAGMA temp_store=MEMORY",           // Store temp tables in memory
		"PRAGMA wal_autocheckpoint=1000",     // Checkpoint every 1000 pages
		"PRAGMA journal_size_limit=67108864", // Limit WAL to 64MB
	}
	for _, pragma := range pragmas {
		if _, err := tc.db.Exec(pragma); err != nil {
			// Non-fatal: pragmas may not be supported on all SQLite versions
		}
	}

	// Create schema
	schema := `
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	);

	CREATE TABLE IF NOT EXISTS token_events (
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
	);

	CREATE INDEX IF NOT EXISTS idx_timestamp_unix ON token_events(timestamp_unix);
	CREATE INDEX IF NOT EXISTS idx_model ON token_events(model);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_source_line ON token_events(source_file, line_number);

	CREATE TABLE IF NOT EXISTS file_state (
		source_file TEXT PRIMARY KEY,
		last_line INTEGER DEFAULT 0,
		last_modified INTEGER DEFAULT 0
	);
	`

	_, err = tc.db.Exec(schema)
	if err != nil {
		return err
	}

	// Check/set schema version
	var version int
	err = tc.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		_, err = tc.db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion)
	}

	return err
}

// Close closes the database connection
func (tc *TokenCache) Close() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db != nil {
		return tc.db.Close()
	}
	return nil
}

// GetDB returns the underlying database for direct queries (e.g., from DuckDB)
func (tc *TokenCache) GetDB() *sql.DB {
	return tc.db
}

// GetDBPath returns the path to the SQLite database file
func (tc *TokenCache) GetDBPath() string {
	return tc.dbPath
}

// InsertTokenEvent inserts a single token event into the database
func (tc *TokenCache) InsertTokenEvent(timestamp time.Time, model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64, sourceFile string, lineNumber int64) error {
	return tc.InsertTokenEventContext(context.Background(), timestamp, model, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens, sourceFile, lineNumber)
}

// InsertTokenEventContext inserts a single token event with context support
func (tc *TokenCache) InsertTokenEventContext(ctx context.Context, timestamp time.Time, model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64, sourceFile string, lineNumber int64) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetryNoResult(ctx, func() error {
		_, err := tc.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO token_events
			(timestamp, timestamp_unix, model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, source_file, line_number)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, timestamp.Format(time.RFC3339Nano), timestamp.Unix(), model, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens, sourceFile, lineNumber)
		return err
	})
}

// InsertTokenEventBatch inserts multiple token events in a single transaction
func (tc *TokenCache) InsertTokenEventBatch(events []TokenEvent) error {
	return tc.InsertTokenEventBatchContext(context.Background(), events)
}

// InsertTokenEventBatchContext inserts multiple token events with context support
func (tc *TokenCache) InsertTokenEventBatchContext(ctx context.Context, events []TokenEvent) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil || len(events) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetryNoResult(ctx, func() error {
		tx, err := tc.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		stmt, err := tx.PrepareContext(ctx, `
			INSERT OR IGNORE INTO token_events
			(timestamp, timestamp_unix, model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, source_file, line_number)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, e := range events {
			_, err = stmt.ExecContext(ctx, e.Timestamp.Format(time.RFC3339Nano), e.Timestamp.Unix(), e.Model, e.InputTokens, e.OutputTokens, e.CacheReadTokens, e.CacheCreationTokens, e.SourceFile, e.LineNumber)
			if err != nil {
				return err
			}
		}

		return tx.Commit()
	})
}

// TokenEvent represents a single token usage event for batch insertion
type TokenEvent struct {
	Timestamp           time.Time
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	SourceFile          string
	LineNumber          int64
}

// QueryTokensSince returns aggregated token metrics since a given timestamp
func (tc *TokenCache) QueryTokensSince(since time.Time) (*AggregatedTokens, error) {
	return tc.QueryTokensSinceContext(context.Background(), since)
}

// QueryTokensSinceContext returns aggregated token metrics with context support
func (tc *TokenCache) QueryTokensSinceContext(ctx context.Context, since time.Time) (*AggregatedTokens, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.db == nil {
		return &AggregatedTokens{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetry(ctx, func() (*AggregatedTokens, error) {
		result := &AggregatedTokens{
			ModelTokens: make(map[string]int64),
		}

		var sinceUnix int64
		if !since.IsZero() {
			sinceUnix = since.Unix()
		}

		// Aggregate totals
		query := `
			SELECT
				COALESCE(SUM(input_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cache_read_tokens), 0),
				COALESCE(SUM(cache_creation_tokens), 0),
				MIN(timestamp_unix),
				MAX(timestamp_unix),
				COUNT(*)
			FROM token_events
			WHERE timestamp_unix >= ?
		`

		var minTS, maxTS sql.NullInt64
		err := tc.db.QueryRowContext(ctx, query, sinceUnix).Scan(
			&result.InputTokens,
			&result.OutputTokens,
			&result.CacheReadTokens,
			&result.CacheCreationTokens,
			&minTS,
			&maxTS,
			&result.EventCount,
		)
		if err != nil {
			return nil, err
		}

		if minTS.Valid {
			result.EarliestTimestamp = time.Unix(minTS.Int64, 0)
		}
		if maxTS.Valid {
			result.LatestTimestamp = time.Unix(maxTS.Int64, 0)
		}

		// Aggregate by model
		modelQuery := `
			SELECT
				model,
				SUM(input_tokens) as input,
				SUM(output_tokens) as output,
				SUM(cache_read_tokens) as cache_read,
				SUM(cache_creation_tokens) as cache_create
			FROM token_events
			WHERE timestamp_unix >= ?
			GROUP BY model
		`

		rows, err := tc.db.QueryContext(ctx, modelQuery, sinceUnix)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		result.ModelMetrics = make(map[string]*ModelAggregation)
		for rows.Next() {
			var model string
			var input, output, cacheRead, cacheCreate int64
			if err := rows.Scan(&model, &input, &output, &cacheRead, &cacheCreate); err != nil {
				continue
			}
			result.ModelTokens[model] = input + output + cacheRead + cacheCreate
			result.ModelMetrics[model] = &ModelAggregation{
				InputTokens:         input,
				OutputTokens:        output,
				CacheReadTokens:     cacheRead,
				CacheCreationTokens: cacheCreate,
			}
		}

		return result, nil
	})
}

// AggregatedTokens contains the result of a token query
type AggregatedTokens struct {
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	EarliestTimestamp   time.Time
	LatestTimestamp     time.Time
	EventCount          int64
	ModelTokens         map[string]int64
	ModelMetrics        map[string]*ModelAggregation
}

// ModelAggregation contains per-model token breakdown
type ModelAggregation struct {
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

// QueryRecentEvents returns token events from the last N seconds for rate calculation
func (tc *TokenCache) QueryRecentEvents(seconds int64) ([]TimestampedTokens, error) {
	return tc.QueryRecentEventsContext(context.Background(), seconds)
}

// QueryRecentEventsContext returns token events with context support
func (tc *TokenCache) QueryRecentEventsContext(ctx context.Context, seconds int64) ([]TimestampedTokens, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.db == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetry(ctx, func() ([]TimestampedTokens, error) {
		cutoff := time.Now().Unix() - seconds

		query := `
			SELECT timestamp_unix, input_tokens + output_tokens + cache_read_tokens + cache_creation_tokens
			FROM token_events
			WHERE timestamp_unix >= ?
			ORDER BY timestamp_unix ASC
		`

		rows, err := tc.db.QueryContext(ctx, query, cutoff)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var events []TimestampedTokens
		for rows.Next() {
			var ts int64
			var tokens int64
			if err := rows.Scan(&ts, &tokens); err != nil {
				continue
			}
			events = append(events, TimestampedTokens{
				Timestamp: time.Unix(ts, 0),
				Tokens:    tokens,
			})
		}

		return events, nil
	})
}

// TimestampedTokens represents tokens at a specific timestamp
type TimestampedTokens struct {
	Timestamp time.Time
	Tokens    int64
}

// GetFileState returns the last processed line and modification time for a file
func (tc *TokenCache) GetFileState(sourceFile string) (lastLine int64, lastModified time.Time, exists bool) {
	return tc.GetFileStateContext(context.Background(), sourceFile)
}

// GetFileStateContext returns file state with context support
func (tc *TokenCache) GetFileStateContext(ctx context.Context, sourceFile string) (lastLine int64, lastModified time.Time, exists bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.db == nil {
		return 0, time.Time{}, false
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	type fileState struct {
		lastLine int64
		lastMod  int64
	}

	result, err := withRetry(ctx, func() (*fileState, error) {
		var ll, lm int64
		err := tc.db.QueryRowContext(ctx, "SELECT last_line, last_modified FROM file_state WHERE source_file = ?", sourceFile).Scan(&ll, &lm)
		if err != nil {
			return nil, err
		}
		return &fileState{lastLine: ll, lastMod: lm}, nil
	})

	if err != nil {
		return 0, time.Time{}, false
	}

	return result.lastLine, time.Unix(result.lastMod, 0), true
}

// SetFileState updates the last processed line and modification time for a file
func (tc *TokenCache) SetFileState(sourceFile string, lastLine int64, lastModified time.Time) error {
	return tc.SetFileStateContext(context.Background(), sourceFile, lastLine, lastModified)
}

// SetFileStateContext updates file state with context support
func (tc *TokenCache) SetFileStateContext(ctx context.Context, sourceFile string, lastLine int64, lastModified time.Time) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetryNoResult(ctx, func() error {
		_, err := tc.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO file_state (source_file, last_line, last_modified)
			VALUES (?, ?, ?)
		`, sourceFile, lastLine, lastModified.Unix())
		return err
	})
}

// IsFileStale checks if a file has been modified since last processing
func (tc *TokenCache) IsFileStale(sourceFile string, currentModTime time.Time) bool {
	_, lastMod, exists := tc.GetFileState(sourceFile)
	if !exists {
		return true
	}
	return currentModTime.After(lastMod)
}

// InvalidateFile removes all cached data for a file (used when file is modified)
func (tc *TokenCache) InvalidateFile(sourceFile string) error {
	return tc.InvalidateFileContext(context.Background(), sourceFile)
}

// InvalidateFileContext removes file data with context support
func (tc *TokenCache) InvalidateFileContext(ctx context.Context, sourceFile string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetryNoResult(ctx, func() error {
		tx, err := tc.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		_, err = tx.ExecContext(ctx, "DELETE FROM token_events WHERE source_file = ?", sourceFile)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "DELETE FROM file_state WHERE source_file = ?", sourceFile)
		if err != nil {
			return err
		}

		return tx.Commit()
	})
}

// Clear removes all cached data
func (tc *TokenCache) Clear() error {
	return tc.ClearContext(context.Background())
}

// ClearContext removes all data with context support
func (tc *TokenCache) ClearContext(ctx context.Context) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetryNoResult(ctx, func() error {
		tx, err := tc.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		_, err = tx.ExecContext(ctx, "DELETE FROM token_events")
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "DELETE FROM file_state")
		if err != nil {
			return err
		}

		return tx.Commit()
	})
}

// GetStats returns cache statistics
func (tc *TokenCache) GetStats() (eventCount int64, fileCount int64, dbSizeBytes int64) {
	return tc.GetStatsContext(context.Background())
}

// GetStatsContext returns stats with context support
func (tc *TokenCache) GetStatsContext(ctx context.Context) (eventCount int64, fileCount int64, dbSizeBytes int64) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.db == nil {
		return 0, 0, 0
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	// Use retry for read operations
	withRetry(ctx, func() (struct{}, error) {
		tc.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM token_events").Scan(&eventCount)
		tc.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM file_state").Scan(&fileCount)
		return struct{}{}, nil
	})

	if info, err := os.Stat(tc.dbPath); err == nil {
		dbSizeBytes = info.Size()
	}

	return
}
