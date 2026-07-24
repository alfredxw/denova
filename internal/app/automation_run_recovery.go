package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	agents "denova/internal/agents"
	"denova/internal/automation"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func (s *AutomationAppService) reconcilePersistedAutomationRuns(ctx context.Context) {
	durableRuns, err := s.storeAllWorkspaces().ListDurableObligations()
	if err != nil {
		log.Printf("[automation-recovery] persisted run scan list failed err=%v", err)
		return
	}
	for _, durable := range durableRuns {
		taskDef := durable.Task
		run := durable.Run
		if strings.TrimSpace(run.ID) == "" {
			continue
		}
		needsRuntime := run.Status == automation.RunStatusRunning || run.RuntimeRecoveryRequired || strings.TrimSpace(run.PendingRuntimeCommandID) != ""
		needsEffects := run.CompletionEffectsPending || (run.Status == automation.RunStatusSuccess && !run.CompletionEffectsCompleted)
		if !needsRuntime && !needsEffects {
			continue
		}
		target := automationTargetForRun(taskDef, run)
		snap, operation, targetErr := s.acquireTargetRuntime(ctx, target)
		if targetErr != nil {
			log.Printf("[automation-recovery] persisted run target unavailable task_id=%s run_id=%s workspace=%q err=%v", taskDef.ID, run.ID, run.Workspace, targetErr)
			continue
		}
		func() {
			defer operation.Release()
			if _, _, active := s.activeAutomationTaskByRunID(snap, run.ID); active {
				return
			}
			runStore := storeForSnapshot(snap)
			releaseRun, leaseErr := runStore.AcquireRunLease(operation.Context(), automationTaskStoreID(taskDef), run.ID)
			if leaseErr != nil {
				log.Printf("[automation-recovery] run lease failed task_id=%s run_id=%s err=%v", taskDef.ID, run.ID, leaseErr)
				return
			}
			defer func() {
				if releaseErr := releaseRun(); releaseErr != nil {
					log.Printf("[automation-recovery] release run lease failed task_id=%s run_id=%s err=%v", taskDef.ID, run.ID, releaseErr)
				}
			}()
			latestTask, latestRun, refreshErr := runStore.GetRunByID(run.ID)
			if refreshErr != nil {
				log.Printf("[automation-recovery] refresh persisted run failed task_id=%s run_id=%s err=%v", taskDef.ID, run.ID, refreshErr)
				return
			}
			taskDef = latestTask
			run = latestRun
			needsRuntime = run.Status == automation.RunStatusRunning || run.RuntimeRecoveryRequired || strings.TrimSpace(run.PendingRuntimeCommandID) != ""
			needsEffects = run.CompletionEffectsPending || (run.Status == automation.RunStatusSuccess && !run.CompletionEffectsCompleted)
			if !needsRuntime && !needsEffects {
				return
			}
			if needsEffects && !needsRuntime {
				if _, effectErr := s.completeAutomationRunEffects(operation.Context(), snap, taskDef, run); effectErr != nil {
					log.Printf("[automation-recovery] completion effect retry failed task_id=%s run_id=%s err=%v", taskDef.ID, run.ID, effectErr)
				}
				return
			}
			reconciled, ok, reconcileErr := s.reconcileAutomationRunReceipt(operation.Context(), snap, taskDef, run)
			if reconcileErr != nil {
				log.Printf("[automation-recovery] runtime projection reconcile failed task_id=%s run_id=%s command_id=%s operation_id=%s err=%v", taskDef.ID, run.ID, run.RuntimeCommandID, run.RuntimeOperationID, reconcileErr)
				return
			}
			if !ok {
				return
			}
			if reconciled.RuntimeRecoveryRequired {
				if _, _, recoveryErr := s.ensureAutomationRecoveryTask(operation.Context(), snap, taskDef, reconciled); recoveryErr != nil {
					log.Printf("[automation-recovery] recovery observation admission failed task_id=%s run_id=%s command_id=%s operation_id=%s err=%v", taskDef.ID, run.ID, reconciled.RuntimeCommandID, reconciled.RuntimeOperationID, recoveryErr)
				}
				return
			}
			if _, effectErr := s.completeAutomationRunEffects(operation.Context(), snap, taskDef, reconciled); effectErr != nil {
				log.Printf("[automation-recovery] reconciled terminal effect retry failed task_id=%s run_id=%s err=%v", taskDef.ID, run.ID, effectErr)
			}
		}()
	}
}

