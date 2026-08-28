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

func TestUKeyChecksForUpdateWhenNoneKnown(t *testing.T) {
	d := &Dashboard{
		version:    "v1.0.3",
		width:      200,
		height:     24,
		layoutMode: LayoutUltraWide,
		lastUpdate: time.Now(),
		updater:    updater.NewUpdater("1.0.3"),
	}

	model, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if model != d {
		t.Fatal("pressing u should keep the existing dashboard model")
	}
	if cmd == nil {
		t.Fatal("pressing u with no known update should schedule a recheck command")
	}
	if !d.checkingUpdate {
		t.Fatal("pressing u with no known update should enter the checking state")
	}
	if d.updating {
		t.Fatal("checking for an update must not be confused with installing one")
	}
	if got := d.renderStatusBar(); !strings.Contains(got, "Checking for updates") {
		t.Fatalf("status bar should show the checking state, got %q", got)
	}

	// Pressing u again while a check is already in flight must not fire a
	// second one.
	if _, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}); cmd != nil {
		t.Fatal("pressing u while already checking should be a no-op")
	}
}

func TestUKeyInstallsAnAlreadyKnownUpdate(t *testing.T) {
	d := &Dashboard{
		version:    "v1.0.3",
		width:      200,
		height:     24,
		layoutMode: LayoutUltraWide,
		lastUpdate: time.Now(),
		updater:    updater.NewUpdater("1.0.3"),
		updateInfo: &updater.UpdateInfo{
			LatestVersion:   "1.0.4",
			UpdateAvailable: true,
		},
	}

	if _, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}); cmd == nil {
		t.Fatal("pressing u with a known update should schedule the install command")
	}
	if !d.updating {
		t.Fatal("pressing u with a known update should start installing it, not rechecking")
	}
	if d.checkingUpdate {
		t.Fatal("installing an update must not also enter the checking state")
	}
}

func TestUpdateCheckMsgReportsUpToDateOnlyForManualChecks(t *testing.T) {
	d := &Dashboard{version: "v1.0.3", updater: updater.NewUpdater("1.0.3")}

	// A background (non-manual) check result should not produce user-facing
	// status text - only the once-at-startup silent check does this.
	d.Update(updateCheckMsg{info: &updater.UpdateInfo{CurrentVersion: "1.0.3"}})
	if d.updateStatus != "" {
		t.Fatalf("background check should not set status text, got %q", d.updateStatus)
	}

	// A manual (u-triggered) check that finds nothing new reports "up to date".
	d.checkingUpdate = true
	d.Update(updateCheckMsg{info: &updater.UpdateInfo{CurrentVersion: "1.0.3"}})
	if d.checkingUpdate {
		t.Fatal("receiving the check result should clear the checking state")
	}
	if d.updateStatus != "Up to date (v1.0.3)" {
		t.Fatalf("expected an up-to-date message, got %q", d.updateStatus)
	}
	if d.updateStatusIsError {
		t.Fatal("up-to-date is not an error")
	}

	// A manual check that errors reports the failure, distinctly from "up to date".
	d.checkingUpdate = true
	d.Update(updateCheckMsg{info: &updater.UpdateInfo{CurrentVersion: "1.0.3", Error: "network unreachable"}})
	if !d.updateStatusIsError {
		t.Fatal("a failed check must be flagged as an error")
	}
	if !strings.Contains(d.updateStatus, "network unreachable") {
		t.Fatalf("expected the error to be surfaced, got %q", d.updateStatus)
	}

	// A manual check that finds a real update clears any stale status text so
	// the "available" notice can render instead.
	d.updateStatus = "Up to date (v1.0.3)"
	d.checkingUpdate = true
	d.Update(updateCheckMsg{info: &updater.UpdateInfo{CurrentVersion: "1.0.3", LatestVersion: "1.0.4", UpdateAvailable: true}})
	if d.updateStatus != "" {
		t.Fatalf("finding an update should clear stale status text, got %q", d.updateStatus)
	}
}
