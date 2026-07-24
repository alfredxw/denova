package app

import (
	"context"
	"strings"

	agents "denova/internal/agents"
)

// closeWorkspaceRuntimeBindings evicts durable harness actors only after the
// App workspace generation has drained. ChatService always owns the durable
// harness, including for isolated in-memory tests.
func (a *App) closeWorkspaceRuntimeBindings(ctx context.Context, workspaces ...string) error {
	if a == nil || a.chatService == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(workspaces))
	for _, workspace := range workspaces {
		workspace = lifecycleWorkspaceKey(workspace)
		if strings.TrimSpace(workspace) == "" {
			continue
		}
		if _, exists := seen[workspace]; exists {
			continue
		}
		seen[workspace] = struct{}{}
		selector, err := agents.RuntimeWorkspaceBindingSelector(workspace)
		if err != nil {
			return err
		}
		if err := a.chatService.CloseRuntimeBindings(ctx, selector); err != nil {
			return err
		}
	}
	return nil
}