func automationRecoveryOptions(task automation.Task, run automation.RunRecord) agents.RunOptions {
	return agents.RunOptions{
		AgentKind: agents.AgentKindAutomation, TaskID: run.ID, AutomationTaskID: task.ID,
		SessionID: run.SessionID, Workspace: run.Workspace, Mode: "automation",
	}
}

// ensureAutomationRecoveryTask turns a cold, uncertain StartTurn into an owned
// observation Task. It attaches only to durable state; it never rebuilds an
// Engine and therefore cannot replay model or tool effects.
func (s *AutomationAppService) ensureAutomationRecoveryTask(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	taskDef automation.Task,
	run automation.RunRecord,
) (*Task, automation.RunRecord, error) {
	if !run.RuntimeRecoveryRequired {
		return nil, run, nil
	}
	if snap == nil || snap.chatService == nil {
		return nil, automation.RunRecord{}, agents.ErrRuntimeProjectionUnavailable
	}
	if activeTask, activeRun, ok := s.activeAutomationTaskByRunID(snap, run.ID); ok {
		return activeTask, activeRun, nil
	}
	taskStoreID := automationTaskStoreID(taskDef)
	claim, owner, err := s.reserveActiveAutomationRun(ctx, snap, taskStoreID, run)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if !owner {
		return claim.task, claim.run, nil
	}
	claimActivated := false
	defer func() {
		if !claimActivated {
			s.releaseAutomationClaim(claim)
		}
	}()

	recovery, err := snap.chatService.OpenRecoveryObservation(ctx, automationRecoveryOptions(taskDef, run))
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	status := recovery.InitialStatus()
	var attach agents.RuntimeRecoveryAction
	for _, action := range agents.RuntimeRecoveryActions(status) {
		if action.Kind == agents.RuntimeRecoveryAttach {
			attach = action
			break
		}
	}
	if attach.CommandID == "" || attach.OperationID == "" {
		recovery.Close()
		if status.Phase == runstate.PhaseIdle {
			reconciled, ok, reconcileErr := s.reconcileAutomationRunReceipt(ctx, snap, taskDef, run)
			if reconcileErr != nil {
				return nil, run, reconcileErr
			}
			if !ok {
				return nil, run, fmt.Errorf(
					"automation run %s remains recovery-required but its durable operation is not retained in the idle projection",
					run.ID,
				)
			}
			if reconciled.Status != automation.RunStatusRunning {
				reconciled, reconcileErr = s.completeAutomationRunEffects(ctx, snap, taskDef, reconciled)
			}
			return nil, reconciled, reconcileErr
		}
		return nil, automation.RunRecord{}, fmt.Errorf(
			"automation run %s is recovery-required without an attachable StartTurn operation", run.ID,
		)
	}

	var displayTask *Task
	displayTask, err = NewDeferredRegisteredTask(func(displayTask *Task) error {
		if err := s.activateAutomationClaim(claim, displayTask); err != nil {
			return err
		}
		claimActivated = true
		return nil
	})
	if err != nil {
		recovery.Close()
		return nil, automation.RunRecord{}, err
	}
	displayTask.emit(agents.Event{Type: "automation_run", Data: run})
	receipt, err := recovery.Resume(ctx, attach, displayTask.ID(), displayTask.emit)
	if err != nil {
		recovery.Close()
		displayTask.failBeforeStart(err)
		s.app.unregisterWorkspaceTask(displayTask)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, automation.RunRecord{}, err
	}
	if err := applyAutomationCurrentReceipt(&run, receipt, strings.TrimSpace(run.RuntimeCommandID)); err != nil {
		recovery.Close()
		displayTask.failBeforeStart(err)
		s.app.unregisterWorkspaceTask(displayTask)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, automation.RunRecord{}, err
	}
	if _, err := storeForSnapshot(snap).AppendRun(taskStoreID, run); err != nil {
		recovery.Close()
		displayTask.failBeforeStart(err)
		s.app.unregisterWorkspaceTask(displayTask)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, automation.RunRecord{}, err
	}
	s.updateActiveAutomationRun(snap, taskStoreID, run)
	displayTask.emit(agents.Event{Type: agents.RuntimeRecoveryRequiredEventType, Data: map[string]any{
		"code":       agents.RuntimeRecoveryRequiredEventCode,
		"message":    "自动化运行需要显式终止或等待持久化终态 / Automation run requires explicit control or durable terminal reconciliation",
		"command_id": attach.CommandID, "operation_id": attach.OperationID,
	}})
	if err := displayTask.Start(func(taskCtx context.Context, displayTask *Task, emit func(agents.Event)) {
		defer s.app.unregisterWorkspaceTask(displayTask)
		defer s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		defer recovery.Close()
		outcome := recovery.Wait(taskCtx, emit)
		finalRun, reconcileErr := s.finalizeRecoveredAutomationRun(taskCtx, snap, taskDef, run, outcome)
		if reconcileErr != nil {
			log.Printf("[automation-recovery] terminal reconciliation failed task_id=%s run_id=%s operation_id=%s err=%v", taskDef.ID, run.ID, run.RuntimeOperationID, reconcileErr)
			emit(agents.Event{Type: "error", Data: map[string]string{"message": reconcileErr.Error()}})
			return
		}
		emit(agents.Event{Type: "automation_run", Data: finalRun})
		log.Printf("[automation-recovery] observation settled task_id=%s run_id=%s operation_id=%s status=%s outcome=%s", taskDef.ID, run.ID, finalRun.RuntimeOperationID, finalRun.Status, outcome.Status)
	}); err != nil {
		recovery.Close()
		displayTask.failBeforeStart(err)
		s.app.unregisterWorkspaceTask(displayTask)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, automation.RunRecord{}, err
	}
	return displayTask, run, nil
}

