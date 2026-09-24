// Package agentruntime is Denova's runtime boundary. It binds product sessions
// to Native execution or external adapters without changing the independent
// agent module. Product preparation and domain commits remain with callers.
package agentruntime

import (
	"errors"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
)

var ErrOperationActive = errors.New("agent operation is already active")

// Command is shared by every conversation surface. Binding identity and native
// delivery semantics belong to the selected Session implementation.
type Command struct {
	Kind            agentexecution.CommandKind
	CommandID       string
	OperationID     agentrun.OperationID
	TargetCommandID agentrun.CommandID
	Reason          string
	Input           agentchat.ChatRequest
}

func RecoveryActionKey(action agentexecution.RuntimeRecoveryAction) string {
	return strings.Join([]string{action.ActionID, string(action.Kind), string(action.CommandID), string(action.OperationID)}, "\x00")
}

func ValidateRecoveryAction(status agentrun.RuntimeStatus, selected agentexecution.RuntimeRecoveryAction) error {
	for _, action := range agentexecution.RuntimeRecoveryActions(status) {
		if action == selected {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: action_id=%q kind=%q command_id=%q operation_id=%q",
		agentexecution.ErrRecoveryActionChanged,
		selected.ActionID,
		selected.Kind,
		selected.CommandID,
		selected.OperationID,
	)
}
