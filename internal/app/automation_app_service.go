package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	agents "denova/internal/agents"
	"denova/internal/automation"
)

// AutomationAppService is a thin facade over the live App. It never stores a
// workspace snapshot as a field; instead, snapshots are constructed on demand
// (runtimeSnapshot for the active workspace, automationSnapshotForTarget for
// cross-workspace) and passed as parameters to the methods that need them.
type AutomationAppService struct {
	app                *App
	semanticEvaluator  semanticTriggerEvaluationFunc
	runtimeProjector   automationRuntimeProjectionFunc
	hostEffectTransfer func(context.Context, automation.HostEffectObligation, admittedToolMutationPayload) (bool, error)
	followUpAdmission  sync.Mutex
	followUps          automationFollowUpRegistry
}

func (a *App) StartAutomationScheduler(ctx context.Context) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.schedulerStarted || a.closed {
		a.mu.Unlock()
		return
	}
	if err := a.initializeLifecycleLocked(); err != nil {
		a.mu.Unlock()
		log.Printf("[automation] scheduler admission failed err=%v", err)
		return
	}
	schedulerCtx, lease, err := a.rootScope.AcquireContext(ctx)
	if err != nil {
		a.mu.Unlock()
		log.Printf("[automation] scheduler admission failed err=%v", err)
		return
	}
	schedulerCtx, cancel := context.WithCancel(schedulerCtx)
	a.schedulerStarted = true
	a.schedulerCancel = cancel
	a.schedulerWG.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.schedulerWG.Done()
		defer lease.Release()
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[automation] scheduler panic recovered err=%v", recovered)
			}
		}()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		a.automation().reconcilePersistedHostEffects(schedulerCtx)
		a.automation().reconcilePersistedAutomationRuns(schedulerCtx)
		for {
			select {
			case <-schedulerCtx.Done():
				log.Printf("[automation] scheduler stopped err=%v", schedulerCtx.Err())
				return
			case now := <-ticker.C:
				a.runAutomationSchedulerTick(schedulerCtx, now)
			case <-a.automationEffectWake:
				a.automation().reconcilePersistedHostEffects(schedulerCtx)
				a.automation().reconcilePersistedAutomationRuns(schedulerCtx)
			}
		}
	}()
}

func (a *App) signalAutomationEffectReconciliation() {
	if a == nil || a.automationEffectWake == nil {
		return
	}
	select {
	case a.automationEffectWake <- struct{}{}:
	default:
	}
}

func (a *App) runAutomationSchedulerTick(ctx context.Context, now time.Time) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[automation] scheduler tick panic recovered workspace=%q err=%v", a.Workspace(), recovered)
		}
	}()
	a.automation().reconcilePersistedHostEffects(ctx)
	a.automation().reconcilePersistedAutomationRuns(ctx)
	a.RunDueAutomations(ctx, now)
}

func (a *App) Automations() ([]automation.Task, error) {
	return a.automation().List()
}

func (s *AutomationAppService) List() ([]automation.Task, error) {
	return s.storeAllWorkspaces().List()
}

func (a *App) AutomationTemplates(locale string) []automation.TaskTemplate {
	return a.automation().Templates(locale)
}

func (s *AutomationAppService) Templates(locale string) []automation.TaskTemplate {
	return automation.BuiltinTaskTemplates(locale)
}

func (a *App) CreateAutomation(task automation.Task) (automation.Task, error) {
	return a.automation().Create(task)
}

func (s *AutomationAppService) Create(task automation.Task) (automation.Task, error) {
	return s.storeAllWorkspaces().Create(task)
}

func (a *App) UpdateAutomation(id string, task automation.Task) (automation.Task, error) {
	return a.automation().Update(id, task)
}

func (a *App) UpdateAutomationIfRevision(id string, task automation.Task, baseRevision string) (automation.Task, error) {
	return a.automation().UpdateIfRevision(id, task, baseRevision)
}

func (s *AutomationAppService) Update(id string, task automation.Task) (automation.Task, error) {
	return s.storeAllWorkspaces().Update(id, task)
}

func (s *AutomationAppService) UpdateIfRevision(id string, task automation.Task, baseRevision string) (automation.Task, error) {
	return s.storeAllWorkspaces().UpdateIfRevision(id, task, baseRevision)
}

func (a *App) DeleteAutomation(id string) error {
	return a.automation().Delete(id)
}

