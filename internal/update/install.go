package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"

	"denova/internal/hostruntime"
)

func (s *Service) Install(ctx context.Context) (InstallResult, error) {
	return s.InstallWithProgress(ctx, nil)
}

func (s *Service) InstallWithProgress(ctx context.Context, progress func(InstallProgress)) (InstallResult, error) {
	installCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), updateInstallTimeout)
	defer cancel()

	reportInstallProgress(progress, InstallProgress{Phase: "checking"})
	check, err := s.Check(installCtx)
	if err != nil {
		return InstallResult{}, err
	}
	if !check.UpdateAvailable {
		return InstallResult{}, errors.New("no update is available")
	}
	if check.Asset == nil {
		return InstallResult{}, errors.New("no release asset matches the current platform")
	}
	if s.executablePath == "" {
		return InstallResult{}, errors.New("cannot locate the current executable")
	}

	installDir := filepath.Dir(s.executablePath)
	updateDir := updateDataDir(installDir)
	downloadDir := filepath.Join(updateDir, "downloads")
	extractDir := filepath.Join(updateDir, "extract-"+safeUpdateName(check.LatestVersion))
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create update download directory: %w", err)
	}
	if err := os.RemoveAll(extractDir); err != nil {
		return InstallResult{}, fmt.Errorf("clear update extraction directory: %w", err)
	}

	archivePath := filepath.Join(downloadDir, check.Asset.Name)
	if err := s.downloadAsset(installCtx, updateAssetDownloadURL(check.Asset), archivePath, check.Asset.Size, progress); err != nil {
		return InstallResult{}, err
	}
	reportInstallProgress(progress, InstallProgress{Phase: "verifying", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100})
	if err := s.verifyChecksum(installCtx, check.Asset.Name, archivePath); err != nil {
		return InstallResult{}, err
	}

	reportInstallProgress(progress, InstallProgress{Phase: "extracting", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100})
	if err := extractArchive(archivePath, extractDir); err != nil {
		return InstallResult{}, err
	}
	packageRoot := filepath.Join(extractDir, releasePackageRootName)
	if fi, err := os.Stat(packageRoot); err != nil || !fi.IsDir() {
		return InstallResult{}, fmt.Errorf("invalid update package: missing %s directory", releasePackageRootName)
	}

	reportInstallProgress(progress, InstallProgress{Phase: "staging", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100})
	result, err := s.stageUpdate(packageRoot, check)
	if err == nil {
		reportInstallProgress(progress, InstallProgress{Phase: "staged", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100})
	}
	return result, err
}

func (s *Service) downloadAsset(ctx context.Context, url, target string, expectedSize int64, progress func(InstallProgress)) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("download update: release asset has no download URL")
	}
	slog.InfoContext(ctx, fmt.Sprintf("[update] Starting package download url=%s target=%s", url, target))
	downloadCtx, cancel := context.WithTimeout(ctx, updateDownloadTimeout)
	defer cancel()

	tempTarget := target + ".download"
	_ = os.Remove(tempTarget)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create update download directory: %w", err)
	}

	req, err := grab.NewRequest(tempTarget, url)
	if err != nil {
		return err
	}
	req = req.WithContext(downloadCtx)
	req.NoResume = true
	if expectedSize > 0 {
		req.Size = expectedSize
	}
	req.HTTPRequest.Header.Set("Accept", "application/octet-stream")
	req.HTTPRequest.Header.Set("User-Agent", "denova-updater")

	client := grab.NewClient()
	client.HTTPClient = s.downloadHTTPClient()
	client.UserAgent = "denova-updater"
	resp := client.Do(req)
	assetName := filepath.Base(target)
	reportInstallProgress(progress, downloadProgress(assetName, target, resp, expectedSize))

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			reportInstallProgress(progress, downloadProgress(assetName, target, resp, expectedSize))
		case <-resp.Done:
			if err := resp.Err(); err != nil {
				_ = os.Remove(tempTarget)
				return fmt.Errorf("download update package: %w", err)
			}
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				_ = os.Remove(tempTarget)
				return fmt.Errorf("save update package: %w", err)
			}
			if err := os.Rename(tempTarget, target); err != nil {
				_ = os.Remove(tempTarget)
				return fmt.Errorf("save update package: %w", err)
			}
			reportInstallProgress(progress, InstallProgress{
				Phase:           "downloading",
				AssetName:       assetName,
				ArchivePath:     target,
				DownloadedBytes: maxInt64(resp.BytesComplete(), expectedSize),
				TotalBytes:      maxInt64(resp.Size(), expectedSize),
				Percent:         100,
			})
			slog.InfoContext(ctx, fmt.Sprintf("[update] Package download completed target=%s size=%d", target, resp.BytesComplete()))
			return nil
		}
	}
}

