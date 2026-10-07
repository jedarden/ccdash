package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func TestRenderTmuxPanelHighlightsSessionsNeedingYou(t *testing.T) {
	d := &Dashboard{tmuxMetrics: &metrics.TmuxMetrics{
		Available: true,
		Total:     3,
		Sessions: []metrics.TmuxSession{
			{Name: "ready", Status: metrics.StatusReady},
			{Name: "asking-one", Status: metrics.StatusAsking},
			{Name: "asking-two", Status: metrics.StatusAsking},
		},
	}}

	view := d.renderTmuxPanel(100, 12)
	if !strings.Contains(view, "needs you (2)") {
		t.Fatalf("sessions header should show human-attention count:\n%s", view)
	}
	if !strings.Contains(view, "🟣") || !strings.Contains(view, "⚪") {
		t.Fatalf("sessions panel should use ASKING purple and READY neutral indicators:\n%s", view)
	}
}

func TestGroupSessionsDetectsWorkersAndHonorsClassificationMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workerLogDir := filepath.Join(home, ".beads-workers")
	if err := os.Mkdir(workerLogDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workerLogDir, "custom-log-worker.log"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	sessions := []metrics.TmuxSession{
		{Name: "claude-code-glm-47-alpha"},
		{Name: "opencode-glm-47-bravo"},
		{Name: "custom-log-worker"},
		{
			Name:   "custom-needle-worker",
			Source: "needle",
			Worker: &metrics.WorkerMetadata{FullName: "claude-code-glm-47-custom-needle-worker"},
		},
		{Name: "interactive-claude-code-sonnet", Source: "tmux"},
		{Name: "claude-code-sonnet-explicitly-interactive", SessionType: metrics.SessionTypeInteractive},
		{Name: "ordinary-session-explicitly-worker", SessionType: metrics.SessionTypeWorker},
		{Name: "alpha"},
	}

	workers, interactive := (&Dashboard{}).groupSessions(sessions)
	assertNames := func(group string, got []metrics.TmuxSession, want []string, wantType metrics.SessionType) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s group has %d sessions, want %d: %+v", group, len(got), len(want), got)
		}
		for i, name := range want {
			if got[i].Name != name || got[i].SessionType != wantType {
				t.Errorf("%s[%d] = (%q, %q), want (%q, %q)", group, i, got[i].Name, got[i].SessionType, name, wantType)
			}
		}
	}

	assertNames("workers", workers, []string{
		"claude-code-glm-47-alpha",
		"opencode-glm-47-bravo",
		"custom-log-worker",
		"custom-needle-worker",
		"ordinary-session-explicitly-worker",
	}, metrics.SessionTypeWorker)
	assertNames("interactive", interactive, []string{
		"interactive-claude-code-sonnet",
		"claude-code-sonnet-explicitly-interactive",
		"alpha",
	}, metrics.SessionTypeInteractive)
	if workers[3].Worker == nil || workers[3].Worker.FullName != "claude-code-glm-47-custom-needle-worker" {
		t.Fatalf("NEEDLE metadata was not retained in the worker group: %+v", workers[3])
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
		t.Fatalf("compact cell should truncate the worker name only as needed for status: %q", cell)
	}
}

func TestRenderSessionRowsPrioritizesWorkerNamesOverMetadata(t *testing.T) {
	names := []string{
		"glm-roam-01", "glm-roam-02", "glm-roam-03", "glm-roam-04",
		"glm-roam-05", "glm-roam-06", "glm-roam-07", "glm-roam-08",
	}
	sessions := make([]metrics.TmuxSession, 0, len(names))
	for _, name := range names {
		sessions = append(sessions, metrics.TmuxSession{
			Name:        name,
			SessionType: metrics.SessionTypeWorker,
			Status:      metrics.StatusWorking,
			Worker: &metrics.WorkerMetadata{
				Workspace:           "/home/coding/workspaces/very/long/nested/project/name",
				Agent:               "claude-code",
				Provider:            "anthropic",
				Model:               "glm-4.7",
				CurrentBead:         "ccdash-f0f62bea",
				State:               "EXECUTING",
				BeadStatusAvailable: true,
			},
		})
	}

	for _, width := range []int{28, 44, 72} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			rows := (&Dashboard{}).renderSessionRows(sessions, 1, width)
			if len(rows) != len(names) {
				t.Fatalf("rendered %d worker rows, want %d", len(rows), len(names))
			}
			for i, row := range rows {
				if got := lipgloss.Width(row); got > width {
					t.Errorf("row %d width = %d, want <= %d: %q", i, got, width, row)
				}
				if !strings.Contains(row, names[i]) {
					t.Errorf("row %d lost worker name %q to metadata: %q", i, names[i], row)
				}
			}
		})
	}
}