func (s *AutomationAppService) Delete(id string) error {
	store := s.storeAllWorkspaces()
	task, err := store.Get(id)
	if err != nil {
		return err
	}
	if s.hasActiveAutomationDefinition(automationTaskStoreID(task)) {
		return fmt.Errorf("%w: task_id=%s", automation.ErrTaskHasActiveRun, automationTaskStoreID(task))
	}
	return store.Delete(automationTaskStoreID(task))
}

func (a *App) StartAutomationTaskWithEvidence(ctx context.Context, id, trigger string, evidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	return a.automation().StartTaskWithEvidence(ctx, id, trigger, evidence)
}

// StartAutomationTaskCommand starts an HTTP/manual run with a caller-owned
// idempotency key. The derived run ID is persisted and reconciled across
// process restarts; transport retries therefore cannot allocate another run.
func (a *App) StartAutomationTaskCommand(ctx context.Context, id, commandID string, evidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	return a.automation().StartTaskCommand(ctx, id, commandID, evidence)
}

func (s *AutomationAppService) StartTaskCommand(ctx context.Context, id, commandID string, evidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	task, err := s.storeAllWorkspaces().Get(id)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if task.ArchivedAt != nil {
		return nil, automation.RunRecord{}, fmt.Errorf("%w: task_id=%s", automation.ErrTaskArchived, automationTaskStoreID(task))
	}
	// CatalogID and the per-workspace ID are aliases for one definition. Resolve
	// the alias before deriving durable command identity so transport retries
	// through either route can never admit two StartTurn operations.
	taskStoreID := automationTaskStoreID(task)
	runID, err := automationManualRunID(taskStoreID, commandID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, task.Target)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	defer operation.Release()
	started, run, err := s.startTaskWithSourceRunID(operation.Context(), snap, taskStoreID, automation.TriggerManual, "", runID, evidence)
	if err != nil || started != nil {
		return started, run, err
	}
	return replayAutomationRunTask(run), run, nil
}

// replayAutomationRunTask adapts a terminal persisted run to the same bounded
// SSE Task contract as a live execution. It never opens an Agent runner.
func replayAutomationRunTask(run automation.RunRecord) *Task {
	return NewTask(func(_ context.Context, _ *Task, emit func(agents.Event)) {
		emit(agents.Event{Type: "automation_run", Data: run})
		if run.Status == automation.RunStatusFailed && strings.TrimSpace(run.Error) != "" {
			emit(agents.Event{Type: "error", Data: map[string]string{"message": run.Error}})
		}
	})
}

func (s *AutomationAppService) StartTaskWithEvidence(ctx context.Context, id, trigger string, evidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	task, err := s.storeAllWorkspaces().Get(id)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if task.ArchivedAt != nil {
		return nil, automation.RunRecord{}, fmt.Errorf("%w: task_id=%s", automation.ErrTaskArchived, automationTaskStoreID(task))
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, task.Target)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	defer operation.Release()
	return s.startTaskWithSourceRun(operation.Context(), snap, automationTaskStoreID(task), trigger, "", evidence)
}

func (s *AutomationAppService) startTaskWithSourceRun(ctx context.Context, snap *automationWorkspaceSnapshot, id, trigger, sourceRunID string, triggerEvidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	return s.startTaskWithSourceRunID(ctx, snap, id, trigger, sourceRunID, "", triggerEvidence)
}

