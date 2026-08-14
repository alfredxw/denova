package execution

import (
	"errors"

	agentrun "denova/internal/agents/run"
)

const (
	RuntimeRecoveryRequiredEventType = "runtime_recovery_required"
	RuntimeRecoveryRequiredEventCode = "agent_runtime.recovery_required"
)

var (
	// ErrRecoveryActionChanged means the selected server-derived action is no
	// longer current.
	ErrRecoveryActionChanged = errors.New("Agent runtime recovery action changed")
)

type RuntimeRecoveryActionKind string

const (
	RuntimeRecoveryAttach RuntimeRecoveryActionKind = "start_turn"
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
	if snapshot.Phase == agentrun.RunPhaseRunning && snapshot.ActiveOperation != "" {
		return []RuntimeRecoveryAction{{
			Kind: RuntimeRecoveryAttach, CommandID: snapshot.ActiveCommandID, OperationID: snapshot.ActiveOperation,
		}}
	}
	return nil
}
