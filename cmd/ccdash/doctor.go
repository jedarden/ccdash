package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jedarden/ccdash/internal/config"
	"github.com/jedarden/ccdash/internal/metrics"
)

// Doctor check levels. A FAIL makes `ccdash --doctor` exit 1; a WARN is
// something optional that is missing or degraded.
const (
	doctorOK   = "ok"
	doctorWarn = "warn"
	doctorFail = "fail"
	doctorInfo = "info"
)

const doctorSchemaVersion = 1

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type doctorReport struct {
	SchemaVersion int           `json:"schema_version"`
	Version       string        `json:"version"`
	Checks        []doctorCheck `json:"checks"`
	Problems      int           `json:"problems"`
	Warnings      int           `json:"warnings"`
}

// doctorEnv is what the report reads, so tests can point it at a temporary
// home directory instead of the real one.
type doctorEnv struct {
	Home       string
	ExtraDirs  []string
	OpenCodeDB string
	Now        time.Time
	LookPath   func(string) (string, error)
	// Hooks report installation state; nil skips the check.
	ClaudeHooks func() bool
	CodexHooks  func() bool
}

func (r *doctorReport) add(name, status, format string, args ...any) {
	r.Checks = append(r.Checks, doctorCheck{Name: name, Status: status, Detail: fmt.Sprintf(format, args...)})
	switch status {
	case doctorFail:
		r.Problems++
	case doctorWarn:
		r.Warnings++
	}
}

func buildDoctorReport(env doctorEnv) doctorReport {
	r := doctorReport{SchemaVersion: doctorSchemaVersion, Version: version}
	checkDoctorConfig(&r, env)
	found := checkDoctorSources(&r, env)
	if found == 0 {
		r.add("token sources", doctorFail, "no Claude, Codex or OpenCode usage data found - the token panel will be empty")
	}
	checkDoctorCache(&r, env)
	checkDoctorHooks(&r, env)
	if _, err := env.LookPath("tmux"); err != nil {
		r.add("tmux", doctorWarn, "tmux not found on PATH - sessions are tracked only through hooks and NEEDLE")
	} else {
		r.add("tmux", doctorOK, "available")
	}
	return r
}

func checkDoctorConfig(r *doctorReport, env doctorEnv) {
	path := filepath.Join(env.Home, ".ccdash", "config.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		r.add("config", doctorOK, "%s not present - using defaults", tildePath(path, env.Home))
		return
	}
	cfg, err := config.LoadFrom(path)
	if err != nil {
		r.add("config", doctorFail, "%s: %v", tildePath(path, env.Home), err)
		return
	}
	parts := []string{tildePath(path, env.Home) + " parsed"}
	if cfg.Alerts.CostThresholdUSD > 0 {
		parts = append(parts, fmt.Sprintf("cost alert at $%.2f", cfg.Alerts.CostThresholdUSD))
	}
	if n := len(cfg.Pricing.Models); n > 0 {
		parts = append(parts, fmt.Sprintf("%d price override(s)", n))
	}
	r.add("config", doctorOK, "%s", strings.Join(parts, ", "))

	switch {
	case !cfg.Notify.Enabled:
		r.add("notifications", doctorInfo, "disabled")
	case strings.TrimSpace(cfg.Notify.WebhookURL) == "":
		r.add("notifications", doctorFail, "notify.enabled is true but notify.webhook_url is empty")
	default:
		r.add("notifications", doctorOK, "enabled, webhook %s (run --test-notify to send one)", redactURL(cfg.Notify.WebhookURL))
	}
}

