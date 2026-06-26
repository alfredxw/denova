package update

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestRunUpdaterAppliesStagedPackageAndRelaunches(t *testing.T) {
	installDir := t.TempDir()
	sourceDir := filepath.Join(t.TempDir(), "nova")
	backupDir := filepath.Join(installDir, ".nova-updates", "backup-test")
	targetExe := filepath.Join(installDir, "nova")
	updaterName := updaterExecutableName()
	stagedUpdater := filepath.Join(sourceDir, updaterName)
	writeUpdateTestPackage(t, installDir, "old executable", "old updater", "old web", "old skill")
	writeUpdateTestPackage(t, sourceDir, "new executable", "new updater", "new web", "new skill")
	manifest := ApplyManifest{
		SourceDir:         sourceDir,
		InstallDir:        installDir,
		BackupDir:         backupDir,
		CurrentPID:        12345,
		TargetExecutable:  targetExe,
		UpdaterExecutable: stagedUpdater,
		RelaunchArgs:      []string{targetExe, "--port", "8080", "--no-open"},
		Version:           "0.2.0",
		LogPath:           filepath.Join(installDir, ".nova-updates", "apply.log"),
	}
	manifestPath := filepath.Join(t.TempDir(), manifestFileName)
	if err := writeManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	var launched string
	err := RunUpdater(context.Background(), manifestPath, UpdaterOptions{
		ProcessAlive: func(int) bool { return false },
		StartProcess: func(executable string, args []string, env []string) error {
			launched = executable
			if len(args) == 0 || args[len(args)-1] != "--no-open" {
				t.Fatalf("relaunch args should force --no-open: %#v", args)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RunUpdater failed: %v", err)
	}
	assertFileContent(t, targetExe, "new executable")
	assertFileContent(t, filepath.Join(installDir, updaterName), "new updater")
	assertFileContent(t, filepath.Join(installDir, "web", "index.html"), "new web")
	assertFileContent(t, filepath.Join(installDir, "skills", "demo", "SKILL.md"), "new skill")
	assertFileContent(t, filepath.Join(backupDir, "nova"), "old executable")
	assertFileContent(t, filepath.Join(backupDir, updaterName), "old updater")
	if launched != targetExe {
		t.Fatalf("launched executable = %q, want %q", launched, targetExe)
	}
	if _, err := os.Stat(manifest.LogPath); err != nil {
		t.Fatalf("apply log missing: %v", err)
	}
}

func TestRollbackUpdateRestoresBackups(t *testing.T) {
	installDir := t.TempDir()
	sourceDir := filepath.Join(t.TempDir(), "nova")
	backupDir := filepath.Join(installDir, ".nova-updates", "backup-test")
	targetExe := filepath.Join(installDir, "nova")
	updaterName := updaterExecutableName()
	writeUpdateTestPackage(t, installDir, "new executable", "new updater", "new web", "new skill")
	writeUpdateTestPackage(t, sourceDir, "staged executable", "staged updater", "staged web", "staged skill")
	writeUpdateTestPackage(t, backupDir, "old executable", "old updater", "old web", "old skill")
	manifest := ApplyManifest{
		SourceDir:         sourceDir,
		InstallDir:        installDir,
		BackupDir:         backupDir,
		TargetExecutable:  targetExe,
		UpdaterExecutable: filepath.Join(sourceDir, updaterName),
	}
	if err := rollbackUpdate(manifest, log.New(io.Discard, "", 0)); err != nil {
		t.Fatalf("rollbackUpdate failed: %v", err)
	}
	assertFileContent(t, targetExe, "old executable")
	assertFileContent(t, filepath.Join(installDir, updaterName), "old updater")
	assertFileContent(t, filepath.Join(installDir, "web", "index.html"), "old web")
	assertFileContent(t, filepath.Join(installDir, "skills", "demo", "SKILL.md"), "old skill")
}

func writeUpdateTestPackage(t *testing.T, root, exeContent, updaterContent, webContent, skillContent string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nova"), []byte(exeContent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, updaterExecutableName()), []byte(updaterContent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web", "index.html"), []byte(webContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "demo", "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
