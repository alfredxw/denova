package automationapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"denova/internal/automation"
)

// Service owns automation scheduling, trigger coordination, active-run
// identity, and replay state. Workspace resources are captured on demand and
// never retained as mutable service state.
type Service struct {
	host               Host
	mu                 sync.RWMutex
	closed             bool
	activeTasks        map[string]*apptask.Task
	activeRuns         map[string]automationRunState
	activeClaims       map[string]*automationRunClaim
	triggers           *automationTriggerCoordinator
	schedulerCancel    context.CancelFunc
	schedulerWG        sync.WaitGroup
	schedulerStarted   bool
	effectWake         chan struct{}
	semanticEvaluator  semanticTriggerEvaluationFunc
	runtimeProjector   automationRuntimeProjectionFunc
	hostEffectTransfer func(context.Context, automation.HostEffectObligation, admittedToolMutationPayload) (bool, error)
}

// NewService creates the process-wide automation application service. The Host is
// retained only as a lifecycle and immutable-runtime boundary.
func NewService(host Host) *Service {
	return &Service{
		host:         host,
		activeTasks:  make(map[string]*apptask.Task),
		activeRuns:   make(map[string]automationRunState),
		activeClaims: make(map[string]*automationRunClaim),
		triggers:     newAutomationTriggerCoordinator(),
		effectWake:   make(chan struct{}, 1),
	}
}

// StartScheduler starts durable recovery and periodic trigger evaluation once.
func (s *Service) StartScheduler(ctx context.Context) {
	if s == nil || s.host == nil {
		return
	}
	s.mu.Lock()
	if s.schedulerStarted || s.closed {
		s.mu.Unlock()
		return
	}
	operation, err := s.host.AcquireRootOperation(ctx)
	if err != nil {
		s.mu.Unlock()
		slog.ErrorContext(ctx, fmt.Sprintf("[automation] scheduler admission failed err=%v", err))
		return
	}
	schedulerCtx, cancel := context.WithCancel(operation.Context())
	s.schedulerStarted = true
	s.schedulerCancel = cancel
	s.schedulerWG.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.schedulerWG.Done()
		defer operation.Release()
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[automation] scheduler panic recovered err=%v", recovered))
			}
		}()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		s.reconcilePersistedHostEffects(schedulerCtx)
		s.reconcilePersistedAutomationRuns(schedulerCtx)
		for {
			select {
			case <-schedulerCtx.Done():
				slog.InfoContext(ctx, fmt.Sprintf("[automation] scheduler stopped err=%v", schedulerCtx.Err()))
				return
			case now := <-ticker.C:
				s.runSchedulerTick(schedulerCtx, now)
			case <-s.effectWake:
				s.reconcilePersistedHostEffects(schedulerCtx)
				s.reconcilePersistedAutomationRuns(schedulerCtx)
			}
		}
	}()
}

// SignalReconciliation asks the scheduler to drain durable obligations soon.
func (s *Service) SignalReconciliation() {
	if s == nil || s.effectWake == nil {
		return
	}
	select {
	case s.effectWake <- struct{}{}:
	default:
	}
}

// Close fences new automation work, stops background coordinators, and waits
// for every admitted display task to settle.
func (s *Service) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.schedulerCancel
	tasks := make([]*apptask.Task, 0, len(s.activeTasks))
	seen := make(map[*apptask.Task]struct{}, len(s.activeTasks))
	for _, task := range s.activeTasks {
		if task != nil {
			if _, ok := seen[task]; !ok {
				seen[task] = struct{}{}
				tasks = append(tasks, task)
			}
		}
	}
	for key, claim := range s.activeClaims {
		if claim != nil && claim.task == nil {
			s.removeAutomationClaimLocked(key, claim)
		}
	}
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if s.triggers != nil {
		s.triggers.Close()
	}
	for _, task := range tasks {
		if !task.Finished() {
			task.Abort()
		}
	}
	for _, task := range tasks {
		if task.Finished() {
			continue
		}
		select {
		case <-task.Done():
		case <-ctx.Done():
			return fmt.Errorf("wait for automation task %s: %w", task.ID(), ctx.Err())
		}
	}
	s.schedulerWG.Wait()
	return nil
}

