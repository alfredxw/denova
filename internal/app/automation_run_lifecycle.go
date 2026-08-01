package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/automation"
)

func (a *App) ContinueAutomationRun(ctx context.Context, runID, commandID, message string) (*Task, automation.RunRecord, error) {
	return a.automation().ContinueRun(ctx, runID, commandID, message)
}

func (s *AutomationAppService) ContinueRun(ctx context.Context, runID, commandID, message string) (*Task, automation.RunRecord, error) {
	identity, err := newAutomationFollowUpIdentity(runID, commandID, message)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	s.followUpAdmission.Lock()
	defer s.followUpAdmission.Unlock()
	if replay, run, ok, err := s.followUps.replay(identity); err != nil {
		return nil, automation.RunRecord{}, err
	} else if ok {
		slog.InfoContext(ctx, fmt.Sprintf("[automation] replay follow-up run_id=%s command_id=%s status=%s", identity.runID, identity.commandID, replay.Status()))
		return replay, run, nil
	}
	if _, _, ok := s.ActiveAutomationTaskByRunID(identity.runID); ok {
		return nil, automation.RunRecord{}, ErrAgentOperationActive
	}
	run, err := s.automationRunByID(nil, identity.runID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return nil, automation.RunRecord{}, fmt.Errorf("automation run %s has no session history", identity.runID)
	}
	target := automation.ExecutionTarget{Kind: automation.TargetKindUser}
	if strings.TrimSpace(run.Workspace) != "" {
		target = automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, WorkspaceID: run.ProjectID, Workspace: run.Workspace}
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, target)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	defer operation.Release()
	return s.continueRunWithSnapshot(operation.Context(), snap, identity)
}

