package terminal

import (
	"os"
	"runtime"
	"strings"
)

// baseEnv returns the environment delta applied on top of the backend process env.
// Only variables the terminal renderer needs are added; everything else is inherited so
// the shell keeps the existing PATH and login state that CLIs such as codex or claude rely on.
func baseEnv(spec Spec) []string {
	env := []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	}
	if spec.Workspace != "" {
		env = append(env, "DENOVA_WORKSPACE="+spec.Workspace)
	}
	// Interactive CLIs decide whether wide characters are printable from LANG, and that
	// variable is frequently missing when the backend runs as a service, so default to UTF-8.
	if os.Getenv("LANG") == "" && runtime.GOOS != "windows" {
		env = append(env, "LANG=en_US.UTF-8")
	}
	return env
}

// terminalProcessEnv builds the exact child environment instead of passing through renderer
// hints from Denova's own host process. In particular, desktop/agent launchers commonly set
// NO_COLOR for their logs; leaking it into an interactive PTY makes color-aware programs such as
// Claude Code disable their UI palette even though the terminal advertises truecolor support.
func terminalProcessEnv(inherited, overrides []string) []string {
	overridden := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		key := environmentKey(entry)
		if key != "" {
			overridden[key] = struct{}{}
		}
	}

	env := make([]string, 0, len(inherited)+len(overrides))
	for _, entry := range inherited {
		key := environmentKey(entry)
		if key == "NO_COLOR" {
			continue
		}
		if _, replaced := overridden[key]; replaced {
			continue
		}
		env = append(env, entry)
	}
	for _, entry := range overrides {
		if environmentKey(entry) != "NO_COLOR" {
			env = append(env, entry)
		}
	}
	return env
}

func environmentKey(entry string) string {
	key, _, found := strings.Cut(entry, "=")
	if !found {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(key))
}

// platformDefaultShell infers the platform's interactive shell when none is configured.
func platformDefaultShell() string {
	if runtime.GOOS == "windows" {
		if comspec := strings.TrimSpace(os.Getenv("COMSPEC")); comspec != "" {
			return comspec
		}
		return "powershell.exe"
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/bash"
}
