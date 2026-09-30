package main

import (
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jedarden/ccdash/internal/metrics"
)

func TestCheckHooksCommandHelper(t *testing.T) {
	if os.Getenv("CCDASH_CHECK_HOOKS_HELPER") != "1" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("ccdash", flag.ExitOnError)
	os.Args = []string{"ccdash", "--check-hooks"}
	main()
}

func TestCheckHooksCommandOutput(t *testing.T) {
	homes := map[string]string{
		"none":   t.TempDir(),
		"claude": t.TempDir(),
		"codex":  t.TempDir(),
		"both":   t.TempDir(),
	}
	for _, name := range []string{"claude", "both"} {
		installHooksForHome(t, homes[name], true, false)
	}
	for _, name := range []string{"codex", "both"} {
		installHooksForHome(t, homes[name], false, true)
	}

	tests := []struct {
		name       string
		wantExit   int
		wantOutput []string
		absent     []string
	}{
		{
			name:     "none",
			wantExit: 1,
			wantOutput: []string{
				"Claude Code and Codex hooks are NOT installed",
				"Run 'ccdash --install-hooks' or 'ccdash --install-codex-hooks'",
			},
		},
		{
			name:     "claude",
			wantExit: 0,
			wantOutput: []string{
				"Claude Code hooks are installed",
				"Hook scripts:",
				"Active sessions: 0",
			},
			absent: []string{"Codex hooks are installed"},
		},
		{
			name:     "codex",
			wantExit: 0,
			wantOutput: []string{
				"Codex hooks are installed",
				"Hook configuration:",
				"Active sessions: 0",
			},
			absent: []string{"Claude Code hooks are installed"},
		},
		{
			name:     "both",
			wantExit: 0,
			wantOutput: []string{
				"Claude Code hooks are installed",
				"Codex hooks are installed",
				"Active sessions: 0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, exitCode := runCheckHooksCommand(t, homes[tt.name])
			if exitCode != tt.wantExit {
				t.Fatalf("--check-hooks exited %d, want %d; output:\n%s", exitCode, tt.wantExit, output)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("--check-hooks output missing %q:\n%s", want, output)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(output, absent) {
					t.Errorf("--check-hooks output unexpectedly contains %q:\n%s", absent, output)
				}
			}
		})
	}
}

func installHooksForHome(t *testing.T, home string, claude, codex bool) {
	t.Helper()
	if !claude && !codex {
		return
	}
	t.Setenv("HOME", home)
	if claude {
		collector, err := metrics.NewHookSessionCollector()
		if err != nil {
			t.Fatal(err)
		}
		if err := collector.InstallHooks(); err != nil {
			t.Fatal(err)
		}
	}
	if codex {
		if err := metrics.NewCodexHookInstaller().InstallHooks(); err != nil {
			t.Fatal(err)
		}
	}
}

func runCheckHooksCommand(t *testing.T, home string) (string, int) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestCheckHooksCommandHelper$")
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "HOME=") && !strings.HasPrefix(value, "CCDASH_CHECK_HOOKS_HELPER=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+home, "CCDASH_CHECK_HOOKS_HELPER=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(output), exitErr.ExitCode()
	}
	t.Fatalf("run helper process: %v", err)
	return string(output), -1
}
