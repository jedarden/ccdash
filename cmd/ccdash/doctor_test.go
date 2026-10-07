package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func doctorTestEnv(t *testing.T) doctorEnv {
	t.Helper()
	return doctorEnv{
		Home:        t.TempDir(),
		Now:         time.Now(),
		LookPath:    func(string) (string, error) { return "/usr/bin/tmux", nil },
		ClaudeHooks: func() bool { return true },
		CodexHooks:  func() bool { return true },
	}
}

func writeDoctorFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findCheck(r doctorReport, name string) (doctorCheck, bool) {
	for _, c := range r.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return doctorCheck{}, false
}

func TestDoctorFailsWithNoTokenSources(t *testing.T) {
	r := buildDoctorReport(doctorTestEnv(t))
	if c, ok := findCheck(r, "token sources"); !ok || c.Status != doctorFail {
		t.Fatalf("want a failing 'token sources' check, got %+v", r.Checks)
	}
	if r.Problems != 1 {
		t.Fatalf("problems = %d, want 1: %+v", r.Problems, r.Checks)
	}
}

func TestDoctorCountsSourcesAndPasses(t *testing.T) {
	env := doctorTestEnv(t)
	writeDoctorFile(t, filepath.Join(env.Home, ".claude", "projects", "p", "a.jsonl"), "{}\n")
	writeDoctorFile(t, filepath.Join(env.Home, ".claude", "projects", "p", "b.jsonl"), "{}\n")
	writeDoctorFile(t, filepath.Join(env.Home, ".codex", "sessions", "2026", "10", "07", "rollout-x.jsonl"), "{}\n")
	env.OpenCodeDB = filepath.Join(env.Home, "opencode.db")
	writeDoctorFile(t, env.OpenCodeDB, "x")

	r := buildDoctorReport(env)
	if r.Problems != 0 {
		t.Fatalf("problems = %d, want 0: %+v", r.Problems, r.Checks)
	}
	for name, want := range map[string]string{"claude": "2 transcripts", "codex": "1 rollout logs", "opencode": "opencode.db"} {
		c, ok := findCheck(r, name)
		if !ok || c.Status != doctorOK || !strings.Contains(c.Detail, want) {
			t.Errorf("%s check = %+v, want ok containing %q", name, c, want)
		}
	}
}

func TestDoctorConfigProblemsAndWebhookRedaction(t *testing.T) {
	env := doctorTestEnv(t)
	cfg := filepath.Join(env.Home, ".ccdash", "config.yaml")

	writeDoctorFile(t, cfg, "notify:\n  enabled: true\n")
	if c, _ := findCheck(buildDoctorReport(env), "notifications"); c.Status != doctorFail {
		t.Errorf("enabled notifications without a URL = %+v, want fail", c)
	}

	writeDoctorFile(t, cfg, "notify:\n  enabled: true\n  webhook_url: https://hooks.example.com/services/T000/SECRETPART?token=abc\n")
	c, _ := findCheck(buildDoctorReport(env), "notifications")
	if c.Status != doctorOK || !strings.Contains(c.Detail, "https://hooks.example.com/…") {
		t.Errorf("notifications = %+v, want ok with redacted host", c)
	}
	if strings.Contains(c.Detail, "SECRETPART") || strings.Contains(c.Detail, "token=") {
		t.Errorf("webhook path or query leaked: %q", c.Detail)
	}

	writeDoctorFile(t, cfg, "notify: [not, a, map\n")
	if c, _ := findCheck(buildDoctorReport(env), "config"); c.Status != doctorFail {
		t.Errorf("unparseable config = %+v, want fail", c)
	}
}

func TestDoctorWarnsOnMissingHooksAndTmux(t *testing.T) {
	env := doctorTestEnv(t)
	env.ClaudeHooks = func() bool { return false }
	env.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	r := buildDoctorReport(env)
	for _, name := range []string{"claude hooks", "tmux"} {
		if c, _ := findCheck(r, name); c.Status != doctorWarn {
			t.Errorf("%s = %+v, want warn", name, c)
		}
	}
}

func TestFormatCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 35550: "35,550", 1234567: "1,234,567"} {
		if got := formatCount(n); got != want {
			t.Errorf("formatCount(%d) = %q, want %q", n, got, want)
		}
	}
}
