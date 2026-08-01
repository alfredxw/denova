package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"
)

// AbortAutomationRunCommand durably targets one exact Automation operation.
// It intentionally resolves persisted runs as well as live display Tasks so an
// uncertain transport retry can replay the original runtime receipt after the
// operation has already settled or the process has restarted.
func (a *App) AbortAutomationRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID agentrun.OperationID,
	reason string,
) (agentrun.CommandReceipt, error) {
	return a.automation().AbortRunCommand(ctx, runID, commandID, targetOperationID, reason)
}

func (s *AutomationAppService) AbortRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID agentrun.OperationID,
	reason string,
) (agentrun.CommandReceipt, error) {
	commandID = strings.TrimSpace(commandID)
	runID = strings.TrimSpace(runID)
	targetOperationID = agentrun.OperationID(strings.TrimSpace(string(targetOperationID)))
	if runID == "" || commandID == "" || targetOperationID == "" {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: run_id, command_id, and target_operation_id are required", agentrun.ErrInvalidCommand)
	}
	if err := agentrun.ValidateCommandID(commandID); err != nil {
		return agentrun.CommandReceipt{}, err
	}

	store := s.storeAllWorkspaces()
	taskDef, run, err := store.GetRunByID(runID)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	if persisted := strings.TrimSpace(run.RuntimeOperationID); persisted != "" && persisted != string(targetOperationID) {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: target=%s run=%s", agentrun.ErrStaleOperation, targetOperationID, persisted)
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, automationTargetForRun(taskDef, run))
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	defer operation.Release()
	if snap.chatService == nil {
		return agentrun.CommandReceipt{}, ErrNoWorkspace
	}
	return snap.chatService.SubmitCommand(operation.Context(), agentharness.CommandSpec{
		Kind:        agentharness.CommandAbort,
		CommandID:   commandID,
		OperationID: targetOperationID,
		Reason:      strings.TrimSpace(reason),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindAutomation, ProjectID: snap.projectID, StateRoot: snap.stateRoot,
			TaskID: run.ID, AutomationTaskID: taskDef.ID,
			SessionID: run.SessionID, Workspace: run.Workspace, Mode: "automation",
		},
	})
}
