package runtimetools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscoverForExecutableFindsBundledRipgrep(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, denovaExecutableName())
	if err := os.WriteFile(executable, []byte("denova"), 0o755); err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join(root, "tools", RipgrepExecutableName())
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toolPath, []byte("ripgrep"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := DiscoverForExecutable(executable)
	if got.Ripgrep != toolPath {
		t.Fatalf("Ripgrep = %q, want %q", got.Ripgrep, toolPath)
	}
}

func TestDiscoverForExecutableLeavesMissingRipgrepUnset(t *testing.T) {
	executable := filepath.Join(t.TempDir(), denovaExecutableName())
	if got := DiscoverForExecutable(executable); got.Ripgrep != "" {
		t.Fatalf("Ripgrep = %q, want PATH fallback", got.Ripgrep)
	}
}

func denovaExecutableName() string {
	if runtime.GOOS == "windows" {
		return "denova.exe"
	}
	return "denova"
}
