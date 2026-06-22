package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nova/internal/buildinfo"
)

const (
	githubAPIBase         = "https://api.github.com/repos"
	updateInstallTimeout  = 30 * time.Minute
	updateDownloadTimeout = 20 * time.Minute
)

type Service struct {
	repository     string
	currentVersion string
	httpClient     *http.Client
	executablePath string
	githubAPIBase  string
}

func NewService() *Service {
	exe, _ := os.Executable()
	return &Service{
		repository:     buildinfo.Repository,
		currentVersion: buildinfo.Version,
		httpClient:     &http.Client{Timeout: 60 * time.Second},
		executablePath: exe,
		githubAPIBase:  githubAPIBase,
	}
}

func (s *Service) Check(ctx context.Context) (CheckResult, error) {
	release, err := s.latestRelease(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	platform := platformKey(runtime.GOOS, runtime.GOARCH)
	asset := selectAsset(release.Assets, platform)
	current := s.currentVersion
	latest := normalizeVersion(release.TagName)
	updateAvailable := !isDevVersion(current) && latest != "" && compareVersions(current, latest) < 0
	result := CheckResult{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: updateAvailable,
		CanInstall:      updateAvailable && asset != nil,
		Platform:        platform,
		ReleaseURL:      release.HTMLURL,
		PublishedAt:     release.PublishedAt,
		ReleaseNotes:    release.Body,
		Message:         "当前已是最新版本",
	}
	if asset != nil {
		result.Asset = &Asset{Name: asset.Name, Size: asset.Size, DownloadURL: asset.DownloadURL, BrowserDownloadURL: asset.BrowserDownloadURL}
	}
	switch {
	case isDevVersion(current):
		result.Message = "开发版本不支持应用内安装更新，请使用 Release 包运行后再检查"
	case latest == "":
		result.Message = "GitHub Release 未提供版本号"
	case !updateAvailable:
		result.Message = "当前已是最新版本"
	case asset == nil:
		result.Message = fmt.Sprintf("找到新版本，但没有匹配当前平台的安装包: %s", platform)
	default:
		result.Message = "发现可用更新"
	}
	return result, nil
}

func (s *Service) Install(ctx context.Context) (InstallResult, error) {
	installCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), updateInstallTimeout)
	defer cancel()

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

	workDir, err := os.MkdirTemp("", "nova-update-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("创建更新临时目录失败: %w", err)
	}
	defer os.RemoveAll(workDir)

	archivePath := filepath.Join(workDir, check.Asset.Name)
	if err := s.downloadAsset(installCtx, updateAssetDownloadURL(check.Asset), archivePath); err != nil {
		return InstallResult{}, err
	}
	if err := s.verifyChecksum(installCtx, check.Asset.Name, archivePath); err != nil {
		return InstallResult{}, err
	}

	extractDir := filepath.Join(workDir, "extract")
	if err := extractArchive(archivePath, extractDir); err != nil {
		return InstallResult{}, err
	}
	packageRoot := filepath.Join(extractDir, "nova")
	if fi, err := os.Stat(packageRoot); err != nil || !fi.IsDir() {
		return InstallResult{}, fmt.Errorf("更新包结构无效，缺少 nova 目录")
	}

	if runtime.GOOS == "windows" {
		return s.stageWindowsUpdate(packageRoot, check)
	}
	return s.installNow(packageRoot, check)
}

func (s *Service) latestRelease(ctx context.Context) (githubRelease, error) {
	url := s.githubLatestReleaseURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nova-update-checker")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("检查 GitHub Release 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return githubRelease{}, fmt.Errorf("检查 GitHub Release 失败: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("解析 GitHub Release 响应失败: %w", err)
	}
	return release, nil
}