func (s *AutomationAppService) continueRunWithSnapshot(ctx context.Context, snap *automationWorkspaceSnapshot, identity automationFollowUpIdentity) (*Task, automation.RunRecord, error) {
	run, err := s.automationRunByID(snap, identity.runID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return nil, automation.RunRecord{}, fmt.Errorf("automation run %s has no session history", identity.runID)
	}
	// The snapshot is already scoped to one exact Project, so the immutable
	// local task ID also resolves ledgers imported from path-owned catalogs.
	taskDef, err := storeForSnapshot(snap).Get(run.TaskID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if taskDef.ArchivedAt != nil {
		return nil, automation.RunRecord{}, fmt.Errorf("%w: task_id=%s", automation.ErrTaskArchived, automationTaskStoreID(taskDef))
	}
	taskStoreID := automationTaskStoreID(taskDef)
	releaseRun, err := storeForSnapshot(snap).AcquireRunLease(ctx, taskStoreID, run.ID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	defer func() {
		if releaseErr := releaseRun(); releaseErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] release follow-up run lease failed task_id=%s run_id=%s err=%v", taskDef.ID, run.ID, releaseErr))
		}
	}()
	// The lease may have waited behind recovery/effect reconciliation. Refresh
	// the exact run before evaluating successor intent.
	_, run, err = storeForSnapshot(snap).GetRunByID(identity.runID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	taskDef, run, err = s.fenceAutomationRunSuccessor(ctx, snap, taskDef, run)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if run.RuntimeCommandID == identity.commandID && strings.TrimSpace(run.RuntimeIntentHash) != "" {
		if run.RuntimeIntentHash != identity.fingerprint {
			return nil, automation.RunRecord{}, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, identity.commandID)
		}
		if strings.TrimSpace(run.PendingRuntimeCommandID) == "" && run.Status != automation.RunStatusRunning {
			return replayAutomationRunTask(run), run, nil
		}
	}
	if pending := strings.TrimSpace(run.PendingRuntimeCommandID); pending != "" &&
		(pending != identity.commandID || run.PendingRuntimeIntentHash != identity.fingerprint) {
		return nil, automation.RunRecord{}, fmt.Errorf("%w: pending successor command_id=%q", ErrAgentCommandConflict, pending)
	}
	activeRun := run
	activeRun.Status = automation.RunStatusRunning
	activeRun.Error = ""
	claim, owner, err := s.reserveActiveAutomationRun(ctx, snap, taskStoreID, activeRun)
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
	conversation, err := s.newRunConversation(snap, run, taskDef)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	var execution *automationAcceptedFollowUp
	task, err := NewDeferredRegisteredTaskWithContext(ctx, func(task *Task) error {
		if err := s.activateAutomationClaim(claim, task); err != nil {
			return err
		}
		claimActivated = true
		return nil
	})
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	reservation, err := s.followUps.reserve(identity, task)
	if err != nil {
		task.failBeforeStart(err)
		s.app.unregisterWorkspaceTask(task)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, automation.RunRecord{}, err
	}
	defer reservation.rollback()
	task.emit(agents.Event{Type: "automation_run", Data: activeRun})
	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	execution, err = s.startAutomationFollowUp(acceptCtx, snap, taskDef, run, conversation, identity.commandID, identity.fingerprint, identity.message, task.emit)
	releaseAcceptance()
	if err != nil {
		task.emit(agents.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		task.failBeforeStart(err)
		s.app.unregisterWorkspaceTask(task)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		if errors.Is(err, agents.ErrInvalidCommand) {
			return nil, automation.RunRecord{}, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, identity.commandID)
		}
		return nil, automation.RunRecord{}, err
	}
	task.emit(agents.Event{Type: "automation_run", Data: execution.run})
	reservation.bind(execution.run)
	if err := task.Start(func(taskCtx context.Context, task *Task, _ func(agents.Event)) {
		defer s.app.unregisterWorkspaceTask(task)
		defer s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		outcome := s.waitAutomationFollowUp(taskCtx, execution)
		finalRun := execution.run
		switch outcome.Status {
		case agents.RunOutcomeCompleted:
			finalRun.Status = automation.RunStatusSuccess
			finalRun.Error = ""
			finalRun.WriteConfirmationRequired = false
			finalRun.WriteConfirmationPolicyCaptured = true
			stageAutomationTerminalEffects(&finalRun, finalRun.CompletionMutationPaths)
		case agents.RunOutcomeAborted, agents.RunOutcomePreempted:
			finalRun.Status = automation.RunStatusAborted
			finalRun.Error = automationRunOutcomeError(outcome).Error()
			stageAutomationTerminalEffects(&finalRun, finalRun.CompletionMutationPaths)
		case agents.RunOutcomeFailed:
			finalRun.Status = automation.RunStatusFailed
			finalRun.Error = automationRunOutcomeError(outcome).Error()
			stageAutomationTerminalEffects(&finalRun, finalRun.CompletionMutationPaths)
		}
		finalRun.FinishedAt = time.Now().UTC()
		finalRun.RuntimeRecoveryRequired = false
		if _, appendErr := storeForSnapshot(snap).AppendRun(taskStoreID, finalRun); appendErr != nil {
			slog.ErrorContext(taskCtx, fmt.Sprintf("[automation] persist follow-up terminal failed task_id=%s run_id=%s operation_id=%s err=%v", taskDef.ID, run.ID, finalRun.RuntimeOperationID, appendErr))
			task.emit(agents.Event{Type: "error", Data: map[string]string{"message": appendErr.Error()}})
		} else if _, persistedRun, loadErr := storeForSnapshot(snap).GetRunByID(finalRun.ID); loadErr != nil {
			slog.ErrorContext(taskCtx, fmt.Sprintf("[automation] reload follow-up terminal effects failed task_id=%s run_id=%s operation_id=%s err=%v", taskDef.ID, run.ID, finalRun.RuntimeOperationID, loadErr))
			task.emit(agents.Event{Type: "error", Data: map[string]string{"message": loadErr.Error()}})
		} else {
			finalRun = persistedRun
			if persistedRun.CompletionEffectsPending {
				completedRun, completionErr := s.completeAutomationRunEffects(context.WithoutCancel(taskCtx), snap, taskDef, persistedRun)
				if completionErr != nil {
					slog.ErrorContext(taskCtx, fmt.Sprintf("[automation] persist follow-up completion effects failed task_id=%s run_id=%s operation_id=%s err=%v", taskDef.ID, run.ID, finalRun.RuntimeOperationID, completionErr))
					task.emit(agents.Event{Type: "error", Data: map[string]string{"message": completionErr.Error()}})
				} else {
					finalRun = completedRun
				}
			}
		}
		task.emit(agents.Event{Type: "automation_run", Data: finalRun})
	}); err != nil {
		task.Abort()
		_ = s.waitAutomationFollowUp(task.ctx, execution)
		task.finish()
		s.app.unregisterWorkspaceTask(task)
		s.clearActiveAutomationTask(snap, taskStoreID, run.ID)
		return nil, automation.RunRecord{}, err
	}
	return task, execution.run, nil
}

