// Package runtimetools discovers host executables without leaking release or
// PATH lookup details into reusable modules.
package runtimetools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Set contains host-owned executable paths. Empty fields deliberately let
// consumers decide whether absence is fatal or supports a local fallback.
type Set struct {
	Ripgrep string
	Bash    string
	Pwsh    string
}

// DiscoverForExecutable returns the bundled tools beside executablePath and
// resolves host-provided shells once at the application boundary.
func DiscoverForExecutable(executablePath string) Set {
	return Set{
		Ripgrep: discoverBundledRipgrep(executablePath),
		Bash:    discoverExecutable("bash", "bash.exe"),
		Pwsh:    discoverExecutable("pwsh", "pwsh.exe", "powershell", "powershell.exe"),
	}
}

func discoverBundledRipgrep(executablePath string) string {
	if strings.TrimSpace(executablePath) == "" {
		return ""
	}
	candidate := filepath.Join(filepath.Dir(executablePath), "tools", RipgrepExecutableName())
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return ""
	}
	return candidate
}

func discoverExecutable(candidates ...string) string {
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil && strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}

// RipgrepExecutableName returns the platform-specific bundled filename.
func RipgrepExecutableName() string {
	if runtime.GOOS == "windows" {
		return "rg.exe"
	}
	return "rg"
}
