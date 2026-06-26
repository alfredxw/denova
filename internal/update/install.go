package update

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

func (s *Service) Install(ctx context.Context) (InstallResult, error) {
	return s.InstallWithProgress(ctx, nil)
}

func (s *Service) InstallWithProgress(ctx context.Context, progress func(InstallProgress)) (InstallResult, error) {
	installCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), updateInstallTimeout)
	defer cancel()

	reportInstallProgress(progress, InstallProgress{Phase: "checking", Message: "正在检查更新"})
	check, err := s.Check(installCtx)
	if err != nil {
		return InstallResult{}, err
	}
	if !check.UpdateAvailable {
		return InstallResult{}, errors.New(check.Message)
	}
	if check.Asset == nil {
		return InstallResult{}, errors.New(check.Message)
	}
	if s.executablePath == "" {
		return InstallResult{}, errors.New("无法定位当前可执行文件")
	}

	installDir := filepath.Dir(s.executablePath)
	updateDir := filepath.Join(installDir, ".nova-updates")
	downloadDir := filepath.Join(updateDir, "downloads")
	extractDir := filepath.Join(updateDir, "extract-"+safeUpdateName(check.LatestVersion))
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("创建更新下载目录失败: %w", err)
	}
	if err := os.RemoveAll(extractDir); err != nil {
		return InstallResult{}, fmt.Errorf("清理更新解压目录失败: %w", err)
	}

	archivePath := filepath.Join(downloadDir, check.Asset.Name)
	if err := s.downloadAsset(installCtx, updateAssetDownloadURL(check.Asset), archivePath, check.Asset.Size, progress); err != nil {
		return InstallResult{}, err
	}
	reportInstallProgress(progress, InstallProgress{Phase: "verifying", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: "正在校验更新包"})
	if err := s.verifyChecksum(installCtx, check.Asset.Name, archivePath); err != nil {
		return InstallResult{}, err
	}

	reportInstallProgress(progress, InstallProgress{Phase: "extracting", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: "正在解压更新包"})
	if err := extractArchive(archivePath, extractDir); err != nil {
		return InstallResult{}, err
	}
	packageRoot := filepath.Join(extractDir, "nova")
	if fi, err := os.Stat(packageRoot); err != nil || !fi.IsDir() {
		return InstallResult{}, fmt.Errorf("更新包结构无效，缺少 nova 目录")
	}

	reportInstallProgress(progress, InstallProgress{Phase: "staging", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: "正在暂存更新"})
	result, err := s.stageUpdate(packageRoot, check)
	if err == nil {
		reportInstallProgress(progress, InstallProgress{Phase: "staged", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: result.Message})
	}
	return result, err
}

func (s *Service) downloadAsset(ctx context.Context, url, target string, expectedSize int64, progress func(InstallProgress)) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("下载更新包失败: Release 资源缺少下载地址")
	}
	log.Printf("[update] 开始下载更新包 url=%s target=%s", url, target)
	downloadCtx, cancel := context.WithTimeout(ctx, updateDownloadTimeout)
	defer cancel()

	tempTarget := target + ".download"
	_ = os.Remove(tempTarget)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("创建更新下载目录失败: %w", err)
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
	req.HTTPRequest.Header.Set("User-Agent", "nova-updater")

	client := grab.NewClient()
	client.HTTPClient = s.downloadHTTPClient()
	client.UserAgent = "nova-updater"
	resp := client.Do(req)
	assetName := filepath.Base(target)
	reportInstallProgress(progress, downloadProgress(assetName, target, resp, expectedSize, "正在下载更新包"))

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			reportInstallProgress(progress, downloadProgress(assetName, target, resp, expectedSize, "正在下载更新包"))
		case <-resp.Done:
			if err := resp.Err(); err != nil {
				_ = os.Remove(tempTarget)
				return fmt.Errorf("下载更新包失败: %w", err)
			}
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				_ = os.Remove(tempTarget)
				return fmt.Errorf("保存更新包失败: %w", err)
			}
			if err := os.Rename(tempTarget, target); err != nil {
				_ = os.Remove(tempTarget)
				return fmt.Errorf("保存更新包失败: %w", err)
			}
			reportInstallProgress(progress, InstallProgress{
				Phase:           "downloading",
				AssetName:       assetName,
				ArchivePath:     target,
				DownloadedBytes: maxInt64(resp.BytesComplete(), expectedSize),
				TotalBytes:      maxInt64(resp.Size(), expectedSize),
				Percent:         100,
				Message:         "更新包下载完成",
			})
			log.Printf("[update] 更新包下载完成 target=%s size=%d", target, resp.BytesComplete())
			return nil
		}
	}
}

func (s *Service) stageUpdate(packageRoot string, check CheckResult) (InstallResult, error) {
	installDir := filepath.Dir(s.executablePath)
	updateDir := filepath.Join(installDir, ".nova-updates")
	stagedRoot := filepath.Join(updateDir, "pending-"+safeUpdateName(check.LatestVersion))
	stagedDir := filepath.Join(stagedRoot, "nova")
	backupDir := filepath.Join(updateDir, "backup-"+time.Now().Format("20060102-150405"))
	if err := validateReleasePackage(packageRoot, filepath.Base(s.executablePath), updaterExecutableName()); err != nil {
		return InstallResult{}, err
	}
	if err := os.RemoveAll(stagedRoot); err != nil {
		return InstallResult{}, err
	}
	if err := copyDir(packageRoot, stagedDir); err != nil {
		return InstallResult{}, fmt.Errorf("暂存更新包失败: %w", err)
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
	log.Printf("[update] 更新已暂存 old=%s new=%s staged=%s manifest=%s", check.CurrentVersion, check.LatestVersion, stagedDir, manifestPath)
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
		Message:          "更新已暂存，重启并安装后生效",
	}, nil
}

func reportInstallProgress(progress func(InstallProgress), event InstallProgress) {
	if progress == nil {
		return
	}
	progress(event)
}

func downloadProgress(assetName, archivePath string, resp *grab.Response, expectedSize int64, message string) InstallProgress {
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
		Message:         message,
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
			return fmt.Errorf("更新包缺少可执行文件 %s: %w", name, err)
		} else if fi.IsDir() {
			return fmt.Errorf("更新包中的可执行文件是目录: %s", name)
		}
	}
	for _, name := range []string{"web", "skills"} {
		path := filepath.Join(packageRoot, name)
		if fi, err := os.Stat(path); err != nil {
			return fmt.Errorf("更新包缺少目录 %s: %w", name, err)
		} else if !fi.IsDir() {
			return fmt.Errorf("更新包中的 %s 不是目录", name)
		}
	}
	return nil
}
