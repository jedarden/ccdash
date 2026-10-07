package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
)

// Token panel column layout constants, shared between renderTokenPanel
// (which lays the columns out) and calculateRequiredTokenWidth /
// renderUltraWide (which decide how much width the panel gets). Kept in one
// place so the panel's sizing can't drift out of sync with what it renders,
// which is what let model names silently start truncating.
const (
	tokenPanelBorderPadding = 4 // panelStyle border(2) + padding(2)
	// tokenSideBySideMinWidth is the narrowest token panel that keeps the
	// models column beside the stats with 10-column model names. Narrower
	// panels stack the models below the stats, where a short terminal cuts
	// them off.
	tokenSideBySideMinWidth = tokenLeftColWidth + tokenColSeparatorWidth + 10 + tokenRightColReserve + tokenPanelBorderPadding
	tokenLeftColWidth       = 22 // fixed stats column ("Total:", "Cost:", ...)
	tokenColSeparatorWidth  = 2  // "│ " between columns
	tokenRightColReserve    = 22 // worst-case " $XXX.XX (XXX.XB)" cost+token suffix per model line
)

// renderTokenPanel renders the token usage panel with side-by-side layout
// Left side: Total token stats, Right side: Per-model costs
func (d *Dashboard) renderTokenPanel(width, height int) string {
	style := panelStyle

	if d.tokenMetrics == nil {
		return style.Width(width).Height(height).Render("Loading token metrics...")
	}

	contentWidth := width - 4 // Account for borders and padding

	// Title with lookback info aligned right
	title := successStyle.Render("💰 Token Usage")
	lookbackInfo := ""
	if d.tokenMetrics != nil && !d.tokenMetrics.LookbackFrom.IsZero() {
		// Format start time - use date if not this week
		now := time.Now()
		startTime := d.tokenMetrics.LookbackFrom
		elapsed := now.Sub(startTime)

		// Choose format based on how long ago
		var timeStr string
		if elapsed < 7*24*time.Hour {
			timeStr = startTime.Format("Mon 3:04pm")
		} else {
			timeStr = startTime.Format("Jan 2 3:04pm")
		}

		// Add human-readable duration
		durationStr := metrics.FormatDuration(elapsed)
		lookbackInfo = dimStyle.Render(fmt.Sprintf("%s → Now (%s)", timeStr, durationStr))
	} else if d.tokenMetrics != nil {
		lookbackInfo = dimStyle.Render("All time")
	}

	titleLen := lipgloss.Width(title)
	lookbackLen := lipgloss.Width(lookbackInfo)
	spacing := contentWidth - titleLen - lookbackLen
	if spacing < 1 {
		spacing = 1
	}
	headerLine := title + strings.Repeat(" ", spacing) + lookbackInfo

	if !d.tokenMetrics.Available {
		var lines []string
		lines = append(lines, headerLine)
		lines = append(lines, errorStyle.Render("Not Available"))
		if d.tokenMetrics.Error != "" {
			lines = append(lines, wrapText(d.tokenMetrics.Error, width-4))
		}
		content := strings.Join(lines, "\n")
		return style.Width(width).Height(height).Render(content)
	}

	// Helper to get model style by name
	getModelStyle := func(modelName string) lipgloss.Style {
		if strings.Contains(modelName, "opus") {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")) // Red for Opus
		} else if strings.Contains(modelName, "sonnet") {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4")) // Cyan for Sonnet
		} else if strings.Contains(modelName, "haiku") {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#95e1d3")) // Light green for Haiku
		} else if strings.Contains(modelName, "glm") {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#00bfff")) // Blue for GLM
		}
		return dimStyle
	}

	// Build left column: Total stats
	var leftLines []string
	hasCacheRead := d.tokenMetrics.CacheReadTokens > 0
	hasCacheCreate := d.tokenMetrics.CacheCreationTokens > 0
	hasRate := d.tokenMetrics.Rate > 0
	hasAvg := d.tokenMetrics.SessionAvgRate > 0

	// Compact format for left column
	leftLines = append(leftLines, fmt.Sprintf("In:    %s", metrics.FormatTokensCompact(d.tokenMetrics.InputTokens)))
	leftLines = append(leftLines, fmt.Sprintf("Out:   %s", metrics.FormatTokensCompact(d.tokenMetrics.OutputTokens)))
	if hasCacheRead {
		leftLines = append(leftLines, fmt.Sprintf("Cache: %s", metrics.FormatTokensCompact(d.tokenMetrics.CacheReadTokens)))
	}
	if hasCacheCreate {
		leftLines = append(leftLines, fmt.Sprintf("Create:%s", metrics.FormatTokensCompact(d.tokenMetrics.CacheCreationTokens)))
	}
	totalLine := fmt.Sprintf("Total: %s", boldStyle.Render(metrics.FormatTokensCompact(d.tokenMetrics.TotalTokens)))
	leftLines = append(leftLines, totalLine)
	leftLines = append(leftLines, fmt.Sprintf("Reqs:  %d", d.tokenMetrics.Prompts))

	// Check cost threshold and apply warning color if exceeded
	costValue := metrics.FormatCost(d.tokenMetrics.TotalCost)
	costDisplay := costStyle.Render(costValue)
	if d.notifyConfig != nil && d.notifyConfig.Alerts.CostThresholdUSD > 0 && d.tokenMetrics.TotalCost >= d.notifyConfig.Alerts.CostThresholdUSD {
		costDisplay = warningStyle.Render(costValue)
	}
	costLine := fmt.Sprintf("Cost:  %s", costDisplay)
	leftLines = append(leftLines, costLine)
	// headline rows are kept first when the panel is too short for every
	// stat: cost, then the week projection and budget that qualify it.
	headline := []string{costLine}
	if proj, ok := metrics.ProjectWeekCost(d.tokenMetrics.LookbackFrom, panelNow(), d.tokenMetrics.TotalCost); ok {
		projLine := fmt.Sprintf("Proj:  %s", dimStyle.Render(metrics.FormatCost(proj.Cost)+"/wk"))
		leftLines = append(leftLines, projLine)
		headline = append(headline, projLine)
	}
	if d.notifyConfig != nil && d.notifyConfig.Alerts.CostThresholdUSD > 0 {
		used := d.tokenMetrics.TotalCost / d.notifyConfig.Alerts.CostThresholdUSD * 100
		budgetLine := "Budget " + d.renderMiniBar(used, 14)
		leftLines = append(leftLines, budgetLine)
		headline = append(headline, budgetLine)
	}
	// Trend sparkline is rendered as its own full-width row below both columns
	// (see below) rather than appended here, so it can't overflow the fixed
	// left column and wrap the Models column beside it.
	var trendSparkline string
	if hasRate {
		leftLines = append(leftLines, fmt.Sprintf("Rate:  %s", dimStyle.Render(metrics.FormatTokenRateCompact(d.tokenMetrics.Rate))))

		if cache := d.tokenCollector.GetCache(); cache != nil {
			buckets, _ := cache.QueryBuckets(30*60, 60) // 30 min, 1-min buckets
			bucketValues := make([]int64, len(buckets))
			for i, b := range buckets {
				bucketValues[i] = b.Tokens
			}
			trendSparkline = d.renderSparkline(bucketValues)
		}
	}
	if hasAvg {
		leftLines = append(leftLines, fmt.Sprintf("Avg:   %s", dimStyle.Render(metrics.FormatTokenRateCompact(d.tokenMetrics.SessionAvgRate))))
	}

	// Cost and Total are the headline numbers. When the panel is too short for
	// every stat (one row goes to the header), lead with them so trimming drops
	// the breakdown rows instead.
	if 1+len(leftLines) > height {
		prioritized := append(append([]string{}, headline...), totalLine)
		keep := make(map[string]bool, len(prioritized))
		for _, line := range prioritized {
			keep[line] = true
		}
		for _, line := range leftLines {
			if !keep[line] {
				prioritized = append(prioritized, line)
			}
		}
		leftLines = prioritized
	}

	// Determine layout based on width
	// For narrow panels, stack vertically; for wider panels, use side-by-side
	modelUsages := nonzeroModelUsages(d.tokenMetrics.ModelUsages)
	modelCount := len(modelUsages)
	useSideBySide := width >= tokenSideBySideMinWidth && modelCount > 0

	// Calculate available width for model names based on layout: the names
	// get whatever the widest actual " $cost (tokens)" suffix leaves, not a
	// fixed worst case, so they are not truncated while the column has room.
	leftWidth := tokenLeftColWidth
	rightWidth := contentWidth - leftWidth - tokenColSeparatorWidth
	if !useSideBySide {
		rightWidth = contentWidth
	}
	maxModelNameWidth := rightWidth - modelSuffixWidth(modelUsages)
	if maxModelNameWidth < 10 {
		maxModelNameWidth = 10 // Minimum display width
	}

	// Build right column: Per-model costs with dynamic name width
	var rightLines []string
	if modelCount > 0 {
		hasEstimatedPricing := false
		for _, usage := range modelUsages {
			if usage.PricingEstimated {
				hasEstimatedPricing = true
				break
			}
		}
		modelsLabel := "Models:"
		if hasEstimatedPricing {
			modelsLabel = "Models (? = estimated):"
		}
		rightLines = append(rightLines, boldStyle.Render(modelsLabel))
		for _, usage := range modelUsages {
			displayName := shortenModelName(usage.Model)
			estimatedMarker := ""
			if usage.PricingEstimated {
				estimatedMarker = "?"
			}
			// Dynamically truncate based on available space
			nameWidth := maxModelNameWidth - len(estimatedMarker)
			if nameWidth < 1 {
				nameWidth = 1
			}
			if len(displayName) > nameWidth {
				displayName = displayName[:nameWidth-1] + "…"
			}
			displayName += estimatedMarker
			modelStyle := getModelStyle(usage.Model)
			// All model info on one line: Name Cost (Tokens)
			line := fmt.Sprintf("%s %s %s",
				modelStyle.Render(displayName),
				costStyle.Render(metrics.FormatCost(usage.Cost)),
				dimStyle.Render("("+metrics.FormatTokensCompact(usage.TotalTokens)+")"))
			rightLines = append(rightLines, line)
		}
	}

	var lines []string
	lines = append(lines, headerLine)

	if useSideBySide {
		// Side-by-side: left column for totals, right for models

		// Pad columns to equal height
		maxLines := len(leftLines)
		if len(rightLines) > maxLines {
			maxLines = len(rightLines)
		}
		for len(leftLines) < maxLines {
			leftLines = append(leftLines, "")
		}
		for len(rightLines) < maxLines {
			rightLines = append(rightLines, "")
		}

		// Combine columns
		for i := 0; i < maxLines; i++ {
			left := leftLines[i]
			right := rightLines[i]
			// Pad left to fixed width
			leftPadded := left + strings.Repeat(" ", max(0, leftWidth-lipgloss.Width(left)))
			// Truncate right if needed (safety fallback)
			if lipgloss.Width(right) > rightWidth {
				right = right[:rightWidth-1] + "…"
			}
			lines = append(lines, leftPadded+"│ "+right)
		}
	} else {
		// Stacked layout for narrow panels
		lines = append(lines, leftLines...)
		if modelCount > 0 {
			lines = append(lines, "")
			lines = append(lines, rightLines...)
		}
	}

	if trendSparkline != "" {
		const prefix = "Trend (30m): "
		spark := []rune(trendSparkline)
		if maxSparkWidth := contentWidth - len(prefix); maxSparkWidth < len(spark) {
			if maxSparkWidth < 0 {
				maxSparkWidth = 0
			}
			spark = spark[:maxSparkWidth]
		}
		lines = append(lines, "", dimStyle.Render(prefix+string(spark)))
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

func nonzeroModelUsages(usages []metrics.ModelUsage) []metrics.ModelUsage {
	visible := make([]metrics.ModelUsage, 0, len(usages))
	for _, usage := range usages {
		if usage.TotalTokens == 0 && usage.Cost == 0 {
			continue
		}
		visible = append(visible, usage)
	}
	return visible
}

// legacyModelNames overrides shortenModelName's generic parse for model IDs
// that don't carry a plain "<family>-<version>" number: GLM's qualitative
// variant names, and Claude 3.x's "<version>-<family>" token ordering.
var legacyModelNames = map[string]string{
	"claude-3-5-sonnet-20241022": "Sonnet 3.5",
	"claude-3-5-haiku-20241022":  "Haiku 3.5",
	"claude-3-opus-20240229":     "Opus 3",
	"claude-3-sonnet-20240229":   "Sonnet 3",
	"claude-3-haiku-20240307":    "Haiku 3",
	"glm-4-alltools":             "GLM 4 AllTools",
	"glm-4-9b-chat":              "GLM 4 9B",
	"glm-4-air":                  "GLM 4 Air",
	"glm-4-flash":                "GLM 4 Flash",
	"glm-4-plus":                 "GLM 4+",
}

// shortenModelName produces a compact display label for a model ID by
// pairing its family (Opus/Sonnet/Haiku/Fable/GLM) with the version number
// found next to it — "claude-sonnet-4-6" -> "Sonnet 4.6",
// "claude-opus-4-5-20251101" -> "Opus 4.5" (the trailing snapshot date is
// dropped rather than read as a version part). This is a generic parse
// rather than a table of known IDs so a new version number displays
// correctly without a code change; see legacyModelNames for the IDs that
// don't fit the pattern.
func shortenModelName(name string) string {
	if base, long := strings.CutSuffix(name, metrics.CodexLongContextSuffix); long {
		return shortenModelName(base) + " >272K"
	}
	if short, ok := legacyModelNames[name]; ok {
		return short
	}

	tokens := strings.Split(name, "-")
	families := map[string]string{
		"opus":   "Opus",
		"sonnet": "Sonnet",
		"haiku":  "Haiku",
		"fable":  "Fable",
		"glm":    "GLM",
	}

	familyIdx, label := -1, ""
	for i, tok := range tokens {
		if l, ok := families[tok]; ok {
			familyIdx, label = i, l
			break
		}
	}
	if familyIdx < 0 {
		return name
	}

	if version := modelVersionNear(tokens, familyIdx); version != "" {
		return label + " " + version
	}
	return label
}

// modelVersionNear collects the version number adjacent to a model family
// token, e.g. ["claude","sonnet","4","6"] at index 1 -> "4.6". It looks
// forward first (current Claude/GLM naming: "sonnet-4-6", "glm-5.1"), then
// backward (legacy Claude 3.x naming: "3-5-sonnet"). A token that is purely
// 8 digits is treated as a YYYYMMDD release date and ends the scan without
// being included.
func modelVersionNear(tokens []string, familyIdx int) string {
	isVersionPart := func(s string) bool {
		if s == "" {
			return false
		}
		for _, r := range s {
			if (r < '0' || r > '9') && r != '.' {
				return false
			}
		}
		return true
	}
	isReleaseDate := func(s string) bool {
		return len(s) == 8 && !strings.Contains(s, ".") && isVersionPart(s)
	}

	var forward []string
	for i := familyIdx + 1; i < len(tokens); i++ {
		if isReleaseDate(tokens[i]) || !isVersionPart(tokens[i]) {
			break
		}
		forward = append(forward, tokens[i])
	}
	if len(forward) > 0 {
		return strings.Join(forward, ".")
	}

	var backward []string
	for i := familyIdx - 1; i >= 0 && isVersionPart(tokens[i]); i-- {
		backward = append(backward, tokens[i])
	}
	for l, r := 0, len(backward)-1; l < r; l, r = l+1, r-1 {
		backward[l], backward[r] = backward[r], backward[l]
	}
	return strings.Join(backward, ".")
}

// calculateRequiredTokenWidth returns the token panel width needed to show
// the longest current model name in the Models column without truncation.
// It mirrors the column math in renderTokenPanel exactly (via the shared
// tokenLeftColWidth/tokenColSeparatorWidth constants and modelSuffixWidth)
// so the two can't drift apart the way the old hand-maintained "60" ideal
// width did once model IDs grew past what it assumed.
func (d *Dashboard) calculateRequiredTokenWidth() int {
	maxNameLen := 10 // default floor, matches renderTokenPanel's minimum

	if d.tokenMetrics != nil {
		for _, usage := range nonzeroModelUsages(d.tokenMetrics.ModelUsages) {
			nameLen := len(shortenModelName(usage.Model))
			if usage.PricingEstimated {
				nameLen++
			}
			if nameLen > maxNameLen {
				maxNameLen = nameLen
			}
		}
	}

	rightWidth := maxNameLen
	if d.tokenMetrics != nil {
		rightWidth += modelSuffixWidth(nonzeroModelUsages(d.tokenMetrics.ModelUsages))
	}
	// Never ask for less than the side-by-side layout needs: renderUltraWide
	// caps the panel at this width, and below the minimum the models list
	// stacks under the stats.
	return max(tokenLeftColWidth+tokenColSeparatorWidth+rightWidth+tokenPanelBorderPadding, tokenSideBySideMinWidth)
}

// modelSuffixWidth is the width of the widest " $cost (tokens)" suffix in a
// models list, capped at tokenRightColReserve (the worst case the side-by-side
// minimum width is built on).
func modelSuffixWidth(usages []metrics.ModelUsage) int {
	widest := 0
	for _, usage := range usages {
		w := lipgloss.Width(" " + metrics.FormatCost(usage.Cost) + " (" + metrics.FormatTokensCompact(usage.TotalTokens) + ")")
		widest = max(widest, w)
	}
	return min(widest, tokenRightColReserve)
}

func isModelVersion(value string) bool {
	if value == "" {
		return false
	}
	hasDigit := false
	for _, char := range value {
		if char >= '0' && char <= '9' {
			hasDigit = true
			continue
		}
		if char != '.' && char != '_' {
			return false
		}
	}
	return hasDigit
}

// renderSparkline creates a compact sparkline using unicode block characters
// Takes a slice of token values and renders them as a vertical sparkline
// Uses 8 levels: ▁ ▂ ▃ ▄ ▅ ▆ ▇ █
func (d *Dashboard) renderSparkline(values []int64) string {
	if len(values) == 0 {
		return "░"
	}

	// Find min and max for scaling
	minVal := int64(0)
	maxVal := int64(0)
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// If all values are the same, return a flat line
	if maxVal == minVal {
		if maxVal == 0 {
			return "░" // Empty-ish
		}
		return strings.Repeat("▄", len(values)) // Mid-level flat
	}

	// Scale values to 0-7 range and map to unicode block chars
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var result strings.Builder

	for _, v := range values {
		// Scale to 0-7 range
		normalized := float64(v-minVal) / float64(maxVal-minVal)
		index := int(normalized * 7.99) // 0-7 inclusive
		if index < 0 {
			index = 0
		}
		if index > 7 {
			index = 7
		}
		result.WriteRune(blocks[index])
	}

	return result.String()
}
