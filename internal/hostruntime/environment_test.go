package hostruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfiguredLoginShellFallsBackFromForeignHostPath(t *testing.T) {
	fallback, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", fallback)
	configured := filepath.Join(t.TempDir(), "missing-shell")
	got, err := resolveConfiguredLoginShell(configured)
	if err != nil {
		t.Fatal(err)
	}
	if got != fallback {
		t.Fatalf("shell = %q, want current-host fallback %q", got, fallback)
	}
}
