package tools

import (
	"context"

	agent "github.com/alfredxw/denova/agent"
)

// Interrupt uses the existing resumable Session pause. It never promotes a
// queued message and never substitutes tree abort for an exact Run control.
func (tasks *LocalTasks) Interrupt(ctx context.Context, ref TaskRef, request agent.SuspendRequest) (agent.CommandReceipt, error) {
	_, session, err := tasks.open(ctx, ref)
	if err != nil {
		return agent.CommandReceipt{}, err
	}
	request.RunID = ref.Run
	suspension, err := session.SuspendAndClose(ctx, request)
	return suspension.Receipt, err
}
