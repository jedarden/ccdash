package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	// GitHubRepo is the repository for ccdash
	GitHubRepo = "jedarden/ccdash"
	// GitHubAPIURL is the GitHub API endpoint for releases
	GitHubAPIURL = "https://api.github.com/repos/" + GitHubRepo + "/releases/latest"
)

// Release represents a GitHub release
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// UpdateInfo contains information about available updates
type UpdateInfo struct {
	CurrentVersion   string
	LatestVersion    string
	UpdateAvailable  bool
	DownloadURL      string
	ReleaseNotes     string
	LastChecked      time.Time
	Error            string
}

// Updater handles checking for and applying updates
type Updater struct {
	currentVersion string
	httpClient     *http.Client
	lastCheck      time.Time
	cachedInfo     *UpdateInfo
	checkInterval  time.Duration
}

// NewUpdater creates a new Updater instance
func NewUpdater(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		checkInterval: 5 * time.Minute, // Check every 5 minutes
	}
}

// CheckForUpdate checks GitHub for a newer version
func (u *Updater) CheckForUpdate() *UpdateInfo {
	// Use cached result if recent enough
	if u.cachedInfo != nil && time.Since(u.lastCheck) < u.checkInterval {
		return u.cachedInfo
	}

	info := &UpdateInfo{
		CurrentVersion: u.currentVersion,
		LastChecked:    time.Now(),
	}

	// Fetch latest release from GitHub
	req, err := http.NewRequest("GET", GitHubAPIURL, nil)
	if err != nil {
		info.Error = fmt.Sprintf("Failed to create request: %v", err)
		return info
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ccdash/"+u.currentVersion)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		info.Error = fmt.Sprintf("Failed to check for updates: %v", err)
		return info
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("GitHub API returned status %d", resp.StatusCode)
		return info
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		info.Error = fmt.Sprintf("Failed to parse release info: %v", err)
		return info
	}

	// Parse version (remove 'v' prefix if present)
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	info.LatestVersion = latestVersion
	info.ReleaseNotes = release.Name

	// Compare versions
	info.UpdateAvailable = compareVersions(u.currentVersion, latestVersion) < 0

	// Find the appropriate download URL for this platform
	if info.UpdateAvailable {
		info.DownloadURL = u.findDownloadURL(release.Assets)
	}

	u.lastCheck = time.Now()
	u.cachedInfo = info

	return info
}

// findDownloadURL finds the appropriate binary for the current platform
func (u *Updater) findDownloadURL(assets []Asset) string {
	// Build expected asset name based on OS and arch
	var expectedName string
	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH == "amd64" {
			expectedName = "ccdash-linux-amd64"
		} else if runtime.GOARCH == "arm64" {
			expectedName = "ccdash-linux-arm64"
		}
	case "darwin":
		if runtime.GOARCH == "amd64" {
			expectedName = "ccdash-darwin-amd64"
		} else if runtime.GOARCH == "arm64" {
			expectedName = "ccdash-darwin-arm64"
		}
	}

	for _, asset := range assets {
		if asset.Name == expectedName {
			return asset.BrowserDownloadURL
		}
	}

	// Fallback: look for any matching pattern
	for _, asset := range assets {
		if strings.Contains(asset.Name, runtime.GOOS) && strings.Contains(asset.Name, runtime.GOARCH) {
			return asset.BrowserDownloadURL
		}
	}

	return ""
}

// PerformUpdate downloads and applies the update, then restarts the application
func (u *Updater) PerformUpdate(info *UpdateInfo) error {
	if !info.UpdateAvailable || info.DownloadURL == "" {
		return fmt.Errorf("no update available or download URL not found")
	}

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Download new binary to temp file
	tmpFile, err := os.CreateTemp("", "ccdash-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	resp, err := u.httpClient.Get(info.DownloadURL)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpPath)
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write update: %w", err)
	}

	// Make the new binary executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Replace the current executable
	// First, try to rename directly (works on most systems)
	if err := os.Rename(tmpPath, execPath); err != nil {
		// If direct rename fails (e.g., cross-device), use copy
		if err := copyFile(tmpPath, execPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("failed to replace executable: %w", err)
		}
		os.Remove(tmpPath)
	}

	// Restart the application using syscall.Exec (replaces current process)
	return syscall.Exec(execPath, os.Args, os.Environ())
}

