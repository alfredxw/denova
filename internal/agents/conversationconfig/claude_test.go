package conversationconfig

import (
	"encoding/json"
	"testing"

	"denova/config"
)

func TestClaudeModelPatchIsIndependentAndSnapshotDetached(t *testing.T) {
	runtime := &config.Config{AgentRuntimes: config.AgentRuntimeSettings{General: &config.RuntimePreferences{Selected: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "opus", Effort: "max"}}}}
	base := Default(runtime, config.AgentKindGeneral)
	var patch Patch
	if err := json.Unmarshal([]byte(`{"claude":{"model":"sonnet"}}`), &patch); err != nil {
		t.Fatal(err)
	}
	next, err := Merge(runtime, base, patch)
	if err != nil {
		t.Fatal(err)
	}
	patch.Claude.Model = "edited"
	if base.Engine().Claude.Model != "opus" || next.Engine().Claude.Model != "sonnet" || next.Engine().Claude.Effort != "" {
		t.Fatal("model patch did not preserve independent snapshots")
	}
	copy := next.Clone()
	copy.Runtime.Claude.Model = "edited"
	if next.Engine().Claude.Model != "sonnet" {
		t.Fatal("clone shares runtime pointers")
	}
	for _, body := range []string{`{"claude":null}`, `{"claude":{"model":"sonnet","unknown":true}}`, `{"claude":{"model":"sonnet"},"codex":{"model":"other"}}`, `{"claude":{"model":"sonnet"},"thinking_level":"high"}`} {
		var invalid Patch
		if err := json.Unmarshal([]byte(body), &invalid); err == nil {
			if _, err = Merge(runtime, base, invalid); err == nil {
				t.Fatalf("accepted incompatible patch %s", body)
			}
		}
	}
}
