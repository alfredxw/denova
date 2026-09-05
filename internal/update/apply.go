package update

import (
	"context"
	"fmt"
	"log/slog"
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

// ApplyScheduler starts denova-updater after the HTTP response has had time to
// flush, then exits the current Denova process.
type ApplyScheduler struct {
	Delay        time.Duration
	ManifestPath string
	Manifest     ApplyManifest
	Start        func(ApplyInvocation) error
	Exit         func(int)
	Sleep        func(time.Duration)
	Logger       *slog.Logger
}

func (s ApplyScheduler) Schedule(ctx context.Context) error {
	manifest := s.Manifest
	if manifest.UpdaterExecutable == "" {
		return fmt.Errorf("update manifest is missing updater_executable")
	}
	if _, err := os.Stat(manifest.UpdaterExecutable); err != nil {
		return fmt.Errorf("start updater: %w", err)
	}
	if s.ManifestPath == "" {
		return fmt.Errorf("update manifest path is required")
	}
	if _, err := os.Stat(s.ManifestPath); err != nil {
		return fmt.Errorf("update manifest is missing: %w", err)
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
	if ctx == nil {
		ctx = context.Background()
	}
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	invocation := ApplyInvocation{
		Executable: manifest.UpdaterExecutable,
		Args:       []string{manifest.UpdaterExecutable, "--manifest", s.ManifestPath},
		Env:        append([]string(nil), os.Environ()...),
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(ctx, "updater_schedule_panic_recovered", "error", recovered)
			}
		}()
		logger.InfoContext(ctx, "updater_scheduled", "executable", invocation.Executable, "manifest", s.ManifestPath, "delay", delay)
		sleep(delay)
		if err := start(invocation); err != nil {
			logger.ErrorContext(ctx, "updater_start_failed", "executable", invocation.Executable, "error", err)
			return
		}
		exit(0)
	}()
	return nil
}

func (s *Service) Apply(ctx context.Context) (ApplyResult, error) {
	_ = ctx
	if s.executablePath == "" {
		return ApplyResult{}, fmt.Errorf("cannot locate the current executable")
	}
	installDir := filepath.Dir(s.executablePath)
	manifestPath, err := readPendingManifestRef(updateDataDir(installDir))
	if err != nil {
		if legacyPath, legacyErr := readPendingManifestRef(legacyUpdateDataDir(installDir)); legacyErr == nil {
			manifestPath = legacyPath
			err = nil
		}
	}
	if err != nil {
		return ApplyResult{}, err
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := (ApplyScheduler{ManifestPath: manifestPath, Manifest: manifest}).Schedule(ctx); err != nil {
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
