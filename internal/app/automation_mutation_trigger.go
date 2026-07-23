package app

import (
	"context"

	"denova/internal/agent"
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
