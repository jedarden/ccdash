package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jedarden/ccdash/internal/metrics"
)

// parseSince accepts the named token windows used by the TUI and common script
// windows, plus an explicit RFC3339 timestamp.
func parseSince(value string, now time.Time) (time.Time, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "monday":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 9, 0, 0, 0, now.Location())
		if monday.After(now) {
			monday = monday.AddDate(0, 0, -7)
		}
		return monday, nil
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	case "24h":
		return now.Add(-24 * time.Hour), nil
	case "7d":
		return now.Add(-7 * 24 * time.Hour), nil
	default:
		since, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid --since value %q: use monday, today, 24h, 7d, or an RFC3339 timestamp", value)
		}
		return since, nil
	}
}

func tokenEventsSince(events []metrics.TokenEventExport, since time.Time) []metrics.TokenEventExport {
	if since.IsZero() {
		return events
	}
	filtered := make([]metrics.TokenEventExport, 0, len(events))
	for _, event := range events {
		if !event.Timestamp.Before(since) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// File aggregates are indivisible cached totals. Include one when its latest
// event falls in the window, matching QueryTokensHybrid's lookback behavior.
func fileAggregatesSince(aggregates []metrics.FileAggregateExport, since time.Time) []metrics.FileAggregateExport {
	if since.IsZero() {
		return aggregates
	}
	filtered := make([]metrics.FileAggregateExport, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if !aggregate.LatestTimestamp.Before(since) {
			filtered = append(filtered, aggregate)
		}
	}
	return filtered
}

func askingSessions(sessions []metrics.TmuxSession) []metrics.TmuxSession {
	asking := make([]metrics.TmuxSession, 0)
	for _, session := range sessions {
		if session.Status == metrics.StatusAsking {
			asking = append(asking, session)
		}
	}
	sort.Slice(asking, func(i, j int) bool {
		return asking[i].Name < asking[j].Name
	})
	return asking
}

func attentionExitCode(sessions []metrics.TmuxSession) int {
	if len(askingSessions(sessions)) > 0 {
		return 1
	}
	return 0
}

func printAttentionSessions(w io.Writer, sessions []metrics.TmuxSession) {
	fmt.Fprintln(w, "Sessions asking for human input:")
	for _, session := range sessions {
		fmt.Fprintf(w, "  - %s\n", session.Name)
	}
}

func runAttentionMode() int {
	snapshot := metrics.NewTmuxCollector().Collect()
	asking := askingSessions(snapshot.Sessions)
	if len(asking) == 0 {
		fmt.Println("No sessions are asking for human input.")
		return attentionExitCode(snapshot.Sessions)
	}
	printAttentionSessions(os.Stdout, asking)
	return attentionExitCode(snapshot.Sessions)
}
