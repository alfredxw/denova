package execution

import (
	"errors"
	"strconv"

	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
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
	RuntimeRecoveryResume RuntimeRecoveryActionKind = "resume"
	RuntimeRecoveryAbort  RuntimeRecoveryActionKind = "abort"
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
	Attachments          []agent.Attachment
}

func RuntimeRecoveryActions(snapshot agentrun.RuntimeStatus) []RuntimeRecoveryAction {
	if snapshot.Phase == agentrun.RunPhaseSuspended && snapshot.ActiveOperation != "" {
		id := strconv.FormatUint(uint64(snapshot.Cursor), 10)
		return []RuntimeRecoveryAction{
			{Kind: RuntimeRecoveryResume, ActionID: id, CommandID: snapshot.ActiveCommandID, OperationID: snapshot.ActiveOperation},
			{Kind: RuntimeRecoveryAbort, ActionID: id, CommandID: snapshot.ActiveCommandID, OperationID: snapshot.ActiveOperation},
		}
	}
	if snapshot.Phase == agentrun.RunPhaseRunning && snapshot.ActiveOperation != "" {
		return []RuntimeRecoveryAction{{
			Kind: RuntimeRecoveryAttach, CommandID: snapshot.ActiveCommandID, OperationID: snapshot.ActiveOperation,
		}}
	}
	return nil
}