func TestRenderSessionCellTruncatesLongInteractiveNameOnOneLine(t *testing.T) {
	d := &Dashboard{}
	cell := d.renderSessionCell(metrics.TmuxSession{
		Name:        "interactive-session-with-a-name-that-does-not-fit",
		SessionType: metrics.SessionTypeInteractive,
		Status:      metrics.StatusActive,
	}, 28)
	if got := lipgloss.Width(cell); got > 28 {
		t.Fatalf("truncated cell width = %d, want <= 28: %q", got, cell)
	}
	if strings.Contains(cell, "\n") {
		t.Fatalf("compact session cell wrapped onto multiple lines: %q", cell)
	}
	if !strings.Contains(cell, "…") {
		t.Fatalf("long interactive name should be truncated with an ellipsis: %q", cell)
	}
}

func TestRenderTmuxPanelOmitsEmptySections(t *testing.T) {
	tests := []struct {
		name     string
		sessions []metrics.TmuxSession
		want     string
		omit     []string
	}{
		{
			name: "interactive only",
			sessions: []metrics.TmuxSession{{
				Name: "alpha", SessionType: metrics.SessionTypeInteractive, Status: metrics.StatusActive,
			}},
			want: "Interactive (1)", omit: []string{"Workers (0)"},
		},
		{
			name: "workers only",
			sessions: []metrics.TmuxSession{{
				Name: "worker-alpha", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusWorking,
			}},
			want: "Workers (1)", omit: []string{"Interactive (0)"},
		},
		{
			name: "no sessions",
			omit: []string{"Interactive (0)", "Workers (0)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Dashboard{tmuxMetrics: &metrics.TmuxMetrics{
				Available: true,
				Total:     len(tt.sessions),
				Sessions:  tt.sessions,
			}}
			view := d.renderTmuxPanel(72, 11)
			if tt.want != "" && !strings.Contains(view, tt.want) {
				t.Fatalf("panel is missing %q:\n%s", tt.want, view)
			}
			for _, omit := range tt.omit {
				if strings.Contains(view, omit) {
					t.Fatalf("panel should omit empty section %q:\n%s", omit, view)
				}
			}
			if tt.name == "no sessions" && !strings.Contains(view, "No active sessions") {
				t.Fatalf("empty panel should report no active sessions:\n%s", view)
			}
		})
	}
}

func TestTmuxHelpExplainsTrackedSessionAndProcessCounts(t *testing.T) {
	d := &Dashboard{
		width:    200,
		height:   30,
		helpMode: 3,
		tmuxMetrics: &metrics.TmuxMetrics{
			Available:        true,
			Total:            19,
			RunningProcesses: 37,
		},
	}
	view := d.renderHelpView()
	for _, want := range []string{"N tracked", "M detected agent processes"} {
		if !strings.Contains(view, want) {
			t.Errorf("sessions help should explain process counts with %q:\n%s", want, view)
		}
	}
}

