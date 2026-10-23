package app

import (
	"encoding/json"
	"testing"
)

func TestConversationConfigPatchPreservesExternalModelSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		check func(ConversationConfigPatch) bool
	}{
		{name: "grok", body: `{"grok":{"model":"grok-test"}}`, check: func(p ConversationConfigPatch) bool { return p.Grok != nil && p.Grok.Model == "grok-test" && p.Antigravity == nil }},
		{name: "antigravity", body: `{"antigravity":{"model":"antigravity-test"}}`, check: func(p ConversationConfigPatch) bool { return p.Antigravity != nil && p.Antigravity.Model == "antigravity-test" && p.Grok == nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var patch ConversationConfigPatch
			if err := json.Unmarshal([]byte(tc.body), &patch); err != nil { t.Fatal(err) }
			if !tc.check(patch) { t.Fatalf("CLI model patch was silently discarded: %+v", patch) }
		})
	}
}
