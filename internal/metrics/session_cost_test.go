package metrics

import (
	"math"
	"testing"
	"time"
)

func TestTranscriptSessionID(t *testing.T) {
	cases := map[string]string{
		"/home/u/.claude/projects/-home-u-repo/0c5f0e3a-1111-2222-3333-444455556666.jsonl":                          "0c5f0e3a-1111-2222-3333-444455556666",
		"/home/u/.codex/sessions/2026/10/07/rollout-2026-10-07T06-57-50-01a11603-3073-7a90-b31b-7b4b2db60537.jsonl": "01a11603-3073-7a90-b31b-7b4b2db60537",
	}
	for path, want := range cases {
		if got, ok := transcriptSessionID(path); !ok || got != want {
			t.Errorf("transcriptSessionID(%q) = %q, %v; want %q", path, got, ok, want)
		}
	}
	for _, path := range []string{"/x/notes.txt", "/x/rollout-short.jsonl", "/x/.jsonl"} {
		if got, ok := transcriptSessionID(path); ok {
			t.Errorf("transcriptSessionID(%q) = %q, want no match", path, got)
		}
	}
}

func TestSessionCostsAttributeByTranscriptWithoutDoubleCounting(t *testing.T) {
	cache := newTestTokenCache(t)
	tc := &TokenCollector{sources: []Source{NewClaudeSource(), NewCodexSource()}, cache: cache}

	const claudeID = "0c5f0e3a-1111-2222-3333-444455556666"
	const codexID = "01a11603-3073-7a90-b31b-7b4b2db60537"
	claudeFile := "/p/" + claudeID + ".jsonl"
	codexFile := "/c/rollout-2026-10-07T06-57-50-" + codexID + ".jsonl"
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	event := func(file, source, model string, at time.Time, input int64, line int64) TokenEvent {
		return TokenEvent{Timestamp: at, Model: model, Source: source, InputTokens: input, SourceFile: file, LineNumber: line}
	}

	// Claude transcript: 1M input on claude-sonnet-5 ($2/M), summarised.
	if err := cache.InsertTokenEventBatch([]TokenEvent{event(claudeFile, "claude", "claude-sonnet-5", base, 1_000_000, 1)}); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileComplete(claudeFile); err != nil {
		t.Fatal(err)
	}
	// A late re-insert of the summarised line (another process) is skipped
	// by covered_lines; a resumed line after the summary (0.5M = $1) counts.
	if err := cache.InsertTokenEventBatch([]TokenEvent{
		event(claudeFile, "claude", "claude-sonnet-5", base, 1_000_000, 1),
		event(claudeFile, "claude", "claude-sonnet-5", base.Add(time.Hour), 500_000, 102),
		// Codex rollout: 1M input on gpt-6-sol ($2/M), still active.
		event(codexFile, "codex", "gpt-6-sol", base, 1_000_000, 1),
		// A session nobody asked about.
		event("/p/ffffffff-0000-0000-0000-000000000000.jsonl", "claude", "claude-sonnet-5", base, 9_000_000, 1),
	}); err != nil {
		t.Fatal(err)
	}

	costs, err := tc.SessionCosts([]string{claudeID, codexID, "no-such-session"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{claudeID: 3.0, codexID: 2.0}
	if len(costs) != len(want) {
		t.Fatalf("costs = %v, want exactly %v", costs, want)
	}
	for id, w := range want {
		if math.Abs(costs[id]-w) > 1e-9 {
			t.Errorf("cost[%s] = %.6f, want %.6f", id, costs[id], w)
		}
	}

	// The lookback window applies: nothing in this cache is after base+2h.
	tc.lookbackFrom = base.Add(2 * time.Hour)
	if costs, _ := tc.SessionCosts([]string{claudeID, codexID}); len(costs) != 0 {
		t.Errorf("costs outside the window = %v, want none", costs)
	}
}
