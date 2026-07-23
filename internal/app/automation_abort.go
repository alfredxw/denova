package app

import (
	"context"
	"fmt"
	"strings"

	"denova/internal/agent"
	"denova/internal/agentruntime"
)

// AbortAutomationRunCommand durably targets one exact Automation operation.
// It intentionally resolves persisted runs as well as live display Tasks so an
// uncertain transport retry can replay the original runtime receipt after the
// operation has already settled or the process has restarted.
func (a *App) AbortAutomationRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID agentruntime.OperationID,
	reason string,
) (agentruntime.Receipt, error) {
	return a.automation().AbortRunCommand(ctx, runID, commandID, targetOperationID, reason)
}

func (s *AutomationAppService) AbortRunCommand(
	ctx context.Context,
	runID, commandID string,
	targetOperationID agentruntime.OperationID,
	reason string,
) (agentruntime.Receipt, error) {
	commandID = strings.TrimSpace(commandID)
	runID = strings.TrimSpace(runID)
	targetOperationID = agentruntime.OperationID(strings.TrimSpace(string(targetOperationID)))
	if runID == "" || commandID == "" || targetOperationID == "" {
		return agentruntime.Receipt{}, fmt.Errorf("%w: run_id, command_id, and target_operation_id are required", agentruntime.ErrInvalidCommand)
	}
	if err := agentruntime.ValidateCommandID(commandID, agentruntime.DefaultInputLimits()); err != nil {
		return agentruntime.Receipt{}, err
	}

	store := s.storeAllWorkspaces()
	taskDef, run, err := store.GetRunByID(runID)
	if err != nil {
		return agentruntime.Receipt{}, err
	}
	if persisted := strings.TrimSpace(run.RuntimeOperationID); persisted != "" && persisted != string(targetOperationID) {
		return agentruntime.Receipt{}, fmt.Errorf("%w: target=%s run=%s", agentruntime.ErrStaleOperation, targetOperationID, persisted)
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, automationTargetForRun(taskDef, run))
	if err != nil {
		return agentruntime.Receipt{}, err
	}
	defer operation.Release()
	if snap.chatService == nil {
		return agentruntime.Receipt{}, ErrNoWorkspace
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
