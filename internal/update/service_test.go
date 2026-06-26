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

func TestValidateReleasePackageRequiresUpdater(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nova"), []byte("exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateReleasePackage(dir, "nova", updaterExecutableName()); err == nil {
		t.Fatal("validateReleasePackage should fail when updater is missing")
	}
}

func TestInstallStagesUpdateAndIgnoresRequestCancel(t *testing.T) {
	platform := platformKey(runtime.GOOS, runtime.GOARCH)
	assetName := "nova-v0.2.0-" + platform + ".tar.gz"
	updaterName := updaterExecutableName()
	archive := testReleaseArchive(t, "nova", map[string]string{
		"nova":                 "new executable",
		updaterName:            "new updater",
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
	if err := os.WriteFile(filepath.Join(installDir, updaterName), []byte("old updater"), 0o755); err != nil {
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

	var progress []InstallProgress
	result, err := service.InstallWithProgress(ctx, func(event InstallProgress) {
		progress = append(progress, event)
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result.Status != "staged" || !result.Staged || !result.ApplyReady || !result.RestartRequired || result.Installed || result.InstalledVersion != "0.2.0" {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if assetAPIHit || checksumAPIHit {
		t.Fatalf("install should use browser_download_url, asset_api=%v checksum_api=%v", assetAPIHit, checksumAPIHit)
	}
	if got, err := os.ReadFile(exePath); err != nil || string(got) != "old executable" {
		t.Fatalf("executable should not be replaced before apply: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(installDir, "web", "index.html")); err != nil || string(got) != "old web" {
		t.Fatalf("web assets should not be replaced before apply: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(result.StagedPath, "nova")); err != nil || string(got) != "new executable" {
		t.Fatalf("staged executable missing: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(result.StagedPath, updaterName)); err != nil || string(got) != "new updater" {
		t.Fatalf("staged updater missing: %q err=%v", got, err)
	}
	archivePath := filepath.Join(installDir, ".nova-updates", "downloads", assetName)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("downloaded archive should be kept in install dir: %v", err)
	}
	manifestPath, err := readPendingManifestRef(filepath.Join(installDir, ".nova-updates"))
	if err != nil {
		t.Fatalf("pending manifest ref missing: %v", err)
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		t.Fatalf("manifest unreadable: %v", err)
	}
	if manifest.SourceDir != result.StagedPath || manifest.TargetExecutable != exePath || manifest.UpdaterExecutable != filepath.Join(result.StagedPath, updaterName) {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if len(manifest.RelaunchArgs) == 0 || manifest.RelaunchArgs[len(manifest.RelaunchArgs)-1] != "--no-open" {
		t.Fatalf("manifest should force --no-open: %#v", manifest.RelaunchArgs)
	}
	if !hasProgressPhase(progress, "downloading") || !hasProgressPhase(progress, "staging") || !hasProgressPhase(progress, "staged") {
		t.Fatalf("missing install progress phases: %#v", progress)
	}
	if last := progress[len(progress)-1]; last.Phase != "staged" || last.Percent != 100 {
		t.Fatalf("unexpected final progress event: %#v", last)
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

func hasProgressPhase(events []InstallProgress, phase string) bool {
	for _, event := range events {
		if event.Phase == phase {
			return true
		}
	}
	return false
}
