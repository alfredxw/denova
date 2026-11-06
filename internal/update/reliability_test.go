package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"sync/atomic"
	"testing"

	"denova/internal/hostruntime"
)

func TestInstallPinsReleaseChecksum(t *testing.T) {
	assetName := "denova-v0.2.0-" + platformKey(runtime.GOOS, runtime.GOARCH) + ".tar.gz"
	archive := testReleaseArchive(t, "denova.exe", map[string]string{
		"denova.exe": "new executable", updaterExecutableName(): "new updater",
		"web/index.html": "new web", "skills/demo/SKILL.md": "new skill",
		"tools/" + hostruntime.RipgrepExecutableName(): "ripgrep",
		"licenses/ripgrep/LICENSE-MIT":                 "MIT", "licenses/ripgrep/UNLICENSE": "Unlicense",
	})
	sum := sha256.Sum256(archive)
	checksum := hex.EncodeToString(sum[:]) + "  " + assetName + "\n"
	var checks atomic.Int32
	var selectedChecksum atomic.Int32
	var drift atomic.Bool
	drift.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			tag, checksumPath := "v0.2.0", "/checksums-v020"
			if checks.Add(1) > 1 && drift.Load() {
				tag, checksumPath = "v0.3.0", "/checksums-v030"
			}
			_ = json.NewEncoder(w).Encode(githubRelease{TagName: tag, Assets: []githubAsset{
				{Name: assetName, Size: int64(len(archive)), BrowserDownloadURL: serverURL(r, "/archive-v020")},
				{Name: "checksums.txt", BrowserDownloadURL: serverURL(r, checksumPath)},
			}})
		case "/archive-v020":
			_, _ = w.Write(archive)
		case "/checksums-v020":
			selectedChecksum.Store(20)
			_, _ = w.Write([]byte(checksum))
		case "/checksums-v030":
			selectedChecksum.Store(30)
			_, _ = w.Write([]byte("other-release-checksums"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := &Service{repository: "owner/repo", currentVersion: "0.1.0", httpClient: server.Client(), executablePath: filepath.Join(t.TempDir(), "denova.exe"), githubAPIBase: server.URL + "/repos"}
	result, err := service.Install(context.Background())
	if err != nil || result.Status != "staged" {
		t.Fatalf("release changed during download: result=%+v err=%v", result, err)
	}
	if checks.Load() != 1 || selectedChecksum.Load() != 20 {
		t.Fatalf("release was not pinned: requests=%d checksum=%d", checks.Load(), selectedChecksum.Load())
	}
}

func TestRunUpdaterRollsBackLaunchFailure(t *testing.T) {
	installDir := t.TempDir()
	sourceDir := filepath.Join(updateDataDir(installDir), "pending-test", "denova")
	backupDir := filepath.Join(filepath.Dir(sourceDir), "backup")
	writeUpdateTestPackage(t, installDir, updateTestPackageContents{Executable: "old executable", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old ripgrep", License: "old license"})
	writeUpdateTestPackage(t, sourceDir, updateTestPackageContents{Executable: "new executable", Updater: "new updater", Web: "new web", Skill: "new skill", Ripgrep: "new ripgrep", License: "new license"})
	manifest := ApplyManifest{SourceDir: sourceDir, InstallDir: installDir, BackupDir: backupDir, TargetExecutable: filepath.Join(installDir, "nova"), UpdaterExecutable: filepath.Join(sourceDir, updaterExecutableName()), Version: "0.2.0"}
	manifestPath := filepath.Join(filepath.Dir(sourceDir), manifestFileName)
	if err := writeManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	startFailure := errors.New("simulated Windows executable launch failure")
	err := RunUpdater(context.Background(), manifestPath, UpdaterOptions{ProcessAlive: func(int) bool { return false }, StartProcess: func(string, []string, []string) error { return startFailure }})
	if !errors.Is(err, startFailure) {
		t.Fatalf("unexpected outcome: %v", err)
	}
	assertFileContent(t, manifest.TargetExecutable, "old executable")
	assertFileContent(t, filepath.Join(backupDir, "nova"), "old executable")

	if _, err := os.Stat(manifest.TargetExecutable); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadResumesAndRejectsCorruptPartial(t *testing.T) {
	payload := bytes.Repeat([]byte("update contents"), 1000)
	sum := sha256.Sum256(payload)
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			var ranges atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.Header.Get("Range") != "" {
					ranges.Add(1)
				}
				http.ServeContent(w, r, "release.zip", time.Time{}, bytes.NewReader(payload))
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "release.zip")
			partial := append([]byte(nil), payload[:500]...)
			if corrupt {
				partial[0] ^= 0xff
			}
			if err := os.WriteFile(path+".download", partial, 0o600); err != nil {
				t.Fatal(err)
			}
			service := &Service{httpClient: server.Client()}
			if err := service.downloadAsset(context.Background(), server.URL, path, int64(len(payload)), hex.EncodeToString(sum[:]), nil); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, payload) || ranges.Load() == 0 {
				t.Fatalf("download mismatch: err=%v ranges=%d", err, ranges.Load())
			}
		})
	}
}

