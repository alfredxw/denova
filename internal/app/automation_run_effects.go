package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agents "denova/internal/agents"
	"denova/internal/automation"
)

func automationCompletionMutationPaths(mutations []agents.ToolMutation) []string {
	seen := make(map[string]struct{}, len(mutations))
	paths := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		path := strings.TrimSpace(mutation.Target)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func stageAutomationTerminalEffects(run *automation.RunRecord, mutationPaths []string) {
	if run == nil {
		return
	}
	seen := make(map[string]struct{}, len(run.CompletionMutationPaths)+len(mutationPaths))
	paths := make([]string, 0, len(run.CompletionMutationPaths)+len(mutationPaths))
	for _, group := range [][]string{run.CompletionMutationPaths, mutationPaths} {
		for _, path := range group {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	run.CompletionMutationPaths = paths
	run.CompletionEffectsOperationID = strings.TrimSpace(run.RuntimeOperationID)
	if run.Status == automation.RunStatusSuccess {
		run.CompletionEffectsPending = true
		run.CompletionEffectsCompleted = false
		return
	}
	run.WriteConfirmationRequired = false
	run.WriteConfirmationPolicyCaptured = true
	run.CompletionEffectsPending = len(paths) > 0
	run.CompletionEffectsCompleted = len(paths) == 0
}

// completeAutomationRunEffects drains the persisted terminal outbox. Every
// downstream action has deterministic identity (write-confirmation inbox and
// durable trigger evaluation), so a crash after the effect but before the
// final AppendRun safely replays the same action.
func (s *AutomationAppService) completeAutomationRunEffects(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	task automation.Task,
	run automation.RunRecord,
) (automation.RunRecord, error) {
	// A tool HostEffect can transfer into the run ledger concurrently with the
	// terminal writer. Always drain the authoritative merged record, never the
	// caller's potentially pre-effect snapshot.
	if persistedTask, persistedRun, err := storeForSnapshot(snap).GetRunByID(run.ID); err != nil {
		return run, fmt.Errorf("load automation completion-effects plan: %w", err)
	} else if strings.TrimSpace(persistedRun.RuntimeOperationID) != strings.TrimSpace(run.RuntimeOperationID) {
		return run, fmt.Errorf(
			"%w: run_id=%s completion operation changed from %s to %s",
			automation.ErrRunIdentityConflict,
			run.ID,
			run.RuntimeOperationID,
			persistedRun.RuntimeOperationID,
		)
	} else {
		task, run = persistedTask, persistedRun
	}
	if !run.CompletionEffectsPending && run.CompletionEffectsCompleted {
		return run, nil
	}
	planChanged := false
	if !run.WriteConfirmationPolicyCaptured {
		if run.Status == automation.RunStatusSuccess && run.RuntimeOperationID == run.RootRuntimeOperationID {
			run.WriteConfirmationRequired = automationRunNeedsWriteConfirmation(task, run)
		} else {
			run.WriteConfirmationRequired = false
		}
		run.WriteConfirmationPolicyCaptured = true
		planChanged = true
	}
	if strings.TrimSpace(run.CompletionEffectsOperationID) == "" {
		// One-way migration for terminal records written before the outbox carried
		// an operation epoch. Only the root operation inherits legacy confirmation
		// policy; follow-ups never derive it from mutable task configuration.
		run.CompletionEffectsOperationID = strings.TrimSpace(run.RuntimeOperationID)
		run.CompletionEffectsPending = true
		run.CompletionEffectsCompleted = false
		planChanged = true
	}
	if planChanged {
		if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), run); err != nil {
			return run, fmt.Errorf("persist automation completion-effects plan: %w", err)
		}
	}
	if run.Status == automation.RunStatusSuccess && run.WriteConfirmationRequired {
		if err := s.createWriteConfirmationInboxIfNeeded(snap, task, run, run.Summary); err != nil {
			return run, fmt.Errorf("complete automation write-confirmation effect: %w", err)
		}
	}
	targets := s.chapterContentMutationPaths(snap, run.CompletionMutationPaths)
	if len(targets) > 0 {
		if s.app == nil || s.app.automationTriggers == nil {
			return run, fmt.Errorf("automation mutation-effect coordinator is unavailable")
		}
		effectOperationID := run.CompletionEffectsOperationID
		enqueued := s.app.automationTriggers.EnqueueWithCompletion(
			s,
			snap,
			"automation_agent_post_run:"+effectOperationID,
			targets,
			func(processErr error) {
				if processErr != nil {
					slog.InfoContext(ctx, fmt.Sprintf("[automation] mutation effect remains pending task_id=%s run_id=%s operation_id=%s err=%v", task.ID, run.ID, effectOperationID, processErr))
					return
				}
				if ackErr := s.acknowledgeAutomationRunEffects(snap, run.ID, effectOperationID); ackErr != nil {
					slog.ErrorContext(ctx, fmt.Sprintf("[automation] persist mutation effect receipt failed task_id=%s run_id=%s operation_id=%s err=%v", task.ID, run.ID, effectOperationID, ackErr))
				}
			},
		)
		if !enqueued {
			return run, fmt.Errorf("automation mutation effect could not be admitted")
		}
		// The durable run remains pending until the coordinator callback records
		// its receipt. A crash or callback failure is retried by startup scan.
		return run, nil
	}
	return persistAutomationRunEffectsReceipt(snap, task, run)
}

func automationRunNeedsWriteConfirmation(task automation.Task, run automation.RunRecord) bool {
	return task.WriteMode == automation.WriteModeConfirmWrite &&
		task.WriteScope != "" && task.WriteScope != automation.WriteScopeNone &&
		run.Trigger != automation.TriggerWriteConfirmation
}

func persistAutomationRunEffectsReceipt(
	snap *automationWorkspaceSnapshot,
	task automation.Task,
	run automation.RunRecord,
) (automation.RunRecord, error) {
	run.CompletionEffectsPending = false
	run.CompletionEffectsCompleted = true
	if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), run); err != nil {
		return run, fmt.Errorf("persist automation completion-effects receipt: %w", err)
	}
	return run, nil
}

func (s *AutomationAppService) acknowledgeAutomationRunEffects(
	snap *automationWorkspaceSnapshot,
	runID string,
	effectOperationID string,
) (err error) {
	store := storeForSnapshot(snap)
	task, _, err := store.GetRunByID(runID)
	if err != nil {
		return err
	}
	releaseRun, err := store.AcquireRunLease(context.Background(), automationTaskStoreID(task), runID)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, releaseRun()) }()
	task, run, err := store.GetRunByID(runID)
	if err != nil {
		return err
	}
	if !run.CompletionEffectsPending && run.CompletionEffectsCompleted {
		return nil
	}
	if run.CompletionEffectsOperationID != effectOperationID {
		return fmt.Errorf(
			"%w: run_id=%s completion operation changed from %s to %s",
			automation.ErrRunIdentityConflict,
			runID,
			effectOperationID,
			run.CompletionEffectsOperationID,
		)
	}
	_, err = persistAutomationRunEffectsReceipt(snap, task, run)
	return err
}
