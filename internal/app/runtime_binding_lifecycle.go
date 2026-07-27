package app

import (
	"context"
	"strings"
)

// closeWorkspaceRuntimeBindings evicts foreground-owned harness actors only
// after the App workspace generation has drained. User-level AgentChat actors
// deliberately survive because changing the title-bar book does not change
// their project/session binding.
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
		if err := a.chatService.CloseForegroundWorkspaceBindings(ctx, workspace); err != nil {
			return err
		}
	}
	return nil
}
