package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CachedTokenData represents cached historical token data
type CachedTokenData struct {
	InputTokens         int64            `json:"input_tokens"`
	OutputTokens        int64            `json:"output_tokens"`
	CacheReadTokens     int64            `json:"cache_read_tokens"`
	CacheCreationTokens int64            `json:"cache_creation_tokens"`
	Models              map[string]int64 `json:"models"` // model -> total tokens
	ModelCosts          map[string]float64 `json:"model_costs"` // model -> cost
	LastProcessedLine   int64            `json:"last_processed_line"` // Line count for incremental processing
	LastModified        time.Time        `json:"last_modified"`
	CacheVersion        int              `json:"cache_version"`
}

// TokenCache manages persistent caching of token metrics
type TokenCache struct {
	cacheDir     string
	mu           sync.RWMutex
	fileCache    map[string]*CachedTokenData // filepath -> cached data
	dirty        bool
}

const (
	cacheVersion = 1
	cacheDirName = ".ccdash"
	cacheFileName = "token_cache.json"
)

// NewTokenCache creates a new token cache in the .ccdash directory
func NewTokenCache() *TokenCache {
	// Get directory where binary is invoked (current working directory)
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	cacheDir := filepath.Join(cwd, cacheDirName)

	tc := &TokenCache{
		cacheDir:  cacheDir,
		fileCache: make(map[string]*CachedTokenData),
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err == nil {
		tc.load()
	}

	return tc
}

// GetCachePath returns the full path to the cache file
func (tc *TokenCache) GetCachePath() string {
	return filepath.Join(tc.cacheDir, cacheFileName)
}

// load reads the cache from disk
func (tc *TokenCache) load() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	data, err := os.ReadFile(tc.GetCachePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cache file yet
		}
		return err
	}

	var cache map[string]*CachedTokenData
	if err := json.Unmarshal(data, &cache); err != nil {
		return err
	}

	// Validate cache version
	for _, v := range cache {
		if v.CacheVersion != cacheVersion {
			// Cache version mismatch, invalidate
			tc.fileCache = make(map[string]*CachedTokenData)
			return nil
		}
	}

	tc.fileCache = cache
	return nil
}

// Save persists the cache to disk
func (tc *TokenCache) Save() error {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if !tc.dirty {
		return nil
	}

	data, err := json.MarshalIndent(tc.fileCache, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(tc.cacheDir, 0755); err != nil {
		return err
	}

	tc.dirty = false
	return os.WriteFile(tc.GetCachePath(), data, 0644)
}

// Get retrieves cached data for a file
func (tc *TokenCache) Get(filepath string) (*CachedTokenData, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	data, ok := tc.fileCache[filepath]
	return data, ok
}

// Set stores cached data for a file
func (tc *TokenCache) Set(filepath string, data *CachedTokenData) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	data.CacheVersion = cacheVersion
	tc.fileCache[filepath] = data
	tc.dirty = true
}

// IsStale checks if cached data is older than the file
func (tc *TokenCache) IsStale(filepath string, fileModTime time.Time) bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	cached, ok := tc.fileCache[filepath]
	if !ok {
		return true
	}

	return fileModTime.After(cached.LastModified)
}

// Invalidate removes cached data for a file
func (tc *TokenCache) Invalidate(filepath string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	delete(tc.fileCache, filepath)
	tc.dirty = true
}

// Clear removes all cached data
func (tc *TokenCache) Clear() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.fileCache = make(map[string]*CachedTokenData)
	tc.dirty = true
}

// GetHistoricalTotals returns aggregated historical data (before lookback)
func (tc *TokenCache) GetHistoricalTotals() (inputTokens, outputTokens, cacheRead, cacheCreate int64, modelCosts map[string]float64) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	modelCosts = make(map[string]float64)

	for _, cached := range tc.fileCache {
		inputTokens += cached.InputTokens
		outputTokens += cached.OutputTokens
		cacheRead += cached.CacheReadTokens
		cacheCreate += cached.CacheCreationTokens

		for model, cost := range cached.ModelCosts {
			modelCosts[model] += cost
		}
	}

	return
}
