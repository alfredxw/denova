package app

import (
	"context"

	agents "denova/internal/agents"
	"denova/internal/book"
)

// automationMutationCallback is notification-only. Tool mutations become
// trigger authority exclusively through Runtime's durable HostEffect outbox,
// after the exact operation/cycle output-domain receipt. OnMutationsVerified
// runs before that receipt and must never evaluate Automation directly.
func (a *App) automationMutationCallback(_ string) func(context.Context, []agents.ToolMutation, agents.PostRunVerification) {
	return func(context.Context, []agents.ToolMutation, agents.PostRunVerification) {
		a.signalAutomationEffectReconciliation()
	}
}

// verifiedWorkspaceMutationCallback keeps every post-run side effect behind
// the same verified mutation event: automation reacts immediately, while Git
// version creation is only scheduled after the workspace becomes idle.
func (a *App) verifiedWorkspaceMutationCallback(
	source string,
	versionService *book.VersionService,
	settings book.VersionAutoSettings,
) func(context.Context, []agents.ToolMutation, agents.PostRunVerification) {
	automationCallback := a.automationMutationCallback(source)
	return func(ctx context.Context, mutations []agents.ToolMutation, verification agents.PostRunVerification) {
		automationCallback(ctx, mutations, verification)
		if len(mutations) > 0 {
			scheduleAutoVersion(versionService, settings)
		}
	}
}