// fenceAutomationRunSuccessor closes both sides of the HostEffect handoff
// before StartTurn can admit a new operation: Runtime-to-app global obligations
// and the run-owned completion outbox. Failed and aborted operations are fenced
// exactly like successful ones because their committed tool effects are still
// durable obligations.
func (s *AutomationAppService) fenceAutomationRunSuccessor(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	task automation.Task,
	run automation.RunRecord,
) (automation.Task, automation.RunRecord, error) {
	if err := s.drainAutomationRunHostEffects(ctx, run.ID); err != nil {
		return task, run, err
	}
	latestTask, latestRun, err := storeForSnapshot(snap).GetRunByID(run.ID)
	if err != nil {
		return task, run, err
	}
	task, run = latestTask, latestRun
	terminal := run.Status == automation.RunStatusSuccess || run.Status == automation.RunStatusFailed || run.Status == automation.RunStatusAborted
	if terminal && (run.CompletionEffectsPending || !run.CompletionEffectsCompleted) {
		if _, err := s.completeAutomationRunEffects(ctx, snap, task, run); err != nil {
			return task, run, err
		}
	}
	// A HostEffect transfer can race the first plan read only if another
	// reconciler already owned it. Re-scan globally, then make the refreshed run
	// receipt the final successor-admission decision.
	if err := s.drainAutomationRunHostEffects(ctx, run.ID); err != nil {
		return task, run, err
	}
	task, run, err = storeForSnapshot(snap).GetRunByID(run.ID)
	if err != nil {
		return task, run, err
	}
	if run.CompletionEffectsPending || (terminal && !run.CompletionEffectsCompleted) {
		return task, run, fmt.Errorf("automation run %s completion effects are still pending", run.ID)
	}
	return task, run, nil
}

func (s *AutomationAppService) AutomationRunMessages(runID string) ([]session.HistoryEntry, error) {
	run, err := s.automationRunByID(nil, runID)
	if err != nil {
		return nil, err
	}
	target := automation.ExecutionTarget{Kind: automation.TargetKindUser}
	if strings.TrimSpace(run.Workspace) != "" {
		target = automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, Workspace: run.Workspace}
	}
	snap, operation, err := s.acquireTargetRuntime(context.Background(), target)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	return s.automationRunMessagesWithSnapshot(snap, runID)
}

func (s *AutomationAppService) automationRunMessagesWithSnapshot(snap *automationWorkspaceSnapshot, runID string) ([]session.HistoryEntry, error) {
	run, err := s.automationRunByID(snap, runID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return nil, fmt.Errorf("automation run %s has no session history", runID)
	}
	store := snap.sessionStore
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sess, err := store.Get(run.SessionID)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (a *App) AutomationRunMessages(sessionID string) ([]session.HistoryEntry, error) {
	return a.automation().AutomationRunMessages(sessionID)
}

func (s *AutomationAppService) automationRunByID(snap *automationWorkspaceSnapshot, runID string) (automation.RunRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return automation.RunRecord{}, fmt.Errorf("run_id is required")
	}
	if _, run, ok := s.ActiveAutomationTaskByRunID(runID); ok {
		return run, nil
	}
	store := storeForSnapshot(snap)
	if snap == nil {
		store = s.storeAllWorkspaces()
	}
	_, run, err := store.GetRunByID(runID)
	if err != nil {
		return automation.RunRecord{}, err
	}
	return run, nil
}

