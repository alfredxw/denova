package continuallearning

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/harnessstate"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/trajectory"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"

	agentstate "github.com/alfredxw/denova/agent/state"
)

var ErrDisabled = errors.New("continual learning Lab is disabled")

type Service struct {
	host Host

	initMu      sync.Mutex
	initialized bool
	dataDir     string
	manager     *harnessstate.Manager
	history     *stateHistory
	sessions    *session.Store
	outcomes    *trajectory.OutcomeStore

	admission sync.Mutex
	starts    apptask.StartRegistry

	schedulerOnce sync.Once
	schedulerStop context.CancelFunc
	schedulerDone chan struct{}
	scheduleMu    sync.Mutex
}

func NewService(host Host) *Service {
	return &Service{
		host:   host,
		starts: apptask.NewStartRegistry(apptask.StartRegistryOptions{Label: "Harness Optimizer"}),
	}
}

func (service *Service) initialize() error {
	if service == nil {
		return errors.New("continual learning service is unavailable")
	}
	service.initMu.Lock()
	defer service.initMu.Unlock()
	if service.initialized {
		return nil
	}
	if service.host == nil {
		return errors.New("continual learning host is unavailable")
	}
	runtime := service.host.Runtime()
	dataDir := strings.TrimSpace(runtime.Config.DataDir())
	if dataDir == "" {
		return errors.New("continual learning data directory is required")
	}
	manager, err := harnessstate.OpenWithConfigSource(func() *config.Config {
		current := service.host.Runtime().Config
		return &current
	})
	if err != nil {
		return err
	}
	history, err := openStateHistory(
		manager.Root(),
		filepath.Join(dataDir, "runtime", "harness-state-history.lock"),
	)
	if err != nil {
		return err
	}
	sessions, err := session.NewStore(filepath.Join(dataDir, "continual-learning", "sessions"))
	if err != nil {
		return err
	}
	outcomes, err := trajectory.NewOutcomeStore(dataDir)
	if err != nil {
		_ = sessions.Close()
		return err
	}
	service.dataDir = dataDir
	service.manager = manager
	service.history = history
	service.sessions = sessions
	service.outcomes = outcomes
	service.initialized = true
	return nil
}

// CheckEnabled gates user-facing endpoints before they inspect process-local
// tasks. Disabling the Lab hides the surface and pauses new work without
// deleting State or history.
func (service *Service) CheckEnabled() error {
	_, err := service.requireEnabled()
	return err
}

func (service *Service) requireEnabled() (Runtime, error) {
	if service == nil || service.host == nil {
		return Runtime{}, errors.New("continual learning service is unavailable")
	}
	runtime := service.host.Runtime()
	if !runtime.Config.Labs.ContinualLearning {
		return Runtime{}, ErrDisabled
	}
	if err := service.initialize(); err != nil {
		return Runtime{}, err
	}
	return runtime, nil
}

func (service *Service) State(ctx context.Context) (StateSnapshot, error) {
	if _, err := service.requireEnabled(); err != nil {
		return StateSnapshot{}, err
	}
	snapshot, err := service.manager.Store().Current(ctx)
	if err != nil {
		return StateSnapshot{}, err
	}
	result := StateSnapshot{Revision: snapshot.Revision, Files: make([]StateFile, 0, len(snapshot.Files()))}
	for _, file := range snapshot.Files() {
		result.Files = append(result.Files, StateFile{Path: file.Path, Content: string(file.Content)})
	}
	return result, nil
}

func (service *Service) UpdateState(ctx context.Context, request StateUpdateRequest) (StateUpdateResult, error) {
	if _, err := service.requireEnabled(); err != nil {
		return StateUpdateResult{}, err
	}
	changes := make([]agentstate.Change, len(request.Changes))
	for index, change := range request.Changes {
		changes[index] = agentstate.Change{Path: change.Path, Content: []byte(change.Content), Delete: change.Delete}
	}
	var result StateUpdateResult
	err := service.history.withLock(ctx, func() error {
		updated, err := service.manager.Store().Update(ctx, agentstate.ChangeSet{
			BaseRevision: request.BaseRevision,
			Changes:      changes,
		})
		if err != nil {
			return err
		}
		if updated.CleanupError != nil {
			slog.WarnContext(ctx, "[continual-learning] updated State with deferred cleanup", "error", updated.CleanupError)
		}
		version, _, err := service.history.record(updated.Snapshot, request.Summary)
		if err != nil {
			return err
		}
		result = StateUpdateResult{Version: version, Changed: updated.Changed}
		return nil
	})
	return result, err
}

func (service *Service) Versions(ctx context.Context, limit int) ([]StateVersion, error) {
	if _, err := service.requireEnabled(); err != nil {
		return nil, err
	}
	snapshot, err := service.manager.ValidatedSnapshot(ctx)
	if err != nil {
		var validation *agentstate.ValidationError
		if !errors.As(err, &validation) {
			return nil, err
		}
		// Keep committed Git history available so management can restore a
		// failed live edit even when the current directory is invalid.
		return service.history.versions(ctx, nil, limit)
	}
	return service.history.versions(ctx, &snapshot, limit)
}

