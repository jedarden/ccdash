package metrics

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCodexStatusHooksSerializeWithSessionEnd(t *testing.T) {
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("session serialization uses flock on this host")
	}
	home := t.TempDir()
	installer := newCodexHookInstaller(home)
	if err := installer.InstallHooks(); err != nil {
		t.Fatal(err)
	}
	input := `{"session_id":"test-session","cwd":"/tmp/project"}`
	runCodexStatusHook(t, installer, "codex-session-start.sh", input, "")

	names := []string{
		"codex-prompt-submit.sh", "codex-pre-tool-use.sh", "codex-post-tool-use.sh",
		"codex-permission-request.sh", "codex-stop.sh", "codex-session-end.sh",
	}
	var wg sync.WaitGroup
	errCh := make(chan string, len(names))
	for _, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if output, err := execCodexStatusHook(installer, name, input, ""); err != nil {
				errCh <- name + ": " + err.Error() + ": " + output
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(home, HooksDir, SessionsSubdir, "test-session.json")); !os.IsNotExist(err) {
		t.Fatalf("SessionEnd must prevent late updates from recreating the session, stat error: %v", err)
	}

	// The next session start must leave a complete JSON document for readers.
	runCodexStatusHook(t, installer, "codex-session-start.sh", input, "")
	data, err := os.ReadFile(filepath.Join(home, HooksDir, SessionsSubdir, "test-session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var session HookSession
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("session is not valid JSON: %v", err)
	}
	if session.SessionID != "test-session" || session.Status != "active" {
		t.Fatalf("unexpected session after restart: %+v", session)
	}
}

func TestCodexStatusHookErrorIsLoggedAndTempIsRemoved(t *testing.T) {
	home := t.TempDir()
	outsideTemp := t.TempDir()
	installer := newCodexHookInstaller(home)
	if err := installer.InstallHooks(); err != nil {
		t.Fatal(err)
	}
	sessionsDir := filepath.Join(home, HooksDir, SessionsSubdir)
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "test-session.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	input := `{"session_id":"test-session","cwd":"/tmp/project"}`
	if output, err := execCodexStatusHook(installer, "codex-pre-tool-use.sh", input, outsideTemp); err == nil {
		t.Fatalf("malformed session should fail with a diagnostic, output: %s", output)
	}
	logData, err := os.ReadFile(filepath.Join(home, HooksDir, "codex-hook-errors.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "hook=codex-pre-tool-use.sh") || strings.Contains(string(logData), input) {
		t.Fatalf("error log should identify the hook without recording its input: %s", logData)
	}
	for _, dir := range []string{sessionsDir, outsideTemp} {
		matches, err := filepath.Glob(filepath.Join(dir, ".codex-session.????????"))
		if err != nil || len(matches) != 0 {
			t.Fatalf("temporary session files remain in %s: %v, %v", dir, matches, err)
		}
	}
	entries, err := os.ReadDir(outsideTemp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("hook used external TMPDIR: %v, %v", entries, err)
	}
}

func runCodexStatusHook(t *testing.T, installer *CodexHookInstaller, name, input, tmpDir string) {
	t.Helper()
	if output, err := execCodexStatusHook(installer, name, input, tmpDir); err != nil {
		t.Fatalf("%s failed: %v: %s", name, err, output)
	}
}

func execCodexStatusHook(installer *CodexHookInstaller, name, input, tmpDir string) (string, error) {
	cmd := exec.Command(filepath.Join(installer.hooksDir, name))
	cmd.Env = append(os.Environ(), "HOME="+installer.homeDir)
	if tmpDir != "" {
		cmd.Env = append(cmd.Env, "TMPDIR="+tmpDir)
	}
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