// startTaskWithSourceRunID starts one automation run with an optional stable
// identity supplied by a durable upstream command. Replaying the same identity
// returns the active or persisted run when its semantics match; conflicting
// reuse is rejected and never falls back to a generated ID.
func (s *AutomationAppService) startTaskWithSourceRunID(ctx context.Context, snap *automationWorkspaceSnapshot, id, trigger, sourceRunID, deterministicRunID string, triggerEvidence []automation.TriggerEvidence) (startedTask *Task, resultRun automation.RunRecord, resultErr error) {
	taskDef, err := storeForSnapshot(snap).Get(id)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if taskDef.ArchivedAt != nil {
		return nil, automation.RunRecord{}, fmt.Errorf("%w: task_id=%s", automation.ErrTaskArchived, automationTaskStoreID(taskDef))
	}

	run := s.newRunRecord(snap, taskDef, trigger)
	sourceRunID = strings.TrimSpace(sourceRunID)
	if trigger == automation.TriggerWriteConfirmation && sourceRunID != "" {
		if _, sourceErr := s.automationRunByID(snap, sourceRunID); sourceErr != nil {
			return nil, automation.RunRecord{}, sourceErr
		}
	}
	// Confirmation is a new durable execution. The source run contributes only
	// immutable source identity (and its bounded summary when building input);
	// runtime receipts, terminal state, output paths, and tool manifests must
	// never leak into this run's admission record.
	run.SourceRunID = sourceRunID
	run.TriggerEvidence = boundedRunTriggerEvidence(triggerEvidence)
	deterministicRunID = strings.TrimSpace(deterministicRunID)
	if deterministicRunID != "" {
		run.ID = deterministicRunID
		run.SessionID = automationRunSessionID(deterministicRunID)
		releaseRun, leaseErr := storeForSnapshot(snap).AcquireRunLease(ctx, automationTaskStoreID(taskDef), deterministicRunID)
		if leaseErr != nil {
			return nil, automation.RunRecord{}, leaseErr
		}
		defer func() {
			resultErr = errors.Join(resultErr, releaseRun())
		}()
		if activeTask, activeRun, ok := s.activeAutomationTaskByRunID(snap, deterministicRunID); ok {
			if !sameAutomationRunSemantics(taskDef, run, activeRun) {
				return nil, automation.RunRecord{}, automationRunIDConflict(deterministicRunID)
			}
			return activeTask, activeRun, nil
		}
		persisted, found, lookupErr := persistedAutomationRunByID(storeForSnapshot(snap), deterministicRunID)
		if lookupErr != nil {
			return nil, automation.RunRecord{}, lookupErr
		}
		if found {
			if !sameAutomationRunSemantics(taskDef, run, persisted) {
				return nil, automation.RunRecord{}, automationRunIDConflict(deterministicRunID)
			}
			receiptIncomplete := persisted.RuntimeCommandID == "" || persisted.RuntimeOperationID == "" || persisted.RuntimeReceiptCursor == 0
			if persisted.Status == automation.RunStatusRunning || receiptIncomplete {
				reconciled, ok, reconcileErr := s.reconcileAutomationRunReceipt(ctx, snap, taskDef, persisted)
				if reconcileErr != nil {
					return nil, automation.RunRecord{}, reconcileErr
				}
				if ok {
					if reconciled.RuntimeRecoveryRequired {
						return s.ensureAutomationRecoveryTask(ctx, snap, taskDef, reconciled)
					}
					if reconciled.Status == automation.RunStatusSuccess {
						reconciled, reconcileErr = s.completeAutomationRunEffects(ctx, snap, taskDef, reconciled)
					}
					return nil, reconciled, reconcileErr
				}
				if persisted.Status == automation.RunStatusRunning && !receiptIncomplete {
					persisted.RuntimeRecoveryRequired = true
					if _, appendErr := storeForSnapshot(snap).AppendRun(automationTaskStoreID(taskDef), persisted); appendErr != nil {
						return nil, automation.RunRecord{}, appendErr
					}
					return s.ensureAutomationRecoveryTask(ctx, snap, taskDef, persisted)
				}
			}
			if persisted.CompletionEffectsPending || (persisted.Status == automation.RunStatusSuccess && !persisted.CompletionEffectsCompleted) {
				completed, completionErr := s.completeAutomationRunEffects(ctx, snap, taskDef, persisted)
				return nil, completed, completionErr
			}
			if !receiptIncomplete || (persisted.Status != automation.RunStatusFailed && persisted.Status != automation.RunStatusRunning) {
				return nil, persisted, nil
			}
			// A failed/running record without a runtime receipt never crossed
			// StartTurn admission. Reuse the same deterministic identity and
			// original admission timestamp for retry.
			run.StartedAt = persisted.StartedAt
			if strings.TrimSpace(persisted.SessionID) != "" {
				run.SessionID = persisted.SessionID
			}
		}
		if !found {
			reconciled, ok, reconcileErr := s.reconcileAutomationRunReceipt(ctx, snap, taskDef, run)
			if reconcileErr != nil {
				return nil, automation.RunRecord{}, reconcileErr
			}
			if ok {
				if reconciled.RuntimeRecoveryRequired {
					return s.ensureAutomationRecoveryTask(ctx, snap, taskDef, reconciled)
				}
				if reconciled.Status == automation.RunStatusSuccess {
					reconciled, reconcileErr = s.completeAutomationRunEffects(ctx, snap, taskDef, reconciled)
				}
				return nil, reconciled, reconcileErr
			}
		}
	}
	taskStoreID := automationTaskStoreID(taskDef)
	claim, owner, err := s.reserveActiveAutomationRun(ctx, snap, taskStoreID, run)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if !owner {
		if deterministicRunID != "" && !sameAutomationRunSemantics(taskDef, run, claim.run) {
			return nil, automation.RunRecord{}, automationRunIDConflict(deterministicRunID)
		}
		log.Printf("[automation] attach active run workspace=%q task_id=%s run_id=%s status=%s", snap.workspace, taskDef.ID, claim.run.ID, claim.task.Status())
		return claim.task, claim.run, nil
	}
	claimActivated := false
	defer func() {
		if !claimActivated {
			s.releaseAutomationClaim(claim)
		}
	}()
	conversation, err := s.newRunConversation(snap, run, taskDef)
	if err != nil {
		result, _ := s.failAutomationRun(snap, taskDef, run, nil, false, err)
		return nil, result.Run, err
	}

	var execution *automationAcceptedRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		if err := s.activateAutomationClaim(claim, task); err != nil {
			return err
		}
		claimActivated = true
		return nil
	})
	if err != nil {
		// App registration is the final in-memory admission gate before the
		// durable Runtime is touched. Capacity/lifecycle rejection therefore
		// leaves no failed run ledger entry for an operation that never existed.
		return nil, automation.RunRecord{}, err
	}
	task.emit(agents.Event{Type: "automation_run", Data: run})
	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	execution, err = s.startAutomationRun(acceptCtx, snap, taskDef, run, conversation, task.emit)
	releaseAcceptance()
	if err != nil {
		if execution != nil {
			run = execution.run
		}
		result, _ := s.failAutomationRun(snap, taskDef, run, task.emit, false, err)
		if result.Run.ID != "" {
			task.emit(agents.Event{Type: "automation_run", Data: result.Run})
		}
		task.failBeforeStart(err)
		s.app.unregisterWorkspaceTask(task)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, result.Run, err
	}
	run = execution.run
	task.emit(agents.Event{Type: "automation_run", Data: run})
	if err := task.Start(func(taskCtx context.Context, task *Task, _ func(agents.Event)) {
		defer s.app.unregisterWorkspaceTask(task)
		defer s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		result, _ := s.waitAutomationRun(taskCtx, execution)
		if result.Run.ID != "" {
			task.emit(agents.Event{Type: "automation_run", Data: result.Run})
		}
	}); err != nil {
		task.Abort()
		_, _ = s.waitAutomationRun(task.ctx, execution)
		task.finish()
		s.app.unregisterWorkspaceTask(task)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, automation.RunRecord{}, err
	}
	return task, run, nil
}

