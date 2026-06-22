package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSelectAssetForPlatform(t *testing.T) {
	assets := []githubAsset{
		{Name: "checksums.txt"},
		{Name: "nova-v0.1.11-darwin-arm64.tar.gz", DownloadURL: "asset-api-url"},
		{Name: "nova-v0.1.11-linux-x64.tar.gz"},
	}
	asset := selectAsset(assets, "darwin-arm64")
	if asset == nil || asset.Name != "nova-v0.1.11-darwin-arm64.tar.gz" {
		t.Fatalf("unexpected asset: %#v", asset)
	}
	if got := selectAsset(assets, "windows-x64"); got != nil {
		t.Fatalf("windows asset should not match: %#v", got)
	}
}

func TestPlatformKeyNormalizesAMD64(t *testing.T) {
	if got := platformKey("darwin", "amd64"); got != "darwin-x64" {
		t.Fatalf("platformKey darwin/amd64 = %s", got)
	}
	if got := platformKey("linux", "arm64"); got != "linux-arm64" {
		t.Fatalf("platformKey linux/arm64 = %s", got)
	}
}

func TestInstallUsesBrowserDownloadURLAndIgnoresRequestCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows updates are staged through a cmd script")
	}

	platform := platformKey(runtime.GOOS, runtime.GOARCH)
	assetName := "nova-v0.2.0-" + platform + ".tar.gz"
	archive := testReleaseArchive(t, "nova", map[string]string{
		"nova":                 "new executable",
		"web/index.html":       "<html>new</html>",
		"skills/demo/SKILL.md": "skill",
		"README.md":            "readme",
	})
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + assetName + "\n"
	var assetAPIHit bool
	var checksumAPIHit bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName:     "v0.2.0",
				HTMLURL:     "https://example.com/releases/v0.2.0",
				PublishedAt: time.Now(),
				Assets: []githubAsset{
					{
						Name:               assetName,
						Size:               int64(len(archive)),
						DownloadURL:        serverURL(r, "/api-asset"),
						BrowserDownloadURL: serverURL(r, "/download-asset"),
					},
					{
						Name:               "checksums.txt",
						DownloadURL:        serverURL(r, "/api-checksums"),
						BrowserDownloadURL: serverURL(r, "/download-checksums"),
					},
				},
			})
		case "/api-asset":
			assetAPIHit = true
			http.Error(w, "asset api should not be used", http.StatusInternalServerError)
		case "/api-checksums":
			checksumAPIHit = true
			http.Error(w, "checksum api should not be used", http.StatusInternalServerError)
		case "/download-asset":
			_, _ = w.Write(archive)
		case "/download-checksums":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	exePath := filepath.Join(installDir, "nova")
	if err := os.WriteFile(exePath, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installDir, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "web", "index.html"), []byte("old web"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := &Service{
		repository:     "owner/repo",
		currentVersion: "0.1.0",
		httpClient:     server.Client(),
		executablePath: exePath,
		githubAPIBase:  server.URL + "/repos",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := service.Install(ctx)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if !result.Installed || !result.RestartRequired || result.InstalledVersion != "0.2.0" {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if assetAPIHit || checksumAPIHit {
		t.Fatalf("install should use browser_download_url, asset_api=%v checksum_api=%v", assetAPIHit, checksumAPIHit)
	}
	if got, err := os.ReadFile(exePath); err != nil || string(got) != "new executable" {
		t.Fatalf("executable not replaced: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(installDir, "web", "index.html")); err != nil || string(got) != "<html>new</html>" {
		t.Fatalf("web assets not replaced: %q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(result.BackupPath, "nova")); err != nil {
		t.Fatalf("backup executable missing: %v", err)
	}
}

func testReleaseArchive(t *testing.T, exeName string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		mode := int64(0o644)
		if name == exeName {
			mode = 0o755
		}
		path := filepath.ToSlash(filepath.Join("nova", name))
		if err := tw.WriteHeader(&tar.Header{
			Name: path,
			Mode: mode,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serverURL(r *http.Request, path string) string {
	return "http://" + r.Host + path
}
