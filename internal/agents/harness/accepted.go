package harness

import (
	"context"
	"fmt"

	runstate "github.com/alfredxw/denova/agent/runtime"

	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
)

// AcceptedRun is a StartTurn command that has crossed the durable command
// acceptance boundary. Its caller owns exactly one Wait; cancellation during
// Wait is translated to a typed Abort while durable settlement remains the
// authority for the returned outcome.
type AcceptedRun struct {
	owner            *coordinator
	harness          *runstate.Harness
	observation      runstate.Observation
	receipt          runstate.Receipt
	conversation     agentchat.Conversation
	options          agentrun.Options
	outcomes         <-chan agentrun.Outcome
	emit             func(agentrun.Event)
	stopObserving    context.CancelFunc
	registration     *harnessTurnSpecLease
	binding          runstate.BindingRef
	ephemeralBinding bool
}

// Receipt returns the durable StartTurn acceptance receipt.
func (r *AcceptedRun) Receipt() agentrun.CommandReceipt {
	if r == nil {
		return agentrun.CommandReceipt{}
	}
	return agentrun.CommandReceiptFromRuntime(r.receipt)
}

// Wait blocks until both the adapter outcome and durable operation settlement
// are known. It is intentionally separate from Start so API layers never
// publish a task ID for an unaccepted command.
func (r *AcceptedRun) Wait(ctx context.Context) agentrun.Outcome {
	if r == nil || r.owner == nil {
		err := fmt.Errorf("accepted agent run is unavailable")
		return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.stopObserving != nil {
		defer r.stopObserving()
	}
	if r.registration != nil {
		defer r.registration.release()
	}
	if r.ephemeralBinding && r.owner.runtime != nil {
		defer func() { _ = r.owner.runtime.CloseBinding(context.Background(), r.binding) }()
	}
	outcome := r.owner.waitForOperation(ctx, r.harness, r.observation, r.receipt, r.outcomes, r.emit)
	followedSuccessor := false
	// A replayed command owns its exact historical operation only. A newer
	// independent StartTurn is indistinguishable from an already-settled
	// successor in the bounded status projection, so following after cold replay
	// could return another command's output. Live AcceptedRun observation sees
	// the atomic parent→NextTurn transition and is the only safe follower owner.
	if !r.receipt.Replayed {
		outcome, followedSuccessor = r.owner.followAcceptedSuccessors(
			ctx, r.harness, r.observation, r.receipt.OperationID, outcome, r.emit,
		)
	}
	if r.receipt.Replayed {
		emitReplayedRunOutcome(r.emit, outcome)
		return outcome
	}
	publication := agentcompaction.Publication{}
	if outcome.Status == agentrun.OutcomeCompleted && !followedSuccessor {
		publication.Attempted, publication.Err = r.persistPreparedContextCompaction()
	}
	if !followedSuccessor {
		r.persistPreparedContextCompactionHealth(publication)
	}
	if outcome.Status == agentrun.OutcomeCompleted && r.emit != nil {
		r.emit(agentrun.Event{Type: "done", Data: map[string]string{}})
	}
	return outcome
}

func (r *AcceptedRun) persistPreparedContextCompactionHealth(publication agentcompaction.Publication) {
	provider, ok := r.conversation.(agentconversation.PostSettlementContextCompactionHealthProvider)
	if !ok || provider == nil || r.owner == nil {
		return
	}
	if err := provider.CommitPostSettlementContextCompactionHealth(r.owner.lifecycle, agentrun.OperationID(r.receipt.OperationID), publication); err != nil && r.emit != nil {
		r.emit(agentrun.Event{Type: "context_compaction", Data: map[string]any{
			"phase": "post_settlement", "status": "health_failed", "error": err.Error(),
		}})
	}
}

// emitReplayedRunOutcome reconstructs a bounded terminal display task from the
// durable runtime. It never invokes the Engine, provider, tool layer, or domain
// commit hooks; canonical Session history remains the source for full fidelity.
func emitReplayedRunOutcome(emit func(agentrun.Event), outcome agentrun.Outcome) {
	if emit == nil {
		return
	}
	if outcome.Thinking != "" {
		emit(agentrun.Event{Type: "thinking", Data: map[string]string{"content": outcome.Thinking}})
	}
	if outcome.Content != "" {
		emit(agentrun.Event{Type: "chunk", Data: map[string]string{"content": outcome.Content}})
	}
	switch outcome.Status {
	case agentrun.OutcomeCompleted:
		emit(agentrun.Event{Type: "done", Data: map[string]string{}})
	case agentrun.OutcomeAborted:
		emit(agentrun.Event{Type: "aborted", Data: map[string]string{"reason": outcome.Reason}})
	case agentrun.OutcomeFailed:
		message := outcome.Reason
		if message == "" && outcome.Error != nil {
			message = outcome.Error.Error()
		}
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": message}})
	default:
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": "Agent replay ended in an unsupported state"}})
	}
}

// persistPreparedContextCompaction submits automatic compaction only after the
// parent turn is durably settled. A checkpoint is an optimization over already
// canonical history: failure is visible, but cannot roll back a committed turn.
func (r *AcceptedRun) persistPreparedContextCompaction() (bool, error) {
	if cleanup, ok := r.conversation.(agentconversation.PostSettlementToolResultCleanupProvider); ok && cleanup != nil && r.owner != nil {
		if err := cleanup.CommitPostSettlementToolResultCleanup(r.owner.lifecycle, agentrun.OperationID(r.receipt.OperationID)); err != nil {
			if r.emit != nil {
				r.emit(agentrun.Event{Type: "context_cleanup", Data: map[string]any{
					"phase": "post_settlement", "status": "failed", "error": err.Error(),
				}})
			}
		}
	}
	provider, ok := r.conversation.(agentconversation.PostSettlementContextStructuralProvider)
	if !ok || provider == nil || r.owner == nil {
		return false, nil
	}
	spec, err := provider.PostSettlementContextStructuralSpec(r.owner.lifecycle, agentrun.OperationID(r.receipt.OperationID), r.options)
	if err == nil && spec != nil {
		spec.Emit = r.emit
		_, err = r.owner.executeContextStructuralOperation(r.owner.lifecycle, *spec)
	}
	if err == nil {
		return spec != nil, nil
	}
	if r.emit != nil {
		r.emit(agentrun.Event{Type: "context_compaction", Data: map[string]any{
			"phase": "post_settlement", "status": "failed", "error": err.Error(),
		}})
	}
	return true, err
}