func TestInterruptedUpdateRecoveryIsRepeatable(t *testing.T) {
	for _, phase := range []Phase{PhaseApplying, PhaseRollingBack} {
		t.Run(string(phase), func(t *testing.T) {
			install := t.TempDir()
			source := filepath.Join(updateDataDir(install), "pending-test", "denova")
			old := updateTestPackageContents{Executable: "old", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old tool", License: "old license"}
			writeUpdateTestPackage(t, install, old)
			writeUpdateTestPackage(t, source, updateTestPackageContents{Executable: "new"})
			m := ApplyManifest{ID: "recovery", State: phase, Version: "0.2.0", InstallDir: install, SourceDir: source, BackupDir: filepath.Join(filepath.Dir(source), "backup"), TargetExecutable: filepath.Join(install, "nova"), UpdaterExecutable: filepath.Join(source, updaterExecutableName())}
			if err := backupUpdate(&m); err != nil {
				t.Fatal(err)
			}
			// Partial replacement, including a file absent in the old distribution.
			if err := os.WriteFile(m.TargetExecutable, []byte("new"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(install, "skills")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(install, "README.md"), []byte("new only"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(install, ".denova"), 0o755); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(install, ".denova", "user-content")
			if err := os.WriteFile(sentinel, []byte("protected"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(filepath.Dir(source), manifestFileName)
			if err := writeManifest(path, m); err != nil {
				t.Fatal(err)
			}
			launches := 0
			err := RunUpdater(context.Background(), path, UpdaterOptions{ProcessAlive: func(int) bool { return false }, StartProcess: func(string, []string, []string) error { launches++; return nil }, LogOutput: io.Discard})
			if err == nil || launches != 1 {
				t.Fatalf("recovery: %v, launches=%d", err, launches)
			}
			// Repeating restoration after another interruption must keep the same snapshot.
			if err := rollbackUpdate(context.Background(), m, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
				t.Fatal(err)
			}
			assertFileContent(t, m.TargetExecutable, "old")
			assertFileContent(t, filepath.Join(install, "web", "index.html"), "old web")
			assertFileContent(t, filepath.Join(m.BackupDir, "nova"), "old")
			assertFileContent(t, sentinel, "protected")
			if _, err := os.Stat(filepath.Join(install, "README.md")); !os.IsNotExist(err) {
				t.Fatalf("new-only file survived: %v", err)
			}
			status, err := readManifest(path)
			if err != nil || status.State != PhaseFailed {
				t.Fatalf("recovery status: %+v %v", status, err)
			}
		})
	}
}

func TestReadinessFailureKeepsNewProgramAndUserData(t *testing.T) {
	install := t.TempDir()
	source := filepath.Join(updateDataDir(install), "pending-test", "denova")
	writeUpdateTestPackage(t, install, updateTestPackageContents{Executable: "old"})
	writeUpdateTestPackage(t, source, updateTestPackageContents{Executable: "new"})
	path := filepath.Join(filepath.Dir(source), manifestFileName)
	m := ApplyManifest{ID: "expected-id", State: PhaseStaged, Version: "0.2.0", InstallDir: install, SourceDir: source, BackupDir: filepath.Join(filepath.Dir(source), "backup"), TargetExecutable: filepath.Join(install, "nova"), UpdaterExecutable: filepath.Join(source, updaterExecutableName())}
	if err := writeManifest(path, m); err != nil {
		t.Fatal(err)
	}
	err := RunUpdater(context.Background(), path, UpdaterOptions{ProcessAlive: func(int) bool { return false }, ReadyTimeout: 20 * time.Millisecond, StartProcess: func(string, []string, []string) error {
		return writeJSONFile(path+".ready", "wrong-update-id", 0o600)
	}})
	if err == nil {
		t.Fatal("wrong update readiness was accepted")
	}
	assertFileContent(t, m.TargetExecutable, "new")
	assertFileContent(t, filepath.Join(m.BackupDir, "nova"), "old")
	actual, err := readManifest(path)
	if err != nil || actual.State != PhaseFailed || actual.Error == "" {
		t.Fatalf("missing failure status: %+v %v", actual, err)
	}
}

func TestInstallationLockRejectsOtherService(t *testing.T) {
	install := t.TempDir()
	unlock, err := lockInstallation(install)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	service := &Service{currentVersion: "0.1.0", executablePath: filepath.Join(install, "nova")}
	if _, err := service.lockOperation(); !errors.Is(err, ErrUpdateBusy) {
		t.Fatalf("concurrent update accepted: %v", err)
	}
}

func TestConfirmReadyRequiresTargetVersion(t *testing.T) {
	for _, version := range []string{"0.1.0", "0.2.0"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), manifestFileName)
			t.Setenv("DENOVA_UPDATE_MANIFEST", path)
			t.Setenv("DENOVA_UPDATE_ID", "readiness-test")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(Status{ID: "readiness-test", Phase: PhaseStarting, CurrentVersion: version, Version: "0.2.0"})
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			err := ConfirmReady(ctx, server.URL, "0.2.0")
			if version == "0.1.0" {
				if err == nil {
					t.Fatal("old server was accepted")
				}
				if _, err := os.Stat(path + ".ready"); !os.IsNotExist(err) {
					t.Fatalf("old server wrote ready receipt: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStatusRestoresStagedUpdateWithUsableLogPath(t *testing.T) {
	install := t.TempDir()
	path := filepath.Join(updateDataDir(install), "pending-test", manifestFileName)
	m := ApplyManifest{ID: "staged", State: PhaseStaged, Version: "0.2.0", InstallDir: install, TargetExecutable: filepath.Join(install, "nova"), LogPath: filepath.Join(filepath.Dir(path), "apply.log")}
	if err := writeManifest(path, m); err != nil {
		t.Fatal(err)
	}
	if err := writePendingManifestRef(updateDataDir(install), path); err != nil {
		t.Fatal(err)
	}
	actual, err := (&Service{currentVersion: "0.1.0", executablePath: filepath.Join(install, "nova")}).Status()
	want := Status{Distribution: "web", ID: "staged", Phase: PhaseStaged, Version: "0.2.0", CurrentVersion: "0.1.0", LogPath: ".denova-updates/pending-test/apply.log"}
	if err != nil || actual != want {
		t.Fatalf("status=%+v want=%+v err=%v", actual, want, err)
	}
}
