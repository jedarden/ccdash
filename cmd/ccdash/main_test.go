package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
	"github.com/jedarden/ccdash/internal/ui"
)

func TestHelpMatchesDashboardKeysAndSessionStatuses(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create help output pipe: %v", err)
	}
	os.Stdout = writer
	printHelp()
	if err := writer.Close(); err != nil {
		t.Fatalf("close help output pipe: %v", err)
	}
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read help output: %v", err)
	}

	shortcutLines := helpSectionLines(t, string(output), "KEYBOARD SHORTCUTS:")
	wantShortcuts := ui.DashboardKeyboardShortcuts()
	if len(shortcutLines) != len(wantShortcuts) {
		t.Fatalf("help has %d shortcut lines, want %d", len(shortcutLines), len(wantShortcuts))
	}
	for i, shortcut := range wantShortcuts {
		want := fmt.Sprintf("  %-15s %s", shortcut.Keys, shortcut.Description)
		if shortcutLines[i] != want {
			t.Errorf("shortcut line %d = %q, want %q", i, shortcutLines[i], want)
		}
	}

	statusLines := helpSectionLines(t, string(output), "STATUS INDICATORS:")
	statuses := metrics.SessionStatuses()
	if len(statusLines) != len(statuses) {
		t.Fatalf("help has %d status lines, want %d", len(statusLines), len(statuses))
	}
	for i, status := range statuses {
		wantPrefix := fmt.Sprintf("  %s %-8s - ", status.GetEmoji(), status)
		if !strings.HasPrefix(statusLines[i], wantPrefix) {
			t.Errorf("status line %d = %q, want prefix %q", i, statusLines[i], wantPrefix)
		}
		if strings.TrimSpace(strings.TrimPrefix(statusLines[i], wantPrefix)) == "" {
			t.Errorf("status %q has no help description", status)
		}
	}
}

func helpSectionLines(t *testing.T, output, heading string) []string {
	t.Helper()
	sectionStart := strings.Index(output, heading+"\n")
	if sectionStart < 0 {
		t.Fatalf("help is missing section %q", heading)
	}
	section := output[sectionStart+len(heading)+1:]
	var lines []string
	for _, line := range strings.Split(section, "\n") {
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	return lines
}

func TestRunOnceReadsTokenCacheFromHomeRegardlessOfWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatal(err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}

	cache := metrics.NewTokenCache()
	since := metrics.GetMondayNineAM()
	if err := cache.InsertTokenEvent(time.Now(), "claude-sonnet-4-5-20250929", 100, 20, 30, 4, "session.jsonl", 1); err != nil {
		t.Fatalf("insert token event: %v", err)
	}
	direct, err := cache.QueryTokensHybrid(since)
	if err != nil {
		t.Fatalf("direct hybrid query: %v", err)
	}
	if err := cache.Close(); err != nil {
		t.Fatalf("close seeded cache: %v", err)
	}

	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := runOnceMode(true, "")
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read --once output: %v", err)
	}
	if code != 0 {
		t.Fatalf("runOnceMode exit code = %d, output: %s", code, output)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(output, &snapshot); err != nil {
		t.Fatalf("decode --once snapshot: %v\n%s", err, output)
	}
	wantTotal := direct.InputTokens + direct.OutputTokens + direct.CacheReadTokens + direct.CacheCreationTokens
	if snapshot.Tokens == nil || !snapshot.Tokens.Available || snapshot.Tokens.TotalTokens != wantTotal || snapshot.Tokens.Prompts != direct.EventCount {
		t.Fatalf("--once tokens = %+v; direct query = %d tokens, %d events", snapshot.Tokens, wantTotal, direct.EventCount)
	}
	if snapshot.Tokens.TotalTokens == 0 || snapshot.Tokens.TotalCost <= 0 {
		t.Fatalf("--once should report non-zero tokens and cost, got %d tokens and $%.6f",
			snapshot.Tokens.TotalTokens, snapshot.Tokens.TotalCost)
	}
}