// checkDoctorSources reports each token source and returns how many have data.
func checkDoctorSources(r *doctorReport, env doctorEnv) int {
	found := 0
	claudeDirs := append([]string{filepath.Join(env.Home, ".claude", "projects")}, env.ExtraDirs...)
	var claudeFiles int
	var claudeNewest time.Time
	var present []string
	for _, dir := range claudeDirs {
		n, newest, ok := countFiles(dir, func(name string) bool { return strings.HasSuffix(name, ".jsonl") })
		if !ok {
			continue
		}
		present = append(present, tildePath(dir, env.Home))
		claudeFiles += n
		if newest.After(claudeNewest) {
			claudeNewest = newest
		}
	}
	switch {
	case len(present) == 0:
		r.add("claude", doctorInfo, "%s not found", tildePath(claudeDirs[0], env.Home))
	case claudeFiles == 0:
		r.add("claude", doctorWarn, "%s has no transcripts yet", strings.Join(present, ", "))
	default:
		found++
		r.add("claude", doctorOK, "%s transcripts in %s, newest %s", formatCount(claudeFiles), strings.Join(present, ", "), ago(env.Now, claudeNewest))
	}

	codexDir := filepath.Join(env.Home, ".codex", "sessions")
	n, newest, ok := countFiles(codexDir, func(name string) bool {
		return strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl")
	})
	switch {
	case !ok:
		r.add("codex", doctorInfo, "%s not found", tildePath(codexDir, env.Home))
	case n == 0:
		r.add("codex", doctorWarn, "%s has no rollout logs yet", tildePath(codexDir, env.Home))
	default:
		found++
		r.add("codex", doctorOK, "%s rollout logs in %s, newest %s", formatCount(n), tildePath(codexDir, env.Home), ago(env.Now, newest))
	}

	if env.OpenCodeDB == "" {
		r.add("opencode", doctorInfo, "no store path")
	} else if info, err := os.Stat(env.OpenCodeDB); err != nil {
		r.add("opencode", doctorInfo, "%s not found", tildePath(env.OpenCodeDB, env.Home))
	} else {
		found++
		r.add("opencode", doctorOK, "%s (%s), updated %s", tildePath(env.OpenCodeDB, env.Home), formatBytes(info.Size()), ago(env.Now, info.ModTime()))
	}
	return found
}

func checkDoctorCache(r *doctorReport, env doctorEnv) {
	path := filepath.Join(env.Home, ".ccdash", "tokens.db")
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		r.add("cache", doctorInfo, "%s not created yet - it is built on the first run", tildePath(path, env.Home))
		return
	}
	if err != nil {
		r.add("cache", doctorFail, "%s: %v", tildePath(path, env.Home), err)
		return
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(2000)")
	if err != nil {
		r.add("cache", doctorFail, "%s: %v", tildePath(path, env.Home), err)
		return
	}
	defer db.Close()

	var schema int
	var events, files int64
	if err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&schema); err != nil {
		r.add("cache", doctorFail, "%s is not a readable ccdash cache: %v", tildePath(path, env.Home), err)
		return
	}
	_ = db.QueryRow("SELECT COUNT(*) FROM token_events").Scan(&events)
	_ = db.QueryRow("SELECT COUNT(*) FROM file_aggregates").Scan(&files)
	r.add("cache", doctorOK, "%s (%s), schema v%d, %s recent events, %s summarised files",
		tildePath(path, env.Home), formatBytes(info.Size()), schema, formatCount(int(events)), formatCount(int(files)))

	var instance string
	var expires int64
	if err := db.QueryRow("SELECT instance_id, expires_at FROM collector_lease WHERE id = 1").Scan(&instance, &expires); err != nil {
		r.add("collector", doctorInfo, "no dashboard instance has held the collector lease")
		return
	}
	pid, _ := strconv.Atoi(strings.SplitN(instance, "-", 2)[0])
	expiresAt := time.Unix(expires, 0)
	switch {
	case env.Now.Before(expiresAt) && pidAlive(pid):
		r.add("collector", doctorOK, "dashboard pid %d holds the collector lease; other instances read its system and session metrics", pid)
	case env.Now.Before(expiresAt):
		r.add("collector", doctorWarn, "lease held by pid %d, which is not running here - it will expire by %s", pid, expiresAt.Format(time.TimeOnly))
	default:
		r.add("collector", doctorInfo, "no dashboard running (lease last held by pid %d, expired %s)", pid, ago(env.Now, expiresAt))
	}
}

