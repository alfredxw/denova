package update

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	if runtime.GOOS == "windows" {
		reportInstallProgress(progress, InstallProgress{Phase: "staging", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: "正在暂存 Windows 更新"})
		result, err := s.stageWindowsUpdate(packageRoot, check)
		if err == nil {
			reportInstallProgress(progress, InstallProgress{Phase: "installed", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: result.Message})
		}
		return result, err
	}
	reportInstallProgress(progress, InstallProgress{Phase: "replacing", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: "正在替换本地文件"})
	result, err := s.installNow(packageRoot, check)
	if err == nil {
		reportInstallProgress(progress, InstallProgress{Phase: "installed", AssetName: check.Asset.Name, ArchivePath: archivePath, Percent: 100, Message: result.Message})
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

func (s *Service) installNow(packageRoot string, check CheckResult) (InstallResult, error) {
	installDir := filepath.Dir(s.executablePath)
	backupDir := filepath.Join(installDir, ".nova-updates", "backup-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("创建更新备份目录失败: %w", err)
	}

	exeName := filepath.Base(s.executablePath)
	newExe := filepath.Join(packageRoot, exeName)
	if _, err := os.Stat(newExe); err != nil {
		return InstallResult{}, fmt.Errorf("更新包缺少可执行文件 %s: %w", exeName, err)
	}
	backupExe := filepath.Join(backupDir, exeName)
	if err := replaceFile(s.executablePath, newExe, backupExe); err != nil {
		return InstallResult{}, err
	}
	if err := replaceDir(filepath.Join(installDir, "web"), filepath.Join(packageRoot, "web"), filepath.Join(backupDir, "web")); err != nil {
		return InstallResult{}, err
	}
	if err := replaceDir(filepath.Join(installDir, "skills"), filepath.Join(packageRoot, "skills"), filepath.Join(backupDir, "skills")); err != nil {
		return InstallResult{}, err
	}
	for _, name := range []string{"README.md", "CHANGELOG.md", "LICENSE"} {
		_ = copyFile(filepath.Join(packageRoot, name), filepath.Join(installDir, name), 0o644)
	}
	log.Printf("[update] 更新安装完成 old=%s new=%s install_dir=%s backup=%s", check.CurrentVersion, check.LatestVersion, installDir, backupDir)
	return InstallResult{
		PreviousVersion:  check.CurrentVersion,
		InstalledVersion: check.LatestVersion,
		Installed:        true,
		RestartRequired:  true,
		BackupPath:       backupDir,
		Message:          "更新已安装，重启 Nova 后生效",
	}, nil
}

func (s *Service) stageWindowsUpdate(packageRoot string, check CheckResult) (InstallResult, error) {
	installDir := filepath.Dir(s.executablePath)
	updateDir := filepath.Join(installDir, ".nova-updates")
	stagedDir := filepath.Join(updateDir, "pending-"+check.LatestVersion)
	backupDir := filepath.Join(updateDir, "backup-"+time.Now().Format("20060102-150405"))
	if err := os.RemoveAll(stagedDir); err != nil {
		return InstallResult{}, err
	}
	if err := copyDir(packageRoot, stagedDir); err != nil {
		return InstallResult{}, fmt.Errorf("暂存更新包失败: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return InstallResult{}, err
	}
	script := filepath.Join(updateDir, "apply-update.cmd")
	content := windowsApplyScript(os.Getpid(), stagedDir, installDir, backupDir, filepath.Base(s.executablePath))
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("写入 Windows 更新脚本失败: %w", err)
	}
	cmd := exec.Command("cmd", "/C", "start", "", "/B", script)
	if err := cmd.Start(); err != nil {
		return InstallResult{}, fmt.Errorf("启动 Windows 更新脚本失败: %w", err)
	}
	log.Printf("[update] Windows 更新已暂存 version=%s staged=%s script=%s", check.LatestVersion, stagedDir, script)
	return InstallResult{
		PreviousVersion:  check.CurrentVersion,
		InstalledVersion: check.LatestVersion,
		Installed:        true,
		RestartRequired:  true,
		BackupPath:       backupDir,
		StagedPath:       stagedDir,
		Message:          "更新已暂存，关闭 Nova 后会自动替换文件，下次启动生效",
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
