package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const applyRestartDelay = 500 * time.Millisecond
const applyHandoffTimeout = 5 * time.Second

type ApplyInvocation struct {
	Executable string
	Args       []string
	Env        []string
}

// ApplyScheduler confirms the updater handoff before returning to HTTP, then
// drains application work and exits after the response has had time to flush.
type ApplyScheduler struct {
	Shutdown     func()
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
	if err := start(invocation); err != nil {
		return err
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(ctx, "updater_schedule_panic_recovered", "error", recovered)
			}
		}()
		logger.InfoContext(ctx, "updater_scheduled", "executable", invocation.Executable, "manifest", s.ManifestPath, "delay", delay)
		sleep(delay)
		if s.Shutdown != nil {
			s.Shutdown()
		}
		exit(0)
	}()
	return nil
}

// Apply hands the observed staged transaction to the updater. expectedID must
// match Status.ID; only an acknowledged handoff may trigger shutdown and exit.
func (s *Service) Apply(ctx context.Context, shutdown func(), port int, expectedID string) (ApplyResult, error) {
	if !updateOperation.TryLock() {
		return ApplyResult{}, ErrUpdateBusy
	}
	defer updateOperation.Unlock()
	unlock, err := s.lockOperation()
	if err != nil {
		return ApplyResult{}, err
	}
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()
	if s.executablePath == "" {
		return ApplyResult{}, fmt.Errorf("cannot locate the current executable")
	}
	installDir := filepath.Dir(s.executablePath)
	manifestPath, err := readPendingManifestRef(updateDataDir(installDir))
	if err != nil {
		return ApplyResult{}, err
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return ApplyResult{}, err
	}
	if manifest.State != PhaseStaged || manifest.ID != expectedID {
		return ApplyResult{}, fmt.Errorf("no staged update to apply")
	}
	manifest.CurrentPID = os.Getpid()
	manifest.RelaunchArgs = relaunchArgs(os.Args, s.executablePath)
	if port > 0 {
		manifest.RelaunchArgs = append(manifest.RelaunchArgs, "--port", fmt.Sprint(port))
	}
	if err := setPhase(manifestPath, &manifest, PhaseWaiting, nil); err != nil {
		return ApplyResult{}, err
	}
	unlock()
	locked = false
	if err := (ApplyScheduler{ManifestPath: manifestPath, Manifest: manifest, Shutdown: shutdown}).Schedule(ctx); err != nil {
		// startApplyProcess kills and reaps an unacknowledged child before
		// returning. No files changed and the verified package can be retried.
		unlock, lockErr := lockInstallation(installDir)
		if lockErr != nil {
			return ApplyResult{}, errors.Join(err, lockErr)
		}
		defer unlock()
		return ApplyResult{}, errors.Join(err, setPhase(manifestPath, &manifest, PhaseStaged, err))
	}
	return ApplyResult{Status: "restarting", ID: manifest.ID, Version: manifest.Version, LogPath: manifest.LogPath}, nil
}

func startApplyProcess(invocation ApplyInvocation) error {
	args := []string(nil)
	if len(invocation.Args) > 1 {
		args = invocation.Args[1:]
	}
	pathToManifest := invocation.Args[len(invocation.Args)-1]
	if err := os.Remove(pathToManifest + ".accepted"); err != nil && !os.IsNotExist(err) {
		return err
	}
	manifest, err := readManifest(pathToManifest)
	if err != nil {
		return err
	}
	cmd := exec.Command(invocation.Executable, args...)
	cmd.Env = invocation.Env
	// The updater outlives this launch terminal. Inherited stdout/stderr may be
	// pipes whose reader is already gone, and a child's first write to such a pipe
	// raises SIGPIPE and kills it before the handoff acknowledgement. Its output
	// belongs in the transaction log, which also keeps failures diagnosable.
	output, err := os.OpenFile(manifest.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open updater output log %s: %w", manifest.LogPath, err)
	}
	defer output.Close()
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		return err
	}
	// The child cannot replace files while this process is alive. On failed
	// handoff, kill and reap it before allowing another attempt.
	accepted := false
	defer func() {
		if !accepted {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		} else {
			_ = cmd.Process.Release()
		}
	}()
	deadline := time.Now().Add(applyHandoffTimeout)
	for time.Now().Before(deadline) {
		var id string
		if err := readJSONFile(pathToManifest+".accepted", &id); err == nil && id == manifest.ID {
			accepted = true
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("updater did not acknowledge handoff; see %s", manifest.LogPath)
}
