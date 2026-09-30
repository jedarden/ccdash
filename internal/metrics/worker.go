package metrics

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectSessionType identifies known executor-named workers and falls back to
// the worker log written by the worker launcher for custom session names.
func DetectSessionType(sessionName string) SessionType {
	if hasWorkerExecutorPrefix(sessionName) {
		return SessionTypeWorker
	}

	home, err := os.UserHomeDir()
	if err == nil && hasWorkerLog(home, sessionName) {
		return SessionTypeWorker
	}
	return SessionTypeInteractive
}

func hasWorkerExecutorPrefix(sessionName string) bool {
	name := strings.ToLower(strings.TrimSpace(sessionName))
	name = strings.TrimPrefix(name, "needle-")
	for _, prefix := range []string{"claude-code-", "opencode-"} {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			return true
		}
	}
	return false
}

func hasWorkerLog(home, sessionName string) bool {
	if home == "" || sessionName == "" {
		return false
	}
	path := filepath.Join(home, ".beads-workers", sessionName+".log")
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
