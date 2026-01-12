package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
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
	schemaVersion = 3

	// Threshold for marking a file as complete (no longer being written to)
	fileCompleteThreshold = 30 * time.Minute

	// Metrics cache TTL - how long cached metrics are valid
	metricsCacheTTL = 2 * time.Second

	// Lease duration - how long a collector holds the lease
	collectorLeaseDuration = 5 * time.Second
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

	// Explicitly set WAL mode - the connection string parameter doesn't always work
	// WAL mode is critical for concurrent read/write access across multiple instances
	if _, err := tc.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		// Log but don't fail - will fall back to rollback journal
	}

	// Apply additional PRAGMA settings for better concurrent performance
	pragmas := []string{
		"PRAGMA temp_store=MEMORY",           // Store temp tables in memory
		"PRAGMA wal_autocheckpoint=1000",     // Checkpoint every 1000 pages
		"PRAGMA journal_size_limit=67108864", // Limit WAL to 64MB
		"PRAGMA busy_timeout=30000",          // Ensure busy timeout is set (backup for conn string)
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

	-- Metrics cache for leader election pattern
	-- Only one instance collects, others read from cache
	CREATE TABLE IF NOT EXISTS metrics_cache (
		metric_type TEXT PRIMARY KEY,
		data BLOB NOT NULL,
		updated_at INTEGER NOT NULL
	);

	-- Collector lease for leader election
	-- Instance with valid lease is the collector
	CREATE TABLE IF NOT EXISTS collector_lease (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		instance_id TEXT NOT NULL,
		expires_at INTEGER NOT NULL
	);

	-- Pre-aggregated totals for complete files (not being written to anymore)
	-- Allows skipping file I/O and individual event queries for old sessions
	CREATE TABLE IF NOT EXISTS file_aggregates (
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
	);

	CREATE INDEX IF NOT EXISTS idx_file_aggregates_complete ON file_aggregates(is_complete);
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

// FileAggregate contains pre-computed totals for a complete file
type FileAggregate struct {
	SourceFile          string
	IsComplete          bool
	CompletedAt         time.Time
	TotalInputTokens    int64
	TotalOutputTokens   int64
	TotalCacheRead      int64
	TotalCacheCreation  int64
	EventCount          int64
	EarliestTimestamp   time.Time
	LatestTimestamp     time.Time
	ModelBreakdown      map[string]*ModelAggregation
}

// GetFileAggregate returns the pre-computed aggregate for a file if it exists
func (tc *TokenCache) GetFileAggregate(sourceFile string) (*FileAggregate, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.db == nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
	defer cancel()

	result, err := withRetry(ctx, func() (*FileAggregate, error) {
		var agg FileAggregate
		var completedAt, earliest, latest int64
		var modelJSON string
		var isComplete int

		err := tc.db.QueryRowContext(ctx, `
			SELECT source_file, is_complete, completed_at, total_input_tokens, total_output_tokens,
			       total_cache_read_tokens, total_cache_creation_tokens, event_count,
			       earliest_timestamp, latest_timestamp, model_breakdown
			FROM file_aggregates WHERE source_file = ?
		`, sourceFile).Scan(
			&agg.SourceFile, &isComplete, &completedAt,
			&agg.TotalInputTokens, &agg.TotalOutputTokens,
			&agg.TotalCacheRead, &agg.TotalCacheCreation, &agg.EventCount,
			&earliest, &latest, &modelJSON,
		)
		if err != nil {
			return nil, err
		}

		agg.IsComplete = isComplete == 1
		agg.CompletedAt = time.Unix(completedAt, 0)
		agg.EarliestTimestamp = time.Unix(earliest, 0)
		agg.LatestTimestamp = time.Unix(latest, 0)

		// Parse model breakdown JSON
		agg.ModelBreakdown = make(map[string]*ModelAggregation)
		if modelJSON != "" && modelJSON != "{}" {
			json.Unmarshal([]byte(modelJSON), &agg.ModelBreakdown)
		}

		return &agg, nil
	})

	if err != nil {
		return nil, false
	}
	return result, true
}

// IsFileComplete checks if a file is marked as complete
func (tc *TokenCache) IsFileComplete(sourceFile string) bool {
	agg, ok := tc.GetFileAggregate(sourceFile)
	return ok && agg.IsComplete
}

// MarkFileComplete aggregates all events for a file and marks it as complete
// This allows future queries to skip individual event processing for this file
func (tc *TokenCache) MarkFileComplete(sourceFile string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
	defer cancel()

	return withRetryNoResult(ctx, func() error {
		// Aggregate all events for this file
		var totalInput, totalOutput, totalCacheRead, totalCacheCreate int64
		var eventCount int64
		var minTS, maxTS sql.NullInt64

		err := tc.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			       COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0),
			       COUNT(*), MIN(timestamp_unix), MAX(timestamp_unix)
			FROM token_events WHERE source_file = ?
		`, sourceFile).Scan(&totalInput, &totalOutput, &totalCacheRead, &totalCacheCreate,
			&eventCount, &minTS, &maxTS)
		if err != nil {
			return err
		}

		// Get per-model breakdown
		modelBreakdown := make(map[string]*ModelAggregation)
		rows, err := tc.db.QueryContext(ctx, `
			SELECT model, SUM(input_tokens), SUM(output_tokens),
			       SUM(cache_read_tokens), SUM(cache_creation_tokens)
			FROM token_events WHERE source_file = ?
			GROUP BY model
		`, sourceFile)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var model string
			var input, output, cacheRead, cacheCreate int64
			if err := rows.Scan(&model, &input, &output, &cacheRead, &cacheCreate); err != nil {
				continue
			}
			modelBreakdown[model] = &ModelAggregation{
				InputTokens:         input,
				OutputTokens:        output,
				CacheReadTokens:     cacheRead,
				CacheCreationTokens: cacheCreate,
			}
		}

		modelJSON, _ := json.Marshal(modelBreakdown)

		var earliest, latest int64
		if minTS.Valid {
			earliest = minTS.Int64
		}
		if maxTS.Valid {
			latest = maxTS.Int64
		}

		// Insert or update the aggregate
		_, err = tc.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO file_aggregates
			(source_file, is_complete, completed_at, total_input_tokens, total_output_tokens,
			 total_cache_read_tokens, total_cache_creation_tokens, event_count,
			 earliest_timestamp, latest_timestamp, model_breakdown)
			VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, sourceFile, time.Now().Unix(), totalInput, totalOutput, totalCacheRead, totalCacheCreate,
			eventCount, earliest, latest, string(modelJSON))
		if err != nil {
			return err
		}

		// Delete individual events for this file to save space
		_, err = tc.db.ExecContext(ctx, `DELETE FROM token_events WHERE source_file = ?`, sourceFile)
		return err
	})
}

// MarkFileActive marks a file as no longer complete (it's being written to again)
func (tc *TokenCache) MarkFileActive(sourceFile string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
	defer cancel()

	return withRetryNoResult(ctx, func() error {
		_, err := tc.db.ExecContext(ctx, `
			UPDATE file_aggregates SET is_complete = 0 WHERE source_file = ?
		`, sourceFile)
		return err
	})
}

// GetFileCompleteThreshold returns the threshold duration for marking files as complete
func GetFileCompleteThreshold() time.Duration {
	return fileCompleteThreshold
}

// QueryTokensHybrid returns aggregated token metrics using both pre-aggregated
// complete files and individual events for active files
func (tc *TokenCache) QueryTokensHybrid(since time.Time) (*AggregatedTokens, error) {
	return tc.QueryTokensHybridContext(context.Background(), since)
}

// QueryTokensHybridContext returns aggregated token metrics with context support
func (tc *TokenCache) QueryTokensHybridContext(ctx context.Context, since time.Time) (*AggregatedTokens, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.db == nil {
		return &AggregatedTokens{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
	defer cancel()

	return withRetry(ctx, func() (*AggregatedTokens, error) {
		result := &AggregatedTokens{
			ModelTokens:  make(map[string]int64),
			ModelMetrics: make(map[string]*ModelAggregation),
		}

		var sinceUnix int64
		if !since.IsZero() {
			sinceUnix = since.Unix()
		}

		// Query 1: Sum from complete file aggregates (fast path)
		aggQuery := `
			SELECT COALESCE(SUM(total_input_tokens), 0), COALESCE(SUM(total_output_tokens), 0),
			       COALESCE(SUM(total_cache_read_tokens), 0), COALESCE(SUM(total_cache_creation_tokens), 0),
			       COALESCE(SUM(event_count), 0), MIN(earliest_timestamp), MAX(latest_timestamp)
			FROM file_aggregates
			WHERE is_complete = 1 AND latest_timestamp >= ?
		`

		var aggInput, aggOutput, aggCacheRead, aggCacheCreate, aggCount int64
		var aggMinTS, aggMaxTS sql.NullInt64

		err := tc.db.QueryRowContext(ctx, aggQuery, sinceUnix).Scan(
			&aggInput, &aggOutput, &aggCacheRead, &aggCacheCreate,
			&aggCount, &aggMinTS, &aggMaxTS,
		)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}

		// Get model breakdown from complete files
		aggModelQuery := `
			SELECT model_breakdown FROM file_aggregates
			WHERE is_complete = 1 AND latest_timestamp >= ?
		`
		aggModelRows, err := tc.db.QueryContext(ctx, aggModelQuery, sinceUnix)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if aggModelRows != nil {
			defer aggModelRows.Close()
			for aggModelRows.Next() {
				var modelJSON string
				if err := aggModelRows.Scan(&modelJSON); err != nil {
					continue
				}
				var breakdown map[string]*ModelAggregation
				if json.Unmarshal([]byte(modelJSON), &breakdown) == nil {
					for model, ma := range breakdown {
						if existing, ok := result.ModelMetrics[model]; ok {
							existing.InputTokens += ma.InputTokens
							existing.OutputTokens += ma.OutputTokens
							existing.CacheReadTokens += ma.CacheReadTokens
							existing.CacheCreationTokens += ma.CacheCreationTokens
						} else {
							result.ModelMetrics[model] = &ModelAggregation{
								InputTokens:         ma.InputTokens,
								OutputTokens:        ma.OutputTokens,
								CacheReadTokens:     ma.CacheReadTokens,
								CacheCreationTokens: ma.CacheCreationTokens,
							}
						}
						result.ModelTokens[model] += ma.InputTokens + ma.OutputTokens +
							ma.CacheReadTokens + ma.CacheCreationTokens
					}
				}
			}
		}

		// Query 2: Sum from individual events (for active/incomplete files)
		eventQuery := `
			SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			       COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0),
			       MIN(timestamp_unix), MAX(timestamp_unix), COUNT(*)
			FROM token_events
			WHERE timestamp_unix >= ?
		`

		var evtInput, evtOutput, evtCacheRead, evtCacheCreate, evtCount int64
		var evtMinTS, evtMaxTS sql.NullInt64

		err = tc.db.QueryRowContext(ctx, eventQuery, sinceUnix).Scan(
			&evtInput, &evtOutput, &evtCacheRead, &evtCacheCreate,
			&evtMinTS, &evtMaxTS, &evtCount,
		)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}

		// Get model breakdown from active events
		evtModelQuery := `
			SELECT model, SUM(input_tokens), SUM(output_tokens),
			       SUM(cache_read_tokens), SUM(cache_creation_tokens)
			FROM token_events WHERE timestamp_unix >= ?
			GROUP BY model
		`
		evtModelRows, err := tc.db.QueryContext(ctx, evtModelQuery, sinceUnix)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if evtModelRows != nil {
			defer evtModelRows.Close()
			for evtModelRows.Next() {
				var model string
				var input, output, cacheRead, cacheCreate int64
				if err := evtModelRows.Scan(&model, &input, &output, &cacheRead, &cacheCreate); err != nil {
					continue
				}
				if existing, ok := result.ModelMetrics[model]; ok {
					existing.InputTokens += input
					existing.OutputTokens += output
					existing.CacheReadTokens += cacheRead
					existing.CacheCreationTokens += cacheCreate
				} else {
					result.ModelMetrics[model] = &ModelAggregation{
						InputTokens:         input,
						OutputTokens:        output,
						CacheReadTokens:     cacheRead,
						CacheCreationTokens: cacheCreate,
					}
				}
				result.ModelTokens[model] += input + output + cacheRead + cacheCreate
			}
		}

		// Combine results
		result.InputTokens = aggInput + evtInput
		result.OutputTokens = aggOutput + evtOutput
		result.CacheReadTokens = aggCacheRead + evtCacheRead
		result.CacheCreationTokens = aggCacheCreate + evtCacheCreate
		result.EventCount = aggCount + evtCount

		// Determine earliest/latest timestamps
		var minTS, maxTS int64 = 0, 0
		if aggMinTS.Valid && aggMinTS.Int64 > 0 {
			minTS = aggMinTS.Int64
		}
		if evtMinTS.Valid && evtMinTS.Int64 > 0 {
			if minTS == 0 || evtMinTS.Int64 < minTS {
				minTS = evtMinTS.Int64
			}
		}
		if aggMaxTS.Valid {
			maxTS = aggMaxTS.Int64
		}
		if evtMaxTS.Valid && evtMaxTS.Int64 > maxTS {
			maxTS = evtMaxTS.Int64
		}

		if minTS > 0 {
			result.EarliestTimestamp = time.Unix(minTS, 0)
		}
		if maxTS > 0 {
			result.LatestTimestamp = time.Unix(maxTS, 0)
		}

		return result, nil
	})
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

// TryAcquireLease attempts to acquire or renew the collector lease
// Returns true if this instance is the leader (should collect metrics)
func (tc *TokenCache) TryAcquireLease(instanceID string) bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return true // No DB, collect locally
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
	defer cancel()

	now := time.Now().Unix()
	expiresAt := now + int64(collectorLeaseDuration.Seconds())

	// Try to acquire or renew lease atomically
	// This uses INSERT OR REPLACE with a check that either:
	// 1. No lease exists
	// 2. Lease is expired
	// 3. We already hold the lease
	result, err := withRetry(ctx, func() (sql.Result, error) {
		return tc.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO collector_lease (id, instance_id, expires_at)
			SELECT 1, ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM collector_lease
				WHERE id = 1 AND expires_at > ? AND instance_id != ?
			)
		`, instanceID, expiresAt, now, instanceID)
	})

	if err != nil {
		return true // On error, collect locally to be safe
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return true
	}

	return rowsAffected > 0
}

