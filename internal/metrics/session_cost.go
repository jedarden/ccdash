package metrics

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// transcriptSessionID extracts the harness session ID a transcript file is
// named after: Claude Code writes <session-id>.jsonl, Codex writes
// rollout-<timestamp>-<session-id>.jsonl. ok is false for anything else.
func transcriptSessionID(path string) (string, bool) {
	name, found := strings.CutSuffix(filepath.Base(path), ".jsonl")
	if !found || name == "" {
		return "", false
	}
	if strings.HasPrefix(name, "rollout-") {
		const uuidLen = 36
		if len(name) < len("rollout-")+uuidLen {
			return "", false
		}
		return name[len(name)-uuidLen:], true
	}
	return name, true
}

// AttachSessionCosts sets Cost on every session in m whose transcript the
// token cache can attribute. Errors leave costs unset: no number beats a
// wrong one.
func AttachSessionCosts(tc *TokenCollector, m *TmuxMetrics) {
	if tc == nil || m == nil {
		return
	}
	var ids []string
	for _, s := range m.Sessions {
		if s.SessionID != "" {
			ids = append(ids, s.SessionID)
		}
	}
	if len(ids) == 0 {
		return
	}
	costs, err := tc.SessionCosts(ids)
	if err != nil {
		return
	}
	for i := range m.Sessions {
		if cost, ok := costs[m.Sessions[i].SessionID]; ok {
			c := cost
			m.Sessions[i].Cost = &c
		}
	}
}

func usageCost(p ModelPricing, input, output, cacheRead, cacheCreate int64) float64 {
	return (float64(input)*p.InputPerMillion +
		float64(output)*p.OutputPerMillion +
		float64(cacheRead)*p.CacheReadPerMillion +
		float64(cacheCreate)*p.CacheCreatePerMillion) / 1_000_000
}

// SessionCosts returns the cost, in the collector's lookback window, of each
// requested session whose transcript is in the token cache. Attribution is by
// transcript file name only, so sessions without an identifiable transcript
// (tmux-only rows, NEEDLE registry workers) are simply absent from the result
// rather than given an estimate.
//
// A completed transcript is counted from its summary, as the token panel
// counts it, plus only those events newer than the summary, so events that
// also appear in the summary are not counted twice.
func (tc *TokenCollector) SessionCosts(sessionIDs []string) (map[string]float64, error) {
	costs := make(map[string]float64)
	if tc == nil || tc.cache == nil || len(sessionIDs) == 0 {
		return costs, nil
	}
	want := make(map[string]bool, len(sessionIDs))
	for _, id := range sessionIDs {
		if id != "" {
			want[id] = true
		}
	}
	db := tc.cache.GetDB()
	since := int64(0)
	if !tc.lookbackFrom.IsZero() {
		since = tc.lookbackFrom.Unix()
	}

	summarisedUntil := make(map[string]int64)
	rows, err := db.Query(`SELECT source_file, source, latest_timestamp, model_breakdown
		FROM file_aggregates WHERE is_complete = 1 AND latest_timestamp >= ?`, since)
	if err != nil {
		return costs, err
	}
	for rows.Next() {
		var file, source, breakdown string
		var latest int64
		if err := rows.Scan(&file, &source, &latest, &breakdown); err != nil {
			rows.Close()
			return costs, err
		}
		id, ok := transcriptSessionID(file)
		if !ok || !want[id] {
			continue
		}
		summarisedUntil[file] = latest
		var models map[string]*ModelAggregation
		if err := json.Unmarshal([]byte(breakdown), &models); err != nil {
			continue
		}
		for model, m := range models {
			if m == nil {
				continue
			}
			modelSource := m.Source
			if modelSource == "" {
				modelSource = source
			}
			pricing, _ := tc.pricingForModel(modelSource, model)
			costs[id] += usageCost(pricing, m.InputTokens, m.OutputTokens, m.CacheReadTokens, m.CacheCreationTokens)
		}
	}
	rows.Close()

	rows, err = db.Query(`SELECT source_file, source, model, timestamp_unix,
		input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens
		FROM token_events WHERE timestamp_unix >= ?`, since)
	if err != nil {
		return costs, err
	}
	defer rows.Close()
	for rows.Next() {
		var file, source, model string
		var ts, input, output, cacheRead, cacheCreate int64
		if err := rows.Scan(&file, &source, &model, &ts, &input, &output, &cacheRead, &cacheCreate); err != nil {
			return costs, err
		}
		id, ok := transcriptSessionID(file)
		if !ok || !want[id] {
			continue
		}
		if latest, summarised := summarisedUntil[file]; summarised && ts <= latest {
			continue
		}
		pricing, _ := tc.pricingForModel(source, model)
		costs[id] += usageCost(pricing, input, output, cacheRead, cacheCreate)
	}
	return costs, rows.Err()
}
