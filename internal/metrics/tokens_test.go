package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

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