func persistedAutomationRunByID(store *automation.Store, runID string) (automation.RunRecord, bool, error) {
	_, run, err := store.GetRunByID(runID)
	if errors.Is(err, automation.ErrRunNotFound) {
		return automation.RunRecord{}, false, nil
	}
	if err != nil {
		return automation.RunRecord{}, false, err
	}
	return run, true, nil
}

func sameAutomationRunSemantics(task automation.Task, expected, existing automation.RunRecord) bool {
	if strings.TrimSpace(existing.ID) != strings.TrimSpace(expected.ID) ||
		!automation.TaskMatchesID(task, existing.TaskID) ||
		normalizeAutomationTrigger(existing.Trigger) != normalizeAutomationTrigger(expected.Trigger) ||
		strings.TrimSpace(existing.SourceRunID) != strings.TrimSpace(expected.SourceRunID) ||
		existing.Scope != expected.Scope ||
		canonicalAutomationWorkspace(existing.Workspace) != canonicalAutomationWorkspace(expected.Workspace) ||
		len(existing.TriggerEvidence) != len(expected.TriggerEvidence) {
		return false
	}
	for index := range expected.TriggerEvidence {
		if existing.TriggerEvidence[index] != expected.TriggerEvidence[index] {
			return false
		}
	}
	return true
}

func automationRunIDConflict(runID string) error {
	return fmt.Errorf("%w: %s", automation.ErrRunIdentityConflict, strings.TrimSpace(runID))
}
