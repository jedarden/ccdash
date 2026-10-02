package updater

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
)

func TestCheckForUpdateUsesLatestReleaseRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/jedarden/ccdash/releases/tag/v1.1.17", http.StatusFound)
		case "/jedarden/ccdash/releases/tag/v1.1.17":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	u := NewUpdater("1.1.16")
	u.latestURL = server.URL + "/releases/latest"
	u.httpClient = server.Client()

	info := u.CheckForUpdate(true)
	if info.Error != "" {
		t.Fatalf("CheckForUpdate returned an error: %s", info.Error)
	}
	if info.LatestVersion != "1.1.17" || !info.UpdateAvailable {
		t.Fatalf("unexpected update info: %+v", info)
	}
	asset := platformAssetName(runtime.GOOS, runtime.GOARCH)
	wantURL := "https://github.com/jedarden/ccdash/releases/download/v1.1.17/" + asset
	if info.DownloadURL != wantURL {
		t.Fatalf("DownloadURL = %q, want %q", info.DownloadURL, wantURL)
	}
}

func TestCheckForUpdateSkipsNonReleaseVersions(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "non-release build must not make update requests", http.StatusInternalServerError)
	}))
	defer server.Close()

	for _, version := range []string{"dev", "1.2", "v1.2.3-rc.1", "1.2.3+local", ""} {
		u := NewUpdater(version)
		u.latestURL = server.URL + "/releases/latest"
		u.httpClient = server.Client()

		info := u.CheckForUpdate(true)
		if info == nil || info.CurrentVersion != version || info.UpdateAvailable || info.Error != "" {
			t.Fatalf("CheckForUpdate(%q) = %+v, want an inert result", version, info)
		}
	}
	if requests != 0 {
		t.Fatalf("non-release versions made %d update requests, want 0", requests)
	}
}

func TestIsReleaseVersion(t *testing.T) {
	tests := map[string]bool{
		"1.2.3":       true,
		"v1.2.3":      true,
		"0.0.1":       true,
		"dev":         false,
		"1.2":         false,
		"v1.2.3-rc.1": false,
		"1.2.3+local": false,
		"01.2.3":      false,
	}
	for version, want := range tests {
		if got := IsReleaseVersion(version); got != want {
			t.Errorf("IsReleaseVersion(%q) = %t, want %t", version, got, want)
		}
	}
}

func TestLatestReleaseTag(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{name: "release tag", rawURL: "https://github.com/jedarden/ccdash/releases/tag/v1.1.17", want: "v1.1.17"},
		{name: "latest did not redirect", rawURL: "https://github.com/jedarden/ccdash/releases/latest", wantErr: true},
		{name: "different repository", rawURL: "https://github.com/someone/ccdash/releases/tag/v9.0.0", wantErr: true},
		{name: "tag contains slash", rawURL: "https://github.com/jedarden/ccdash/releases/tag/release%2F1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			releaseURL, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatal(err)
			}
			got, err := latestReleaseTag(releaseURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("latestReleaseTag() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("latestReleaseTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlatformAssetName(t *testing.T) {
	tests := map[string]string{
		"linux/amd64":   "ccdash-linux-amd64",
		"linux/arm64":   "ccdash-linux-arm64",
		"darwin/amd64":  "ccdash-darwin-amd64",
		"darwin/arm64":  "ccdash-darwin-arm64",
		"windows/amd64": "",
		"linux/386":     "",
	}
	for platform, want := range tests {
		goos, goarch, _ := strings.Cut(platform, "/")
		if got := platformAssetName(goos, goarch); got != want {
			t.Errorf("platformAssetName(%q) = %q, want %q", platform, got, want)
		}
	}
}
