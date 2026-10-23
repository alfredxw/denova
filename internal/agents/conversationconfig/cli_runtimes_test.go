package conversationconfig

import (
	"encoding/json"
	"reflect"
	"testing"

	"denova/config"
)

func TestCLIRuntimeConversationModelPatchesRemainAtomicAndDetached(t *testing.T) {
	for _, engine := range []config.RuntimeID{config.RuntimeGrok, config.RuntimeAntigravity} {
		for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral, config.AgentKindInteractiveStory} {
			t.Run(string(engine)+"/"+kind, func(t *testing.T) {
				var initial Patch
				if err := json.Unmarshal([]byte(`{"runtime":{"kind":"`+string(engine)+`","`+string(engine)+`":{"model":"first","effort":"high"}}}`), &initial); err != nil {
					t.Fatal(err)
				}
				base := Config{AgentKind: kind, ProfileID: "dormant", Runtime: initial.Runtime}
				var patch Patch
				if err := json.Unmarshal([]byte(`{"`+string(engine)+`":{"model":"second"}}`), &patch); err != nil {
					t.Fatal(err)
				}
				next, err := Merge(&config.Config{}, base, patch)
				if err != nil {
					t.Fatal(err)
				}
				clone := next.Clone()
				if engine == config.RuntimeGrok {
					clone.Runtime.Grok.Model = "changed"
				} else {
					clone.Runtime.Antigravity.Model = "changed"
				}
				if reflect.DeepEqual(clone, next) || next.ProfileID != base.ProfileID {
					t.Fatal("snapshot aliases mutable settings or changed dormant profile")
				}
				if engine == config.RuntimeGrok && next.Runtime.Grok.Effort != "" || engine == config.RuntimeAntigravity && next.Runtime.Antigravity.Effort != "" {
					t.Fatal("old effort survived model replacement")
				}
				patch.Runtime = initial.Runtime
				if _, err := Merge(&config.Config{}, base, patch); err == nil {
					t.Fatal("ambiguous runtime/model patch accepted")
				}
				base.Runtime = nil
				patch.Runtime = nil
				if _, err := Merge(&config.Config{}, base, patch); err == nil {
					t.Fatal("model patch switched Native runtime")
				}
			})
		}
		for _, body := range []string{`{"` + string(engine) + `":null}`, `{"` + string(engine) + `":{"model":"x","profile_id":"unsupported"}}`} {
			var patch Patch
			if err := json.Unmarshal([]byte(body), &patch); err == nil {
				t.Fatalf("invalid patch accepted: %s", body)
			}
		}
	}
}
