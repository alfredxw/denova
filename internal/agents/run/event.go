package agentrun

import (
	"encoding/json"
	"fmt"

	"denova/internal/observability"
)

// AbortReasonUserRequested identifies an expected pause initiated by the user.
// Other abort reasons remain operational failures and must not become resumable.
const AbortReasonUserRequested = "user_requested"

// Event is the transport-independent output envelope of an Agent run.
type Event struct {
	Type string
	Data any
}

// WithErrorDiagnostics captures the originating task identity before replay.
// Reconnecting transports must not replace it with a new connection's ID.
func (e Event) WithErrorDiagnostics(requestID, taskID string) Event {
	if e.Type != "error" {
		return e
	}
	payload := map[string]any{}
	if raw, err := json.Marshal(e.Data); err == nil {
		_ = json.Unmarshal(raw, &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if payload["request_id"] == nil && requestID != "" {
		payload["request_id"] = requestID
	}
	if payload["task_id"] == nil && taskID != "" {
		payload["task_id"] = taskID
	}
	if payload["code"] == nil {
		if key, ok := payload["error_key"].(string); ok {
			payload["code"] = key
		} else {
			payload["code"] = "agent_runtime.failed"
		}
	}
	observability.EnrichError(payload, "agent.run")
	e.Data = payload
	return e
}

// NewAbortedEvent creates the canonical terminal event consumed by every
// product transport. Abort diagnostics always travel in the reason field.
func NewAbortedEvent(reason string) Event {
	return Event{Type: "aborted", Data: map[string]string{"reason": reason}}
}

// DataString reads one string-like field from the two map representations used
// by transport-independent run events.
func (e Event) DataString(key string) string {
	switch data := e.Data.(type) {
	case map[string]string:
		return data[key]
	case map[string]any:
		if value, ok := data[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}
