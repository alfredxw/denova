package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"denova/internal/buildinfo"
)

const (
	automaticCheckInterval         = 6 * time.Hour
	automaticDownloadRetryInterval = time.Hour
)

// Policy is user-wide. Checking never grants permission to download or restart.
type Policy struct {
	Check    bool
	Download bool
}

func (s *Service) beginOperation(operation string) error {
	if buildinfo.Distribution == "desktop" && operation != "checking" {
		return errors.New("desktop updates are managed by the Electron application")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.operation != "" {
		return ErrUpdateBusy
	}
	s.operation, s.operationError, s.progress = operation, "", nil
	if operation == "downloading" {
		s.downloadAttempt = time.Now()
	}
	return nil
}

func (s *Service) endOperation(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operation = ""
	if err != nil {
		s.operationError = err.Error()
	}
}

func (s *Service) rememberCheck(check CheckResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.check, s.checkedAt = &check, time.Now()
}

// RunAutomatic is an App-owned worker: its caller owns cancellation, panic
// recovery and joining. Status only reads these snapshots and never fetches.
func (s *Service) RunAutomatic(ctx context.Context, policy func() (Policy, error)) {
	if isDevVersion(s.currentVersion) {
		return
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			p, err := policy()
			if err == nil {
				err = s.updateDue(ctx, p, time.Now())
			}
			if err != nil && !errors.Is(err, ErrUpdateBusy) && ctx.Err() == nil {
				slog.WarnContext(ctx, "automatic_update_failed", "error", err)
			}
			timer.Reset(time.Minute)
		}
	}
}

func (s *Service) updateDue(ctx context.Context, policy Policy, now time.Time) error {
	if !policy.Check {
		return nil
	}
	status, err := s.Status()
	if err != nil || status.Operation != "" {
		return err
	}
	// An explicitly staged package, an active transaction, and its recovery
	// facts take precedence over downloading another release.
	switch status.Phase {
	case "idle", PhaseSucceeded, PhaseFailed:
	case PhaseStaged, PhaseWaiting, PhaseBackingUp, PhaseApplying, PhaseStarting, PhaseRollingBack:
		return nil
	default:
		return fmt.Errorf("unsupported update phase %q", status.Phase)
	}
	check := status.Check
	if now.Sub(status.CheckedAt) >= automaticCheckInterval {
		result, err := s.Check(ctx)
		if err != nil {
			return err
		}
		check = &result
	}
	if !policy.Download || check == nil || !check.CanInstall {
		return nil
	}
	// A release that failed installation is retried only explicitly, or after
	// a newer version appears. Do not create a background failure loop.
	if status.Phase == PhaseFailed && sameVersion(status.Version, check.LatestVersion) {
		return nil
	}
	s.mu.Lock()
	attempt := s.downloadAttempt
	s.mu.Unlock()
	if now.Sub(attempt) < automaticDownloadRetryInterval {
		return nil
	}
	_, err = s.Install(ctx)
	return err
}
