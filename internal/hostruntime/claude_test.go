package hostruntime

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDiscoverClaudeWindowsInstallations(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows launcher discovery")
	}
	for _, layout := range []string{"native", "npm-native", "npm-node"} {
		t.Run(layout, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "工具 with spaces")
			t.Setenv("USERPROFILE", t.TempDir())
			write := func(name string) string {
				p := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("not executable code"), 0700); err != nil {
					t.Fatal(err)
				}
				return p
			}
			want := ClaudeLaunch{}
			switch layout {
			case "native":
				want.Executable = write("claude.exe")
			case "npm-native":
				write("claude.cmd")
				arch := runtime.GOARCH
				if arch == "amd64" {
					arch = "x64"
				}
				want.Executable = write("node_modules/@anthropic-ai/claude-code-win32-" + arch + "/claude.exe")
			case "npm-node":
				write("claude.cmd")
				want.Executable = write("node.exe")
				want.Args = []string{write("node_modules/@anthropic-ai/claude-code/cli.js")}
			}
			if got := DiscoverClaude([]string{"PATH=" + dir}); !reflect.DeepEqual(got, want) {
				t.Fatalf("discovery = %#v, want %#v", got, want)
			}
		})
	}
}
