package hostruntime

import (
	"path/filepath"
	"runtime"
)

// DiscoverGrok resolves Grok Build's native binary without executing a shell.
func DiscoverGrok(environment []string) string {
	name := "grok"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{name}
	if directory := environmentValue(environment, "GROK_BIN_DIR"); directory != "" {
		candidates = append(candidates, filepath.Join(directory, name))
	}
	candidates = append(candidates, "~/.grok/bin/"+name, "~/.local/bin/"+name)
	return discoverExecutable(environment, candidates...)
}

// DiscoverAntigravity resolves the standalone agy CLI, not the IDE launcher.
func DiscoverAntigravity(environment []string) string {
	if runtime.GOOS == "windows" {
		candidates := []string{"agy.exe"}
		if local := environmentValue(environment, "LOCALAPPDATA"); local != "" {
			candidates = append(candidates, filepath.Join(local, "agy", "bin", "agy.exe"))
		}
		return discoverExecutable(environment, candidates...)
	}
	return discoverExecutable(environment, "agy", "~/.local/bin/agy")
}