type automationAcceptedRun struct {
	snap           *automationWorkspaceSnapshot
	task           automation.Task
	run            automation.RunRecord
	conversation   automationOutputConversation
	emit           func(agents.Event)
	runtimeCfg     config.Config
	writeMode      string
	writeScope     string
	accepted       *agents.AcceptedRun
	runError       string
	errorForwarded bool
}

func (s *AutomationAppService) startAutomationRun(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord, conversation automationOutputConversation, emit func(agents.Event)) (execution *automationAcceptedRun, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("automation start panic recovered: %v", recovered)
			execution = nil
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] start panic recovered task_id=%s scope=%s workspace=%q trigger=%s err=%v", task.ID, task.Scope, run.Workspace, run.Trigger, recovered))
		}
	}()
	slog.InfoContext(ctx, fmt.Sprintf("[automation] run begin task_id=%s scope=%s workspace=%q trigger=%s template=%s", task.ID, task.Scope, run.Workspace, run.Trigger, task.Template))
	runtimeCfg := conversation.RuntimeConfig()
	writeMode, writeScope := effectiveAutomationWriteModeScope(task, run)
	run.WriteConfirmationRequired = automationRunNeedsWriteConfirmation(task, run)
	run.WriteConfirmationPolicyCaptured = true
	runtimeCfg = constrainAutomationTools(runtimeCfg, writeMode, writeScope)
	if task.Target.Kind == automation.TargetKindUser {
		runtimeCfg = constrainGlobalAutomationTools(runtimeCfg)
	}
	run.ToolManifest = automationToolManifest(&runtimeCfg)
	taskInstruction := agents.AutomationTaskInstruction{
		Name:         task.Name,
		Template:     task.Template,
		Prompt:       task.Prompt,
		WriteMode:    writeMode,
		WriteScope:   writeScope,
		OutputPolicy: task.OutputPolicy,
		OutputPath:   task.OutputPath,
		Workspace:    run.Workspace,
	}
	runner, systemPromptComposition, buildErr := buildAutomationAgentRunnerWithComposition(ctx, &runtimeCfg, snap.bookState, taskInstruction)
	if buildErr != nil {
		return nil, buildErr
	}
	chatService := snap.chatService
	bookService := snap.bookService
	if chatService == nil || (task.Target.Kind == automation.TargetKindWorkspace && bookService == nil) {
		return nil, ErrNoWorkspace
	}
	if err := persistAutomationAdmissionIntent(snap, task, &run); err != nil {
		return nil, err
	}
	execution = &automationAcceptedRun{
		snap: snap, task: task, run: run, conversation: conversation, emit: emit,
		runtimeCfg: runtimeCfg, writeMode: writeMode, writeScope: writeScope,
	}
	forward := func(ev agents.Event) {
		switch ev.Type {
		case "error":
			execution.runError = eventMessage(ev.Data)
			execution.errorForwarded = true
		case "tool_call":
			slog.InfoContext(ctx, fmt.Sprintf("[automation] tool call task_id=%s data=%v", task.ID, ev.Data))
		case "tool_result":
			slog.InfoContext(ctx, fmt.Sprintf("[automation] tool result task_id=%s data=%v", task.ID, ev.Data))
		}
		if emit != nil {
			emit(ev)
		}
	}
	accepted, err := chatService.StartWithOptions(ctx, runner, conversation, bookService, agents.ChatRequest{
		CommandID: automationRunAgentCommandID(run.ID),
		Message:   s.buildAutomationUserMessage(task, run, writeMode, writeScope),
	}, agents.RunOptions{
		AgentKind:          agents.AgentKindAutomation,
		ProjectID:          snap.projectID,
		StateRoot:          snap.stateRoot,
		TaskID:             run.ID,
		AutomationTaskID:   task.ID,
		SessionID:          run.SessionID,
		Workspace:          run.Workspace,
		Mode:               "automation",
		WriteMode:          writeMode,
		WriteScope:         writeScope,
		IdleTimeout:        agentIdleTimeout(runtimeCfg),
		ToolResultMaxBytes: agentToolResultMaxBytes(runtimeCfg),
		SystemPromptLog:    systemPromptComposition,
	}, forward)
	if err != nil {
		return nil, err
	}
	execution.accepted = accepted
	admissionPersisted := true
	if err := persistAcceptedAutomationRun(snap, execution); err != nil {
		// Acceptance is already durable in the Agent journal. A bookkeeping
		// failure must not cancel Wait and fabricate a failed terminal run; keep
		// observing the real runtime outcome and retry the recovery marker.
		admissionPersisted = false
		execution.run.RuntimeRecoveryRequired = true
		slog.WarnContext(ctx, fmt.Sprintf("[automation] accepted run ledger write deferred task_id=%s run_id=%s operation_id=%s err=%v", task.ID, run.ID, execution.run.RuntimeOperationID, err))
		if _, retryErr := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), execution.run); retryErr == nil {
			admissionPersisted = true
		} else {
			slog.WarnContext(ctx, fmt.Sprintf("[automation] accepted run recovery marker remains deferred task_id=%s run_id=%s operation_id=%s err=%v", task.ID, run.ID, execution.run.RuntimeOperationID, retryErr))
		}
	}
	_ = admissionPersisted
	return execution, nil
}

