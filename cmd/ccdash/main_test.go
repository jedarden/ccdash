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
		if !strings.HasSuffix(statusLines[i], status.Description()) {
			t.Errorf("status %q description = %q, want %q", status, statusLines[i], status.Description())
		}
	}
	if statuses[0] != metrics.StatusAsking {
		t.Fatalf("help should list ASKING first, got %s", statuses[0])
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

func TestSnapshotJSONMatchesVersionOneGolden(t *testing.T) {
	timestamp := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	system := metrics.SystemMetrics{
		CPU:        metrics.CPUMetrics{TotalPercent: 12.5, PerCore: []float64{12.5, 0}},
		Load:       metrics.LoadMetrics{Load1: 1.25, Load5: 0.75, Load15: 0.5},
		Memory:     metrics.MemoryMetrics{Used: 100, Total: 400, Percentage: 25},
		Swap:       metrics.SwapMetrics{Used: 10, Total: 100, Percentage: 10},
		DiskUsage:  metrics.DiskUsageMetrics{Used: 200, Total: 1000, Free: 800, Percentage: 20, Path: "/"},
		DiskIO:     metrics.DiskIOMetrics{ReadBytesPerSec: 80, WriteBytesPerSec: 40},
		NetIO:      metrics.NetIOMetrics{RecvBytesPerSec: 128, SentBytesPerSec: 64, Interfaces: []metrics.NetInterface{{Name: "eth0", RecvBytesPerSec: 128, SentBytesPerSec: 64, TotalRecvBytes: 1024, TotalSentBytes: 512}}},
		LastUpdate: timestamp,
	}
	tokens := &metrics.TokenMetrics{
		InputTokens: 2, OutputTokens: 3, CacheReadTokens: 4, CacheCreationTokens: 5,
		TotalTokens: 14, Prompts: 1, TotalCost: 0.25, Rate: 6.5, SessionAvgRate: 7.5,
		TimeSpan: 2 * time.Minute, EarliestTimestamp: timestamp.Add(-time.Minute),
		LatestTimestamp: timestamp, LookbackFrom: timestamp.Add(-time.Hour),
		Models: []string{"test-model"}, ModelUsages: []metrics.ModelUsage{{
			Model: "test-model", Source: "claude", InputTokens: 2, OutputTokens: 3,
			CacheReadTokens: 4, CacheCreationTokens: 5, TotalTokens: 14, Cost: 0.25,
		}}, RateHistory: []int64{2, 4},
		Available: true, LastUpdate: timestamp,
	}
	sessions := &metrics.TmuxMetrics{
		Sessions: []metrics.TmuxSession{{
			Name: "agent-1", SessionType: metrics.SessionTypeWorker,
			Worker: &metrics.WorkerMetadata{
				FullName: "worker", Workspace: "/work", Agent: "codex", Provider: "openai",
				Model: "test-model", State: "working", CurrentBead: "ccdash-0a0b6ab4",
				BeadStatusAvailable: true, BeadsProcessed: 2, BeadsCompleted: 1,
			},
			Windows: 2, Attached: true, Status: metrics.StatusWorking,
			Created: timestamp.Add(-10 * time.Minute), LastContentChange: timestamp.Add(-time.Minute),
			IdleDuration: time.Minute, LastLines: []string{"hello"}, Source: "hooks", Harness: "codex",
		}}, Available: true, LastUpdate: timestamp,
		HooksAvailable: true, HooksInstalled: true, Source: "hooks",
	}

	got, err := json.MarshalIndent(makeSnapshot(timestamp, "test", system, tokens, sessions, 0), "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	got = append(got, '\n')
	want, err := os.ReadFile(filepath.Join("testdata", "once-snapshot-v1.golden.json"))
	if err != nil {
		t.Fatalf("read snapshot golden: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("snapshot JSON differs from version 1 golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
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
	if snapshot.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", snapshot.SchemaVersion)
	}
	if snapshot.Tokens.Rate != nil || snapshot.System.DiskIO.ReadBytesPerSec != nil || snapshot.System.DiskIO.WriteBytesPerSec != nil || snapshot.System.NetIO.RecvBytesPerSec != nil || snapshot.System.NetIO.SentBytesPerSec != nil {
		t.Errorf("one-shot rates should be null: tokens=%v disk=%+v net=%+v", snapshot.Tokens.Rate, snapshot.System.DiskIO, snapshot.System.NetIO)
	}
}
