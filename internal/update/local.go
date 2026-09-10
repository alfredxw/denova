package update

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
)

// MaxLocalArchiveBytes bounds uploaded release archives; multipart overhead is
// allowed separately at the HTTP boundary. Extraction has its own size limit.
const MaxLocalArchiveBytes int64 = 512 << 20

var (
	ErrInvalidPackage  = errors.New("invalid release package")
	ErrPackagePlatform = errors.New("release package does not match this platform")
	ErrPackageVersion  = errors.New("release package must be newer than the running release")
	ErrPackageTooLarge = errors.New("release package exceeds the size limit")
	ErrUpdateBusy      = errors.New("another update operation is in progress")
	updateOperation    sync.Mutex
	archiveNamePattern = regexp.MustCompile(`^denova-v?([0-9]+\.[0-9]+\.[0-9]+)-(darwin|linux|windows)-(x64|arm64)\.(zip|tar\.gz)$`)
)

// InstallLocal stages a user-selected release without contacting GitHub or
// executing uploaded binaries. Version and platform come from the official
// release filename; callers must supply the original archive without renaming.
// The caller owns archive. Applying remains a separate explicit action.
func (s *Service) InstallLocal(ctx context.Context, name string, archive io.Reader) (InstallResult, error) {
	if !updateOperation.TryLock() {
		return InstallResult{}, ErrUpdateBusy
	}
	defer updateOperation.Unlock()
	if isDevVersion(s.currentVersion) {
		return InstallResult{}, ErrPackageVersion
	}
	match := archiveNamePattern.FindStringSubmatch(name)
	if len(match) != 5 {
		return InstallResult{}, ErrInvalidPackage
	}
	version := match[1]
	if match[2]+"-"+match[3] != platformKey(runtime.GOOS, runtime.GOARCH) {
		return InstallResult{}, ErrPackagePlatform
	}
	if compareVersions(s.currentVersion, version) >= 0 {
		return InstallResult{}, ErrPackageVersion
	}
	extension := ".tar.gz"
	if runtime.GOOS == "windows" {
		extension = ".zip"
	}
	if match[4] != extension[1:] {
		return InstallResult{}, ErrInvalidPackage
	}
	if s.executablePath == "" {
		return InstallResult{}, errors.New("cannot locate the current executable")
	}
	updateDir := updateDataDir(filepath.Dir(s.executablePath))
	if err := os.MkdirAll(updateDir, 0o755); err != nil {
		return InstallResult{}, err
	}
	tempDir, err := os.MkdirTemp(updateDir, "local-")
	if err != nil {
		return InstallResult{}, err
	}
	defer os.RemoveAll(tempDir)
	archivePath := filepath.Join(tempDir, name)
	file, err := os.Create(archivePath)
	if err != nil {
		return InstallResult{}, err
	}
	size, copyErr := io.Copy(file, io.LimitReader(archive, MaxLocalArchiveBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return InstallResult{}, copyErr
	}
	if closeErr != nil {
		return InstallResult{}, closeErr
	}
	if size > MaxLocalArchiveBytes {
		return InstallResult{}, ErrPackageTooLarge
	}
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	slog.InfoContext(ctx, "Preparing local update package", "asset", name, "version", version, "size", size)
	return s.stageArchive(ctx, archivePath, version, nil)
}