func (s *AutomationAppService) waitAutomationRun(ctx context.Context, execution *automationAcceptedRun) (result automation.RunResult, err error) {
	task := execution.task
	run := execution.run
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("automation panic recovered: %v", recovered)
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] panic recovered task_id=%s scope=%s workspace=%q trigger=%s err=%v", task.ID, task.Scope, run.Workspace, run.Trigger, recovered))
		}
		if err != nil {
			stageAutomationTerminalEffects(&run, run.CompletionMutationPaths)
			result, err = s.failAutomationRun(execution.snap, task, run, execution.emit, execution.errorForwarded, err)
		}
	}()

	outcome := execution.accepted.Wait(ctx)
	if outcome.Status == agents.RunOutcomeAborted || outcome.Status == agents.RunOutcomePreempted {
		output := execution.conversation.Output()
		run.Summary = strings.TrimSpace(output)
		run.Status = automation.RunStatusAborted
		run.Error = automationRunOutcomeError(outcome).Error()
		run.FinishedAt = time.Now().UTC()
		run.RuntimeRecoveryRequired = false
		stageAutomationTerminalEffects(&run, run.CompletionMutationPaths)
		updated, appendErr := storeForSnapshot(execution.snap).AppendRun(automationTaskStoreID(task), run)
		if appendErr != nil {
			return automation.RunResult{}, appendErr
		}
		_, run, appendErr = storeForSnapshot(execution.snap).GetRunByID(run.ID)
		if appendErr != nil {
			return automation.RunResult{}, appendErr
		}
		if run.CompletionEffectsPending {
			run, appendErr = s.completeAutomationRunEffects(context.WithoutCancel(ctx), execution.snap, updated, run)
			if appendErr != nil {
				return automation.RunResult{Task: updated, Run: run}, appendErr
			}
		}
		slog.InfoContext(ctx, fmt.Sprintf("[automation] run aborted task_id=%s scope=%s workspace=%q trigger=%s", task.ID, task.Scope, run.Workspace, run.Trigger))
		return automation.RunResult{Task: updated, Run: run}, nil
	}
	if outcome.Status != agents.RunOutcomeCompleted {
		if execution.runError != "" {
			err = errors.New(execution.runError)
		} else {
			err = automationRunOutcomeError(outcome)
		}
		return result, err
	}
	if execution.runError != "" {
		err = errors.New(execution.runError)
		return result, err
	}
	output := execution.conversation.Output()
	if strings.TrimSpace(output) == "" {
		output = "自动化任务已完成，Agent 未返回文字摘要。"
	}
	run.Summary = strings.TrimSpace(output)
	if path, writeErr := s.writeOptionalOutput(execution.snap, task, output, execution.runtimeCfg, execution.writeMode, execution.writeScope); writeErr != nil {
		err = writeErr
		return result, err
	} else if path != "" {
		run.OutputPath = path
	}
	run.Status = automation.RunStatusSuccess
	run.FinishedAt = time.Now().UTC()
	run.RuntimeRecoveryRequired = false
	stageAutomationTerminalEffects(&run, run.CompletionMutationPaths)
	updated, err := storeForSnapshot(execution.snap).AppendRun(automationTaskStoreID(task), run)
	if err != nil {
		return automation.RunResult{}, err
	}
	run, err = s.completeAutomationRunEffects(ctx, execution.snap, updated, run)
	if err != nil {
		return automation.RunResult{Task: updated, Run: run}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[automation] run done task_id=%s scope=%s workspace=%q trigger=%s status=%s output_path=%q", task.ID, task.Scope, run.Workspace, run.Trigger, run.Status, run.OutputPath))
	return automation.RunResult{Task: updated, Run: run}, nil
}

