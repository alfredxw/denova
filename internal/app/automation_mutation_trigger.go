package app

import (
	"context"

	agenttool "denova/internal/agents/tool"
	"denova/internal/book"
)

// automationMutationCallback is notification-only. Tool mutations become
// trigger authority exclusively through Runtime's durable HostEffect outbox,
// after the exact operation/cycle output-domain receipt. OnMutationsVerified
// runs before that receipt and must never evaluate Automation directly.
func (a *App) automationMutationCallback(_ string) func(context.Context, []agenttool.Mutation, agenttool.Verification) {
	return func(context.Context, []agenttool.Mutation, agenttool.Verification) {
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
) func(context.Context, []agenttool.Mutation, agenttool.Verification) {
	automationCallback := a.automationMutationCallback(source)
	return func(ctx context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
		automationCallback(ctx, mutations, verification)
		if len(mutations) > 0 {
			scheduleAutoVersion(versionService, settings)
		}
	}
}
