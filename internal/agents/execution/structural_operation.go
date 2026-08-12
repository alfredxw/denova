package execution

import (
	"context"
	"fmt"

	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"

	agent "github.com/alfredxw/denova/agent"
)

// ExecuteStructuralOperation applies one manual compaction mutation to the
// same public Agent Session that owns normal turns and cold recovery.
func (s *Runtime) ExecuteStructuralOperation(ctx context.Context, spec agentstructural.Spec) (agentstructural.Result, error) {
	if s == nil || s.public == nil {
		return agentstructural.Result{}, ErrUnavailable
	}
	return s.public.executeStructural(ctx, spec)
}

func (backend *publicBackend) executeStructural(ctx context.Context, spec agentstructural.Spec) (agentstructural.Result, error) {
	if err := agent.ValidateIdempotencyKey(spec.CommandID); err != nil {
		return agentstructural.Result{}, fmt.Errorf("structural command_id is invalid: %w", err)
	}
	switch spec.Action {
	case agentstructural.Compact, agentstructural.Remove:
	default:
		return agentstructural.Result{}, fmt.Errorf("%w: unsupported structural action %q", agent.ErrInvalidInput, spec.Action)
	}
	session, _, err := backend.openSession(ctx, spec.Options)
	if err != nil {
		return agentstructural.Result{}, err
	}
	switch spec.Action {
	case agentstructural.Compact:
		result, err := session.Compact(ctx, agent.CompactionRequest{
			Force: spec.Ref.Force, IdempotencyKey: spec.CommandID,
			ExpectedID: spec.Ref.CompactionID,
		})
		return agentstructural.Result{Compaction: projectPublicCompaction(result)}, err
	case agentstructural.Remove:
		removed, err := session.RemoveCompaction(ctx, agent.CompactionRemoveRequest{
			ID: spec.Ref.CompactionID, IdempotencyKey: spec.CommandID,
		})
		return agentstructural.Result{Removed: removed}, err
	default:
		return agentstructural.Result{}, fmt.Errorf("%w: structural action changed after validation", agent.ErrInvalidInput)
	}
}

func projectPublicCompaction(result agent.CompactionResult) agentcompaction.Result {
	projected := agentcompaction.Result{
		Triggered: result.Changed, Phase: "manual", Summary: result.State.Summary,
		Epoch: int(result.State.Revision), TokensAfter: result.State.TokenEstimate,
		SourceMessageCount: max(0, result.State.ReplacementTo-result.State.ReplacementFrom),
	}
	if !result.Changed {
		projected.SkippedReason = "no_progress"
	}
	return projected
}
