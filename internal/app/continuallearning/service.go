package continuallearning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"denova/config"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/trajectory"

	agentstate "github.com/alfredxw/denova/agent/state"
)

var ErrDisabled = errors.New("Developer Mode is disabled")

type Service struct {
	host Host

	initMu      sync.Mutex
	initialized bool
	dataDir     string
	manager     *harnessstate.Manager
	published   *harnessstate.Manager
	history     *stateHistory
	outcomes    *trajectory.OutcomeStore

	schedulerOnce sync.Once
	schedulerStop context.CancelFunc
	schedulerDone chan struct{}
	scheduleMu    sync.Mutex
}

func NewService(host Host) *Service {
	return &Service{host: host}
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
	published, err := harnessstate.OpenPublishedWithConfigSource(func() *config.Config {
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
	outcomes, err := trajectory.NewOutcomeStore(dataDir)
	if err != nil {
		return err
	}
	service.dataDir = dataDir
	service.manager = manager
	service.published = published
	service.history = history
	service.outcomes = outcomes
	if err := service.initializePublishedState(context.Background(), &runtime.Config); err != nil {
		return err
	}
	service.initialized = true
	return nil
}

func (service *Service) requireEnabled() (Runtime, error) {
	if service == nil || service.host == nil {
		return Runtime{}, errors.New("continual learning service is unavailable")
	}
	runtime := service.host.Runtime()
	if !runtime.Config.Labs.DeveloperMode {
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
	inspection, err := service.manager.Inspect(ctx)
	if err != nil {
		return StateSnapshot{}, err
	}
	published, err := service.published.Inspect(ctx)
	if err != nil {
		return StateSnapshot{}, err
	}
	if len(published.Diagnostics) != 0 {
		// Keep the management surface available when a later configuration
		// change makes the immutable Published snapshot invalid. Runtime loading
		// still rejects that contribution, while the user can repair the Draft
		// and publish a valid replacement.
		slog.WarnContext(ctx, "[continual-learning] Published Harness State is invalid under the current configuration",
			"revision", published.Snapshot.Revision, "diagnostics", len(published.Diagnostics))
	}
	result := StateSnapshot{
		Revision:          inspection.Snapshot.Revision,
		PublishedRevision: published.Snapshot.Revision,
		Files:             make([]StateFile, 0, len(inspection.Snapshot.Files())),
		ScriptTools:       scriptToolSummaries(inspection.Harness.ScriptToolMetadata()),
		Diagnostics:       append([]StateDiagnostic(nil), inspection.Diagnostics...),
		Source:            StateSourceUser,
		Changed:           inspection.Snapshot.Revision != published.Snapshot.Revision,
	}
	for _, file := range inspection.Snapshot.Files() {
		result.Files = append(result.Files, StateFile{Path: file.Path, Content: string(file.Content)})
	}
	if len(result.Files) == 0 {
		result.Source = StateSourceBuiltin
	}
	return result, nil
}

func (service *Service) initializePublishedState(ctx context.Context, cfg *config.Config) error {
	ready, err := harnessstate.PublishedReady(cfg)
	if err != nil || ready {
		return err
	}
	inspection, err := service.manager.Inspect(ctx)
	if err != nil {
		return err
	}
	if len(inspection.Diagnostics) == 0 {
		current, err := service.published.Store().Current(ctx)
		if err != nil {
			return err
		}
		updated, err := service.published.Store().Update(ctx, agentstate.ChangeSet{
			BaseRevision: current.Revision,
			Changes:      stateReplacementChanges(current.Files(), inspection.Snapshot.Files()),
		})
		if err != nil {
			return fmt.Errorf("seed published Harness State: %w", err)
		}
		if updated.CleanupError != nil {
			slog.WarnContext(ctx, "[continual-learning] seeded Published State with deferred cleanup", "error", updated.CleanupError)
		}
	} else {
		slog.WarnContext(ctx, "[continual-learning] preserving invalid released State as Draft; keeping the safe Published snapshot",
			"diagnostics", len(inspection.Diagnostics))
	}
	if err := harnessstate.MarkPublishedReady(cfg); err != nil {
		return err
	}
	slog.InfoContext(ctx, "[continual-learning] initialized Draft/Published Harness boundary",
		"draft_revision", inspection.Snapshot.Revision)
	return nil
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
		updated, err := service.manager.Store().Write(ctx, agentstate.ChangeSet{
			BaseRevision: request.BaseRevision,
			Changes:      changes,
		})
		if err != nil {
			return err
		}
		if updated.CleanupError != nil {
			slog.WarnContext(ctx, "[continual-learning] updated State with deferred cleanup", "error", updated.CleanupError)
		}
		slog.InfoContext(ctx, "[continual-learning] committed Harness Draft",
			"revision", updated.Snapshot.Revision, "changed", updated.Changed)
		version, _, historyErr := service.history.record(updated.Snapshot, request.Summary)
		if historyErr != nil {
			slog.WarnContext(ctx, "[continual-learning] Harness Draft committed without Git history",
				"revision", updated.Snapshot.Revision, "error", historyErr)
		}
		result = StateUpdateResult{
			Version: version, Revision: updated.Snapshot.Revision, Changed: updated.Changed,
		}
		return nil
	})
	return result, err
}

// RecordCurrentState snapshots a valid Agent-edited workspace into optional
// Git history. Invalid intermediate files remain live and diagnosable; unlike
// an explicit UI save, they are not auto-recorded when an Agent turn settles.
func (service *Service) RecordCurrentState(ctx context.Context, summary string) error {
	if _, err := service.requireEnabled(); err != nil {
		return err
	}
	return service.history.withLock(ctx, func() error {
		snapshot, err := service.manager.ValidatedSnapshot(ctx)
		if err != nil {
			return err
		}
		_, _, err = service.history.record(snapshot, summary)
		return err
	})
}

func scriptToolSummaries(metadata []harnessstate.ScriptToolMetadata) []ScriptToolSummary {
	result := make([]ScriptToolSummary, len(metadata))
	for index, tool := range metadata {
		result[index] = ScriptToolSummary{
			Name: tool.Name, Description: tool.Description, Agents: append([]string(nil), tool.Agents...),
			Enabled: tool.Enabled, Resource: tool.Resource,
			InputSchema: append(json.RawMessage(nil), tool.InputSchema...),
		}
	}
	return result
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
		updated, err := service.manager.Store().Write(ctx, agentstate.ChangeSet{
			BaseRevision: current.Revision,
			Changes:      stateReplacementChanges(current.Files(), files),
		})
		if err != nil {
			return err
		}
		if updated.CleanupError != nil {
			slog.WarnContext(ctx, "[continual-learning] restored State with deferred cleanup", "error", updated.CleanupError)
		}
		version, _, historyErr := service.history.record(updated.Snapshot, "Restore Harness State version")
		if historyErr != nil {
			slog.WarnContext(ctx, "[continual-learning] State restored without a new Git history entry",
				"revision", updated.Snapshot.Revision, "error", historyErr)
		}
		result = StateUpdateResult{
			Version: version, Revision: updated.Snapshot.Revision, Changed: updated.Changed,
		}
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
	return nil
}
