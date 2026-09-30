package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jedarden/ccdash/internal/metrics"
)

func TestRenderTmuxPanelGroupsInteractiveBeforeWorkers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workerLogDir := filepath.Join(home, ".beads-workers")
	if err := os.Mkdir(workerLogDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workerLogDir, "custom-worker.log"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	d := &Dashboard{tmuxMetrics: &metrics.TmuxMetrics{
		Available: true,
		Total:     5,
		Source:    "tmux",
		Sessions: []metrics.TmuxSession{
			{Name: "alpha", Status: metrics.StatusReady, Attached: true},
			{Name: "claude-code-glm-47-bravo", Status: metrics.StatusWorking},
			{Name: "custom-worker", Status: metrics.StatusReady, IdleDuration: 2 * time.Minute},
			{Name: "opencode-glm-47-charlie", Status: metrics.StatusActive},
			{Name: "delta", Harness: "claude", Status: metrics.StatusActive, Attached: true},
		},
	}}

	view := d.renderTmuxPanel(72, 11)
	interactiveHeader := strings.Index(view, "Interactive (2)")
	alpha := strings.Index(view, "💻 alpha")
	delta := strings.Index(view, "💻 delta")
	workersHeader := strings.Index(view, "Workers (3)")
	abbreviatedWorker := strings.Index(view, "🤖 c-glm-bravo")
	fallbackWorker := strings.Index(view, "🤖 custom-worker")
	secondAbbreviatedWorker := strings.Index(view, "🤖 o-glm-charlie")
	if interactiveHeader < 0 || alpha < 0 || delta < 0 || workersHeader < 0 || abbreviatedWorker < 0 || fallbackWorker < 0 || secondAbbreviatedWorker < 0 {
		t.Fatalf("grouped compact panel is missing expected entries:\n%s", view)
	}
	if !(interactiveHeader < alpha && alpha < delta && delta < workersHeader && workersHeader < abbreviatedWorker && abbreviatedWorker < fallbackWorker && fallbackWorker < secondAbbreviatedWorker) {
		t.Fatalf("sessions are not grouped with interactive first:\n%s", view)
	}
	if strings.Contains(view, "claude-code-glm-47-bravo") {
		t.Fatalf("worker name was not abbreviated:\n%s", view)
	}
	if got := lipgloss.Height(view); got != 13 {
		t.Fatalf("compact panel height = %d, want 13 rows for an 11-row panel plus borders:\n%s", got, view)
	}
}

func TestRenderSessionCellFitsCompactWidth(t *testing.T) {
	d := &Dashboard{}
	cell := d.renderSessionCell(metrics.TmuxSession{
		Name:         "claude-code-glm-4_7-alpha",
		SessionType:  metrics.SessionTypeWorker,
		Status:       metrics.StatusWorking,
		IdleDuration: 30 * time.Second,
		Attached:     true,
	}, 28)
	if got := lipgloss.Width(cell); got > 28 {
		t.Fatalf("compact cell width = %d, want <= 28: %q", got, cell)
	}
	if !strings.Contains(cell, "🤖 c-glm-alp…") {
		t.Fatalf("compact cell should abbreviate the worker name: %q", cell)
	}
}

func TestSessionVisibilityAccountsForSectionsAndOverflow(t *testing.T) {
	tests := []struct {
		name                         string
		interactive, workers         int
		rowBudget, width             int
		wantInteractive, wantWorkers int
		wantColumns                  int
		wantMore                     bool
	}{
		{name: "documented five sessions", interactive: 2, workers: 3, rowBudget: 8, width: 69, wantInteractive: 2, wantWorkers: 3, wantColumns: 1},
		{name: "overflow packs grouped rows into columns", interactive: 12, workers: 10, rowBudget: 8, width: 69, wantInteractive: 8, wantWorkers: 2, wantColumns: 2, wantMore: true},
		{name: "workers only", workers: 3, rowBudget: 4, width: 69, wantWorkers: 3, wantColumns: 1},
		{name: "tight height keeps interactive first", interactive: 2, workers: 3, rowBudget: 3, width: 69, wantInteractive: 2, wantColumns: 2, wantMore: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotInteractive, gotWorkers, gotColumns, gotMore := sessionLayout(tt.interactive, tt.workers, tt.rowBudget, tt.width)
			if gotInteractive != tt.wantInteractive || gotWorkers != tt.wantWorkers || gotColumns != tt.wantColumns || gotMore != tt.wantMore {
				t.Fatalf("sessionLayout(%d, %d, %d, %d) = (%d, %d, %d, %t), want (%d, %d, %d, %t)",
					tt.interactive, tt.workers, tt.rowBudget, tt.width,
					gotInteractive, gotWorkers, gotColumns, gotMore,
					tt.wantInteractive, tt.wantWorkers, tt.wantColumns, tt.wantMore)
			}
		})
	}
}