// PerformUpdateWithRestart downloads the update and restarts using multiple methods
func (u *Updater) PerformUpdateWithRestart(info *UpdateInfo) error {
	if !info.UpdateAvailable || info.DownloadURL == "" {
		return fmt.Errorf("no update available or download URL not found")
	}

	// Find all locations where ccdash is installed
	allLocations := findAllBinaryLocations()
	if len(allLocations) == 0 {
		return fmt.Errorf("failed to find any ccdash binary locations")
	}

	// Get current executable path for restart
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	realExecPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realExecPath = execPath
	}

	// Download new binary to temp file
	tmpPath := "/tmp/ccdash-update"

	resp, err := u.httpClient.Get(info.DownloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write update: %w", err)
	}

	// Make the new binary executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Update all found locations
	var updateErrors []string
	var updatedCount int

	for _, targetPath := range allLocations {
		if err := updateBinaryAt(tmpPath, targetPath); err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", targetPath, err))
		} else {
			updatedCount++
		}
	}

	// Cleanup temp file
	os.Remove(tmpPath)

	// If no locations were updated successfully, return error
	if updatedCount == 0 {
		return fmt.Errorf("failed to update any binary location: %v", updateErrors)
	}

	// Try multiple restart methods using the current executable path
	return u.restartApplication(realExecPath)
}

