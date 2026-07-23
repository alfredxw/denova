package app

import (
	"context"
	"fmt"
	"strings"

	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
)

// AbortAutomationRunCommand durably targets one exact Automation operation.
// It intentionally resolves persisted runs as well as live display Tasks so an
// uncertain transport retry can replay the original runtime receipt after the
// operation has already settled or the process has restarted.
func (a *App) AbortAutomationRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID runstate.OperationID,
	reason string,
) (runstate.Receipt, error) {
	return a.automation().AbortRunCommand(ctx, runID, commandID, targetOperationID, reason)
}

func (s *AutomationAppService) AbortRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID runstate.OperationID,
	reason string,
) (runstate.Receipt, error) {
	commandID = strings.TrimSpace(commandID)
	runID = strings.TrimSpace(runID)
	targetOperationID = runstate.OperationID(strings.TrimSpace(string(targetOperationID)))
	if runID == "" || commandID == "" || targetOperationID == "" {
		return runstate.Receipt{}, fmt.Errorf("%w: run_id, command_id, and target_operation_id are required", runstate.ErrInvalidCommand)
	}
	if err := runstate.ValidateCommandID(commandID, runstate.DefaultInputLimits()); err != nil {
		return runstate.Receipt{}, err
	}

	store := s.storeAllWorkspaces()
	taskDef, run, err := store.GetRunByID(runID)
	if err != nil {
		return runstate.Receipt{}, err
	}
	if persisted := strings.TrimSpace(run.RuntimeOperationID); persisted != "" && persisted != string(targetOperationID) {
		return runstate.Receipt{}, fmt.Errorf("%w: target=%s run=%s", runstate.ErrStaleOperation, targetOperationID, persisted)
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, automationTargetForRun(taskDef, run))
	if err != nil {
		return runstate.Receipt{}, err
	}
	defer operation.Release()
	if snap.chatService == nil {
		return runstate.Receipt{}, ErrNoWorkspace
	}
	return snap.chatService.SubmitCommand(operation.Context(), agent.AgentCommandSpec{
		Kind:        agent.AgentCommandAbort,
		CommandID:   commandID,
		OperationID: targetOperationID,
		Reason:      strings.TrimSpace(reason),
		Options: agent.RunOptions{
			AgentKind: agent.AgentKindAutomation, TaskID: run.ID, AutomationTaskID: taskDef.ID,
			SessionID: run.SessionID, Workspace: run.Workspace, Mode: "automation",
		},
	})
}