func (s *Service) downloadAsset(ctx context.Context, url, target string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("下载更新包失败: Release 资源缺少下载地址")
	}
	log.Printf("[update] 开始下载更新包 url=%s target=%s", url, target)
	downloadCtx, cancel := context.WithTimeout(ctx, updateDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "nova-updater")
	resp, err := s.downloadHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("下载更新包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载更新包失败: HTTP %d", resp.StatusCode)
	}
	tempTarget := target + ".download"
	_ = os.Remove(tempTarget)
	out, err := os.OpenFile(tempTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("创建更新包文件失败: %w", err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(tempTarget)
		return fmt.Errorf("写入更新包失败: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tempTarget)
		return fmt.Errorf("关闭更新包文件失败: %w", err)
	}
	if err := os.Rename(tempTarget, target); err != nil {
		_ = os.Remove(tempTarget)
		return fmt.Errorf("保存更新包失败: %w", err)
	}
	log.Printf("[update] 更新包下载完成 target=%s content_length=%d", target, resp.ContentLength)
	return nil
}

func (s *Service) verifyChecksum(ctx context.Context, assetName, archivePath string) error {
	release, err := s.latestRelease(ctx)
	if err != nil {
		return err
	}
	checksumAsset := selectChecksumAsset(release.Assets)
	if checksumAsset == nil {
		log.Printf("[update] Release 未提供 checksums.txt，跳过校验 asset=%s", assetName)
		return nil
	}
	temp, err := os.CreateTemp("", "nova-checksums-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Close(); err != nil {
		return err
	}
	if err := s.downloadAsset(ctx, githubAssetDownloadURL(*checksumAsset), temp.Name()); err != nil {
		return err
	}
	expected, err := checksumForAsset(temp.Name(), assetName)
	if err != nil {
		return err
	}
	actual, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("更新包校验失败: expected=%s actual=%s", expected, actual)
	}
	return nil
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
	cmd := exec.Command("cmd", "/C", "start", "/B", script)
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

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	HTMLURL     string        `json:"html_url"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadURL        string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func platformKey(goos, goarch string) string {
	arch := goarch
	if arch == "amd64" {
		arch = "x64"
	}
	return goos + "-" + arch
}

func selectAsset(assets []githubAsset, platform string) *githubAsset {
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if strings.Contains(name, strings.ToLower(platform)) && strings.HasPrefix(name, "nova-") &&
			(strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip")) {
			return &assets[i]
		}
	}
	return nil
}

func selectChecksumAsset(assets []githubAsset) *githubAsset {
	for i := range assets {
		if strings.EqualFold(assets[i].Name, "checksums.txt") {
			return &assets[i]
		}
	}
	return nil
}

func (s *Service) githubLatestReleaseURL() string {
	base := strings.TrimRight(s.githubAPIBase, "/")
	if base == "" {
		base = githubAPIBase
	}
	return base + "/" + strings.Trim(s.repository, "/") + "/releases/latest"
}

func (s *Service) downloadHTTPClient() *http.Client {
	if s.httpClient == nil {
		return &http.Client{}
	}
	client := *s.httpClient
	client.Timeout = 0
	return &client
}

func updateAssetDownloadURL(asset *Asset) string {
	if asset == nil {
		return ""
	}
	if strings.TrimSpace(asset.BrowserDownloadURL) != "" {
		return asset.BrowserDownloadURL
	}
	return asset.DownloadURL
}

func githubAssetDownloadURL(asset githubAsset) string {
	if strings.TrimSpace(asset.BrowserDownloadURL) != "" {
		return asset.BrowserDownloadURL
	}
	return asset.DownloadURL
}

func checksumForAsset(path, assetName string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksums.txt 中缺少 %s", assetName)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractArchive(archivePath, targetDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, targetDir)
	}
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, targetDir)
	}
	return fmt.Errorf("不支持的更新包格式: %s", filepath.Base(archivePath))
}

func extractZip(archivePath, targetDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开 zip 更新包失败: %w", err)
	}
	defer reader.Close()
	for _, f := range reader.File {
		target, err := safeJoin(targetDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if err := writeExtractedFile(target, rc, f.FileInfo().Mode()); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}

func extractTarGz(archivePath, targetDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开 tar.gz 更新包失败: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("读取 gzip 更新包失败: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(targetDir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := writeExtractedFile(target, reader, header.FileInfo().Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func safeJoin(root, name string) (string, error) {
	target := filepath.Join(root, filepath.Clean(name))
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("更新包包含非法路径: %s", name)
	}
	return target, nil
}

func writeExtractedFile(target string, reader io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, reader)
	return err
}

func replaceFile(target, source, backup string) error {
	if _, err := os.Stat(target); err == nil {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("备份当前可执行文件失败: %w", err)
		}
	}
	if err := copyFile(source, target, 0o755); err != nil {
		_ = os.Rename(backup, target)
		return fmt.Errorf("替换可执行文件失败: %w", err)
	}
	return nil
}

func replaceDir(target, source, backup string) error {
	if _, err := os.Stat(source); err != nil {
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("备份目录失败 target=%s err=%w", target, err)
		}
	}
	if err := copyDir(source, target); err != nil {
		_ = os.Rename(backup, target)
		return fmt.Errorf("替换目录失败 target=%s err=%w", target, err)
	}
	return nil
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDir(source, target string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, dest, info.Mode().Perm())
	})
}

func windowsApplyScript(pid int, source, target, backup, exeName string) string {
	return fmt.Sprintf(`@echo off
setlocal
set "PID=%d"
set "SRC=%s"
set "DST=%s"
set "BACKUP=%s"
:wait
tasklist /FI "PID eq %%PID%%" | find "%%PID%%" >NUL
if not errorlevel 1 (
  timeout /t 1 /nobreak >NUL
  goto wait
)
if not exist "%%BACKUP%%" mkdir "%%BACKUP%%"
if exist "%%DST%%\%s" move /Y "%%DST%%\%s" "%%BACKUP%%\%s" >NUL
if exist "%%DST%%\web" rmdir /S /Q "%%DST%%\web"
if exist "%%DST%%\skills" rmdir /S /Q "%%DST%%\skills"
xcopy /E /I /Y "%%SRC%%\web" "%%DST%%\web" >NUL
xcopy /E /I /Y "%%SRC%%\skills" "%%DST%%\skills" >NUL
copy /Y "%%SRC%%\%s" "%%DST%%\%s" >NUL
copy /Y "%%SRC%%\README.md" "%%DST%%\README.md" >NUL 2>NUL
copy /Y "%%SRC%%\CHANGELOG.md" "%%DST%%\CHANGELOG.md" >NUL 2>NUL
copy /Y "%%SRC%%\LICENSE" "%%DST%%\LICENSE" >NUL 2>NUL
endlocal
`, pid, source, target, backup, exeName, exeName, exeName, exeName, exeName)
}
