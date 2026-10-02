package ui

import (
	"strings"
	"testing"

	"github.com/jedarden/ccdash/internal/metrics"
)

func TestTokenPanelShowsUnavailableCacheInsteadOfZeroTotals(t *testing.T) {
	d := &Dashboard{tokenMetrics: &metrics.TokenMetrics{
		Available: false,
		Error:     "Token cache unavailable",
	}}

	view := d.renderTokenPanel(80, 10)
	if !strings.Contains(view, "Not Available") || !strings.Contains(view, "Token cache unavailable") {
		t.Fatalf("token panel should explain unavailable data:\n%s", view)
	}
	if strings.Contains(view, "Total:") || strings.Contains(view, "Cost:") {
		t.Fatalf("token panel should not render misleading zero totals:\n%s", view)
	}
}

func TestTokenPanelHidesZeroTokenZeroCostModelRows(t *testing.T) {
	d := &Dashboard{tokenMetrics: &metrics.TokenMetrics{
		Available:   true,
		TotalTokens: 120,
		TotalCost:   0.01,
		ModelUsages: []metrics.ModelUsage{
			{Model: "<synthetic>"},
			{Model: "claude-haiku-4-5-20251001", TotalTokens: 120, Cost: 0.01},
		},
	}}

	view := d.renderTokenPanel(100, 10)
	if strings.Contains(view, "<synthetic>") {
		t.Fatalf("zero-token, zero-cost model row should be hidden:\n%s", view)
	}
	if !strings.Contains(view, "Haiku 4.5") {
		t.Fatalf("model row with usage should remain visible:\n%s", view)
	}

	d.tokenMetrics.ModelUsages = []metrics.ModelUsage{{Model: "<synthetic>"}}
	view = d.renderTokenPanel(100, 10)
	if strings.Contains(view, "Models:") || strings.Contains(view, "<synthetic>") {
		t.Fatalf("empty model rows should not produce a Models section:\n%s", view)
	}
}
