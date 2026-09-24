package hostruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CodexHome resolves the user's existing configuration and credential store.
// It is host-local, never copied into Project data or a portable settings file.
func CodexHome(environment []string) (string, error) {
	home := strings.TrimSpace(environmentValue(environment, "CODEX_HOME"))
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(userHome, ".codex")
	} else if expanded, ok := expandHomePath(home); ok {
		home = expanded
	}
	return filepath.Abs(home)
}

// DiscoverCodex resolves a native executable without launching a shell wrapper.
// npm's Windows shim cannot be passed directly to exec.Command; resolve its
// installed optional binary package instead. No discovery starts a process.
func DiscoverCodex(environment []string) string {
	if runtime.GOOS != "windows" {
		return discoverExecutable(environment, "codex")
	}
	if native := discoverExecutable(environment, "codex.exe"); native != "" {
		return native
	}
	shim := discoverExecutable(environment, "codex.cmd")
	if shim == "" {
		return ""
	}
	platform, target := "win32-x64", "x86_64-pc-windows-msvc"
	if runtime.GOARCH == "arm64" {
		platform, target = "win32-arm64", "aarch64-pc-windows-msvc"
	}
	packageRoot := filepath.Join(filepath.Dir(shim), "node_modules", "@openai", "codex")
	for _, vendor := range []string{
		filepath.Join(packageRoot, "node_modules", "@openai", "codex-"+platform, "vendor", target),
		filepath.Join(packageRoot, "vendor", target),
	} {
		// Current npm releases use bin; earlier releases used codex.
		for _, directory := range []string{"bin", "codex"} {
			candidate := filepath.Join(vendor, directory, "codex.exe")
			if executableFile(candidate) {
				return candidate
			}
		}
	}
	return ""
}
