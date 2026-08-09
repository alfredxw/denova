package automationapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"denova/internal/automation"
)

func (s *Service) automationRunByID(snap *automationWorkspaceSnapshot, runID string) (automation.RunRecord, error) {
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
	emit           func(agentrun.Event)
	accepted       ProjectConversationExecution
	runError       string
	errorForwarded bool
}

func (s *Service) startAutomationRun(ctx context.Context, snap *automationWorkspaceSnapshot, displayTask *apptask.Task, task automation.Task, run automation.RunRecord, emit func(agentrun.Event)) (execution *automationAcceptedRun, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("automation start panic recovered: %v", recovered)
			execution = nil
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] start panic recovered task_id=%s scope=%s workspace=%q trigger=%s err=%v", task.ID, task.Scope, run.Workspace, run.Trigger, recovered))
		}
	}()
	slog.InfoContext(ctx, fmt.Sprintf("[automation] run begin task_id=%s scope=%s workspace=%q trigger=%s template=%s", task.ID, task.Scope, run.Workspace, run.Trigger, task.Template))
	toolManifest, policyErr := automationInvocationManifest(snap)
	if policyErr != nil {
		return nil, policyErr
	}
	run.ToolManifest = toolManifest
	if err := persistAutomationAdmissionIntent(snap, task, &run); err != nil {
		return nil, err
	}
	execution = &automationAcceptedRun{
		snap: snap, task: task, run: run, emit: emit,
	}
	forward := func(ev agentrun.Event) {
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
	accepted, err := s.host.AcceptProjectConversationTurn(ctx, displayTask, ProjectConversationTurn{
		ProjectID:        snap.projectID,
		SessionID:        run.SessionID,
		CommandID:        run.TurnID,
		Message:          s.buildAutomationUserMessage(task, run),
		AutomationTaskID: task.ID,
		RunID:            run.ID,
		SessionTitle:     task.Name,
		ModelProfileID:   task.ModelProfileID,
		SessionStrategy:  task.SessionStrategy,
	}, forward)
	if err != nil {
		return nil, err
	}
	execution.accepted = accepted
	if err := persistAcceptedAutomationRun(snap, execution); err != nil {
		// The Agent journal already contains the accepted turn. Keep observing
		// its real outcome even when the automation ledger needs reconciliation.
		execution.run.RuntimeRecoveryRequired = true
		slog.WarnContext(ctx, fmt.Sprintf("[automation] accepted run ledger write deferred task_id=%s run_id=%s operation_id=%s err=%v", task.ID, run.ID, execution.run.RuntimeOperationID, err))
		if _, retryErr := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), execution.run); retryErr != nil {
			slog.WarnContext(ctx, fmt.Sprintf("[automation] accepted run recovery marker remains deferred task_id=%s run_id=%s operation_id=%s err=%v", task.ID, run.ID, execution.run.RuntimeOperationID, retryErr))
		}
	}
	return execution, nil
}

func (s *Service) waitAutomationRun(ctx context.Context, execution *automationAcceptedRun) (result automation.RunResult, err error) {
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
	if outcome.Status == agentrun.OutcomeAborted || outcome.Status == agentrun.OutcomePreempted {
		run.Summary = strings.TrimSpace(outcome.Content)
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
	if outcome.Status != agentrun.OutcomeCompleted {
		if execution.runError != "" {
			return result, errors.New(execution.runError)
		}
		return result, automationRunOutcomeError(outcome)
	}
	if execution.runError != "" {
		return result, errors.New(execution.runError)
	}
	output := strings.TrimSpace(outcome.Content)
	if output == "" {
		output = "Automation completed without a text summary."
	}
	run.Summary = output
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
	slog.InfoContext(ctx, fmt.Sprintf("[automation] run done task_id=%s scope=%s workspace=%q trigger=%s status=%s", task.ID, task.Scope, run.Workspace, run.Trigger, run.Status))
	return automation.RunResult{Task: updated, Run: run}, nil
}

func (s *Service) failAutomationRun(snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord, emit func(agentrun.Event), errorForwarded bool, cause error) (automation.RunResult, error) {
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
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": cause.Error()}})
	}
	return result, cause
}

func (s *Service) runAutomation(ctx context.Context, snap *automationWorkspaceSnapshot, displayTask *apptask.Task, task automation.Task, run automation.RunRecord, emit func(agentrun.Event)) (automation.RunResult, error) {
	execution, err := s.startAutomationRun(ctx, snap, displayTask, task, run, emit)
	if err != nil {
		if execution != nil {
			run = execution.run
		}
		return s.failAutomationRun(snap, task, run, emit, false, err)
	}
	return s.waitAutomationRun(ctx, execution)
}
