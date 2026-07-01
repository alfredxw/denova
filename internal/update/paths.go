package update

import (
	"path/filepath"
	"runtime"
	"strings"
)

const (
	updateDataDirName       = ".denova-updates"
	legacyUpdateDataDirName = ".nova-updates"
	releasePackageRootName  = "denova"
)

func updaterExecutableName() string {
	if runtime.GOOS == "windows" {
		return "denova-updater.exe"
	}
	return "denova-updater"
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

func updateDataDir(installDir string) string {
	return filepath.Join(installDir, updateDataDirName)
}

func legacyUpdateDataDir(installDir string) string {
	return filepath.Join(installDir, legacyUpdateDataDirName)
}
