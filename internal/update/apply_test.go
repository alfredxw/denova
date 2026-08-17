package update

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
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
		Exit: func(code int) {
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
