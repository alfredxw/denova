package app

import (
	"testing"

	"denova/config"
)

func TestModelConfigSnapshotDetachesMutableProfileData(t *testing.T) {
	application := &App{cfg: &config.Config{ModelProfiles: []config.ModelProfileSettings{{
		ID:      "model",
		Headers: map[string]string{"X-Test": "original"},
		ProtocolOptions: map[string]any{
			"nested": map[string]any{"values": []any{map[string]any{"value": "original"}}},
		},
	}}}}
	snapshot := (modelHost{app: application}).ModelConfigSnapshot()
	snapshot.ModelProfiles[0].Headers["X-Test"] = "changed"
	nested := snapshot.ModelProfiles[0].ProtocolOptions["nested"].(map[string]any)
	values := nested["values"].([]any)
	values[0].(map[string]any)["value"] = "changed"

	profile := application.cfg.ModelProfiles[0]
	if profile.Headers["X-Test"] != "original" {
		t.Fatalf("header mutation leaked: %#v", profile.Headers)
	}
	originalNested := profile.ProtocolOptions["nested"].(map[string]any)
	originalValues := originalNested["values"].([]any)
	if originalValues[0].(map[string]any)["value"] != "original" {
		t.Fatalf("protocol option mutation leaked: %#v", profile.ProtocolOptions)
	}
}
