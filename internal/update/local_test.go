package update

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"denova/internal/hostruntime"
)

func TestInstallLocalRelease(t *testing.T) {
	exeName, extension := "denova", ".tar.gz"
	if runtime.GOOS == "windows" {
		exeName, extension = "denova.exe", ".zip"
	}
	packageFiles := map[string]string{
		"web/index.html":                               "<html>release</html>",
		"skills/example/SKILL.md":                      "skill",
		"tools/" + hostruntime.RipgrepExecutableName(): "ripgrep",
		"licenses/ripgrep/LICENSE-MIT":                 "MIT",
		"licenses/ripgrep/UNLICENSE":                   "Unlicense",
	}
	packageFiles[exeName] = "new executable"
	packageFiles[updaterExecutableName()] = "new updater"
	archiveName := "denova-v0.5.0-" + platformKey(runtime.GOOS, runtime.GOARCH) + extension
	archive := localTestArchive(t, packageFiles, extension)
	installDir := t.TempDir()
	service := &Service{currentVersion: "0.4.5", executablePath: filepath.Join(installDir, exeName)}
	if err := os.WriteFile(service.executablePath, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := service.InstallLocal(context.Background(), archiveName, bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	if !result.ApplyReady || !result.Staged || result.Installed || result.PreviousVersion != "0.4.5" || result.InstalledVersion != "0.5.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
	manifestPath, err := readPendingManifestRef(updateDataDir(installDir))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.5.0" || manifest.SourceDir != result.StagedPath {
		t.Fatalf("manifest: %#v", manifest)
	}
	content, _ := os.ReadFile(service.executablePath)
	if string(content) != "current" {
		t.Fatal("upload replaced the running installation before apply")
	}
	for _, tc := range []struct {
		name, current, archive string
		data                   []byte
		want                   error
	}{
		{"same version", "0.5.0", archiveName, archive, ErrPackageVersion},
		{"older version", "0.6.0", archiveName, archive, ErrPackageVersion},
		{"development", "dev", archiveName, archive, ErrPackageVersion},
		{"invalid name", "0.4.5", "source.zip", archive, ErrInvalidPackage},
		{"traversal", "0.4.5", "../" + archiveName, archive, ErrInvalidPackage},
		{"corrupt archive", "0.4.5", archiveName, []byte("invalid archive"), ErrInvalidPackage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service.currentVersion = tc.current
			if _, err := service.InstallLocal(context.Background(), tc.archive, bytes.NewReader(tc.data)); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			pending, err := readPendingManifestRef(updateDataDir(installDir))
			if err != nil || pending != manifestPath {
				t.Fatalf("failed upload changed pending update: %s, %v", pending, err)
			}
		})
	}
	service.currentVersion = "0.4.5"
	wrongPlatform := "windows-x64"
	if runtime.GOOS == "windows" {
		wrongPlatform = "linux-x64"
	}
	if _, err := service.InstallLocal(context.Background(), "denova-v0.5.0-"+wrongPlatform+extension, bytes.NewReader(archive)); !errors.Is(err, ErrPackagePlatform) {
		t.Fatalf("wrong platform: %v", err)
	}
	delete(packageFiles, updaterExecutableName())
	if _, err := service.InstallLocal(context.Background(), archiveName, bytes.NewReader(localTestArchive(t, packageFiles, extension))); !errors.Is(err, ErrInvalidPackage) {
		t.Fatalf("missing updater: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(updateDataDir(installDir), "local-*"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary upload files leaked: %v, %v", entries, err)
	}
	var relaunched string
	if err := RunUpdater(context.Background(), manifestPath, UpdaterOptions{
		ProcessAlive: func(int) bool { return false },
		StartProcess: func(executable string, args, env []string) error { relaunched = executable; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if relaunched != service.executablePath {
		t.Fatalf("relaunch = %s", relaunched)
	}
	backup, err := os.ReadFile(filepath.Join(result.BackupPath, exeName))
	if err != nil || string(backup) != "current" {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	installed, err := os.ReadFile(service.executablePath)
	if err != nil || string(installed) != "new executable" {
		t.Fatalf("installed executable = %q, %v", installed, err)
	}
}

func TestLocalUpdateRejectsConcurrentOperations(t *testing.T) {
	updateOperation.Lock()
	defer updateOperation.Unlock()
	service := NewService()
	if _, err := service.InstallLocal(context.Background(), "package.zip", strings.NewReader("")); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background()); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background()); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
}

func localTestArchive(t *testing.T, files map[string]string, extension string) []byte {
	t.Helper()
	if extension == ".tar.gz" {
		return testReleaseArchive(t, "denova", files)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, content := range files {
		header := &zip.FileHeader{Name: "denova/" + name, Method: zip.Deflate}
		header.SetMode(0o755)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
