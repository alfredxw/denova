package execution

import (
	"context"
	"errors"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

const maxRuntimeRecoveryActions = 16

const (
	RuntimeRecoveryRequiredEventType = "runtime_recovery_required"
	RuntimeRecoveryRequiredEventCode = "agent_runtime.recovery_required"
)

var (
	// ErrRecoveryRequired is the public Agent recovery boundary exposed through
	// the Denova application API.
	ErrRecoveryRequired = agent.ErrRecoveryRequired
	// ErrRecoveryActionChanged means the selected server-derived action is no
	// longer current.
	ErrRecoveryActionChanged = errors.New("Agent runtime recovery action changed")
)

type RuntimeRecoveryActionKind string

const (
	RuntimeRecoveryAttach           RuntimeRecoveryActionKind = "start_turn"
	RuntimeRecoveryAbort            RuntimeRecoveryActionKind = "abort"
	RuntimeRecoverySteer            RuntimeRecoveryActionKind = "steer"
	RuntimeRecoveryFollowUp         RuntimeRecoveryActionKind = "follow_up"
	RuntimeRecoveryNextTurn         RuntimeRecoveryActionKind = "next_turn"
	RuntimeRecoveryCompactContext   RuntimeRecoveryActionKind = "compact_context"
	RuntimeRecoveryRemoveCompaction RuntimeRecoveryActionKind = "remove_compaction"
)

// RuntimeRecoveryAction is safe for public projection. It names accepted work
// without exposing the private Definition restore payload.
type RuntimeRecoveryAction struct {
	Kind        RuntimeRecoveryActionKind
	ActionID    string
	CommandID   agentrun.CommandID
	OperationID agentrun.OperationID
}

type RuntimeRecoveryDisplayMetadata struct {
	Message              string
	RegenerateFromTurnID string
}

func RuntimeRecoveryActions(snapshot agentrun.RuntimeStatus) []RuntimeRecoveryAction {
	actions := make([]RuntimeRecoveryAction, 0, len(snapshot.AgentRecoveryActions)+1)
	if snapshot.RecoveryPaused && snapshot.Phase == agentrun.RunPhaseRunning && snapshot.ActiveOperation != "" {
		actions = append(actions, RuntimeRecoveryAction{
			Kind: RuntimeRecoveryAttach, CommandID: snapshot.ActiveCommandID, OperationID: snapshot.ActiveOperation,
		})
	}
	for _, projected := range snapshot.AgentRecoveryActions {
		action := RuntimeRecoveryAction{
			ActionID: projected.ID, CommandID: agentrun.CommandID(projected.CommandID),
			OperationID: agentrun.OperationID(projected.RunID),
		}
		switch projected.Kind {
		case "abort_run":
			action.Kind = RuntimeRecoveryAbort
		case "resume_compaction":
			if projected.Compaction == "remove" {
				action.Kind = RuntimeRecoveryRemoveCompaction
			} else {
				action.Kind = RuntimeRecoveryCompactContext
			}
		case "resume_input":
			switch projected.Delivery {
			case "steer":
				action.Kind = RuntimeRecoverySteer
			case "follow_up":
				action.Kind = RuntimeRecoveryFollowUp
			case "next_turn":
				action.Kind = RuntimeRecoveryNextTurn
			default:
				continue
			}
		default:
			continue
		}
		if action.ActionID == "" || action.OperationID == "" {
			continue
		}
		actions = append(actions, action)
		if len(actions) >= maxRuntimeRecoveryActions {
			break
		}
	}
	return actions
}

// RuntimeRecoveryStatusProjection opens the exact public Agent Session so
// reconciliation can expose any recovery-paused state without running a model
// or tool effect.
func (s *Runtime) RuntimeRecoveryStatusProjection(ctx context.Context, options agentrun.Options) (agentrun.RuntimeStatus, error) {
	if s == nil || s.public == nil {
		return agentrun.RuntimeStatus{}, ErrRuntimeProjectionUnavailable
	}
	return s.public.status(ctx, options)
}
