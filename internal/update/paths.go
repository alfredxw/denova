package update

import (
	"path/filepath"
	"runtime"
	"strings"
)

func updaterExecutableName() string {
	if runtime.GOOS == "windows" {
		return "nova-updater.exe"
	}
	return "nova-updater"
}

func relaunchArgs(args []string, executable string) []string {
	next := []string{executable}
	if len(args) > 1 {
		for _, arg := range args[1:] {
			if isNoOpenArg(arg) {
				continue
			}
			next = append(next, arg)
		}
	}
	return append(next, "--no-open")
}

func isNoOpenArg(arg string) bool {
	return arg == "--no-open" || arg == "-no-open" ||
		strings.HasPrefix(arg, "--no-open=") || strings.HasPrefix(arg, "-no-open=")
}

func installUpdaterTarget(installDir, stagedUpdater string) string {
	return filepath.Join(installDir, filepath.Base(stagedUpdater))
}
