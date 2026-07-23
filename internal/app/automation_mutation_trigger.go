package app

import (
	"context"

	"denova/internal/agent"
	"denova/internal/book"
)

// automationMutationCallback returns a callback that evaluates triggers after a
// workspace mutation. It captures an immutable snapshot of the current workspace
// so the callback stays bound to the correct runtime even if the app switches
// workspaces before the post-run verification fires.
func (a *App) automationMutationCallback(source string) func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
	snap := a.automationSnapshot()
	if snap == nil {
		return func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {}
	}
	svc := a.automation()
	return func(ctx context.Context, mutations []agent.ToolMutation, _ agent.PostRunVerification) {
		paths := make([]string, 0, len(mutations))
		for _, mutation := range mutations {
			if mutation.Target != "" {
				paths = append(paths, mutation.Target)
			}
		}
		svc.checkTriggersAfterWorkspaceMutation(ctx, snap, source, paths)
	}
}

// verifiedWorkspaceMutationCallback keeps every post-run side effect behind
// the same verified mutation event: automation reacts immediately, while Git
// version creation is only scheduled after the workspace becomes idle.
func (a *App) verifiedWorkspaceMutationCallback(
	source string,
	versionService *book.VersionService,
	settings book.VersionAutoSettings,
) func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
	automationCallback := a.automationMutationCallback(source)
	return func(ctx context.Context, mutations []agent.ToolMutation, verification agent.PostRunVerification) {
		automationCallback(ctx, mutations, verification)
		if len(mutations) > 0 {
			scheduleAutoVersion(versionService, settings)
		}
	}
}