func (s *Service) runSchedulerTick(ctx context.Context, now time.Time) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] scheduler tick panic recovered workspace=%q err=%v", s.host.CurrentWorkspace(), recovered))
		}
	}()
	s.reconcilePersistedHostEffects(ctx)
	s.reconcilePersistedAutomationRuns(ctx)
	s.RunDue(ctx, now)
}

func (s *Service) List() ([]automation.Task, error) {
	return s.storeAllWorkspaces().List()
}

// ListForProject is the project-facing automation catalog. Automations are
// configured and observed through their owning Project Agent, so callers must
// not mix definitions from unrelated Projects into one view.
func (s *Service) ListForProject(projectID, workspace string) ([]automation.Task, error) {
	return s.storeAllWorkspaces().ListForTarget(automation.ExecutionTarget{
		Kind:      automation.TargetKindWorkspace,
		ProjectID: strings.TrimSpace(projectID),
		Workspace: strings.TrimSpace(workspace),
	})
}

func (s *Service) Templates(locale string) []automation.TaskTemplate {
	return automation.BuiltinTaskTemplates(locale)
}

func (s *Service) Create(definition automation.TaskDefinition) (automation.Task, error) {
	if err := requireProjectAutomationDefinition(definition); err != nil {
		return automation.Task{}, err
	}
	return s.storeAllWorkspaces().Create(definition)
}

func (s *Service) Update(id string, task automation.Task) (automation.Task, error) {
	if err := s.requireProjectAutomationUpdate(id, task); err != nil {
		return automation.Task{}, err
	}
	return s.storeAllWorkspaces().Update(id, task)
}

func (s *Service) UpdateIfRevision(id string, task automation.Task, baseRevision string) (automation.Task, error) {
	if err := s.requireProjectAutomationUpdate(id, task); err != nil {
		return automation.Task{}, err
	}
	return s.storeAllWorkspaces().UpdateIfRevision(id, task, baseRevision)
}

func requireProjectAutomationTask(task automation.Task) error {
	if task.Scope == automation.ScopeUser || task.Target.Kind == automation.TargetKindUser {
		return errors.New("自动化任务必须绑定项目 / Automation tasks must target a Project")
	}
	return nil
}

func requireProjectAutomationDefinition(definition automation.TaskDefinition) error {
	if definition.Scope == automation.ScopeUser || definition.Target.Kind == automation.TargetKindUser {
		return errors.New("自动化任务必须绑定项目 / Automation tasks must target a Project")
	}
	return nil
}

func (s *Service) requireProjectAutomationUpdate(id string, patch automation.Task) error {
	if err := requireProjectAutomationTask(patch); err != nil {
		return err
	}
	current, err := s.storeAllWorkspaces().Get(id)
	if err != nil {
		return err
	}
	return requireProjectAutomationTask(current)
}

