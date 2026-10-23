package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestApplySchedulerStartsUpdaterThenExits(t *testing.T) {
	dir := t.TempDir()
	updaterPath := filepath.Join(dir, updaterExecutableName())
	if err := os.WriteFile(updaterPath, []byte("updater"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, manifestFileName)
	if err := os.WriteFile(manifestPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := ApplyInvocation{
		Executable: updaterPath,
		Args:       []string{updaterPath, "--manifest", manifestPath},
	}
	started := make(chan ApplyInvocation, 1)
	cleaned := make(chan struct{})
	exited := make(chan int, 1)
	scheduler := ApplyScheduler{
		Delay:        10 * time.Millisecond,
		ManifestPath: manifestPath,
		Manifest: ApplyManifest{
			UpdaterExecutable: updaterPath,
		},
		Sleep: func(got time.Duration) {
			if got != 10*time.Millisecond {
				t.Fatalf("delay = %s", got)
			}
		},
		Start: func(got ApplyInvocation) error {
			got.Env = nil
			started <- got
			return nil
		},
		Shutdown: func() { close(cleaned) },
		Exit: func(code int) {
			select {
			case <-cleaned:
			default:
				t.Error("exit before cleanup")
			}
			exited <- code
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := scheduler.Schedule(context.Background()); err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}
	select {
	case got := <-started:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("invocation = %#v, want %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("updater was not started")
	}
	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("exit code = %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("process exit was not requested")
	}
}

// A detached launch can leave the parent's stdout/stderr pipes without a reader.
// The updater must write to the transaction log instead, otherwise its first
// write raises SIGPIPE and the handoff never completes.
func TestStartApplyProcessKeepsUpdaterOutputInTransactionLog(t *testing.T) {
	installDir := t.TempDir()
	sourceDir := filepath.Join(updateDataDir(installDir), "pending-test", releasePackageRootName)
	writeUpdateTestPackage(t, sourceDir, updateTestPackageContents{
		Executable: "new executable", Updater: "new updater", Skill: "new skill", Ripgrep: "new ripgrep", License: "new license",
	})
	manifest := ApplyManifest{
		ID: "handoff-output", State: PhaseWaiting, Version: "0.2.0",
		SourceDir:         sourceDir,
		InstallDir:        installDir,
		BackupDir:         filepath.Join(filepath.Dir(sourceDir), "backup"),
		TargetExecutable:  filepath.Join(installDir, "nova"),
		UpdaterExecutable: filepath.Join(sourceDir, updaterExecutableName()),
	}
	manifestPath := filepath.Join(filepath.Dir(sourceDir), manifestFileName)
	if err := writeManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Break only the subprocess's output pipes, never the test runner's.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	defer writer.Close()
	cmd := exec.Command(executable, "-test.run=^TestApplyProcessBrokenOutputParent$", "--", manifestPath)
	cmd.Env = append(os.Environ(), "DENOVA_TEST_APPLY_PARENT=1")
	cmd.Stdout, cmd.Stderr = writer, writer
	if err := cmd.Run(); err != nil {
		t.Fatalf("handoff with disconnected output: %v", err)
	}
	log, err := os.ReadFile(filepath.Join(filepath.Dir(sourceDir), applyLogFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "helper updater output") {
		t.Fatalf("updater output did not reach the transaction log: %q", log)
	}
}

// TestApplyProcessHelper stands in for denova-updater inside the parent test
// binary so the handoff runs on every platform.
func TestApplyProcessHelper(t *testing.T) {
	if os.Getenv("DENOVA_TEST_APPLY_HELPER") != "1" {
		t.Skip("helper process only")
	}
	path := os.Args[len(os.Args)-1]
	manifest, err := readManifest(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	fmt.Fprintln(os.Stderr, "helper updater output")
	if err := writeJSONFile(path+".accepted", manifest.ID, 0o600); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestApplySchedulerReturnsHandoffFailureWithoutExiting(t *testing.T) {
	dir := t.TempDir()
	updater := filepath.Join(dir, updaterExecutableName())
	path := filepath.Join(dir, manifestFileName)
	for _, file := range []string{updater, path} {
		if err := os.WriteFile(file, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cause := errors.New("handoff failed")
	scheduler := ApplyScheduler{ManifestPath: path, Manifest: ApplyManifest{UpdaterExecutable: updater}, Start: func(ApplyInvocation) error { return cause }, Shutdown: func() { t.Error("closed app after failed handoff") }, Exit: func(int) { t.Error("exited after failed handoff") }}
	if err := scheduler.Schedule(context.Background()); !errors.Is(err, cause) {
		t.Fatal(err)
	}
}

func TestApplyProcessBrokenOutputParent(t *testing.T) {
	if os.Getenv("DENOVA_TEST_APPLY_PARENT") != "1" {
		t.Skip("helper process only")
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	path := os.Args[len(os.Args)-1]
	if err := startApplyProcess(ApplyInvocation{
		Executable: executable,
		Args:       []string{executable, "-test.run=^TestApplyProcessHelper$", "--", path},
		Env:        append(os.Environ(), "DENOVA_TEST_APPLY_HELPER=1"),
	}); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestStartApplyProcessRejectsUnavailableOutputLog(t *testing.T) {
	install := t.TempDir()
	source := filepath.Join(updateDataDir(install), "pending-test", releasePackageRootName)
	writeUpdateTestPackage(t, source, updateTestPackageContents{Skill: "new skill"})
	path := filepath.Join(filepath.Dir(source), manifestFileName)
	if err := writeManifest(path, ApplyManifest{
		ID: "blocked-log", State: PhaseWaiting, Version: "0.2.0",
		InstallDir: install, SourceDir: source,
		BackupDir: filepath.Join(filepath.Dir(source), "backup"),
		TargetExecutable: filepath.Join(install, "nova"),
		UpdaterExecutable: filepath.Join(source, updaterExecutableName()),
	}); err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manifest.LogPath, 0o700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	err = startApplyProcess(ApplyInvocation{
		Executable: executable, Args: []string{executable, "-test.run=^TestApplyProcessHelper$", "--", path},
		Env: append(os.Environ(), "DENOVA_TEST_APPLY_HELPER=1"),
	})
	if err == nil || !strings.Contains(err.Error(), "open updater output log") {
		t.Fatalf("unavailable log did not reject launch: %v", err)
	}
	if _, err := os.Stat(path + ".accepted"); !os.IsNotExist(err) {
		t.Fatalf("updater started despite unavailable log: %v", err)
	}
}
