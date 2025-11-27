package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

// PerformUpdateWithRestart downloads the update and restarts in-place
func (u *Updater) PerformUpdateWithRestart(info *UpdateInfo) error {
	if !info.UpdateAvailable || info.DownloadURL == "" {
		return fmt.Errorf("no update available or download URL not found")
	}

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the real path
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

	// Replace the binary - need to handle "text file busy" error
	// by removing the old file first, then copying new one
	if err := os.Remove(realExecPath); err != nil && !os.IsNotExist(err) {
		// Try renaming old binary first
		backupPath := realExecPath + ".old"
		if renameErr := os.Rename(realExecPath, backupPath); renameErr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("failed to backup old binary: %w", renameErr)
		}
		defer os.Remove(backupPath)
	}

	// Copy new binary to target location
	if err := copyFile(tmpPath, realExecPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to install update: %w", err)
	}
	os.Remove(tmpPath)

	// Make sure the new binary is executable
	if err := os.Chmod(realExecPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions on new binary: %w", err)
	}

	// Use syscall.Exec to replace current process with new binary
	// This preserves the terminal and restarts ccdash in-place
	return syscall.Exec(realExecPath, os.Args, os.Environ())
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