func (s *AutomationAppService) failAutomationRun(snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord, emit func(agents.Event), errorForwarded bool, cause error) (automation.RunResult, error) {
	run.Status = automation.RunStatusFailed
	run.Error = cause.Error()
	run.FinishedAt = time.Now().UTC()
	run.RuntimeRecoveryRequired = false
	stageAutomationTerminalEffects(&run, run.CompletionMutationPaths)
	result := automation.RunResult{}
	if updated, appendErr := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), run); appendErr == nil {
		if _, persistedRun, loadErr := storeForSnapshot(snap).GetRunByID(run.ID); loadErr == nil {
			run = persistedRun
		} else {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[automation] reload failed run effects failed task_id=%s run_id=%s err=%v", task.ID, run.ID, loadErr))
		}
		result = automation.RunResult{Task: updated, Run: run}
		if run.CompletionEffectsPending {
			if completed, completionErr := s.completeAutomationRunEffects(context.Background(), snap, task, run); completionErr == nil {
				result.Run = completed
			} else {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[automation] failed run mutation effects remain pending task_id=%s run_id=%s err=%v", task.ID, run.ID, completionErr))
			}
		}
	} else {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[automation] persist failed run failed task_id=%s run_id=%s err=%v", task.ID, run.ID, appendErr))
	}
	if emit != nil && !errorForwarded {
		emit(agents.Event{Type: "error", Data: map[string]string{"message": cause.Error()}})
	}
	return result, cause
}

func (s *AutomationAppService) runAutomation(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord, conversation automationOutputConversation, emit func(agents.Event)) (automation.RunResult, error) {
	execution, err := s.startAutomationRun(ctx, snap, task, run, conversation, emit)
	if err != nil {
		if execution != nil {
			run = execution.run
		}
		return s.failAutomationRun(snap, task, run, emit, false, err)
	}
	return s.waitAutomationRun(ctx, execution)
}

type automationAcceptedFollowUp struct {
	accepted *agents.AcceptedRun
	task     automation.Task
	run      automation.RunRecord
	emit     func(agents.Event)
}

