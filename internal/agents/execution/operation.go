package execution

import (
	"context"
	agentrun "denova/internal/agents/run"
)

// Operation is a StartTurn command that has crossed the durable command
// acceptance boundary. Its caller owns exactly one Wait; cancellation during
// Wait is translated to a typed Abort while durable settlement remains the
// authority for the returned outcome.
type Operation struct {
	publicBackend *publicBackend
	publicHandle  *publicRunHandle
	publicReceipt agentrun.CommandReceipt
}

// Receipt returns the durable StartTurn acceptance receipt.
func (r *Operation) Receipt() agentrun.CommandReceipt {
	if r == nil {
		return agentrun.CommandReceipt{}
	}
	return r.publicReceipt
}

// Wait blocks until both the adapter outcome and durable operation settlement
// are known. It is intentionally separate from Start so API layers never
// publish a task ID for an unaccepted command.
func (r *Operation) Wait(ctx context.Context) agentrun.Outcome {
	if r == nil || r.publicBackend == nil || r.publicHandle == nil {
		return agentrun.NewOutcome(agentrun.OutcomeFailed, ErrUnavailable, ErrUnavailable.Error(), "", "")
	}
	return r.publicBackend.wait(ctx, r.publicHandle)
}