// GetCachedMetrics retrieves cached metrics if they're still valid
// Returns nil if cache is stale or doesn't exist
func (tc *TokenCache) GetCachedMetrics(metricType string) ([]byte, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.db == nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
	defer cancel()

	now := time.Now().Unix()
	cutoff := now - int64(metricsCacheTTL.Seconds())

	result, err := withRetry(ctx, func() ([]byte, error) {
		var data []byte
		err := tc.db.QueryRowContext(ctx, `
			SELECT data FROM metrics_cache
			WHERE metric_type = ? AND updated_at >= ?
		`, metricType, cutoff).Scan(&data)
		return data, err
	})

	if err != nil {
		return nil, false
	}

	return result, true
}

// SetCachedMetrics stores metrics in the cache
func (tc *TokenCache) SetCachedMetrics(metricType string, data []byte) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
	defer cancel()

	now := time.Now().Unix()

	return withRetryNoResult(ctx, func() error {
		_, err := tc.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO metrics_cache (metric_type, data, updated_at)
			VALUES (?, ?, ?)
		`, metricType, data, now)
		return err
	})
}

// ReleaseLease releases the collector lease (called on shutdown)
func (tc *TokenCache) ReleaseLease(instanceID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
	defer cancel()

	withRetryNoResult(ctx, func() error {
		_, err := tc.db.ExecContext(ctx, `
			DELETE FROM collector_lease WHERE id = 1 AND instance_id = ?
		`, instanceID)
		return err
	})
}
