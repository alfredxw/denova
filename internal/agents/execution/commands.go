package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
)

// CommandKind is the closed command vocabulary exposed by Denova's public
// Agent adapter.
type CommandKind string

const (
	CommandStartTurn    CommandKind = "start_turn"
	CommandSteer        CommandKind = "steer"
	CommandFollowUp     CommandKind = "follow_up"
	CommandNextTurn     CommandKind = "next_turn"
	CommandSteerQueued  CommandKind = "steer_queued"
	CommandCancelQueued CommandKind = "cancel_queued"
	CommandAbort        CommandKind = "abort"
)

type CommandRequest struct {
	Kind             CommandKind
	CommandID        string
	OperationID      agentrun.OperationID
	AfterOperationID agentrun.OperationID
	TargetCommandID  agentrun.CommandID
	Reason           string
	Request          agentchat.ChatRequest
	Options          agentrun.Options
	Emit             func(agentrun.Event)
}

// SubmitCommand accepts one command without waiting for its selected run to
// settle. CommandID is the caller-owned idempotency key.
func (s *Runtime) SubmitCommand(ctx context.Context, request CommandRequest) (agentrun.CommandReceipt, error) {
	if s == nil || s.public == nil {
		return agentrun.CommandReceipt{}, ErrUnavailable
	}
	return s.public.submit(ctx, request)
}

// RequestSemanticFingerprint identifies one caller-visible logical request
// independently from server-resolved context and display task state.
func RequestSemanticFingerprint(request agentchat.ChatRequest) string {
	caller := agentchat.CallerView(request)
	caller.CommandID = ""
	return semanticJSONFingerprint("agent-chat-request.v1", caller)
}

// semanticJSONFingerprint provides versioned, deterministic identity for the
// closed JSON-safe descriptors used by the Denova adapter.
func semanticJSONFingerprint(scope string, value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		// Keep identity calculation total. Callers use only closed JSON-safe
		// values, so reaching this branch indicates an implementation defect.
		encoded = []byte("json_error:" + err.Error())
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(scope)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil))
}

func unsupportedCommand(kind CommandKind) error {
	return fmt.Errorf("unsupported Denova public Agent command %q", kind)
}
