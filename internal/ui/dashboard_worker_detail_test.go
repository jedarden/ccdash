package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedarden/ccdash/internal/metrics"
)

func TestWorkerDetailKeyOpensAndReturnsToDashboard(t *testing.T) {
	d := &Dashboard{tmuxMetrics: &metrics.TmuxMetrics{Sessions: []metrics.TmuxSession{{
		Name:        "claude-code-worker-alpha",
		SessionType: metrics.SessionTypeWorker,
	}}}}

	model, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if !model.(*Dashboard).workerDetailMode {
		t.Fatal("pressing w did not open worker details")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if model.(*Dashboard).workerDetailMode {
		t.Fatal("pressing q in worker details did not return to the dashboard")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if model.(*Dashboard).workerDetailMode {
		t.Fatal("pressing Esc in worker details did not return to the dashboard")
	}
}

func TestWorkerDetailNavigationSelectsWorkers(t *testing.T) {
	d := &Dashboard{workerDetailMode: true, tmuxMetrics: &metrics.TmuxMetrics{Sessions: []metrics.TmuxSession{
		{Name: "worker-one", SessionType: metrics.SessionTypeWorker},
		{Name: "worker-two", SessionType: metrics.SessionTypeWorker},
	}}}
	key := func(value string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)} }

	d.Update(key("j"))
	if d.workerDetailOffset != 1 {
		t.Fatalf("worker detail offset after j = %d, want 1", d.workerDetailOffset)
	}
	d.Update(key("j"))
	if d.workerDetailOffset != 1 {
		t.Fatalf("worker detail offset passed the final worker: %d", d.workerDetailOffset)
	}
	d.Update(key("k"))
	if d.workerDetailOffset != 0 {
		t.Fatalf("worker detail offset after k = %d, want 0", d.workerDetailOffset)
	}
}

func TestWorkerDetailShowsFullWorkerContext(t *testing.T) {
	name := "claude-code-glm-4.7-lab-roam-very-long-worker-session"
	workspace := "/home/coding/projects/very-long/nested/workspace/name"
	d := &Dashboard{
		width:            72,
		height:           20,
		layoutMode:       LayoutCompact,
		workerDetailMode: true,
		tmuxMetrics: &metrics.TmuxMetrics{Sessions: []metrics.TmuxSession{{
			Name:         "short-display-name",
			Source:       "needle",
			SessionType:  metrics.SessionTypeWorker,
			Status:       metrics.StatusWorking,
			IdleDuration: 2 * time.Minute,
			Created:      time.Now().Add(-10 * time.Minute),
			Worker: &metrics.WorkerMetadata{
				FullName:            name,
				Workspace:           workspace,
				Agent:               "claude-code",
				Provider:            "anthropic",
				Model:               "sonnet",
				CurrentBead:         "ccdash-2897aab6",
				State:               "EXECUTING",
				BeadStatusAvailable: true,
				BeadsProcessed:      3,
				BeadsCompleted:      2,
			},
		}}},
	}

	view := d.renderWorkerDetailView()
	flattened := strings.ReplaceAll(view, "\n", "")
	for _, want := range []string{
		name,
		workspace,
		"Status: WORKING",
		"Executor: claude-code / anthropic / sonnet",
		"Current bead: ccdash-2897aab6 · worker state: EXECUTING",
		"Cycles: 2 completed / 3 processed",
	} {
		if !strings.Contains(flattened, want) {
			t.Errorf("worker detail view %q does not contain %q", view, want)
		}
	}
}

func TestWorkerDetailWrapsLongFieldsWithoutTruncating(t *testing.T) {
	name := "claude-code-glm-4.7-lab-roam-very-long-worker-session"
	workspace := "/home/coding/projects/very-long/nested/workspace/name"
	lines := workerDetailLines(metrics.TmuxSession{
		Name:        name,
		SessionType: metrics.SessionTypeWorker,
		Worker:      &metrics.WorkerMetadata{Workspace: workspace},
	}, 24)
	joined := strings.Join(lines, "")
	if !strings.Contains(joined, name) || !strings.Contains(joined, workspace) {
		t.Fatalf("wrapped detail lines lost part of the name or path: %q", lines)
	}
	for _, line := range lines {
		if got := len([]rune(line)); got > 24 {
			t.Errorf("detail line has %d runes, want at most 24: %q", got, line)
		}
	}
}
