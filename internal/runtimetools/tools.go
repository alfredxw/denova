// Package runtimetools discovers third-party executables shipped beside the
// Denova binary without leaking the release layout into reusable modules.
package runtimetools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Set contains host-owned executable paths. Empty fields deliberately tell
// consumers to use their development fallback, such as PATH lookup.
type Set struct {
	Ripgrep string
}

// DiscoverForExecutable returns valid runtime tools from the release layout
// rooted beside executablePath.
func DiscoverForExecutable(executablePath string) Set {
	if strings.TrimSpace(executablePath) == "" {
		return Set{}
	}
	candidate := filepath.Join(filepath.Dir(executablePath), "tools", RipgrepExecutableName())
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return Set{}
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return Set{}
	}
	return Set{Ripgrep: candidate}
}

// RipgrepExecutableName returns the platform-specific bundled filename.
func RipgrepExecutableName() string {
	if runtime.GOOS == "windows" {
		return "rg.exe"
	}
	return "rg"
}