func checkDoctorHooks(r *doctorReport, env doctorEnv) {
	if env.ClaudeHooks != nil {
		if env.ClaudeHooks() {
			r.add("claude hooks", doctorOK, "installed")
		} else {
			r.add("claude hooks", doctorWarn, "not installed - run ccdash --install-hooks for accurate ASKING/WORKING status")
		}
	}
	if env.CodexHooks != nil {
		if env.CodexHooks() {
			r.add("codex hooks", doctorOK, "installed")
		} else {
			r.add("codex hooks", doctorWarn, "not installed - run ccdash --install-codex-hooks")
		}
	}
	for _, harness := range []string{"claude", "codex"} {
		log := filepath.Join(env.Home, ".ccdash", harness+"-hook-errors.log")
		if info, err := os.Stat(log); err == nil && info.Size() > 0 && env.Now.Sub(info.ModTime()) < 24*time.Hour {
			r.add(harness+" hook errors", doctorWarn, "%s written %s (%s)", tildePath(log, env.Home), ago(env.Now, info.ModTime()), formatBytes(info.Size()))
		}
	}
}

// countFiles walks dir and counts files whose base name matches. ok is false
// when dir does not exist.
func countFiles(dir string, match func(string) bool) (n int, newest time.Time, ok bool) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return 0, time.Time{}, false
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !match(d.Name()) {
			return nil
		}
		n++
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return n, newest, true
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// redactURL keeps only scheme and host: webhook paths and queries often
// carry the secret.
func redactURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "(unparseable URL)"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

func tildePath(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

func ago(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	if d < time.Minute {
		return "just now"
	}
	return metrics.FormatDuration(d) + " ago"
}

func formatCount(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func printDoctorReport(w io.Writer, r doctorReport, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
		return
	}
	fmt.Fprintf(w, "ccdash doctor (version %s)\n\n", r.Version)
	labels := map[string]string{doctorOK: "OK", doctorWarn: "WARN", doctorFail: "FAIL", doctorInfo: "-"}
	for _, c := range r.Checks {
		fmt.Fprintf(w, "  %-5s %-18s %s\n", labels[c.Status], c.Name, c.Detail)
	}
	fmt.Fprintf(w, "\n%d problem(s), %d warning(s)\n", r.Problems, r.Warnings)
}

func runDoctor(asJSON bool, extraDirs string) int {
	home, _ := os.UserHomeDir()
	env := doctorEnv{
		Home:       home,
		ExtraDirs:  doctorExtraDirs(extraDirs),
		OpenCodeDB: metrics.OpenCodeDBPath(),
		Now:        time.Now(),
		LookPath:   exec.LookPath,
		CodexHooks: metrics.NewCodexHookInstaller().AreHooksInstalled,
	}
	if collector, err := metrics.NewHookSessionCollector(); err == nil {
		env.ClaudeHooks = collector.AreHooksInstalled
	}
	report := buildDoctorReport(env)
	printDoctorReport(os.Stdout, report, asJSON)
	if report.Problems > 0 {
		return 1
	}
	return 0
}

// doctorExtraDirs mirrors the token collector's extra roots: --extra-dirs
// (comma-separated) plus CCDASH_EXTRA_DIRS (colon-separated, globs allowed).
func doctorExtraDirs(flagValue string) []string {
	var dirs []string
	for _, d := range strings.Split(flagValue, ",") {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}
	for _, d := range strings.Split(os.Getenv("CCDASH_EXTRA_DIRS"), ":") {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		if matches, err := filepath.Glob(d); err == nil && len(matches) > 0 {
			dirs = append(dirs, matches...)
		} else {
			dirs = append(dirs, d)
		}
	}
	return dirs
}
