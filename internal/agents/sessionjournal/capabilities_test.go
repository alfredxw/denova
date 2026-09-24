package sessionjournal

import (
	"encoding/json"
	"testing"

	"denova/internal/agents/conversationjournal"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestCapabilityRecoveryAndUpdate(t *testing.T) {
	var projection Projection
	key := agentsession.Named("capability-recovery")
	for i, state := range []string{`{"items":["first"]}`, `{"items":["second"]}`, "", `{"items":["restored"]}`} {
		kind := capabilitySetKind
		data := map[string]any{"capability": "todo"}
		if state == "" {
			kind = capabilityDeleteKind
		} else {
			data["state"] = json.RawMessage(state)
		}
		body, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		revision := agentsession.Revision(i + 1)
		payload, err := json.Marshal(Envelope{Type: RecordType, Key: key, Revision: revision, Kind: kind, Version: 1, Data: body})
		if err != nil {
			t.Fatal(err)
		}
		if handled, err := projection.Apply(conversationjournal.Record{Location: conversationjournal.Location{Cursor: conversationjournal.Cursor(i + 1)}, Payload: payload}); err != nil || !handled {
			t.Fatalf("apply: handled=%v err=%v", handled, err)
		}
		// Exercise the same serialized recovery index used when reopening products.
		snapshot, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		var restored Projection
		if err := json.Unmarshal(snapshot, &restored); err != nil {
			t.Fatal(err)
		}
		if err := restored.Normalize(); err != nil {
			t.Fatal(err)
		}
		for _, current := range []*Projection{&projection, &restored} {
			value, present, err := current.Capability(key, "todo")
			if err != nil || present != (state != "") || string(value) != state {
				t.Fatalf("revision %d: value=%s present=%v err=%v", revision, value, present, err)
			}
			if value, present, err := current.Capability(key, "missing"); err != nil || present || value != nil {
				t.Fatalf("missing capability: value=%s present=%v err=%v", value, present, err)
			}
			next, err := current.CapabilityUpdate(key, "todo", func(raw json.RawMessage, found bool) (json.RawMessage, error) {
				if string(raw) != state || found != (state != "") {
					t.Fatalf("update received stale capability: %s, %v", raw, found)
				}
				return json.RawMessage(`{"items":["next"]}`), nil
			})
			if err != nil || next == nil || next.Revision != revision+1 {
				t.Fatalf("update: envelope=%+v err=%v", next, err)
			}
		}
	}
}
