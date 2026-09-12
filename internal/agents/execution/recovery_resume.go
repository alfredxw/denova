package execution

import (
	"context"
	"fmt"

	agentchat "denova/internal/agents/chat"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

// resume prepares the accepted product cycle and imports its canonical lane
// before restarting the existing Run. No new input or product turn is created.
func (backend *publicBackend) resume(ctx context.Context, session *agent.Session, binding agentrun.RuntimeBinding, action RuntimeRecoveryAction, options agentrun.Options, emit func(agentrun.Event)) (*publicRunHandle, error) {
	input, found, err := session.RunInput(ctx, string(action.OperationID))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, agent.ErrNoActiveRun
	}
	data, err := agentlifecycle.DecodeTurnHostData(input)
	if err != nil {
		return nil, err
	}
	commandID := input.IdempotencyKey
	previous := backend.registration(session.Key(), commandID)
	var cycle Cycle
	if previous != nil {
		previous.mu.RLock()
		if previous.cycle != nil {
			cycle = *previous.cycle
		}
		previous.mu.RUnlock()
	}
	if cycle.Conversation == nil {
		cycle, err = backend.restoreCycle(ctx, binding, agent.PrepareRequest{
			Run: agent.RunView{ID: string(action.OperationID)}, Reason: agent.TurnReasonFollowUp,
		}, data, commandID, options, emit)
		if err != nil {
			return nil, err
		}
	}
	cycle.Options = mergePublicCycleRoute(cycle.Options, options)
	if err := loadCanonicalMessages(ctx, session, cycle.Conversation); err != nil {
		return nil, fmt.Errorf("restore canonical conversation before continuing Agent Run: %w", err)
	}
	registration := &publicCycleRegistration{
		cycle: &cycle, request: cycle.Request, options: cycle.Options, emit: emit,
		commandKind: commandKindFromPublicTurn(data.Kind), projectorBound: true,
		projector: agentchat.NewPublicEventProjector(cycle.Conversation, cycle.Request, cycle.Options, emit),
	}
	backend.rememberRegistration(session.Key(), commandID, registration)
	run, err := backend.agent.ResumeTree(ctx, session.Key(), agent.ResumeRequest{
		RunID: string(action.OperationID), IdempotencyKey: "resume:" + string(action.OperationID) + ":" + action.ActionID,
	})
	if err != nil {
		return nil, err
	}
	return backend.trackRun(session, run, registration, ""), nil
}