func (s *AutomationAppService) startAutomationFollowUp(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord, conversation automationOutputConversation, commandID, intentHash, message string, emit func(agents.Event)) (execution *automationAcceptedFollowUp, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("automation follow-up start panic recovered: %v", recovered)
			execution = nil
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] follow-up start panic recovered task_id=%s run_id=%s err=%v", task.ID, run.ID, recovered))
		}
	}()
	slog.InfoContext(ctx, fmt.Sprintf("[automation] follow-up begin task_id=%s run_id=%s message_len=%d", task.ID, run.ID, len(message)))
	runtimeCfg := conversation.RuntimeConfig()
	writeMode, writeScope := effectiveAutomationWriteModeScope(task, run)
	runtimeCfg = constrainAutomationTools(runtimeCfg, writeMode, writeScope)
	if task.Target.Kind == automation.TargetKindUser {
		runtimeCfg = constrainGlobalAutomationTools(runtimeCfg)
	}
	taskInstruction := agents.AutomationTaskInstruction{
		Name:         task.Name,
		Template:     task.Template,
		Prompt:       task.Prompt,
		WriteMode:    writeMode,
		WriteScope:   writeScope,
		OutputPolicy: task.OutputPolicy,
		OutputPath:   task.OutputPath,
		Workspace:    run.Workspace,
	}
	runner, systemPromptComposition, err := buildAutomationAgentRunnerWithComposition(ctx, &runtimeCfg, snap.bookState, taskInstruction)
	if err != nil {
		return nil, err
	}
	chatService := snap.chatService
	bookService := snap.bookService
	if chatService == nil || (task.Target.Kind == automation.TargetKindWorkspace && bookService == nil) {
		return nil, ErrNoWorkspace
	}
	execution = &automationAcceptedFollowUp{
		task: task, run: run, emit: emit,
	}
	commandID = strings.TrimSpace(commandID)
	intentHash = strings.TrimSpace(intentHash)
	if commandID == "" || intentHash == "" {
		return nil, fmt.Errorf("automation follow-up durable intent is incomplete")
	}
	request := agents.ChatRequest{CommandID: commandID, Message: message}
	options := agents.RunOptions{
		AgentKind:          agents.AgentKindAutomation,
		ProjectID:          snap.projectID,
		StateRoot:          snap.stateRoot,
		TaskID:             run.ID,
		AutomationTaskID:   task.ID,
		SessionID:          run.SessionID,
		Workspace:          run.Workspace,
		Mode:               "automation",
		WriteMode:          writeMode,
		WriteScope:         writeScope,
		IdleTimeout:        agentIdleTimeout(runtimeCfg),
		ToolResultMaxBytes: agentToolResultMaxBytes(runtimeCfg),
		SystemPromptLog:    systemPromptComposition,
	}
	workspace := ""
	if bookService != nil {
		workspace = bookService.Workspace()
	}
	runtimeFingerprint, err := agents.DurableStartTurnFingerprint(request, options, workspace)
	if err != nil {
		return nil, fmt.Errorf("derive automation follow-up runtime fingerprint: %w", err)
	}
	if pending := strings.TrimSpace(execution.run.PendingRuntimeCommandID); pending != "" {
		if pending != commandID || execution.run.PendingRuntimeIntentHash != intentHash ||
			(strings.TrimSpace(execution.run.PendingRuntimeCommandFingerprint) != "" && execution.run.PendingRuntimeCommandFingerprint != runtimeFingerprint) {
			return nil, fmt.Errorf("%w: pending successor command_id=%q", ErrAgentCommandConflict, pending)
		}
		execution.run.PendingRuntimeCommandFingerprint = runtimeFingerprint
	} else {
		execution.run.PendingRuntimeCommandID = commandID
		execution.run.PendingRuntimeIntentHash = intentHash
		execution.run.PendingRuntimeCommandFingerprint = runtimeFingerprint
		execution.run.RuntimeSuccessorConflict = ""
		if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), execution.run); err != nil {
			return nil, fmt.Errorf("persist automation follow-up intent %s: %w", execution.run.ID, err)
		}
	}
	accepted, err := chatService.StartWithOptions(ctx, runner, conversation, bookService, request, options, emit)
	if err != nil {
		if errors.Is(err, agents.ErrInvalidCommand) {
			// A semantic command conflict proves this successor was not accepted.
			// Clear its write-ahead intent so startup recovery cannot mistake an
			// older command with the same ID for this follow-up.
			execution.run.PendingRuntimeCommandID = ""
			execution.run.PendingRuntimeIntentHash = ""
			execution.run.PendingRuntimeCommandFingerprint = ""
			execution.run.RuntimeSuccessorConflict = err.Error()
			if _, clearErr := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), execution.run); clearErr != nil {
				return nil, errors.Join(err, fmt.Errorf("clear rejected automation follow-up intent: %w", clearErr))
			}
		}
		return nil, err
	}
	execution.accepted = accepted
	rootReceipt := automationRootReceipt(execution.run)
	if err := validateAutomationReceipt(rootReceipt, automationRunAgentCommandID(execution.run.ID)); err != nil {
		abortCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = accepted.Wait(abortCtx)
		return execution, fmt.Errorf("automation follow-up root receipt is invalid: %w", err)
	}
	if err := applyAutomationRootReceipt(&execution.run, rootReceipt); err != nil {
		abortCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = accepted.Wait(abortCtx)
		return execution, err
	}
	if err := applyAutomationFollowUpReceipt(&execution.run, accepted.Receipt(), commandID, runtimeFingerprint); err != nil {
		abortCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = accepted.Wait(abortCtx)
		return execution, fmt.Errorf("automation follow-up receipt is invalid: %w", err)
	}
	execution.run.RuntimeIntentHash = intentHash
	execution.run.PendingRuntimeCommandID = ""
	execution.run.PendingRuntimeIntentHash = ""
	execution.run.PendingRuntimeCommandFingerprint = ""
	execution.run.RuntimeSuccessorConflict = ""
	execution.run.Status = automation.RunStatusRunning
	execution.run.FinishedAt = time.Time{}
	execution.run.Error = ""
	execution.run.RuntimeRecoveryRequired = false
	if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), execution.run); err != nil {
		abortCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = accepted.Wait(abortCtx)
		return execution, fmt.Errorf("persist accepted automation follow-up %s: %w", execution.run.ID, err)
	}
	s.updateActiveAutomationRun(snap, automationTaskStoreID(task), execution.run)
	return execution, nil
}

