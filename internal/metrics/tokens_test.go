package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFollowerReadsTokenTotalsFromSharedHomeCache(t *testing.T) {
	home := t.TempDir()
	leaderDir := t.TempDir()
	followerDir := t.TempDir()
	t.Setenv("HOME", home)

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	if err := os.Chdir(leaderDir); err != nil {
		t.Fatal(err)
	}
	leader := NewTokenCache()
	t.Cleanup(func() { _ = leader.Close() })
	if got, want := leader.GetDBPath(), filepath.Join(home, cacheDirName, cacheDBName); got != want {
		t.Fatalf("leader cache path = %q, want shared home cache %q", got, want)
	}

	since := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := leader.InsertTokenEvent(time.Now(), "claude-sonnet-4-5-20250929", 100, 20, 30, 4, "session.jsonl", 1); err != nil {
		t.Fatalf("insert token event: %v", err)
	}
	if !leader.TryAcquireLease("leader") {
		t.Fatal("leader failed to acquire collector lease")
	}

	if err := os.Chdir(followerDir); err != nil {
		t.Fatal(err)
	}
	follower := NewTokenCache()
	t.Cleanup(func() { _ = follower.Close() })
	if got, want := follower.GetDBPath(), leader.GetDBPath(); got != want {
		t.Fatalf("follower cache path = %q, want leader cache path %q", got, want)
	}
	if follower.TryAcquireLease("follower") {
		t.Fatal("follower unexpectedly acquired the leader's lease")
	}

	direct, err := follower.QueryTokensHybrid(since)
	if err != nil {
		t.Fatalf("direct hybrid query: %v", err)
	}
	collector := &TokenCollector{
		sources:      []Source{NewClaudeSource()},
		sourceDirs:   map[string][]string{"claude": {filepath.Join(home, ".claude", "projects")}},
		lookbackFrom: since,
		cache:        follower,
	}
	got, err := collector.Collect()
	if err != nil {
		t.Fatalf("collect follower token metrics: %v", err)
	}
	wantTotal := direct.InputTokens + direct.OutputTokens + direct.CacheReadTokens + direct.CacheCreationTokens
	if !got.Available || got.TotalTokens != wantTotal || got.Prompts != direct.EventCount {
		t.Fatalf("follower metrics = available %t, tokens %d, prompts %d; direct query = tokens %d, events %d",
			got.Available, got.TotalTokens, got.Prompts, wantTotal, direct.EventCount)
	}
	if got.TotalTokens == 0 || got.TotalCost <= 0 {
		t.Fatalf("follower metrics should be non-zero, got %d tokens and $%.6f", got.TotalTokens, got.TotalCost)
	}
}

func TestCollectMarksUninitializedTokenCacheUnavailable(t *testing.T) {
	collector := &TokenCollector{
		sourceDirs: map[string][]string{"claude": {"configured-transcript-directory"}},
		cache:      &TokenCache{},
	}
	got, err := collector.Collect()
	if err != nil {
		t.Fatalf("collect with unavailable cache: %v", err)
	}
	if got.Available || got.Error == "" {
		t.Fatalf("metrics should report unavailable cache, got Available=%t Error=%q", got.Available, got.Error)
	}
}

func TestExpandGlobPatterns(t *testing.T) {
	// Create temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "ccdash-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test directories
	testDirs := []string{
		filepath.Join(tmpDir, "project1"),
		filepath.Join(tmpDir, "project2"),
		filepath.Join(tmpDir, "other"),
	}

	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create test dir %s: %v", dir, err)
		}
	}

	// Test 1: Glob pattern matching
	globPath := filepath.Join(tmpDir, "project*")
	expanded := ExpandGlobPatterns([]string{globPath})

	if len(expanded) != 2 {
		t.Errorf("Expected 2 matches for glob pattern %s, got %d", globPath, len(expanded))
	}

	// Test 2: Mixed glob and literal paths
	mixedPaths := []string{
		filepath.Join(tmpDir, "project1"),
		filepath.Join(tmpDir, "other"),
	}
	expanded = ExpandGlobPatterns(mixedPaths)

	if len(expanded) != 2 {
		t.Errorf("Expected 2 matches for mixed paths, got %d", len(expanded))
	}

	// Test 3: Non-existent paths (should be filtered out)
	expanded = ExpandGlobPatterns([]string{
		filepath.Join(tmpDir, "nonexistent"),
		filepath.Join(tmpDir, "also-nonexistent"),
	})

	if len(expanded) != 0 {
		t.Errorf("Expected 0 matches for non-existent paths, got %d", len(expanded))
	}

	// Test 4: Glob pattern with no matches
	badGlob := filepath.Join(tmpDir, "nomatch*")
	expanded = ExpandGlobPatterns([]string{badGlob})

	if len(expanded) != 0 {
		t.Errorf("Expected 0 matches for glob with no matches, got %d", len(expanded))
	}

	// Test 5: Empty list
	expanded = ExpandGlobPatterns([]string{})
	if len(expanded) != 0 {
		t.Errorf("Expected 0 matches for empty list, got %d", len(expanded))
	}
}

func TestExpandGlobPatternsDeduplication(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "ccdash-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a single test directory
	testDir := filepath.Join(tmpDir, "testproj")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// Test deduplication: same path specified multiple times
	paths := []string{testDir, testDir, testDir}
	expanded := ExpandGlobPatterns(paths)

	if len(expanded) != 1 {
		t.Errorf("Expected 1 unique path after deduplication, got %d", len(expanded))
	}
}