func (service *Service) Diff(ctx context.Context, from, to StateVersionID) (StateVersionDiff, error) {
	if _, err := service.requireEnabled(); err != nil {
		return StateVersionDiff{}, err
	}
	return service.history.diff(ctx, from, to)
}

func (service *Service) Restore(ctx context.Context, id StateVersionID) (StateUpdateResult, error) {
	if _, err := service.requireEnabled(); err != nil {
		return StateUpdateResult{}, err
	}
	var result StateUpdateResult
	err := service.history.withLock(ctx, func() error {
		current, err := service.manager.Store().Current(ctx)
		if err != nil {
			return err
		}
		commit, err := stateCommitForVersion(service.history.repo, id)
		if err != nil {
			return err
		}
		files, err := stateFilesFromCommit(commit)
		if err != nil {
			return err
		}
		updated, err := service.manager.Store().Update(ctx, agentstate.ChangeSet{
			BaseRevision: current.Revision,
			Changes:      stateReplacementChanges(current.Files(), files),
		})
		if err != nil {
			return err
		}
		if updated.CleanupError != nil {
			slog.WarnContext(ctx, "[continual-learning] restored State with deferred cleanup", "error", updated.CleanupError)
		}
		version, _, err := service.history.record(updated.Snapshot, "Restore Harness State version")
		if err != nil {
			return err
		}
		result = StateUpdateResult{Version: version, Changed: updated.Changed}
		return nil
	})
	return result, err
}

func (service *Service) RecordOutcome(outcome trajectory.Outcome) (trajectory.Outcome, error) {
	if _, err := service.requireEnabled(); err != nil {
		return trajectory.Outcome{}, err
	}
	return service.outcomes.Append(outcome)
}

func (service *Service) Outcomes(limit int) ([]trajectory.Outcome, error) {
	if _, err := service.requireEnabled(); err != nil {
		return nil, err
	}
	return service.outcomes.List(limit)
}

func (service *Service) Messages(ctx context.Context, before, limit int) (session.HistoryPage, error) {
	if _, err := service.requireEnabled(); err != nil {
		return session.HistoryPage{}, err
	}
	target, err := service.optimizerSession()
	if err != nil {
		return session.HistoryPage{}, err
	}
	return target.ReadHistoryPage(ctx, before, limit)
}

func (service *Service) Clear(ctx context.Context) error {
	runtime, err := service.requireEnabled()
	if err != nil {
		return err
	}
	service.admission.Lock()
	defer service.admission.Unlock()
	sessionID, err := optimizerSessionID()
	if err != nil {
		return err
	}
	if latest := service.starts.Latest(userScope, sessionID).Task; latest != nil && !latest.Finished() {
		return appagentruntime.ErrOperationActive
	}
	if runtime.Execution != nil {
		if err := runtime.Execution.CloseSessionBindings(ctx, agentrun.AgentKindHarnessOptimizer, "", sessionID); err != nil {
			return err
		}
	}
	target, err := service.optimizerSession()
	if err != nil {
		return err
	}
	if err := target.Clear(); err != nil {
		return err
	}
	service.starts.ReleaseScope(userScope, sessionID)
	return nil
}

func (service *Service) AnswerAsk(ctx context.Context, askID string, answers []agentconversation.HostAskAnswer) (agentconversation.HostAskResolution, error) {
	if _, err := service.requireEnabled(); err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	target, err := service.optimizerSession()
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	return service.host.ResolveAsk(ctx, target, askID, session.AskAnswered, answers, "")
}

func (service *Service) CancelAsk(ctx context.Context, askID, reason string) (agentconversation.HostAskResolution, error) {
	if _, err := service.requireEnabled(); err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	target, err := service.optimizerSession()
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	return service.host.ResolveAsk(ctx, target, askID, session.AskCancelled, nil, reason)
}

func (service *Service) optimizerSession() (*session.Session, error) {
	if service.sessions == nil {
		return nil, errors.New("Harness Optimizer session store is unavailable")
	}
	sessionID, err := optimizerSessionID()
	if err != nil {
		return nil, err
	}
	return service.sessions.GetOrCreate(sessionID)
}

func optimizerSessionID() (string, error) {
	sessionID, ok := session.AgentSessionID(config.AgentKindHarnessOptimizer)
	if !ok {
		return "", fmt.Errorf("Harness Optimizer Agent session is not configured")
	}
	return sessionID, nil
}

func optimizerRunOptions(dataDir, sessionID string) agentrun.Options {
	return agentrun.Options{
		AgentKind: agentrun.AgentKindHarnessOptimizer,
		StateRoot: filepath.Join(dataDir, "continual-learning"),
		SessionID: sessionID,
		Mode:      RuntimeMode,
	}
}

func (service *Service) Close(ctx context.Context) error {
	if service == nil {
		return nil
	}
	if service.schedulerStop != nil {
		service.schedulerStop()
	}
	if service.schedulerDone != nil {
		select {
		case <-service.schedulerDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if service.sessions != nil {
		if task := service.ActiveTask(); task != nil && !task.Finished() {
			if err := appagentruntime.AbortAndWait(ctx, task); err != nil {
				return err
			}
		}
	}
	if service.sessions != nil {
		return service.sessions.Close()
	}
	return nil
}