func (s *AutomationAppService) waitAutomationFollowUp(ctx context.Context, execution *automationAcceptedFollowUp) (outcome agents.RunOutcome) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] follow-up panic recovered task_id=%s run_id=%s err=%v", execution.task.ID, execution.run.ID, recovered))
			panicErr := fmt.Errorf("automation follow-up panic recovered: %v", recovered)
			if execution.emit != nil {
				execution.emit(agents.Event{Type: "error", Data: map[string]string{"message": panicErr.Error()}})
			}
			outcome = agents.RunOutcome{Status: agents.RunOutcomeFailed, Error: panicErr, Reason: panicErr.Error()}
		}
	}()
	outcome = execution.accepted.Wait(ctx)
	slog.InfoContext(ctx, fmt.Sprintf("[automation] follow-up end task_id=%s run_id=%s", execution.task.ID, execution.run.ID))
	return outcome
}

func (s *AutomationAppService) runAutomationFollowUp(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord, conversation automationOutputConversation, commandID, message string, emit func(agents.Event)) agents.RunOutcome {
	identity, identityErr := newAutomationFollowUpIdentity(run.ID, commandID, message)
	if identityErr != nil {
		if emit != nil {
			emit(agents.Event{Type: "error", Data: map[string]string{"message": identityErr.Error()}})
		}
		return agents.RunOutcome{Status: agents.RunOutcomeFailed, Error: identityErr, Reason: identityErr.Error()}
	}
	execution, err := s.startAutomationFollowUp(ctx, snap, task, run, conversation, identity.commandID, identity.fingerprint, identity.message, emit)
	if err != nil {
		if emit != nil {
			emit(agents.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		}
		return agents.RunOutcome{Status: agents.RunOutcomeFailed, Error: err, Reason: err.Error()}
	}
	outcome := s.waitAutomationFollowUp(ctx, execution)
	slog.InfoContext(ctx, fmt.Sprintf("[automation] follow-up end task_id=%s run_id=%s", task.ID, run.ID))
	return outcome
}
