package app

import (
	"context"

	"denova/internal/agent"
	"denova/internal/book"
)

// automationMutationCallback is notification-only. Tool mutations become
// trigger authority exclusively through Runtime's durable HostEffect outbox,
// after the exact operation/cycle output-domain receipt. OnMutationsVerified
// runs before that receipt and must never evaluate Automation directly.
func (a *App) automationMutationCallback(_ string) func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
	return func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
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
) func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
	automationCallback := a.automationMutationCallback(source)
	return func(ctx context.Context, mutations []agent.ToolMutation, verification agent.PostRunVerification) {
		automationCallback(ctx, mutations, verification)
		if len(mutations) > 0 {
			scheduleAutoVersion(versionService, settings)
		}
	}
}
