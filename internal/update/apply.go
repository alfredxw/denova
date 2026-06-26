package update

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const applyRestartDelay = 500 * time.Millisecond

type ApplyInvocation struct {
	Executable string
	Args       []string
	Env        []string
}

// ApplyScheduler starts nova-updater after the HTTP response has had time to
// flush, then exits the current Nova process.
type ApplyScheduler struct {
	Delay        time.Duration
	ManifestPath string
	Manifest     ApplyManifest
	Start        func(ApplyInvocation) error
	Exit         func(int)
	Sleep        func(time.Duration)
	Logf         func(string, ...any)
}

func (s ApplyScheduler) Schedule() error {
	manifest := s.Manifest
	if manifest.UpdaterExecutable == "" {
		return fmt.Errorf("更新清单缺少 updater_executable")
	}
	if _, err := os.Stat(manifest.UpdaterExecutable); err != nil {
		return fmt.Errorf("无法启动 updater: %w", err)
	}
	if s.ManifestPath == "" {
		return fmt.Errorf("更新清单路径不能为空")
	}
	if _, err := os.Stat(s.ManifestPath); err != nil {
		return fmt.Errorf("更新清单不存在: %w", err)
	}
	delay := s.Delay
	if delay <= 0 {
		delay = applyRestartDelay
	}
	start := s.Start
	if start == nil {
		start = startApplyProcess
	}
	exit := s.Exit
	if exit == nil {
		exit = os.Exit
	}
	sleep := s.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	logf := s.Logf
	if logf == nil {
		logf = log.Printf
	}
	invocation := ApplyInvocation{
		Executable: manifest.UpdaterExecutable,
		Args:       []string{manifest.UpdaterExecutable, "--manifest", s.ManifestPath},
		Env:        append([]string(nil), os.Environ()...),
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logf("[update] updater schedule panic recovered err=%v", recovered)
			}
		}()
		logf("[update] updater scheduled executable=%s manifest=%s delay=%s", invocation.Executable, s.ManifestPath, delay)
		sleep(delay)
		if err := start(invocation); err != nil {
			logf("[update] updater start failed executable=%s err=%v", invocation.Executable, err)
			return
		}
		exit(0)
	}()
	return nil
}

func (s *Service) Apply(ctx context.Context) (ApplyResult, error) {
	_ = ctx
	if s.executablePath == "" {
		return ApplyResult{}, fmt.Errorf("无法定位当前可执行文件")
	}
	updateDir := filepath.Join(filepath.Dir(s.executablePath), ".nova-updates")
	manifestPath, err := readPendingManifestRef(updateDir)
	if err != nil {
		return ApplyResult{}, err
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := (ApplyScheduler{ManifestPath: manifestPath, Manifest: manifest}).Schedule(); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Status: "restarting", Version: manifest.Version, LogPath: manifest.LogPath}, nil
}

func startApplyProcess(invocation ApplyInvocation) error {
	args := []string(nil)
	if len(invocation.Args) > 1 {
		args = invocation.Args[1:]
	}
	cmd := exec.Command(invocation.Executable, args...)
	cmd.Env = invocation.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
