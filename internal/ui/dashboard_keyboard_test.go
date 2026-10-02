package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedarden/ccdash/internal/updater"
)

func TestDocumentedDashboardKeysHaveHandlers(t *testing.T) {
	shortcuts := DashboardKeyboardShortcuts()
	if len(shortcuts) != len(dashboardKeyBindings) {
		t.Fatalf("got %d documented shortcuts for %d bindings", len(shortcuts), len(dashboardKeyBindings))
	}

	seen := make(map[string]bool)
	for i, binding := range dashboardKeyBindings {
		if binding.handle == nil {
			t.Errorf("shortcut %q has no handler", binding.Keys)
			continue
		}
		if shortcuts[i] != binding.KeyboardShortcut {
			t.Errorf("documented shortcut %d = %+v, want registered binding %+v", i, shortcuts[i], binding.KeyboardShortcut)
		}
		if len(binding.keys) == 0 {
			t.Errorf("shortcut %q has no input keys", binding.Keys)
			continue
		}

		for _, key := range binding.keys {
			if seen[key] {
				t.Errorf("key %q is registered more than once", key)
			}
			seen[key] = true

			d := &Dashboard{updateInfo: &updater.UpdateInfo{UpdateAvailable: true}}
			_, cmd := d.Update(keyMessage(key))
			if cmd == nil && d.helpMode == 0 && !d.workerDetailMode && !d.lookbackMode && !d.updateDismissed && !d.checkingUpdate && !d.updating {
				t.Errorf("documented key %q did not reach a dashboard handler", key)
			}
		}
	}
}

func keyMessage(key string) tea.KeyMsg {
	switch key {
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}