func TestRenderTmuxPanelFitsDocumentedLayouts(t *testing.T) {
	sessions := []metrics.TmuxSession{
		{Name: "alpha", SessionType: metrics.SessionTypeInteractive, Status: metrics.StatusActive},
		{Name: "delta", SessionType: metrics.SessionTypeInteractive, Status: metrics.StatusReady},
		{Name: "worker-alpha", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusWorking},
		{Name: "worker-bravo", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusReady},
		{Name: "worker-charlie", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusActive},
	}
	tests := []struct {
		name          string
		width, height int
		wantVisible   []string
		wantOverflow  bool
	}{
		// README.md documents stacked narrow panels, two-panel wide layouts,
		// and three-panel ultra-wide layouts. These are the corresponding
		// sessions-panel sizes for representative 80x24, 160x40, and 240x30
		// terminals. The final case is the documented 199x14 tmux panel.
		{
			// Eight interior rows hold the title, both section headers and
			// all five sessions; the borders are outside the height.
			name: "narrow 80x24", width: 78, height: 8,
			wantVisible: []string{"💻 alpha", "💻 delta", "🤖 worker-alpha", "🤖 worker-bravo", "🤖 worker-charlie"},
		},
		{
			name: "tight 78x6", width: 78, height: 6,
			wantVisible:  []string{"💻 alpha"},
			wantOverflow: true,
		},
		{
			name: "wide 160x40", width: 158, height: 18,
			wantVisible: []string{"💻 alpha", "💻 delta", "🤖 worker-alpha", "🤖 worker-bravo", "🤖 worker-charlie"},
		},
		{
			name: "ultra-wide 240x30", width: 120, height: 27,
			wantVisible: []string{"💻 alpha", "💻 delta", "🤖 worker-alpha", "🤖 worker-bravo", "🤖 worker-charlie"},
		},
		{
			name: "199x14", width: 72, height: 11,
			wantVisible: []string{"💻 alpha", "💻 delta", "🤖 worker-alpha", "🤖 worker-bravo", "🤖 worker-charlie"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Dashboard{tmuxMetrics: &metrics.TmuxMetrics{
				Available: true,
				Total:     len(sessions),
				Sessions:  sessions,
			}}
			view := d.renderTmuxPanel(tt.width, tt.height)
			if got := lipgloss.Height(view); got != tt.height+2 {
				t.Fatalf("panel height = %d, want %d including borders:\n%s", got, tt.height+2, view)
			}
			interactiveHeader := strings.Index(view, "Interactive (2)")
			workersHeader := strings.Index(view, "Workers (3)")
			if interactiveHeader < 0 || workersHeader < 0 || interactiveHeader >= workersHeader {
				t.Fatalf("panel should render nonempty sections with interactive first:\n%s", view)
			}
			for _, want := range tt.wantVisible {
				if !strings.Contains(view, want) {
					t.Errorf("panel is missing visible session %q:\n%s", want, view)
				}
			}
			if got := strings.Contains(view, "... +"); got != tt.wantOverflow {
				t.Errorf("overflow indicator present = %t, want %t:\n%s", got, tt.wantOverflow, view)
			}

			lines := strings.Split(view, "\n")
			for _, want := range tt.wantVisible {
				lineMatches := 0
				for _, line := range lines {
					if strings.Contains(line, want) {
						lineMatches++
					}
				}
				if lineMatches != 1 {
					t.Errorf("session %q should render on exactly one line, got %d:\n%s", want, lineMatches, view)
				}
			}
		})
	}
}

func TestDashboardViewFitsFixedTerminalSizes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name          string
		width, height int
		wantOneRow    bool
	}{
		{name: "80x24", width: 80, height: 24},
		{name: "100x35", width: 100, height: 35},
		{name: "160x45", width: 160, height: 45, wantOneRow: true},
		{name: "199x14", width: 199, height: 14, wantOneRow: true},
		{name: "240x50", width: 240, height: 50, wantOneRow: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Dashboard{
				width:  tt.width,
				height: tt.height,
				tokenMetrics: &metrics.TokenMetrics{
					Available:    true,
					InputTokens:  1200,
					OutputTokens: 850,
					TotalTokens:  2050,
					Prompts:      7,
					TotalCost:    0.25,
					ModelUsages: []metrics.ModelUsage{{
						Model:       "claude-sonnet-4-20250514",
						TotalTokens: 2050,
						Cost:        0.25,
					}},
				},
				tmuxMetrics: &metrics.TmuxMetrics{
					Available: true,
					Total:     5,
					Source:    "tmux",
					Sessions: []metrics.TmuxSession{
						{Name: "alpha", SessionType: metrics.SessionTypeInteractive, Status: metrics.StatusActive},
						{Name: "delta", SessionType: metrics.SessionTypeInteractive, Status: metrics.StatusReady},
						{Name: "worker-alpha", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusWorking},
						{Name: "worker-bravo", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusReady},
						{Name: "worker-charlie", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusActive},
					},
				},
				version: "test",
			}
			d.updateLayout()

			view := d.View()
			if got := lipgloss.Height(view); got > tt.height {
				t.Fatalf("View() height = %d, want <= %d:\n%s", got, tt.height, view)
			}
			for i, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(line); got > tt.width {
					t.Errorf("View() line %d width = %d, want <= %d: %q", i+1, got, tt.width, line)
				}
			}

			if tt.wantOneRow {
				lines := strings.Split(view, "\n")
				var topRow, bottomRow int = -1, -1
				for i, line := range lines {
					if strings.Count(line, "╭") == 3 {
						topRow = i
					}
					if strings.Count(line, "╰") == 3 {
						bottomRow = i
					}
				}
				if topRow < 0 || bottomRow <= topRow {
					t.Fatalf("three-panel row does not have aligned top and bottom borders (top=%d bottom=%d):\n%s", topRow, bottomRow, view)
				}
			}
		})
	}
}

