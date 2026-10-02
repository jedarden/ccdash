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
