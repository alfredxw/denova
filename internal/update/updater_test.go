package update

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/runtimetools"
)

func TestRunUpdaterAppliesStagedPackageAndRelaunches(t *testing.T) {
	installDir := t.TempDir()
	sourceDir := filepath.Join(t.TempDir(), "nova")
	backupDir := filepath.Join(installDir, ".denova-updates", "backup-test")
	targetExe := filepath.Join(installDir, "nova")
	updaterName := updaterExecutableName()
	stagedUpdater := filepath.Join(sourceDir, updaterName)
	writeUpdateTestPackage(t, installDir, updateTestPackageContents{
		Executable: "old executable", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old ripgrep", License: "old license",
	})
	writeUpdateTestPackage(t, sourceDir, updateTestPackageContents{
		Executable: "new executable", Updater: "new updater", Web: "new web", Skill: "new skill", Ripgrep: "new ripgrep", License: "new license",
	})
	manifest := ApplyManifest{
		SourceDir:         sourceDir,
		InstallDir:        installDir,
		BackupDir:         backupDir,
		CurrentPID:        12345,
		TargetExecutable:  targetExe,
		UpdaterExecutable: stagedUpdater,
		RelaunchArgs:      []string{targetExe, "--port", "8080", "--no-open"},
		Version:           "0.2.0",
		LogPath:           filepath.Join(installDir, ".denova-updates", "apply.log"),
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
	assertFileContent(t, filepath.Join(installDir, "tools", runtimetools.RipgrepExecutableName()), "new ripgrep")
	assertFileContent(t, filepath.Join(installDir, "licenses", "ripgrep", "LICENSE-MIT"), "new license MIT")
	assertFileContent(t, filepath.Join(backupDir, "nova"), "old executable")
	assertFileContent(t, filepath.Join(backupDir, updaterName), "old updater")
	assertFileContent(t, filepath.Join(backupDir, "tools", runtimetools.RipgrepExecutableName()), "old ripgrep")
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
	backupDir := filepath.Join(installDir, ".denova-updates", "backup-test")
	targetExe := filepath.Join(installDir, "nova")
	updaterName := updaterExecutableName()
	writeUpdateTestPackage(t, installDir, updateTestPackageContents{
		Executable: "new executable", Updater: "new updater", Web: "new web", Skill: "new skill", Ripgrep: "new ripgrep", License: "new license",
	})
	writeUpdateTestPackage(t, sourceDir, updateTestPackageContents{
		Executable: "staged executable", Updater: "staged updater", Web: "staged web", Skill: "staged skill", Ripgrep: "staged ripgrep", License: "staged license",
	})
	writeUpdateTestPackage(t, backupDir, updateTestPackageContents{
		Executable: "old executable", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old ripgrep", License: "old license",
	})
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
	assertFileContent(t, filepath.Join(installDir, "tools", runtimetools.RipgrepExecutableName()), "old ripgrep")
	assertFileContent(t, filepath.Join(installDir, "licenses", "ripgrep", "UNLICENSE"), "old license Unlicense")
}

type updateTestPackageContents struct {
	Executable string
	Updater    string
	Web        string
	Skill      string
	Ripgrep    string
	License    string
}

func writeUpdateTestPackage(t *testing.T, root string, content updateTestPackageContents) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "licenses", "ripgrep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nova"), []byte(content.Executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, updaterExecutableName()), []byte(content.Updater), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web", "index.html"), []byte(content.Web), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "demo", "SKILL.md"), []byte(content.Skill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tools", runtimetools.RipgrepExecutableName()), []byte(content.Ripgrep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "licenses", "ripgrep", "LICENSE-MIT"), []byte(content.License+" MIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "licenses", "ripgrep", "UNLICENSE"), []byte(content.License+" Unlicense"), 0o644); err != nil {
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