func TestRenderWorkerCellShowsWorkspaceExecutorAndBeadStatus(t *testing.T) {
	d := &Dashboard{}
	cell := d.renderSessionCell(metrics.TmuxSession{
		Name:        "claude-code-worker-1",
		SessionType: metrics.SessionTypeWorker,
		Status:      metrics.StatusWorking,
		Source:      "needle",
		Worker: &metrics.WorkerMetadata{
			Workspace:           "/home/coding/project/.worktrees/current",
			Agent:               "claude-code",
			Provider:            "anthropic",
			Model:               "sonnet",
			CurrentBead:         "ccdash-current",
			State:               "EXECUTING",
			BeadStatusAvailable: true,
			BeadsProcessed:      3,
			BeadsCompleted:      2,
		},
	}, 180)
	for _, want := range []string{
		"/home/coding/project/.worktrees/current",
		"claude-code/anthropic/sonnet",
		"ccdash-current (EXECUTING)",
		"2 closed/3 cycles",
	} {
		if !strings.Contains(cell, want) {
			t.Errorf("worker cell %q does not contain %q", cell, want)
		}
	}
	if got := lipgloss.Width(cell); got > 180 {
		t.Fatalf("worker cell width = %d, want <= 180: %q", got, cell)
	}
}

func TestRenderWorkerCellFallsBackWhenMetadataIsMissing(t *testing.T) {
	d := &Dashboard{}
	cell := d.renderSessionCell(metrics.TmuxSession{
		Name:        "claude-code-worker-1",
		SessionType: metrics.SessionTypeWorker,
		Status:      metrics.StatusReady,
		Source:      "needle",
		Worker:      &metrics.WorkerMetadata{State: "SELECTING"},
	}, 140)
	for _, want := range []string{
		"workspace unavailable",
		"executor unavailable",
		"bead status unavailable (SELECTING)",
	} {
		if !strings.Contains(cell, want) {
			t.Errorf("worker cell %q does not contain fallback %q", cell, want)
		}
	}
}