func (s *Service) stageUpdate(packageRoot string, check CheckResult) (InstallResult, error) {
	installDir := filepath.Dir(s.executablePath)
	updateDir := updateDataDir(installDir)
	stagedRoot := filepath.Join(updateDir, "pending-"+safeUpdateName(check.LatestVersion))
	stagedDir := filepath.Join(stagedRoot, releasePackageRootName)
	backupDir := filepath.Join(updateDir, "backup-"+time.Now().Format("20060102-150405"))
	if err := validateReleasePackage(packageRoot, filepath.Base(s.executablePath), updaterExecutableName()); err != nil {
		return InstallResult{}, err
	}
	if err := os.RemoveAll(stagedRoot); err != nil {
		return InstallResult{}, err
	}
	if err := copyDir(packageRoot, stagedDir); err != nil {
		return InstallResult{}, fmt.Errorf("stage update package: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return InstallResult{}, err
	}

	manifestPath := filepath.Join(stagedRoot, manifestFileName)
	manifest := ApplyManifest{
		SourceDir:         stagedDir,
		InstallDir:        installDir,
		BackupDir:         backupDir,
		CurrentPID:        os.Getpid(),
		TargetExecutable:  s.executablePath,
		UpdaterExecutable: filepath.Join(stagedDir, updaterExecutableName()),
		RelaunchArgs:      relaunchArgs(os.Args, s.executablePath),
		Version:           check.LatestVersion,
		LogPath:           filepath.Join(stagedRoot, applyLogFileName),
	}
	if err := writeManifest(manifestPath, manifest); err != nil {
		return InstallResult{}, err
	}
	if err := writePendingManifestRef(updateDir, manifestPath); err != nil {
		return InstallResult{}, err
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[update] Update staged old=%s new=%s staged=%s manifest=%s", check.CurrentVersion, check.LatestVersion, stagedDir, manifestPath))
	return InstallResult{
		PreviousVersion:  check.CurrentVersion,
		InstalledVersion: check.LatestVersion,
		Status:           "staged",
		Staged:           true,
		ApplyReady:       true,
		RestartRequired:  true,
		BackupPath:       backupDir,
		StagedPath:       stagedDir,
		ApplyLogPath:     manifest.LogPath,
	}, nil
}

func reportInstallProgress(progress func(InstallProgress), event InstallProgress) {
	if progress == nil {
		return
	}
	progress(event)
}

func downloadProgress(assetName, archivePath string, resp *grab.Response, expectedSize int64) InstallProgress {
	total := maxInt64(resp.Size(), expectedSize)
	downloaded := resp.BytesComplete()
	percent := resp.Progress() * 100
	if total > 0 && downloaded > 0 {
		percent = float64(downloaded) / float64(total) * 100
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return InstallProgress{
		Phase:           "downloading",
		AssetName:       assetName,
		ArchivePath:     archivePath,
		DownloadedBytes: downloaded,
		TotalBytes:      total,
		Percent:         percent,
	}
}

func safeUpdateName(version string) string {
	name := strings.TrimSpace(version)
	if name == "" {
		name = time.Now().Format("20060102-150405")
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(name)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func validateReleasePackage(packageRoot, exeName, updaterName string) error {
	requiredFiles := []string{exeName, updaterName}
	for _, name := range requiredFiles {
		path := filepath.Join(packageRoot, name)
		if fi, err := os.Stat(path); err != nil {
			return fmt.Errorf("update package is missing executable %s: %w", name, err)
		} else if fi.IsDir() {
			return fmt.Errorf("update executable is a directory: %s", name)
		}
	}
	for _, name := range []string{"web", "skills"} {
		path := filepath.Join(packageRoot, name)
		if fi, err := os.Stat(path); err != nil {
			return fmt.Errorf("update package is missing directory %s: %w", name, err)
		} else if !fi.IsDir() {
			return fmt.Errorf("update package entry %s is not a directory", name)
		}
	}
	if runtimeExecutables := hostruntime.DiscoverForExecutable(filepath.Join(packageRoot, exeName)); runtimeExecutables.Ripgrep == "" {
		return fmt.Errorf("update package is missing executable bundled ripgrep")
	}
	for _, name := range []string{"LICENSE-MIT", "UNLICENSE"} {
		licensePath := filepath.Join(packageRoot, "licenses", "ripgrep", name)
		if info, err := os.Stat(licensePath); err != nil {
			return fmt.Errorf("update package is missing ripgrep license %s: %w", name, err)
		} else if !info.Mode().IsRegular() {
			return fmt.Errorf("invalid ripgrep license in update package: %s", name)
		}
	}
	return nil
}
