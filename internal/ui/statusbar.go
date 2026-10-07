package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderStatusBar renders the status bar at the bottom of the view.
//
// Wide/ultrawide (≥120 cols): single line —
//
//	time+version  |  repo link or update notice  |  dimensions+shortcuts
//
// Compact (<120 cols): two lines —
//
//	Line 1: repo link (always) or update notice
//	Line 2: time+version on the left, dimensions+shortcuts on the right
func (d *Dashboard) renderStatusBar() string {
	left := fmt.Sprintf("%s %s", d.lastUpdate.Format("15:04:05"), d.version)

	shortcuts := "u:check-update l:lookback h:help w:workers q:quit r:refresh"
	if d.workerDetailMode {
		shortcuts = "j/k or ↑/↓:next w/q/Esc:back r:refresh"
	} else if d.updateNoticeVisible() && !d.updating {
		shortcuts = "l:lookback h:help w:workers q:quit r:refresh"
	}
	right := fmt.Sprintf("%dx%d %s", d.width, d.height, shortcuts)

	// Build the repo/update middle string
	var middle string
	if d.updating {
		middle = warningStyle.Render(d.updateStatus)
	} else if d.checkingUpdate {
		middle = warningStyle.Render("Checking for updates...")
	} else if d.updateStatus != "" && d.updateStatusIsError {
		middle = errorStyle.Render(d.updateStatus)
	} else if d.updateStatus != "" {
		middle = successStyle.Render(d.updateStatus)
	} else if d.updateNoticeVisible() {
		middle = successStyle.Render(fmt.Sprintf("⬆ %s available! Press u to update, esc to dismiss", d.updateInfo.LatestVersion))
	} else {
		middle = dimStyle.Render("https://github.com/jedarden/ccdash")
	}

	if d.layoutMode != LayoutCompact {
		// Wide / ultrawide: single line with repo link centred between left and right
		totalContent := lipgloss.Width(left) + lipgloss.Width(middle) + lipgloss.Width(right)
		availableSpace := d.width - totalContent - 2 // -2 for statusBarStyle padding
		if availableSpace < 4 {
			// Not enough room — drop the middle
			compactShortcuts := "l h w q r"
			if d.workerDetailMode {
				compactShortcuts = "j/k w/q/esc r"
			} else if d.updateNoticeVisible() {
				compactShortcuts = "u esc l h w q r"
			}
			return statusBarStyle.Render(fmt.Sprintf("%s %s %dx%d %s",
				d.lastUpdate.Format("15:04"), d.version, d.width, d.height, compactShortcuts))
		}
		leftSpacer := strings.Repeat(" ", availableSpace/2)
		rightSpacer := strings.Repeat(" ", availableSpace-availableSpace/2)
		return statusBarStyle.Render(left + leftSpacer + middle + rightSpacer + right)
	}

	// Compact: two lines so the repo link is always visible alongside the shortcuts
	topWidth := lipgloss.Width(middle)
	topPad := max(0, (d.width-topWidth)/2)
	topLine := strings.Repeat(" ", topPad) + middle

	totalContent := lipgloss.Width(left) + lipgloss.Width(right)
	availableSpace := d.width - totalContent - 2
	var statusLine string
	if availableSpace < 2 {
		compactShortcuts := "h w q r"
		if d.workerDetailMode {
			compactShortcuts = "j/k w/q/esc r"
		} else if d.updateNoticeVisible() {
			compactShortcuts = "h w q r"
		}
		statusLine = fmt.Sprintf("%s %s %dx%d %s",
			d.lastUpdate.Format("15:04"), d.version, d.width, d.height, compactShortcuts)
	} else {
		statusLine = left + strings.Repeat(" ", availableSpace) + right
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		topLine,
		statusBarStyle.Render(statusLine),
	)
}
