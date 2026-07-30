package app

import (
	"context"
	"fmt"
	"strings"

	agents "denova/internal/agents"
)

// AbortAutomationRunCommand durably targets one exact Automation operation.
// It intentionally resolves persisted runs as well as live display Tasks so an
// uncertain transport retry can replay the original runtime receipt after the
// operation has already settled or the process has restarted.
func (a *App) AbortAutomationRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID agents.OperationID,
	reason string,
) (agents.CommandReceipt, error) {
	return a.automation().AbortRunCommand(ctx, runID, commandID, targetOperationID, reason)
}

func (s *AutomationAppService) AbortRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID agents.OperationID,
	reason string,
) (agents.CommandReceipt, error) {
	commandID = strings.TrimSpace(commandID)
	runID = strings.TrimSpace(runID)
	targetOperationID = agents.OperationID(strings.TrimSpace(string(targetOperationID)))
	if runID == "" || commandID == "" || targetOperationID == "" {
		return agents.CommandReceipt{}, fmt.Errorf("%w: run_id, command_id, and target_operation_id are required", agents.ErrInvalidCommand)
	}
	if err := agents.ValidateCommandID(commandID); err != nil {
		return agents.CommandReceipt{}, err
	}

	store := s.storeAllWorkspaces()
	taskDef, run, err := store.GetRunByID(runID)
	if err != nil {
		return agents.CommandReceipt{}, err
	}
	if persisted := strings.TrimSpace(run.RuntimeOperationID); persisted != "" && persisted != string(targetOperationID) {
		return agents.CommandReceipt{}, fmt.Errorf("%w: target=%s run=%s", agents.ErrStaleOperation, targetOperationID, persisted)
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, automationTargetForRun(taskDef, run))
	if err != nil {
		return agents.CommandReceipt{}, err
	}
	defer operation.Release()
	if snap.chatService == nil {
		return agents.CommandReceipt{}, ErrNoWorkspace
	}
	return snap.chatService.SubmitCommand(operation.Context(), agents.AgentCommandSpec{
		Kind:        agents.AgentCommandAbort,
		CommandID:   commandID,
		OperationID: targetOperationID,
		Reason:      strings.TrimSpace(reason),
		Options: agents.RunOptions{
			AgentKind: agents.AgentKindAutomation, ProjectID: snap.projectID, StateRoot: snap.stateRoot,
			TaskID: run.ID, AutomationTaskID: taskDef.ID,
			SessionID: run.SessionID, Workspace: run.Workspace, Mode: "automation",
		},
	})
}