func (s *Service) Delete(id string) error {
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

// StartTaskCommand starts an HTTP/manual run with a caller-owned
// idempotency key. The derived run ID is persisted and reconciled across
// process restarts; transport retries therefore cannot allocate another run.
func (s *Service) StartTaskCommand(ctx context.Context, id, commandID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
	task, err := s.storeAllWorkspaces().Get(id)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if task.ArchivedAt != nil {
		return nil, automation.RunRecord{}, fmt.Errorf("%w: task_id=%s", automation.ErrTaskArchived, automationTaskStoreID(task))
	}
	if err := requireProjectAutomationTask(task); err != nil {
		return nil, automation.RunRecord{}, err
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
	legacyTaskStoreID := automation.CatalogTaskID(task.Scope, task.Target.Workspace, task.ID)
	if legacyTaskStoreID != taskStoreID {
		legacyRunID, legacyIDErr := automationManualRunID(legacyTaskStoreID, commandID)
		if legacyIDErr != nil {
			return nil, automation.RunRecord{}, legacyIDErr
		}
		if _, found, lookupErr := persistedAutomationRunByID(storeForSnapshot(snap), legacyRunID); lookupErr != nil {
			return nil, automation.RunRecord{}, lookupErr
		} else if found {
			runID = legacyRunID
		}
	}
	started, run, err := s.startTaskWithSourceRunID(operation.Context(), snap, taskStoreID, automation.TriggerManual, "", runID, evidence)
	if err != nil || started != nil {
		return started, run, err
	}
	return replayAutomationRunTask(run), run, nil
}

// replayAutomationRunTask adapts a terminal persisted run to the same bounded
// SSE Task contract as a live execution. It never opens an Agent runner.
func replayAutomationRunTask(run automation.RunRecord) *apptask.Task {
	return apptask.New(func(_ context.Context, _ *apptask.Task, emit func(agentrun.Event)) {
		emit(agentrun.Event{Type: "automation_run", Data: run})
		if run.Status == automation.RunStatusFailed && strings.TrimSpace(run.Error) != "" {
			emit(agentrun.Event{Type: "error", Data: map[string]string{"message": run.Error}})
		}
	})
}

func (s *Service) StartTaskWithEvidence(ctx context.Context, id, trigger string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
	task, err := s.storeAllWorkspaces().Get(id)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if task.ArchivedAt != nil {
		return nil, automation.RunRecord{}, fmt.Errorf("%w: task_id=%s", automation.ErrTaskArchived, automationTaskStoreID(task))
	}
	if err := requireProjectAutomationTask(task); err != nil {
		return nil, automation.RunRecord{}, err
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, task.Target)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	defer operation.Release()
	return s.startTaskWithSourceRun(operation.Context(), snap, automationTaskStoreID(task), trigger, "", evidence)
}

func (s *Service) startTaskWithSourceRun(ctx context.Context, snap *automationWorkspaceSnapshot, id, trigger, sourceRunID string, triggerEvidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
	return s.startTaskWithSourceRunID(ctx, snap, id, trigger, sourceRunID, "", triggerEvidence)
}

// startTaskWithSourceRunID starts one automation run with an optional stable
// identity supplied by a durable upstream command. Replaying the same identity
// returns the active or persisted run when its semantics match; conflicting
// reuse is rejected and never falls back to a generated ID.
func (s *Service) startTaskWithSourceRunID(ctx context.Context, snap *automationWorkspaceSnapshot, id, trigger, sourceRunID, deterministicRunID string, triggerEvidence []automation.TriggerEvidence) (startedTask *apptask.Task, resultRun automation.RunRecord, resultErr error) {
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
		run.SessionID = automationSessionID(taskDef, deterministicRunID)
		run.TurnID = automationRunAgentCommandID(deterministicRunID)
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
			if automation.RunHasDurableObligation(persisted) &&
				(persisted.CompletionEffectsPending || persisted.Status == automation.RunStatusSuccess && !persisted.CompletionEffectsCompleted) {
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
		slog.InfoContext(ctx, fmt.Sprintf("[automation] attach active run workspace=%q task_id=%s run_id=%s status=%s", snap.workspace, taskDef.ID, claim.run.ID, claim.task.Status()))
		return claim.task, claim.run, nil
	}
	claimActivated := false
	defer func() {
		if !claimActivated {
			s.releaseAutomationClaim(claim)
		}
	}()
	var execution *automationAcceptedRun
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
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
	task.Emit(agentrun.Event{Type: "automation_run", Data: run})
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	execution, err = s.startAutomationRun(acceptCtx, snap, task, taskDef, run, task.Emit)
	releaseAcceptance()
	if err != nil {
		if execution != nil {
			run = execution.run
		}
		result, _ := s.failAutomationRun(snap, taskDef, run, task.Emit, false, err)
		if result.Run.ID != "" {
			task.Emit(agentrun.Event{Type: "automation_run", Data: result.Run})
		}
		task.RejectStart(err)
		s.host.UnregisterTask(task)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, result.Run, err
	}
	run = execution.run
	task.Emit(agentrun.Event{Type: "automation_run", Data: run})
	if err := task.Start(func(taskCtx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer s.host.UnregisterTask(task)
		defer s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		result, _ := s.waitAutomationRun(taskCtx, execution)
		if result.Run.ID != "" {
			task.Emit(agentrun.Event{Type: "automation_run", Data: result.Run})
		}
	}); err != nil {
		task.Abort()
		_, _ = s.waitAutomationRun(task.Context(), execution)
		task.Finish()
		s.host.UnregisterTask(task)
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
