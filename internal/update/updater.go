package update

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	defaultWaitTimeout  = 60 * time.Second
	defaultWaitInterval = 500 * time.Millisecond
)

type UpdaterOptions struct {
	WaitTimeout  time.Duration
	WaitInterval time.Duration
	ProcessAlive func(int) bool
	StartProcess func(string, []string, []string) error
	Sleep        func(time.Duration)
	LogOutput    io.Writer
}

type replaceEntry struct {
	Source   string
	Target   string
	Backup   string
	Dir      bool
	Optional bool
}

// RunUpdater applies a staged update and relaunches Nova. It is the entrypoint
// used by cmd/nova-updater.
func RunUpdater(ctx context.Context, manifestPath string, options UpdaterOptions) error {
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	logger, closeLog, err := updaterLogger(manifest.LogPath, options.LogOutput)
	if err != nil {
		return err
	}
	defer closeLog()
	logger.Printf("start manifest=%s version=%s install_dir=%s source_dir=%s", manifestPath, manifest.Version, manifest.InstallDir, manifest.SourceDir)
	if err := waitForProcessExit(ctx, manifest.CurrentPID, options, logger); err != nil {
		logger.Printf("wait failed: %v", err)
		return err
	}
	if err := applyStagedUpdate(manifest, logger); err != nil {
		logger.Printf("apply failed: %v", err)
		if rollbackErr := rollbackUpdate(manifest, logger); rollbackErr != nil {
			logger.Printf("rollback failed: %v", rollbackErr)
		}
		startErr := startRelaunch(manifest, options, logger)
		if startErr != nil {
			logger.Printf("restart after failure failed: %v", startErr)
		}
		return err
	}
	logger.Printf("apply done, relaunching nova")
	return startRelaunch(manifest, options, logger)
}

func updaterLogger(logPath string, extra io.Writer) (*log.Logger, func(), error) {
	writers := []io.Writer{}
	var closeLog func()
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return nil, func() {}, err
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, func() {}, err
		}
		writers = append(writers, f)
		closeLog = func() { _ = f.Close() }
	} else {
		closeLog = func() {}
	}
	if extra != nil {
		writers = append(writers, extra)
	}
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}
	return log.New(io.MultiWriter(writers...), "[nova-updater] ", log.LstdFlags), closeLog, nil
}

func waitForProcessExit(ctx context.Context, pid int, options UpdaterOptions, logger *log.Logger) error {
	alive := options.ProcessAlive
	if alive == nil {
		alive = processRunning
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	timeout := options.WaitTimeout
	if timeout <= 0 {
		timeout = defaultWaitTimeout
	}
	interval := options.WaitInterval
	if interval <= 0 {
		interval = defaultWaitInterval
	}
	deadline := time.Now().Add(timeout)
	for alive(pid) {
		if time.Now().After(deadline) {
			return fmt.Errorf("等待 Nova 退出超时 pid=%d", pid)
		}
		logger.Printf("waiting for nova exit pid=%d", pid)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sleep(interval)
		}
	}
	return nil
}

func applyStagedUpdate(manifest ApplyManifest, logger *log.Logger) error {
	if err := validateApplyManifest(manifest); err != nil {
		return err
	}
	if err := os.MkdirAll(manifest.BackupDir, 0o755); err != nil {
		return fmt.Errorf("创建更新备份目录失败: %w", err)
	}
	entries := updateEntries(manifest)
	for _, entry := range entries {
		if entry.Optional {
			if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
				continue
			}
		}
		if err := backupEntry(entry); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if entry.Optional {
			if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
				continue
			}
		}
		logger.Printf("replace target=%s source=%s", entry.Target, entry.Source)
		if err := copyEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

func validateApplyManifest(manifest ApplyManifest) error {
	if manifest.SourceDir == "" || manifest.InstallDir == "" || manifest.BackupDir == "" || manifest.TargetExecutable == "" || manifest.UpdaterExecutable == "" {
		return fmt.Errorf("更新清单字段不完整")
	}
	if err := validateReleasePackage(manifest.SourceDir, filepath.Base(manifest.TargetExecutable), filepath.Base(manifest.UpdaterExecutable)); err != nil {
		return err
	}
	return nil
}

