package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
)

// renderSystemPanel renders the system resources panel
func (d *Dashboard) renderSystemPanel(width, height int) string {
	style := panelStyle

	var lines []string

	// Title (with emoji like unified-dashboard)
	lines = append(lines, successStyle.Render("⚡ System Resources"))

	// Load average
	if d.systemMetrics.Load.Error == nil {
		lines = append(lines, fmt.Sprintf("Load: %.2f %.2f %.2f",
			d.systemMetrics.Load.Load1,
			d.systemMetrics.Load.Load5,
			d.systemMetrics.Load.Load15))
	} else {
		lines = append(lines, errorStyle.Render("Load: N/A"))
	}

	// Calculate content width (panel width minus borders and padding)
	contentWidth := width - 4 // -2 for borders, -2 for padding

	// CPU Total - use same calculation method as Mem/Swap for consistent bar width
	if d.systemMetrics.CPU.Error == nil {
		// Format: "CPU [|||||| XX.X%]" - "CPU " is 4 chars
		cpuBarWidth := contentWidth - 4 // Subtract "CPU " prefix
		if cpuBarWidth < 10 {
			cpuBarWidth = 10
		}
		lines = append(lines, fmt.Sprintf("CPU %s", d.renderBar(d.systemMetrics.CPU.TotalPercent, cpuBarWidth)))

		// CPU per-core - use up to 6 lines for CPU display
		maxCoreLines := 6
		totalCores := len(d.systemMetrics.CPU.PerCore)

		// Determine label width based on total cores (for alignment)
		labelWidth := 1
		if totalCores >= 100 {
			labelWidth = 3
		} else if totalCores >= 10 {
			labelWidth = 2
		}

		// Determine cores per line
		var coresPerLine int
		if totalCores <= 6 {
			coresPerLine = 1 // One core per line - bars stretch full width
		} else {
			// Multiple cores per line - calculate how many fit
			// Each core needs: labelWidth + ":[" + barContent + "]" + space
			// Minimum reasonable bar content is about 12 chars
			minCharsPerCore := labelWidth + 3 + 12 // label + ":[]" + min bar
			coresPerLine = contentWidth / minCharsPerCore
			if coresPerLine < 2 {
				coresPerLine = 2 // At least 2 per line when splitting
			}
		}

		// Calculate bar width for cores - align brackets by using fixed widths
		var barWidth int
		if coresPerLine == 1 {
			// Single core per line - match memory/swap calculation for consistency
			// Format: "NN:[||||... XXX%]"
			// labelWidth + ":[]" = labelWidth + 3 chars overhead
			barWidth = contentWidth - labelWidth - 3
			if barWidth < 10 {
				barWidth = 10
			}
		} else {
			// Multiple cores per line - split width evenly
			// Account for spaces between cores (1 space separator)
			spacesBetween := coresPerLine - 1
			widthPerCore := (contentWidth - spacesBetween) / coresPerLine
			// Subtract label overhead: labelWidth + ":[]"
			barWidth = widthPerCore - labelWidth - 3
			if barWidth < 8 {
				barWidth = 8
			}
		}

		// Max cores we can display with 6 lines
		maxDisplayCores := coresPerLine * maxCoreLines
		maxCores := totalCores
		if maxCores > maxDisplayCores {
			maxCores = maxDisplayCores
		}

		var coreLine strings.Builder
		linesUsed := 0
		for i := 0; i < maxCores; i++ {
			if i > 0 && i%coresPerLine == 0 {
				// Start new line
				lines = append(lines, coreLine.String())
				coreLine.Reset()
				linesUsed++
				if linesUsed >= maxCoreLines {
					break
				}
			}
			if coreLine.Len() > 0 {
				coreLine.WriteString(" ")
			}
			// Render progress bar for this core with calculated width
			// Use consistent label width for alignment - brackets align because
			// we use fixed-width labels and fixed-width bar content
			percent := d.systemMetrics.CPU.PerCore[i]
			miniBar := d.renderMiniBar(percent, barWidth)
			coreLine.WriteString(fmt.Sprintf("%*d:[%s]", labelWidth, i, miniBar))
		}
		// Add remaining cores on current line
		if coreLine.Len() > 0 {
			lines = append(lines, coreLine.String())
		}

		if totalCores > maxCores {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("+%d more cores", totalCores-maxCores)))
		}
	} else {
		lines = append(lines, errorStyle.Render("CPU: N/A"))
	}

	// Memory - always compact (one line)
	if d.systemMetrics.Memory.Error == nil {
		memUsed := metrics.FormatBytes(d.systemMetrics.Memory.Used)
		memTotal := metrics.FormatBytes(d.systemMetrics.Memory.Total)
		// Format: "Mem [||||...] XX.XX GB/XX.XX GB"
		// Calculate bar width: contentWidth - "Mem " (4) - " " (1) - "used/total" - margins
		barWidth := contentWidth - 5 - len(memUsed) - 1 - len(memTotal)
		if barWidth < 10 {
			barWidth = 10
		}
		lines = append(lines, fmt.Sprintf("Mem %s %s/%s",
			d.renderBar(d.systemMetrics.Memory.Percentage, barWidth),
			memUsed, memTotal))
	} else {
		lines = append(lines, errorStyle.Render("Mem: N/A"))
	}

	// Swap - always compact (one line)
	if d.systemMetrics.Swap.Error == nil && d.systemMetrics.Swap.Total > 0 {
		swpUsed := metrics.FormatBytes(d.systemMetrics.Swap.Used)
		swpTotal := metrics.FormatBytes(d.systemMetrics.Swap.Total)
		// Use same calculation as Memory for consistency
		barWidth := contentWidth - 5 - len(swpUsed) - 1 - len(swpTotal)
		if barWidth < 10 {
			barWidth = 10
		}
		lines = append(lines, fmt.Sprintf("Swp %s %s/%s",
			d.renderBar(d.systemMetrics.Swap.Percentage, barWidth),
			swpUsed, swpTotal))
	}

	// Disk Usage - always compact (one line)
	if d.systemMetrics.DiskUsage.Error == nil {
		diskUsed := metrics.FormatBytes(d.systemMetrics.DiskUsage.Used)
		diskTotal := metrics.FormatBytes(d.systemMetrics.DiskUsage.Total)
		// Use same calculation as Memory for consistency
		barWidth := contentWidth - 5 - len(diskUsed) - 1 - len(diskTotal)
		if barWidth < 10 {
			barWidth = 10
		}
		lines = append(lines, fmt.Sprintf("Dsk %s %s/%s",
			d.renderBar(d.systemMetrics.DiskUsage.Percentage, barWidth),
			diskUsed, diskTotal))
	} else {
		lines = append(lines, errorStyle.Render("Dsk: N/A"))
	}

	// Disk I/O - verbose format with pipe separators
	if d.systemMetrics.DiskIO.Error == nil {
		lines = append(lines, fmt.Sprintf("Disk I/O | Read: %s | Write: %s",
			metrics.FormatRate(d.systemMetrics.DiskIO.ReadBytesPerSec),
			metrics.FormatRate(d.systemMetrics.DiskIO.WriteBytesPerSec)))
	} else {
		lines = append(lines, errorStyle.Render("Disk I/O | N/A"))
	}

	// Net I/O - verbose format with pipe separators
	if d.systemMetrics.NetIO.Error == nil {
		lines = append(lines, fmt.Sprintf("Net I/O  | Recv: %s | Sent: %s",
			metrics.FormatRate(d.systemMetrics.NetIO.RecvBytesPerSec),
			metrics.FormatRate(d.systemMetrics.NetIO.SentBytesPerSec)))
	} else {
		lines = append(lines, errorStyle.Render("Net I/O  | N/A"))
	}

	content := strings.Join(lines, "\n")
	return style.Width(width).Height(height).Render(content)
}

