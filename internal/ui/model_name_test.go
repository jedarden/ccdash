package ui

import (
	"testing"

	"github.com/jedarden/ccdash/internal/metrics"
)

func TestShortenModelName(t *testing.T) {
	cases := map[string]string{
		// Live model IDs pulled from a real ~/.ccdash/tokens.db on 2026-08-28 —
		// the whole point of the generic parse is that these needed no
		// per-model code change to display their version correctly.
		"claude-fable-5":            "Fable 5",
		"claude-haiku-4-5-20251001": "Haiku 4.5",
		"claude-opus-4-7":           "Opus 4.7",
		"claude-opus-4-8":           "Opus 4.8",
		"claude-sonnet-4-6":         "Sonnet 4.6",
		"claude-sonnet-5":           "Sonnet 5",
		"glm-4.7":                   "GLM 4.7",
		"glm-5-turbo":               "GLM 5",
		"glm-5.1":                   "GLM 5.1",
		// Previously-supported dated snapshots must keep working.
		"claude-opus-4-5-20251101":   "Opus 4.5",
		"claude-sonnet-4-5-20250929": "Sonnet 4.5",
		"claude-haiku-4-5-20250929":  "Haiku 4.5",
		// Legacy Claude 3.x "<version>-<family>" ordering.
		"claude-3-5-sonnet-20241022": "Sonnet 3.5",
		"claude-3-5-haiku-20241022":  "Haiku 3.5",
		"claude-3-opus-20240229":     "Opus 3",
		"claude-3-sonnet-20240229":   "Sonnet 3",
		"claude-3-haiku-20240307":    "Haiku 3",
		// GLM qualitative variant names, not version numbers.
		"glm-4-alltools": "GLM 4 AllTools",
		"glm-4-9b-chat":  "GLM 4 9B",
		"glm-4-air":      "GLM 4 Air",
		"glm-4-flash":    "GLM 4 Flash",
		"glm-4-plus":     "GLM 4+",
		// Unknown family: returned unchanged rather than mangled.
		"<synthetic>":       "<synthetic>",
		"some-future-model": "some-future-model",
	}

	for input, want := range cases {
		if got := shortenModelName(input); got != want {
			t.Errorf("shortenModelName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCalculateRequiredTokenWidthMatchesRenderColumns(t *testing.T) {
	d := &Dashboard{
		tokenMetrics: &metrics.TokenMetrics{
			ModelUsages: []metrics.ModelUsage{
				{Model: "claude-sonnet-4-6"}, // shortens to "Sonnet 4.6", 10 chars
				{Model: "claude-opus-4-7"},   // shortens to "Opus 4.7", 8 chars
			},
		},
	}
	got := d.calculateRequiredTokenWidth()
	// Longest current label is "Sonnet 4.6" at 10 chars, same as the
	// pre-existing default floor, so this must reproduce the panel's
	// long-standing default width exactly: no regression for today's names,
	// only growth once one actually exceeds 10 chars.
	if want := 60; got != want {
		t.Errorf("calculateRequiredTokenWidth() = %d, want %d", got, want)
	}

	d.tokenMetrics.ModelUsages = append(d.tokenMetrics.ModelUsages, metrics.ModelUsage{
		Model: "claude-sonnet-4-12", // hypothetical double-digit minor version, 11 chars shortened
	})
	if got, want := d.calculateRequiredTokenWidth(), 61; got != want {
		t.Errorf("calculateRequiredTokenWidth() with an 11-char name = %d, want %d", got, want)
	}
}
