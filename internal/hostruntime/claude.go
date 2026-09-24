package hostruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ClaudeLaunch is a host-local invocation. Args contains only a resolved npm
// entrypoint when needed; neither shell shims nor portable settings are used.
type ClaudeLaunch struct {
	Executable string
	Args       []string
}

func DiscoverClaude(environment []string) ClaudeLaunch {
	if runtime.GOOS != "windows" {
		return ClaudeLaunch{Executable: discoverExecutable(environment, "claude", "~/.local/bin/claude")}
	}
	if native := discoverExecutable(environment, "claude.exe", "~/.local/bin/claude.exe"); native != "" {
		return ClaudeLaunch{Executable: native}
	}
	shim := discoverExecutable(environment, "claude.cmd")
	if shim == "" {
		return ClaudeLaunch{}
	}
	root := filepath.Join(filepath.Dir(shim), "node_modules", "@anthropic-ai")
	// Current npm releases install a native platform package. Resolve that
	// binary directly, including installations whose postinstall was skipped.
	for _, candidate := range []string{
		filepath.Join(root, "claude-code-win32-"+runtime.GOARCH, "claude.exe"),
		filepath.Join(root, "claude-code", "bin", "claude.exe"),
	} {
		candidate = strings.ReplaceAll(candidate, "win32-amd64", "win32-x64")
		if executableFile(candidate) {
			return ClaudeLaunch{Executable: candidate}
		}
	}
	entry := filepath.Join(filepath.Dir(shim), "node_modules", "@anthropic-ai", "claude-code", "cli.js")
	info, err := os.Stat(entry)
	if err != nil || !info.Mode().IsRegular() {
		return ClaudeLaunch{}
	}
	node := discoverExecutable(environment, filepath.Join(filepath.Dir(shim), "node.exe"), "node.exe")
	if node == "" {
		return ClaudeLaunch{}
	}
	return ClaudeLaunch{Executable: node, Args: []string{entry}}
}
