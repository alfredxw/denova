package agentrun

import "fmt"

// AbortReasonUserRequested identifies an expected pause initiated by the user.
// Other abort reasons remain operational failures and must not become resumable.
const AbortReasonUserRequested = "user_requested"

// Event is the transport-independent output envelope of an Agent run.
type Event struct {
	Type string
	Data any
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
