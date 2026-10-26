package update

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Phase describes program-file installation only. User-data migrations remain
// application-owned and are never undone by this updater.
type Phase string

const (
	PhaseStaged      Phase = "staged"
	PhaseWaiting     Phase = "waiting"
	PhaseBackingUp   Phase = "backing_up"
	PhaseApplying    Phase = "applying"
	PhaseStarting    Phase = "starting"
	PhaseRollingBack Phase = "rolling_back"
	PhaseSucceeded   Phase = "succeeded"
	PhaseFailed      Phase = "failed"
)

// Status is the stable UI boundary; internal filesystem transaction fields stay private.
type Status struct {
	CompletedUpdate *CompletedUpdate `json:"completed_update,omitempty"`
	Check           *CheckResult     `json:"check,omitempty"`
	CheckedAt       time.Time        `json:"checked_at,omitempty"`
	Operation       string           `json:"operation,omitempty"`
	Progress        *InstallProgress `json:"progress,omitempty"`
	OperationError  string           `json:"operation_error,omitempty"`
	RestartWaiting  bool             `json:"restart_waiting,omitempty"`
	Busy            bool             `json:"busy,omitempty"`
	CurrentVersion  string           `json:"current_version"`
	ID              string           `json:"id,omitempty"`
	Version         string           `json:"version,omitempty"`
	Phase           Phase            `json:"phase"`
	Error           string           `json:"error,omitempty"`
	LogPath         string           `json:"log_path,omitempty"`
}

func (s *Service) Status() (Status, error) {
	status := Status{CurrentVersion: s.currentVersion, Phase: "idle"}
	completed, err := s.CompletedUpdate()
	if err != nil {
		return status, err
	}
	status.CompletedUpdate = completed
	s.mu.Lock()
	status.Check, status.CheckedAt = s.check, s.checkedAt
	status.Operation, status.Progress, status.OperationError = s.operation, s.progress, s.operationError
	s.mu.Unlock()
	path, err := readPendingManifestRef(updateDataDir(filepath.Dir(s.executablePath)))
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	m, err := readManifest(path)
	if err != nil {
		return status, err
	}
	status.ID, status.Version, status.Phase, status.Error, status.LogPath = m.ID, m.Version, m.State, m.Error, m.LogPath
	// An installation-relative log path remains useful in copied diagnostics,
	// whose privacy filter removes host-specific absolute directories.
	if relative, err := filepath.Rel(m.InstallDir, m.LogPath); err == nil {
		status.LogPath = filepath.ToSlash(relative)
	}
	return status, nil
}

func setPhase(path string, m *ApplyManifest, phase Phase, cause error) error {
	m.State = phase
	m.Error = ""
	if cause != nil {
		m.Error = cause.Error()
	}
	slog.Info("update_state_changed", "id", m.ID, "version", m.Version, "phase", phase, "error", m.Error)
	return writeManifest(path, *m)
}

func lockInstallation(installDir string) (func(), error) {
	root := updateDataDir(installDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(root, "update.lock"), flock.SetPermissions(0o600))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrUpdateBusy
	}
	return func() {
		if err := lock.Unlock(); err != nil {
			slog.Error("update_unlock_failed", "error", err)
		}
	}, nil
}

func (s *Service) lockOperation() (func(), error) {
	if s.executablePath == "" {
		return nil, fmt.Errorf("cannot locate current executable")
	}
	unlock, err := lockInstallation(filepath.Dir(s.executablePath))
	if err != nil {
		return nil, err
	}
	status, err := s.Status()
	if err == nil {
		switch status.Phase {
		case "idle", PhaseStaged, PhaseSucceeded, PhaseFailed:
		case PhaseWaiting, PhaseBackingUp, PhaseApplying, PhaseStarting, PhaseRollingBack:
			err = ErrUpdateBusy
		default:
			err = fmt.Errorf("unsupported update phase %q", status.Phase)
		}
	}
	if err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

// Keep the most recent successful transaction (including its backup and log).
// Older generated packages are unnecessary after a confirmed startup.
func pruneCompletedUpdates(m ApplyManifest, logger *slog.Logger) {
	root := updateDataDir(m.InstallDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		logger.Warn("update_cleanup_failed", "error", err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || (entry.Name() != "downloads" && !strings.HasPrefix(entry.Name(), "pending-")) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if filepath.Clean(path) == filepath.Dir(m.SourceDir) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			logger.Warn("update_cleanup_failed", "path", path, "error", err)
		}
	}
}
