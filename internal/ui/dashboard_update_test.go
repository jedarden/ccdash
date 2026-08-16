package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedarden/ccdash/internal/updater"
)

func TestUpdateNoticeCanBeDismissed(t *testing.T) {
	d := &Dashboard{
		version:    "v1.0.3",
		width:      200,
		height:     24,
		layoutMode: LayoutUltraWide,
		lastUpdate: time.Now(),
		updateInfo: &updater.UpdateInfo{
			LatestVersion:   "1.0.4",
			UpdateAvailable: true,
		},
	}

	if !d.updateNoticeVisible() {
		t.Fatal("update notice should be visible before dismissal")
	}
	if got := d.renderStatusBar(); !strings.Contains(got, "esc to dismiss") {
		t.Fatalf("status bar should advertise dismissal, got %q", got)
	}

	model, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if model != d {
		t.Fatal("dismissal should keep the existing dashboard model")
	}
	if cmd != nil {
		t.Fatal("dismissal should not schedule a command")
	}
	if d.updateNoticeVisible() {
		t.Fatal("update notice should be hidden after dismissal")
	}
	if !d.updateInfo.UpdateAvailable {
		t.Fatal("dismissing the notice must preserve update availability")
	}
	if strings.Contains(d.renderStatusBar(), "available") {
		t.Fatal("dismissed status bar should not contain the update notice")
	}
}
