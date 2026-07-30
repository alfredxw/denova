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

func TestDiscoverForExecutableResolvesShellsFromPATH(t *testing.T) {
	toolsDir := t.TempDir()
	bashName := "bash"
	pwshName := "pwsh"
	if runtime.GOOS == "windows" {
		bashName += ".exe"
		pwshName += ".exe"
	}
	bashPath := filepath.Join(toolsDir, bashName)
	pwshPath := filepath.Join(toolsDir, pwshName)
	for _, path := range []string{bashPath, pwshPath} {
		if err := os.WriteFile(path, []byte("runtime tool"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", toolsDir)

	got := DiscoverForExecutable("")
	if got.Bash != bashPath {
		t.Fatalf("Bash = %q, want %q", got.Bash, bashPath)
	}
	if got.Pwsh != pwshPath {
		t.Fatalf("Pwsh = %q, want %q", got.Pwsh, pwshPath)
	}
}

func TestDiscoverForExecutableLeavesMissingShellsUnset(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	got := DiscoverForExecutable("")
	if got.Bash != "" || got.Pwsh != "" {
		t.Fatalf("missing shells = Bash:%q Pwsh:%q, want empty", got.Bash, got.Pwsh)
	}
}

func TestDiscoverForExecutableUsesCapturedEnvironmentAndBashOverride(t *testing.T) {
	toolsDir := t.TempDir()
	bashPath := filepath.Join(toolsDir, "custom-bash")
	pwshName := "pwsh"
	if runtime.GOOS == "windows" {
		bashPath += ".exe"
		pwshName += ".exe"
	}
	pwshPath := filepath.Join(toolsDir, pwshName)
	for _, path := range []string{bashPath, pwshPath} {
		if err := os.WriteFile(path, []byte("runtime tool"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := DiscoverForExecutableWithEnvironment("", []string{"PATH=" + toolsDir}, bashPath)
	if got.Bash != bashPath || got.Pwsh != pwshPath {
		t.Fatalf("captured environment discovery = %#v", got)
	}
}

func denovaExecutableName() string {
	if runtime.GOOS == "windows" {
		return "denova.exe"
	}
	return "denova"
}