// renderBar renders a progress bar with percentage inside (unified-dashboard style)
func (d *Dashboard) renderBar(percent float64, width int) string {
	if width < 10 {
		return ""
	}

	// Determine color based on threshold (unified-dashboard style)
	var color string
	if percent >= 95 {
		color = "#ff0000" // Red
	} else if percent >= 80 {
		color = "#ffaa00" // Orange
	} else if percent >= 60 {
		color = "#ffff00" // Yellow
	} else {
		color = "#00ff00" // Green
	}

	// Format percentage
	percentText := fmt.Sprintf("%.1f%%", percent)

	// Calculate fill width (accounting for percentage text)
	barWidth := width - 2                             // Account for brackets
	availableWidth := barWidth - len(percentText) - 1 // -1 for space before %
	fillWidth := int(percent / 100.0 * float64(availableWidth))
	if fillWidth > availableWidth {
		fillWidth = availableWidth
	}
	if fillWidth < 0 {
		fillWidth = 0
	}

	// Create filled and empty portions (vertical bar style like unified-dashboard)
	filled := strings.Repeat("|", fillWidth)
	empty := strings.Repeat(" ", availableWidth-fillWidth)

	// Apply styling
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	var bar strings.Builder
	bar.WriteString("[")
	bar.WriteString(barStyle.Render(filled))
	bar.WriteString(dimStyle.Render(empty))
	bar.WriteString(" ")
	bar.WriteString(percentText)
	bar.WriteString("]")

	return bar.String()
}

// renderMiniBar creates a compact progress bar with percentage inside
// Format: "||| 42%" for use in CPU core display
// Returns a fixed-width string to ensure bracket alignment
func (d *Dashboard) renderMiniBar(percent float64, barWidth int) string {
	// Determine color based on threshold
	var color string
	if percent >= 95 {
		color = "#ff0000" // Red
	} else if percent >= 80 {
		color = "#ffaa00" // Orange
	} else if percent >= 60 {
		color = "#ffff00" // Yellow
	} else {
		color = "#00ff00" // Green
	}

	// Format percentage with fixed width (4 chars: "XXX%" or " XX%")
	percentText := fmt.Sprintf("%3.0f%%", percent)

	// Reserve space for percentage text (4 chars) + 1 space
	percentSpace := 5
	barAvailableWidth := barWidth - percentSpace
	if barAvailableWidth < 1 {
		barAvailableWidth = 1
	}

	// Calculate fill width
	fillWidth := int(percent / 100.0 * float64(barAvailableWidth))
	if fillWidth > barAvailableWidth {
		fillWidth = barAvailableWidth
	}
	if fillWidth < 0 {
		fillWidth = 0
	}

	// Create filled and empty portions
	filled := strings.Repeat("|", fillWidth)
	empty := strings.Repeat(" ", barAvailableWidth-fillWidth)

	// Apply styling
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	return barStyle.Render(filled) + dimStyle.Render(empty) + " " + percentText
}
