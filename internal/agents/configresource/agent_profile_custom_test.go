package configresource

import (
	"context"
	"strings"
	"testing"

	"denova/config"
)

func TestConfigApplyAgentProfileCreatesCustomAgentWithinBaseCeiling(t *testing.T) {
	novaDir := t.TempDir()
	cfg := &config.Config{NovaDir: novaDir, Workspace: t.TempDir()}
	readTool := configManagerToolByName(t, cfg, "config_read")
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	readOutput, err := runToolForTest(context.Background(), readTool, `{"operation":"list","resource":"agent_profile"}`)
	if err != nil {
		t.Fatal(err)
	}
	read := decodeAgentProfileListResult(t, readOutput)

	createdOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "agent_profile", "scope": "user", "id": "focused-editor",
		"revision": read.Revisions.User,
		"value": map[string]any{"kind": "custom_agent", "custom_agent": map[string]any{
			"id": "focused-editor", "name": "Focused editor", "base_kind": config.AgentKindIDE,
			"tools": map[string]any{config.AgentToolWebSearch: false},
		}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	created := decodeConfigMutationReceipt(t, createdOutput)
	if created.ID != "focused-editor" || created.Revision == "" {
		t.Fatalf("custom Agent receipt = %#v", created)
	}
	settings, err := config.ReadSettingsFile(config.UserConfigPath(novaDir))
	if err != nil {
		t.Fatal(err)
	}
	custom, ok := findCustomAgentByID(settings.CustomAgents, "focused-editor")
	if !ok || custom.Name != "Focused editor" || custom.BaseKind != config.AgentKindIDE || custom.Tools[config.AgentToolWebSearch] {
		t.Fatalf("persisted custom Agent = %#v, present=%v", custom, ok)
	}

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "agent_profile", "scope": "user", "id": "focused-editor",
		"revision": created.Revision,
		"value": map[string]any{"kind": "custom_agent", "custom_agent": map[string]any{
			"id": "focused-editor", "base_kind": config.AgentKindGeneral,
		}},
	})); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("custom Agent base kind mutation should fail, got %v", err)
	}

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "agent_profile", "scope": "user", "id": "focused-editor",
		"revision": created.Revision,
		"value": map[string]any{"kind": "custom_agent", "custom_agent": map[string]any{
			"id": "focused-editor", "tools": map[string]any{config.AgentToolEventRead: true},
		}},
	})); err == nil || !strings.Contains(err.Error(), config.AgentToolEventRead) {
		t.Fatalf("custom Agent tool ceiling should reject escalation, got %v", err)
	}
}
