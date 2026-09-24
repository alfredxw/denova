package automationapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"denova/internal/automation"
)

func automationRuntimeOptions(snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord) agentrun.Options {
	return agentrun.Options{AgentKind: projectAgentKind(snap), ProjectID: snap.projectID, StateRoot: snap.stateRoot,
		TaskID: run.ID, AutomationTaskID: task.ID, SessionID: run.SessionID, Workspace: snap.workspace, Mode: agentrun.ModeAgentChat}
}

func commandProjection(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord) (agentexecution.OperationProjection, bool, error) {
	// A new trigger has no transcript yet. Do not open a recovery Session before
	// the common conversation admission path has created that transcript.
	if snap.sessionStore != nil && !snap.sessionStore.Exists(run.SessionID) {
		return agentexecution.OperationProjection{}, false, nil
	}
	return snap.executionRuntime.CommandProjection(ctx, automationRuntimeOptions(snap, task, run), run.TurnID)
}

// deliverRun owns only the handoff. Once accepted, retries return the original
// receipt and never start, resume, cancel, or wait for Agent execution.
func (s *Service) deliverRun(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord) (*apptask.Task, automation.RunRecord, error) {
	if validateAutomationRunRootReceipt(run) == nil {
		return nil, s.projectRun(ctx, snap, task, run), nil
	}
	view, found, err := commandProjection(ctx, snap, task, run)
	if err != nil {
		return nil, run, err
	}
	if found {
		if err := applyAutomationRootReceipt(&run, view.Receipt); err != nil {
			return nil, run, err
		}
		run.DeliveryStatus = automation.DeliveryAccepted
		_, err = storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), run)
		return nil, projectRunView(run, view), err
	}
	if run.Input == nil {
		run.Input = &automation.RunInput{Message: s.buildAutomationUserMessage(task, run), ModelProfileID: task.ModelProfileID, SessionTitle: task.Name}
	}
	run.DeliveryStatus = automation.DeliveryPending
	if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), run); err != nil {
		return nil, run, err
	}
	accepted, err := s.host.AcceptProjectConversationTurn(ctx, ProjectConversationTurn{
		ProjectID: snap.projectID, SessionID: run.SessionID, CommandID: run.TurnID,
		Message: run.Input.Message, ModelProfileID: run.Input.ModelProfileID, SessionTitle: run.Input.SessionTitle,
		AutomationTaskID: task.ID, RunID: run.ID,
	})
	if err != nil {
		if errors.Is(err, ErrOperationActive) {
			slog.InfoContext(ctx, "[automation] trigger waits for Project Agent availability", "task_id", task.ID, "run_id", run.ID)
			return nil, s.projectRun(ctx, snap, task, run), nil
		}
		slog.WarnContext(ctx, "[automation] trigger delivery remains pending", "task_id", task.ID, "run_id", run.ID, "err", err)
		return nil, run, err
	}
	// Even when writing the delivery receipt fails, AgentChat must own its
	// accepted worker. Reconciliation later recovers this exact canonical receipt.
	receiptErr := applyAutomationRootReceipt(&run, accepted.Receipt())
	if receiptErr == nil {
		run.DeliveryStatus = automation.DeliveryAccepted
		_, receiptErr = storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), run)
	}
	startErr := accepted.Start()
	if err := errors.Join(receiptErr, startErr); err != nil {
		s.SignalReconciliation()
		return accepted.Task(), run, err
	}
	slog.InfoContext(ctx, "[automation] trigger delivered to Project Agent", "task_id", task.ID, "run_id", run.ID, "operation_id", run.RootRuntimeOperationID)
	return accepted.Task(), s.projectRun(ctx, snap, task, run), nil
}

func (s *Service) automationRunByID(snap *automationWorkspaceSnapshot, runID string) (automation.RunRecord, error) {
	store := storeForSnapshot(snap)
	if snap == nil {
		store = s.storeAllWorkspaces()
	}
	task, run, err := store.GetRunByID(strings.TrimSpace(runID))
	if err != nil {
		return run, err
	}
	if snap == nil {
		runtime, err := s.host.RuntimeForTarget(context.Background(), automationTargetForRun(task, run))
		if err != nil {
			return run, err
		}
		snap = snapshotFromRuntime(runtime)
	}
	run = s.projectRun(context.Background(), snap, task, run)
	return run, nil
}

// Reconciliation retries delivery and released legacy effect outboxes only.
// Accepted Project Agent work is recovered by the common AgentChat lifecycle.
func (s *Service) reconcilePersistedAutomationRuns(ctx context.Context) {
	obligations, err := s.storeAllWorkspaces().ListDurableObligations()
	if err != nil {
		slog.ErrorContext(ctx, "[automation] list delivery obligations failed", "err", err)
		return
	}
	for _, item := range obligations {
		err := func() (err error) {
			snap, operation, err := s.acquireTargetRuntime(ctx, automationTargetForRun(item.Task, item.Run))
			if err != nil {
				return err
			}
			defer operation.Release()
			store := storeForSnapshot(snap)
			release, err := store.AcquireRunLease(ctx, automationTaskStoreID(item.Task), item.Run.ID)
			if err != nil {
				return err
			}
			defer func() { err = errors.Join(err, release()) }()
			task, run, err := store.GetRunByID(item.Run.ID)
			if err != nil {
				return err
			}
			if run.DeliveryStatus == automation.DeliveryPending {
				_, _, err = s.deliverRun(operation.Context(), snap, task, run)
				return err
			}
			// Released records may still carry an old execution mirror. A canonical
			// acceptance ends Automation ownership regardless of its current phase.
			if run.DeliveryStatus == "" && automation.RunHasRuntimeObligation(run) {
				if run.TurnID == "" {
					run.TurnID = automationRunAgentCommandID(run.ID)
				}
				view, found, err := commandProjection(ctx, snap, task, run)
				if err != nil {
					return err
				}
				if !found {
					return fmt.Errorf("legacy delivery %s has no canonical acceptance", run.ID)
				}
				if err := applyAutomationRootReceipt(&run, view.Receipt); err != nil {
					return err
				}
				run.DeliveryStatus = automation.DeliveryAccepted
				if !run.CompletionEffectsCompleted && len(run.CompletionMutationPaths) > 0 {
					run.CompletionEffectsPending = true
					if run.CompletionEffectsOperationID == "" {
						run.CompletionEffectsOperationID = run.RuntimeOperationID
					}
				}
				if _, err := store.AppendRun(automationTaskStoreID(task), run); err != nil {
					return err
				}
			}
			if run.CompletionEffectsPending || (run.Status == automation.RunStatusSuccess && !run.CompletionEffectsCompleted) {
				view, found, err := snap.executionRuntime.OperationProjection(ctx, automationRuntimeOptions(snap, task, run), run.CompletionEffectsOperationID)
				if err != nil {
					return err
				}
				if !found || view.Outcome == nil {
					return nil
				}
				_, err = s.completeAutomationRunEffects(ctx, snap, task, run)
				return err
			}
			return nil
		}()
		if err != nil {
			slog.WarnContext(ctx, "[automation] delivery obligation remains pending", "run_id", item.Run.ID, "err", err)
		}
	}
}