func (s *AutomationAppService) finalizeRecoveredAutomationRun(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	taskDef automation.Task,
	run automation.RunRecord,
	outcome agents.RunOutcome,
) (automation.RunRecord, error) {
	if reconciled, ok, err := s.reconcileAutomationRunReceipt(context.WithoutCancel(ctx), snap, taskDef, run); err != nil {
		return automation.RunRecord{}, err
	} else if ok {
		if reconciled.Status != automation.RunStatusRunning {
			return s.completeAutomationRunEffects(context.WithoutCancel(ctx), snap, taskDef, reconciled)
		}
		return reconciled, fmt.Errorf(
			"automation run %s recovery observation ended while durable operation %s remains active",
			reconciled.ID,
			reconciled.RuntimeOperationID,
		)
	}
	// The observer's durable terminal is authoritative even if a bounded status
	// projection was concurrently evicted. This fallback never executes an
	// Engine and preserves the already-validated root/current receipts.
	if outcome.Status == agents.RunOutcomeFailed {
		run.RuntimeRecoveryRequired = true
		if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(taskDef), run); err != nil {
			return automation.RunRecord{}, err
		}
		return run, fmt.Errorf(
			"automation run %s recovery observation failed without a durable terminal projection: %w",
			run.ID,
			automationRunOutcomeError(outcome),
		)
	}
	run.RuntimeRecoveryRequired = false
	run.FinishedAt = time.Now().UTC()
	switch outcome.Status {
	case agents.RunOutcomeCompleted:
		run.Status = automation.RunStatusSuccess
		run.Error = ""
		if summary := recoveredAutomationRunSummary(snap, run); summary != "" {
			run.Summary = summary
		}
		stageAutomationTerminalEffects(&run, run.CompletionMutationPaths)
		if run.RuntimeOperationID == run.RootRuntimeOperationID {
			if !run.WriteConfirmationPolicyCaptured {
				run.WriteConfirmationRequired = automationRunNeedsWriteConfirmation(taskDef, run)
			}
		} else {
			run.WriteConfirmationRequired = false
		}
		run.WriteConfirmationPolicyCaptured = true
	case agents.RunOutcomeAborted, agents.RunOutcomePreempted:
		run.Status = automation.RunStatusAborted
		run.Error = automationRunOutcomeError(outcome).Error()
		stageAutomationTerminalEffects(&run, run.CompletionMutationPaths)
	default:
		run.Status = automation.RunStatusFailed
		run.Error = automationRunOutcomeError(outcome).Error()
		stageAutomationTerminalEffects(&run, run.CompletionMutationPaths)
	}
	if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(taskDef), run); err != nil {
		return automation.RunRecord{}, err
	}
	return s.completeAutomationRunEffects(context.WithoutCancel(ctx), snap, taskDef, run)
}
