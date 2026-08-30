package hostruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscoverExecutablesFallsBackFromUnavailableBashOverride(t *testing.T) {
	directory := t.TempDir()
	name := "bash"
	if runtime.GOOS == "windows" {
		name = "bash.exe"
	}
	bashPath := filepath.Join(directory, name)
	if err := os.WriteFile(bashPath, []byte("test executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment := []string{"PATH=" + directory, "PATHEXT=.EXE"}
	discovered := DiscoverForExecutableWithEnvironment("", environment, filepath.Join(directory, "missing-bash"))
	discoveredInfo, discoveredErr := os.Stat(discovered.Bash)
	wantInfo, wantErr := os.Stat(bashPath)
	if discoveredErr != nil || wantErr != nil || !os.SameFile(discoveredInfo, wantInfo) {
		t.Fatalf("Bash = %q, want fallback %q", discovered.Bash, bashPath)
	}
}
