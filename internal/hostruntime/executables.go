// Package hostruntime resolves host executables and exported shell state
// without leaking PATH or release-layout details into callers.
package hostruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Set contains host-owned executable paths. Empty fields deliberately let
// consumers decide whether absence is fatal or supports a local fallback.
type Executables struct {
	Ripgrep string
	Bash    string
	Pwsh    string
}

// DiscoverForExecutable returns the bundled tools beside executablePath and
// resolves host-provided shells once at the application boundary.
func DiscoverForExecutable(executablePath string) Executables {
	return DiscoverForExecutableWithEnvironment(executablePath, os.Environ(), "")
}

// DiscoverForExecutableWithEnvironment resolves host tools against the same
// immutable environment that Agent processes will receive. This is essential
// for GUI launches whose process PATH differs from the login shell PATH.
func DiscoverForExecutableWithEnvironment(executablePath string, environment []string, bashOverride string) Executables {
	bashCandidates := []string{"bash", "bash.exe"}
	if strings.TrimSpace(bashOverride) != "" {
		bashCandidates = []string{bashOverride}
	}
	return Executables{
		Ripgrep: discoverBundledRipgrep(executablePath),
		Bash:    discoverExecutable(environment, bashCandidates...),
		Pwsh:    discoverExecutable(environment, "pwsh", "pwsh.exe", "powershell", "powershell.exe"),
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

func discoverExecutable(environment []string, candidates ...string) string {
	for _, candidate := range candidates {
		if path := executableInEnvironment(candidate, environment); path != "" {
			return path
		}
	}
	return ""
}

func executableInEnvironment(candidate string, environment []string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	if expanded, ok := expandHomeExecutable(candidate); ok {
		candidate = expanded
	}
	if filepath.IsAbs(candidate) || strings.ContainsRune(candidate, filepath.Separator) {
		if executableFile(candidate) {
			return candidate
		}
		return ""
	}
	pathValue := environmentValue(environment, "PATH")
	extensions := []string{""}
	if runtime.GOOS == "windows" && filepath.Ext(candidate) == "" {
		extensions = filepath.SplitList(environmentValue(environment, "PATHEXT"))
		if len(extensions) == 0 {
			extensions = []string{".COM", ".EXE", ".BAT", ".CMD"}
		}
	}
	for _, directory := range filepath.SplitList(pathValue) {
		if directory == "" {
			continue
		}
		for _, extension := range extensions {
			path := filepath.Join(directory, candidate+extension)
			if executableFile(path) {
				return path
			}
		}
	}
	return ""
}

func expandHomeExecutable(value string) (string, bool) {
	if value != "~" && !strings.HasPrefix(value, "~/") && !strings.HasPrefix(value, `~\`) {
		return value, false
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return value, false
	}
	if value == "~" {
		return home, true
	}
	return filepath.Join(home, filepath.FromSlash(strings.TrimLeft(value[1:], `/\`))), true
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() &&
		(runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0)
}

func environmentValue(environment []string, name string) string {
	for index := len(environment) - 1; index >= 0; index-- {
		key, value, ok := strings.Cut(environment[index], "=")
		if ok && strings.EqualFold(key, name) {
			return value
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
