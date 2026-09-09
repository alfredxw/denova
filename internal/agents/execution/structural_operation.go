package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// ExecuteStructuralOperation applies one manual compaction mutation to the
// same public Agent Session that owns normal turns. The caller prepares a fresh
// Cycle for the current product binding, without a player/user message. Its
// Definition is scoped to this command; no preceding turn or HostData is needed.
func (s *Runtime) ExecuteStructuralOperation(ctx context.Context, cycle Cycle, spec agentstructural.Spec) (agentstructural.Result, error) {
	if s == nil || s.public == nil {
		return agentstructural.Result{}, ErrUnavailable
	}
	return s.public.executeStructural(ctx, cycle, spec)
}

type structuralDefinitionContextKey struct{}

type structuralDefinition struct {
	session    string
	commandID  string
	definition agent.Definition
}

func resolveStructuralDefinition(ctx context.Context, request agent.PrepareRequest) (agent.Definition, error) {
	prepared, ok := ctx.Value(structuralDefinitionContextKey{}).(structuralDefinition)
	if !ok {
		return agent.Definition{}, errors.New("Denova structural command has no prepared Definition")
	}
	key, err := agentsession.CanonicalKey(request.Session.Key)
	if err != nil {
		return agent.Definition{}, err
	}
	if prepared.session != key || prepared.commandID != request.Run.CommandID {
		return agent.Definition{}, errors.New("Denova structural command Definition binding changed")
	}
	return prepared.definition, nil
}

func (backend *publicBackend) executeStructural(ctx context.Context, cycle Cycle, spec agentstructural.Spec) (agentstructural.Result, error) {
	if err := agent.ValidateIdempotencyKey(spec.CommandID); err != nil {
		return agentstructural.Result{}, fmt.Errorf("structural command_id is invalid: %w", err)
	}
	switch spec.Action {
	case agentstructural.Compact, agentstructural.Remove:
	default:
		return agentstructural.Result{}, fmt.Errorf("%w: unsupported structural action %q", agent.ErrInvalidInput, spec.Action)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cycle.Options = cycle.Options.Normalize(cycle.Options.Workspace)
	session, _, err := backend.openSession(ctx, cycle.Options)
	if err != nil {
		return agentstructural.Result{}, err
	}
	if cycle.Conversation == nil || cycle.Definition.Model == nil {
		return agentstructural.Result{}, errors.New("Denova structural command requires a prepared product conversation and Definition")
	}
	if err := loadCanonicalMessages(ctx, session, cycle.Conversation); err != nil {
		return agentstructural.Result{}, err
	}
	key, err := agentsession.CanonicalKey(session.Key())
	if err != nil {
		return agentstructural.Result{}, err
	}
	definition, err := backend.bindDefinition(ctx, agent.PrepareRequest{
		Session: agent.SessionView{Key: session.Key()}, Reason: agent.TurnReasonStructural,
		Run: agent.RunView{ID: spec.CommandID, CommandID: spec.CommandID, Cycle: 1},
	}, cycle, &publicCycleRegistration{cycle: &cycle, request: cycle.Request, options: cycle.Options})
	if err != nil {
		return agentstructural.Result{}, err
	}
	ctx = context.WithValue(ctx, structuralDefinitionContextKey{}, structuralDefinition{
		session: key, commandID: spec.CommandID, definition: definition,
	})
	slog.InfoContext(ctx, "[agent] prepared manual context operation from current product history",
		"session_namespace", session.Key().Namespace, "session_id", session.Key().ID,
		"command_id", spec.CommandID, "action", spec.Action)
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
