package hostruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAgentCLIsDiscoverNativeExecutablesAndRejectIDELauncher(t *testing.T) {
	directory := t.TempDir()
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(filepath.Join(directory, "antigravity.exe"), []byte("IDE launcher"), 0700); err != nil {
			t.Fatal(err)
		}
		if found := DiscoverAntigravity([]string{"PATH=" + directory}); found != "" {
			t.Fatalf("IDE launcher selected as CLI: %q", found)
		}
	}
	for _, name := range []string{"grok", "agy"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(directory, name), []byte("native fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	environment := []string{"PATH=" + directory}
	for name, found := range map[string]string{"grok": DiscoverGrok(environment), "agy": DiscoverAntigravity(environment)} {
		filename := name
		if runtime.GOOS == "windows" {
			filename += ".exe"
		}
		if found != filepath.Join(directory, filename) {
			t.Fatalf("%s discovery = %q", name, found)
		}
	}
	if runtime.GOOS == "windows" {
		installed := filepath.Join(directory, "agy", "bin", "agy.exe")
		if err := os.MkdirAll(filepath.Dir(installed), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(installed, []byte("native fixture"), 0700); err != nil {
			t.Fatal(err)
		}
		if found := DiscoverAntigravity([]string{"LOCALAPPDATA=" + directory}); found != installed {
			t.Fatalf("installer layout = %q", found)
		}
	}
}
