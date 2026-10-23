package app

import (
	"context"
	"strings"

	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	interactiveapp "denova/internal/app/interactive"
)

// AbandonInteractiveInterruption is an explicit user operation for a detached
// CLI draft. It cannot discard an active controller or another interruption;
// repeated requests after successful settlement are harmless.
func (a *App) AbandonInteractiveInterruption(ctx context.Context, storyID, branchID, interruptionID string) error {
	if a == nil {
		return ErrNoWorkspace
	}
	storyID, branchID, interruptionID = strings.TrimSpace(storyID), strings.TrimSpace(branchID), strings.TrimSpace(interruptionID)
	if storyID == "" || interruptionID == "" {
		return ErrAgentContextChanged
	}
	service := a.interactiveService()
	service.admission.Lock()
	defer service.admission.Unlock()

	binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: a.ProjectID(), StoryID: storyID, BranchID: branchID}
	store, runtimeCfg, err := a.interactiveConversationRuntime(binding)
	if err != nil {
		return err
	}
	_, options, err := a.interactiveSessionRuntime(binding)
	if err != nil {
		return err
	}
	a.mu.RLock()
	task := interactiveTaskForScopeLocked(a, runtimeCfg.Workspace, options.StoryID, options.BranchID)
	a.mu.RUnlock()
	if task != nil && !task.Finished() {
		return ErrAgentOperationActive
	}
	pending, err := interactiveapp.ExternalTurnInterruption(store, storyID, options.BranchID)
	if err != nil {
		return err
	}
	if pending == nil {
		return nil
	}
	if pending.ID != interruptionID {
		return ErrAgentContextChanged
	}
	controller, selected, err := a.externalController(options)
	if err != nil {
		return err
	}
	if !selected {
		return agentruntime.ErrOperationActive
	}
	status, err := controller.Status(ctx)
	if err != nil {
		return err
	}
	if status.Phase != agentrun.RunPhaseIdle {
		return agentruntime.ErrOperationActive
	}
	return store.AbandonPendingTurn(storyID, options.BranchID, pending.PlayerInputID)
}