// updateBinaryAt updates the binary at the specified path
// On Linux/macOS, a running binary can be renamed but not overwritten (ETXTBSY).
// The correct approach is: rename old -> copy new to original path -> delete old
func updateBinaryAt(srcPath, targetPath string) error {
	backupPath := targetPath + ".old"

	// Remove any existing backup
	os.Remove(backupPath)

	// Check if target exists
	if _, err := os.Stat(targetPath); err == nil {
		// Rename current binary to backup (works even while running on Unix)
		if err := os.Rename(targetPath, backupPath); err != nil {
			// Check if this is a permission error - might need sudo
			if os.IsPermission(err) {
				return updateBinaryWithSudo(srcPath, targetPath)
			}
			return fmt.Errorf("failed to rename current binary: %w", err)
		}
	}

	// Copy new binary to target location (path is now free)
	if err := copyFile(srcPath, targetPath); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, targetPath)
		// Check if this is a permission error - might need sudo
		if os.IsPermission(err) {
			return updateBinaryWithSudo(srcPath, targetPath)
		}
		return fmt.Errorf("failed to install update: %w", err)
	}

	// Make sure the new binary is executable
	if err := os.Chmod(targetPath, 0755); err != nil {
		// Restore backup on failure
		os.Remove(targetPath)
		os.Rename(backupPath, targetPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Cleanup backup (don't care if this fails)
	os.Remove(backupPath)

	return nil
}

// updateBinaryWithSudo uses sudo to update a binary in a protected directory
func updateBinaryWithSudo(srcPath, targetPath string) error {
	// Create a shell script that does the update
	script := fmt.Sprintf(`#!/bin/sh
set -e
BACKUP="%s.old"
rm -f "$BACKUP"
if [ -f "%s" ]; then
    mv "%s" "$BACKUP"
fi
cp "%s" "%s"
chmod +x "%s"
rm -f "$BACKUP"
`, targetPath, targetPath, targetPath, srcPath, targetPath, targetPath)

	scriptPath := "/tmp/ccdash-update-script.sh"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to create update script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Try running with sudo
	cmd := exec.Command("sudo", "-n", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// sudo -n failed (no password-less sudo available)
		// Try with pkexec as alternative (graphical sudo prompt)
		cmd = exec.Command("pkexec", scriptPath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to update with elevated privileges: %v (output: %s)", err, string(output))
		}
	}

	return nil
}

// restartApplication tries multiple methods to restart the application
func (u *Updater) restartApplication(execPath string) error {
	args := os.Args
	env := os.Environ()

	// Method 1: syscall.Exec (replaces current process)
	// This is the cleanest method - process is replaced in-place
	execErr := syscall.Exec(execPath, args, env)

	// If we get here, syscall.Exec failed - try other methods

	// Method 2: Start new process and exit
	// This works well when the terminal can handle the new process
	cmd := exec.Command(execPath, args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Start(); err == nil {
		// New process started, exit current one
		os.Exit(0)
	}

	// Method 3: Use setsid to create new session (detaches from terminal)
	setsidPath, _ := exec.LookPath("setsid")
	if setsidPath != "" {
		setsidCmd := exec.Command(setsidPath, append([]string{execPath}, args[1:]...)...)
		setsidCmd.Env = env
		if err := setsidCmd.Start(); err == nil {
			os.Exit(0)
		}
	}

	// Method 4: Use nohup to start detached process
	nohupPath, _ := exec.LookPath("nohup")
	if nohupPath != "" {
		nohupCmd := exec.Command(nohupPath, append([]string{execPath}, args[1:]...)...)
		nohupCmd.Env = env
		if err := nohupCmd.Start(); err == nil {
			os.Exit(0)
		}
	}

	// Method 5: Use shell to spawn background process
	// The shell forks, launches ccdash, then exits - ccdash inherits terminal
	shellCmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("%s %s &", execPath, strings.Join(args[1:], " ")))
	shellCmd.Env = env
	if err := shellCmd.Start(); err == nil {
		// Give the shell a moment to spawn the process
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}

	// All methods failed, return the original exec error
	return fmt.Errorf("all restart methods failed, syscall.Exec error: %v", execErr)
}

// findAllBinaryLocations finds all locations where ccdash binary exists
// This includes the current executable and common installation paths
func findAllBinaryLocations() []string {
	locations := make(map[string]bool)

	// 1. Get the currently running executable (most important)
	if execPath, err := os.Executable(); err == nil {
		if realPath, err := filepath.EvalSymlinks(execPath); err == nil {
			locations[realPath] = true
		} else {
			locations[execPath] = true
		}
	}

	// 2. Check PATH for all ccdash binaries
	if pathEnv := os.Getenv("PATH"); pathEnv != "" {
		for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
			candidate := filepath.Join(dir, "ccdash")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				if realPath, err := filepath.EvalSymlinks(candidate); err == nil {
					locations[realPath] = true
				} else {
					locations[candidate] = true
				}
			}
		}
	}

	// 3. Check common installation directories
	homeDir, _ := os.UserHomeDir()
	commonPaths := []string{
		"/usr/local/bin/ccdash",
		"/usr/bin/ccdash",
		"/opt/homebrew/bin/ccdash",
		filepath.Join(homeDir, "bin", "ccdash"),
		filepath.Join(homeDir, ".local", "bin", "ccdash"),
		filepath.Join(homeDir, "go", "bin", "ccdash"),
	}

	for _, path := range commonPaths {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if realPath, err := filepath.EvalSymlinks(path); err == nil {
				locations[realPath] = true
			} else {
				locations[path] = true
			}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(locations))
	for loc := range locations {
		result = append(result, loc)
	}
	return result
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// compareVersions compares two semantic version strings
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Pad to same length
	for len(parts1) < len(parts2) {
		parts1 = append(parts1, "0")
	}
	for len(parts2) < len(parts1) {
		parts2 = append(parts2, "0")
	}

	for i := 0; i < len(parts1); i++ {
		var n1, n2 int
		fmt.Sscanf(parts1[i], "%d", &n1)
		fmt.Sscanf(parts2[i], "%d", &n2)

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return 0
}
