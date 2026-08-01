package agentrun

import "fmt"

// Event is the transport-independent output envelope of an Agent run.
type Event struct {
	Type string
	Data any
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