func updateEntries(manifest ApplyManifest) []replaceEntry {
	installDir := manifest.InstallDir
	return []replaceEntry{
		fileEntry(manifest.SourceDir, manifest.TargetExecutable, manifest.BackupDir, filepath.Base(manifest.TargetExecutable), false),
		fileEntry(manifest.SourceDir, installUpdaterTarget(installDir, manifest.UpdaterExecutable), manifest.BackupDir, filepath.Base(manifest.UpdaterExecutable), false),
		dirEntry(manifest.SourceDir, installDir, manifest.BackupDir, "web", false),
		dirEntry(manifest.SourceDir, installDir, manifest.BackupDir, "skills", false),
		fileEntry(manifest.SourceDir, filepath.Join(installDir, "README.md"), manifest.BackupDir, "README.md", true),
		fileEntry(manifest.SourceDir, filepath.Join(installDir, "CHANGELOG.md"), manifest.BackupDir, "CHANGELOG.md", true),
		fileEntry(manifest.SourceDir, filepath.Join(installDir, "LICENSE"), manifest.BackupDir, "LICENSE", true),
	}
}

func fileEntry(sourceDir, target, backupDir, name string, optional bool) replaceEntry {
	return replaceEntry{
		Source:   filepath.Join(sourceDir, name),
		Target:   target,
		Backup:   filepath.Join(backupDir, name),
		Optional: optional,
	}
}

func dirEntry(sourceDir, installDir, backupDir, name string, optional bool) replaceEntry {
	return replaceEntry{
		Source:   filepath.Join(sourceDir, name),
		Target:   filepath.Join(installDir, name),
		Backup:   filepath.Join(backupDir, name),
		Dir:      true,
		Optional: optional,
	}
}

func backupEntry(entry replaceEntry) error {
	if _, err := os.Stat(entry.Target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(entry.Backup), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(entry.Backup); err != nil {
		return err
	}
	if err := os.Rename(entry.Target, entry.Backup); err != nil {
		return fmt.Errorf("备份更新目标失败 target=%s err=%w", entry.Target, err)
	}
	return nil
}

func copyEntry(entry replaceEntry) error {
	if entry.Dir {
		if err := copyDir(entry.Source, entry.Target); err != nil {
			return fmt.Errorf("替换目录失败 target=%s err=%w", entry.Target, err)
		}
		return nil
	}
	info, err := os.Stat(entry.Source)
	if err != nil {
		if entry.Optional && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("更新包缺少文件 %s: %w", filepath.Base(entry.Source), err)
	}
	if info.IsDir() {
		return fmt.Errorf("更新包中的文件是目录: %s", entry.Source)
	}
	if err := copyFile(entry.Source, entry.Target, info.Mode().Perm()); err != nil {
		return fmt.Errorf("替换文件失败 target=%s err=%w", entry.Target, err)
	}
	return nil
}

func rollbackUpdate(manifest ApplyManifest, logger *log.Logger) error {
	var firstErr error
	for _, entry := range updateEntries(manifest) {
		if _, err := os.Stat(entry.Backup); err != nil {
			continue
		}
		logger.Printf("rollback target=%s backup=%s", entry.Target, entry.Backup)
		if err := os.RemoveAll(entry.Target); err != nil && firstErr == nil {
			firstErr = err
			continue
		}
		if err := os.Rename(entry.Backup, entry.Target); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func startRelaunch(manifest ApplyManifest, options UpdaterOptions, logger *log.Logger) error {
	start := options.StartProcess
	if start == nil {
		start = startProcess
	}
	args := manifest.RelaunchArgs
	if len(args) == 0 {
		args = []string{manifest.TargetExecutable, "--no-open"}
	}
	logger.Printf("start nova executable=%s args=%d", manifest.TargetExecutable, len(args))
	return start(manifest.TargetExecutable, args, os.Environ())
}

func startProcess(executable string, args []string, env []string) error {
	cmdArgs := []string(nil)
	if len(args) > 1 {
		cmdArgs = args[1:]
	}
	cmd := exec.Command(executable, cmdArgs...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