func TestRenderWorkerCellShowsAnEmptyQueue(t *testing.T) {
	d := &Dashboard{}
	cell := d.renderSessionCell(metrics.TmuxSession{
		Name:        "claude-code-worker-1",
		SessionType: metrics.SessionTypeWorker,
		Status:      metrics.StatusReady,
		Source:      "needle",
		Worker: &metrics.WorkerMetadata{
			Workspace:           "/home/coding/project",
			Agent:               "claude-code",
			State:               "EXHAUSTED",
			BeadStatusAvailable: true,
		},
	}, 140)
	if !strings.Contains(cell, "queue empty") {
		t.Fatalf("worker row does not show an empty queue: %q", cell)
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

func TestCompactLayoutKeepsTokenCostVisible(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, size := range []struct{ width, height int }{{80, 24}, {100, 35}} {
		d := &Dashboard{
			width:  size.width,
			height: size.height,
			tokenMetrics: &metrics.TokenMetrics{
				Available:           true,
				InputTokens:         32_200_000,
				OutputTokens:        4_500_000,
				CacheReadTokens:     580_800_000,
				CacheCreationTokens: 10_100_000,
				TotalTokens:         627_600_000,
				Prompts:             6670,
				TotalCost:           215.26,
				SessionAvgRate:      331_500,
				ModelUsages: []metrics.ModelUsage{
					{Model: "claude-opus-5-5", TotalTokens: 146_300_000, Cost: 119.17},
					{Model: "gpt-5.6-luna", TotalTokens: 249_200_000, Cost: 48.29},
					{Model: "claude-sonnet-5", TotalTokens: 76_900_000, Cost: 23.69},
				},
			},
			tmuxMetrics: &metrics.TmuxMetrics{
				Available: true,
				Total:     2,
				Source:    "tmux",
				Sessions: []metrics.TmuxSession{
					{Name: "worker-alpha", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusWorking},
					{Name: "worker-bravo", SessionType: metrics.SessionTypeWorker, Status: metrics.StatusReady},
				},
			},
			version: "test",
		}
		d.updateLayout()

		view := d.View()
		if got := lipgloss.Height(view); got > size.height {
			t.Fatalf("%dx%d: View() height = %d, want <= %d", size.width, size.height, got, size.height)
		}
		for _, want := range []string{"Cost:", "$215.26", "Total:"} {
			if !strings.Contains(view, want) {
				t.Errorf("%dx%d: View() is missing %q:\n%s", size.width, size.height, want, view)
			}
		}
	}
}

func TestUsedInteriorRowsIgnoresTrailingBlankRows(t *testing.T) {
	panel := "╭────╮\n│ a  │\n│ b  │\n│    │\n│    │\n╰────╯"
	if got := usedInteriorRows(panel); got != 2 {
		t.Fatalf("usedInteriorRows = %d, want 2", got)
	}
}

func TestWideLayoutKeepsTokenModelsBesideStats(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// 214x18 with a full fleet: the sessions panel wants three columns, and
	// before the fix squeezed the token panel below its side-by-side width,
	// stacking the models list under the stats where it was cut off.
	var sessions []metrics.TmuxSession
	for i := 0; i < 33; i++ {
		sessions = append(sessions, metrics.TmuxSession{
			Name: fmt.Sprintf("glm-front-worker-%02d", i), SessionType: metrics.SessionTypeWorker, Status: metrics.StatusReady,
		})
	}
	d := &Dashboard{
		width:  214,
		height: 18,
		tokenMetrics: &metrics.TokenMetrics{
			Available: true, InputTokens: 267_700_000, OutputTokens: 33_500_000,
			CacheReadTokens: 4_100_000_000, CacheCreationTokens: 19_300_000,
			TotalTokens: 4_400_000_000, Prompts: 53669, TotalCost: 840.38, SessionAvgRate: 1_600_000,
			ModelUsages: []metrics.ModelUsage{
				{Model: "gpt-6-astra", TotalTokens: 158_500_000, Cost: 260.07},
				{Model: "gpt-6-sol", TotalTokens: 638_200_000, Cost: 171.71},
				{Model: "claude-opus-5-5", TotalTokens: 194_500_000, Cost: 90.83},
				{Model: "gpt-5.6-luna", TotalTokens: 249_200_000, Cost: 48.29},
				{Model: "claude-sonnet-5-5", TotalTokens: 27_400_000, Cost: 11.89},
			},
		},
		tmuxMetrics: &metrics.TmuxMetrics{Available: true, Total: len(sessions), Source: "tmux", Sessions: sessions},
		version:     "test",
	}
	d.updateLayout()

	view := d.View()
	if got := lipgloss.Height(view); got > d.height {
		t.Fatalf("View() height = %d, want <= %d", got, d.height)
	}
	lines := strings.Split(view, "\n")
	modelsRow, statsRow := -1, -1
	for i, line := range lines {
		if modelsRow < 0 && strings.Contains(line, "Models") {
			modelsRow = i
		}
		if statsRow < 0 && strings.Contains(line, "In:") {
			statsRow = i
		}
	}
	if modelsRow < 0 || modelsRow != statsRow {
		t.Fatalf("models header on row %d, stats start on row %d; want them side by side:\n%s", modelsRow, statsRow, view)
	}
	for _, want := range []string{"$260.07", "$171.71", "$90.83", "$48.29", "$11.89", "Cost:"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() is missing %q:\n%s", want, view)
		}
	}
}

func TestTaskRowsDistinguishOutcomesAndShowActualDecision(t *testing.T) {
	d := &Dashboard{}
	for _, tc := range []struct {
		status metrics.SessionStatus
		label  string
	}{
		{metrics.StatusWaitingExternal, "WAIT"}, {metrics.StatusComplete, "DONE"},
		{metrics.StatusResumable, "RESUME"}, {metrics.StatusPaused, "PAUSE"},
	} {
		got := d.renderSessionCell(metrics.TmuxSession{Name: "project", Status: tc.status}, 100)
		if !strings.Contains(got, tc.label) {
			t.Fatalf("%s row missing %s: %s", tc.status, tc.label, got)
		}
	}
	got := d.renderSessionCell(metrics.TmuxSession{Name: "project", Status: metrics.StatusAsking, AttentionReason: "Which region?"}, 100)
	if !strings.Contains(got, "Which region?") {
		t.Fatalf("question missing: %s", got)
	}
	got = d.renderSessionCell(metrics.TmuxSession{Name: "idle", Status: metrics.StatusReady, IdleDuration: time.Hour}, 100)
	if strings.Contains(got, "⏰") {
		t.Fatalf("idle session was promoted to attention: %s", got)
	}
}
