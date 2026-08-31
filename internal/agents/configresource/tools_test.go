package configresource

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	workspacelayout "denova/internal/workspace"
)

func runToolForTest(ctx context.Context, candidate any, arguments string) (string, error) {
	var tool agent.Tool
	switch value := candidate.(type) {
	case agent.ToolDefinition:
		tool = value.Tool
	case agent.Tool:
		tool = value
	}
	if tool == nil {
		return "", fmt.Errorf("unsupported config resource tool test value %T", candidate)
	}
	result, err := tool.Run(ctx, arguments)
	return result.ModelContent, err
}

func TestConfigurationToolFactoryExposesOnlyRegistryTools(t *testing.T) {
	definitions, err := NewTools(&config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := configManagerToolNameSet(t, definitions); len(got) != 2 || !got["config_read"] || !got["config_apply"] {
		t.Fatalf("configuration tools = %v, want only config_read/config_apply", got)
	}
	for _, definition := range definitions {
		info, infoErr := definition.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		switch info.Name {
		case "config_read":
			if definition.Descriptor.Capability != config.AgentToolConfigRead || definition.Descriptor.Execution != agent.ToolExecutionParallelRead || definition.Descriptor.MutationScope != agent.ToolMutationNone {
				t.Fatalf("config_read descriptor = %#v", definition.Descriptor)
			}
		case "config_apply":
			if definition.Descriptor.Capability != config.AgentToolConfigApply || definition.Descriptor.Execution != agent.ToolExecutionConfigExclusive || definition.Descriptor.MutationScope != agent.ToolMutationConfig || definition.Descriptor.PostCheck != agent.ToolPostCheckConfigRevision {
				t.Fatalf("config_apply descriptor = %#v", definition.Descriptor)
			}
		}
	}
}

func TestConfigApplySchemaDocumentsAgentProfileRevisionAndDeleteKind(t *testing.T) {
	definition := configManagerDefinitionByName(t, &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}, "config_apply")
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info.Desc, "agent_profile SubAgent creates require the latest revision") ||
		!strings.Contains(info.Desc, "agent_profile deletes require value.kind") {
		t.Fatalf("config_apply description does not document resource-specific contracts: %q", info.Desc)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	revision, ok := schema.Properties.Get("revision")
	if !ok || !strings.Contains(revision.Description, "agent_profile SubAgent create") || !strings.Contains(revision.Description, "exact-scope revision") {
		t.Fatalf("config_apply revision schema = %#v", revision)
	}
	value, ok := schema.Properties.Get("value")
	if !ok || !strings.Contains(value.Description, "agent_profile delete requires value.kind") {
		t.Fatalf("config_apply value schema = %#v", value)
	}

	referencePath := filepath.Join("..", "..", "..", "skills", "configuration", "references", "agent-profile.md")
	reference, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("read agent-profile reference: %v", err)
	}
	if text := string(reference); !strings.Contains(text, "SubAgent create requires the latest revision for the exact target scope") ||
		!strings.Contains(text, "Every delete must include `value.kind`") {
		t.Fatalf("agent-profile reference does not document revision/delete routing contracts:\n%s", text)
	}
}

func TestConfigReadDescribesEveryResourceIncludingRuleSystem(t *testing.T) {
	readTool := configManagerToolByName(t, &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}, "config_read")
	output, err := runToolForTest(context.Background(), readTool, `{"operation":"describe"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"style_reference", "narrative_style", "story_director", "event_package", "rule_system",
		"state_system", "image_preset", "automation", "skill", "agent_profile",
	} {
		if !strings.Contains(output, `"name":"`+name+`"`) {
			t.Fatalf("config_read describe missing %s:\n%s", name, output)
		}
	}
	if strings.Contains(output, "list_tellers") || strings.Contains(output, "write_agent_configs") {
		t.Fatalf("legacy tool names leaked into resource registry:\n%s", output)
	}
}

func TestConfigReadSchemaHasNoArbitraryIDCountLimit(t *testing.T) {
	definition := configManagerDefinitionByName(t, &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}, "config_read")
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"maxItems":32`) {
		t.Fatalf("config_read schema still caps exact IDs: %s", encoded)
	}
}

func TestConfigReadReturnsExistingItemsAndMissingIDs(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	readTool := configManagerToolByName(t, cfg, "config_read")
	createdOutput, err := runToolForTest(context.Background(), applyTool, `{
		"operation":"create","resource":"style_reference",
		"value":{"name":"Partial","filename":"partial.md","content":"# Partial"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	created := decodeConfigMutationReceipt(t, createdOutput)
	output, err := runToolForTest(context.Background(), readTool, mustJSON(t, map[string]any{
		"operation": "get", "resource": "style_reference", "ids": []string{created.ID, ".denova/styles/missing.md"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Items      []json.RawMessage `json:"items"`
		MissingIDs []string          `json:"missing_ids"`
		Status     string            `json:"status"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || !reflect.DeepEqual(result.MissingIDs, []string{".denova/styles/missing.md"}) || result.Status != "partial" {
		t.Fatalf("partial config read = %s", output)
	}
	if _, err := runToolForTest(context.Background(), readTool, `{
		"operation":"get","resource":"style_reference","ids":[".denova/styles/missing.md"]
	}`); err == nil {
		t.Fatal("all-missing config read should fail")
	}
}

func TestConfigApplyUsesReadRevisionForStyleReference(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	applyTool := configManagerToolByName(t, cfg, "config_apply")

	createdOutput, err := runToolForTest(context.Background(), applyTool, `{
		"operation":"create",
		"resource":"style_reference",
		"value":{"name":"Noir","filename":"noir.md","content":"# Noir\n\nShort sentences."}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	created := decodeConfigMutationReceipt(t, createdOutput)
	if created.ID != ".denova/styles/noir.md" || created.Revision == "" {
		t.Fatalf("create receipt = %#v", created)
	}
	if _, err := runToolForTest(context.Background(), applyTool, `{
		"operation":"create",
		"resource":"style_reference",
		"value":{"name":"Noir","filename":"noir.md","content":"must not overwrite"}
	}`); err == nil {
		t.Fatal("style_reference create overwrote an existing resource")
	}

	updateInput := map[string]any{
		"operation": "update", "resource": "style_reference", "id": created.ID,
		"revision": created.Revision, "value": map[string]any{"content": "# Noir\n\nSharper sentences."},
	}
	updatedOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, updateInput))
	if err != nil {
		t.Fatal(err)
	}
	updated := decodeConfigMutationReceipt(t, updatedOutput)
	if updated.Revision == "" || updated.Revision == created.Revision {
		t.Fatalf("update revision = %q, previous %q", updated.Revision, created.Revision)
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, updateInput)); err == nil {
		t.Fatal("stale config_apply update unexpectedly succeeded")
	}

	deleteInput := map[string]any{
		"operation": "delete", "resource": "style_reference", "id": created.ID, "revision": updated.Revision,
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, deleteInput)); err != nil {
		t.Fatal(err)
	}
}

func TestConfigApplyReturnsBoundedStructuredReceiptDetails(t *testing.T) {
	definition := configManagerDefinitionByName(t, &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}, "config_apply")
	result, err := definition.Tool.Run(context.Background(), `{
		"operation":"create",
		"resource":"style_reference",
		"value":{"name":"Receipt","filename":"receipt.md","content":"# Receipt"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	var details configApplyReceiptDetails
	if err := json.Unmarshal(result.Details, &details); err != nil {
		t.Fatal(err)
	}
	if details.Schema != "config.mutation_receipt.v1" || details.Status != "applied" ||
		details.Resource != "style_reference" || details.Operation != "create" || details.ID == "" || details.Revision == "" {
		t.Fatalf("config receipt details = %#v", details)
	}
	if result.Metadata.Target != details.ID {
		t.Fatalf("result target = %q, want %q", result.Metadata.Target, details.ID)
	}
}

func TestConfigApplyRejectsUnknownResourceValueFields(t *testing.T) {
	applyTool := configManagerToolByName(t, &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}, "config_apply")
	_, err := runToolForTest(context.Background(), applyTool, `{
		"operation":"create",
		"resource":"automation",
		"value":{"target":{"kind":"user"},"name":"Review","created_at":"2026-01-01T00:00:00Z"}
	}`)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown automation fields should be rejected, got %v", err)
	}
}

func TestConfigAutomationResourceUsesDefinitionRevisionForCRUD(t *testing.T) {
	cfg := &config.Config{
		NovaDir: t.TempDir(), ProjectID: "project-contract",
		Workspace: t.TempDir(), ProjectStateDir: t.TempDir(),
	}
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	readTool := configManagerToolByName(t, cfg, "config_read")
	createdOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "automation", "scope": "workspace",
		"value": map[string]any{
			"target": map[string]any{"kind": "workspace"}, "name": "Contract automation",
			"template": "custom_prompt", "prompt": "Review the current project.",
			"session_strategy": "per_task",
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	created := decodeConfigMutationReceipt(t, createdOutput)
	if created.ID == "" || created.Revision == "" {
		t.Fatalf("create receipt = %#v", created)
	}
	if output, readErr := runToolForTest(context.Background(), readTool, mustJSON(t, map[string]any{
		"operation": "get", "resource": "automation", "ids": []string{created.ID}, "scope": "workspace",
	})); readErr != nil || !strings.Contains(output, "Contract automation") || !strings.Contains(output, `"session_strategy":"per_task"`) {
		t.Fatalf("get output=%s error=%v", output, readErr)
	}
	updatedOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "automation", "id": created.ID, "scope": "workspace", "revision": created.Revision,
		"value": map[string]any{"name": "Updated automation"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	updated := decodeConfigMutationReceipt(t, updatedOutput)
	if updated.Revision == "" || updated.Revision == created.Revision {
		t.Fatalf("update receipt = %#v", updated)
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "delete", "resource": "automation", "id": updated.ID, "scope": "workspace", "revision": updated.Revision,
	})); err != nil {
		t.Fatal(err)
	}
}

func TestConfigAutomationResourceBindsCurrentProjectAndRejectsUserScope(t *testing.T) {
	novaDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "current-workspace")
	projectStateDir := filepath.Join(t.TempDir(), "project-state")
	outside := filepath.Join(t.TempDir(), "outside-workspace")
	cfg := &config.Config{
		NovaDir: novaDir, ProjectID: "project-current", Workspace: workspace,
		ProjectStateDir: projectStateDir,
	}
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	readTool := configManagerToolByName(t, cfg, "config_read")

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "automation", "scope": "user",
		"value": map[string]any{
			"target": map[string]any{"kind": "user"}, "name": "Rejected user automation",
		},
	})); err == nil {
		t.Fatal("Config Manager created a user-scoped automation")
	}

	workspaceOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "automation", "scope": "workspace",
		"value": map[string]any{
			"target": map[string]any{"kind": "workspace", "workspace": outside}, "name": "Current workspace automation",
			"template": "custom_prompt", "prompt": "Review this workspacelayout.",
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	workspaceReceipt := decodeConfigMutationReceipt(t, workspaceOutput)
	if !strings.HasPrefix(workspaceReceipt.ID, "project-current:") {
		t.Fatalf("automation receipt did not use the bound Project identity: %#v", workspaceReceipt)
	}
	if _, err := os.Stat(filepath.Join(projectStateDir, "automations", "tasks.json")); err != nil {
		t.Fatalf("Project-owned automation was not stored in central Project state: %v", err)
	}
	if _, err := os.Stat(workspacelayout.Path(workspace, "automations", "tasks.json")); !os.IsNotExist(err) {
		t.Fatalf("Project-owned automation leaked into the content workspace: %v", err)
	}
	if _, err := os.Stat(workspacelayout.Path(outside, "automations", "tasks.json")); !os.IsNotExist(err) {
		t.Fatalf("untrusted value.target.workspace was written: %v", err)
	}

	workspaceList, err := runToolForTest(context.Background(), readTool, `{"operation":"list","resource":"automation","scope":"workspace"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, runtimeField := range []string{"trigger_state", "last_run", "recent_runs", "runtime_command_id"} {
		if strings.Contains(workspaceList, `"`+runtimeField+`"`) {
			t.Fatalf("automation definition projection leaked runtime field %q: %s", runtimeField, workspaceList)
		}
	}
	for _, removedField := range []string{"write_mode", "write_scope", "output_policy", "output_path"} {
		if strings.Contains(workspaceList, `"`+removedField+`"`) {
			t.Fatalf("automation definition projection retained removed field %q: %s", removedField, workspaceList)
		}
	}
	if !strings.Contains(workspaceList, "Current workspace automation") || strings.Contains(workspaceList, outside) {
		t.Fatalf("Project automation list exposed an untrusted path: %s", workspaceList)
	}

	if _, err := runToolForTest(context.Background(), readTool, mustJSON(t, map[string]any{
		"operation": "get", "resource": "automation", "scope": "user", "ids": []string{workspaceReceipt.ID},
	})); err == nil {
		t.Fatal("user-scoped get resolved a Project automation")
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "automation", "scope": "user", "id": workspaceReceipt.ID,
		"revision": workspaceReceipt.Revision, "value": map[string]any{"name": "scope escape"},
	})); err == nil {
		t.Fatal("user-scoped update mutated a Project automation")
	}
	if _, err := runToolForTest(context.Background(), readTool, `{"operation":"list","resource":"automation"}`); err == nil {
		t.Fatal("automation list accepted an omitted scope")
	}
}

func TestAgentProfileReadNeverReturnsSecrets(t *testing.T) {
	novaDir := t.TempDir()
	workspace := t.TempDir()
	if err := config.WriteSettingsFile(config.UserConfigPath(novaDir), config.Settings{
		OpenAIAPIKey:  "top-secret",
		ModelProfiles: []config.ModelProfileSettings{{ID: "model", APIKey: "profile-secret", Model: "model-v1"}},
		ImageAPIProfiles: []config.ImageAPIProfileSettings{{
			ID: "comfy", Provider: config.ImageProviderComfyUI, BaseURL: "http://127.0.0.1:8188",
			ComfyUI: &config.ComfyUIProfileSettings{WorkflowMode: config.ComfyUIWorkflowAPI, WorkflowName: "portrait.json", Workflow: "workflow-secret"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	readTool := configManagerToolByName(t, &config.Config{NovaDir: novaDir, Workspace: workspace}, "config_read")
	output, err := runToolForTest(context.Background(), readTool, `{"operation":"list","resource":"agent_profile"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"top-secret", "profile-secret", "workflow-secret", "openai_api_key"} {
		if strings.Contains(output, secret) {
			t.Fatalf("agent_profile leaked %q:\n%s", secret, output)
		}
	}
	if !strings.Contains(output, "model-v1") || !strings.Contains(output, "portrait.json") || !strings.Contains(output, `"revisions"`) {
		t.Fatalf("agent_profile response missing safe data or revisions:\n%s", output)
	}
}

func TestAgentProfileGetUsesExactSingletonSnapshotContract(t *testing.T) {
	readTool := configManagerToolByName(t, &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}, "config_read")
	output, err := runToolForTest(context.Background(), readTool, `{"operation":"get","resource":"agent_profile","ids":["registry"]}`)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeAgentProfileGetResult(t, output)
	if result.ID != agentProfileSnapshotID {
		t.Fatalf("agent_profile snapshot id = %q, want %q", result.ID, agentProfileSnapshotID)
	}
	for _, arguments := range []string{
		`{"operation":"get","resource":"agent_profile","ids":["ide"]}`,
		`{"operation":"get","resource":"agent_profile","ids":["registry"],"scope":"user"}`,
		`{"operation":"list","resource":"agent_profile","ids":["registry"]}`,
	} {
		if _, err := runToolForTest(context.Background(), readTool, arguments); err == nil {
			t.Fatalf("agent_profile accepted invalid read contract: %s", arguments)
		}
	}
}

func TestConfigApplyAgentProfileHonorsSettingsRevision(t *testing.T) {
	novaDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg := &config.Config{NovaDir: novaDir, Workspace: workspace}
	readTool := configManagerToolByName(t, cfg, "config_read")
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	output, err := runToolForTest(context.Background(), readTool, `{"operation":"list","resource":"agent_profile"}`)
	if err != nil {
		t.Fatal(err)
	}
	read := decodeAgentProfileListResult(t, output)
	input := map[string]any{
		"operation": "update", "resource": "agent_profile", "id": config.AgentKindIDE,
		"scope": "user", "revision": read.Revisions.User,
		"value": map[string]any{"kind": "agent", "prompt": map[string]any{"system_prompt": "Keep answers concise."}},
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, input)); err != nil {
		t.Fatal(err)
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, input)); err == nil {
		t.Fatal("stale agent_profile revision unexpectedly succeeded")
	}
}

func TestConfigApplyAgentProfileCreateCannotBypassScopeRevision(t *testing.T) {
	novaDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg := &config.Config{NovaDir: novaDir, Workspace: workspace}
	readTool := configManagerToolByName(t, cfg, "config_read")
	applyTool := configManagerToolByName(t, cfg, "config_apply")

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "agent_profile", "scope": "user", "id": config.AgentKindIDE,
		"value": map[string]any{"kind": "agent", "prompt": map[string]any{"system_prompt": "blind overwrite"}},
	})); err == nil {
		t.Fatal("agent_profile create bypassed the settings revision")
	}

	output, err := runToolForTest(context.Background(), readTool, `{"operation":"list","resource":"agent_profile"}`)
	if err != nil {
		t.Fatal(err)
	}
	read := decodeAgentProfileListResult(t, output)
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "agent_profile", "scope": "user", "id": config.AgentKindIDE,
		"revision": read.Revisions.User,
		"value":    map[string]any{"kind": "agent", "prompt": map[string]any{"system_prompt": "still not a new profile"}},
	})); err == nil {
		t.Fatal("agent_profile create overwrote a fixed Agent profile")
	}
	createdOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "agent_profile", "scope": "user", "revision": read.Revisions.User,
		"value": map[string]any{"kind": "sub_agent", "sub_agent": map[string]any{
			"id": "researcher", "description": "Research bounded sources.", "system_prompt": "Return concise findings.",
		}},
	}))
	if err != nil {
		t.Fatalf("revision-protected SubAgent create failed: %v", err)
	}
	if receipt := decodeConfigMutationReceipt(t, createdOutput); receipt.ID != "researcher" {
		t.Fatalf("SubAgent create receipt id = %q, want researcher", receipt.ID)
	}
}

func TestConfigApplyAgentProfileDeleteRequiresKindAndRoutesByKind(t *testing.T) {
	novaDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	disabled := false
	userPath := config.UserConfigPath(novaDir)
	if err := config.WriteSettingsFile(userPath, config.Settings{
		AgentPrompts:     config.AgentPromptSettings{IDE: config.AgentPromptOverride{SystemPrompt: "preserve this Agent prompt"}},
		GeneralSubAgents: config.AgentGeneralSubAgentSettings{IDE: &disabled},
		SubAgents: []config.SubAgentConfig{{
			ID: "researcher", Description: "Research bounded sources.", SystemPrompt: "Return concise findings.",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.EnsureAgentProfiles(novaDir); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{NovaDir: novaDir, Workspace: workspace}
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	revision, err := config.UserSettingsRevision(novaDir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "delete", "resource": "agent_profile", "scope": "user", "id": config.AgentKindIDE,
		"revision": revision,
	})); err == nil || !strings.Contains(err.Error(), "value.kind") {
		t.Fatalf("agent_profile delete without value.kind should fail, got %v", err)
	}
	if got, err := config.UserSettingsRevision(novaDir); err != nil || got != revision {
		t.Fatalf("rejected ambiguous delete changed revision: got=%q want=%q err=%v", got, revision, err)
	}

	generalOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "delete", "resource": "agent_profile", "scope": "user", "id": config.AgentKindIDE,
		"revision": revision, "value": map[string]any{"kind": "general_sub_agent"},
	}))
	if err != nil {
		t.Fatalf("delete General SubAgent override: %v", err)
	}
	generalReceipt := decodeConfigMutationReceipt(t, generalOutput)
	afterGeneral, _, err := config.LoadAgentProfileSettings(novaDir)
	if err != nil {
		t.Fatal(err)
	}
	if afterGeneral.GeneralSubAgents.IDE != nil {
		t.Fatalf("General SubAgent override was not deleted: %#v", afterGeneral.GeneralSubAgents.IDE)
	}
	if afterGeneral.AgentPrompts.IDE.SystemPrompt != "preserve this Agent prompt" {
		t.Fatalf("General SubAgent delete was misrouted to Agent profile: %#v", afterGeneral.AgentPrompts.IDE)
	}
	if _, exists := findSubAgentByID(afterGeneral.SubAgents, "researcher"); !exists {
		t.Fatal("General SubAgent delete removed a custom SubAgent")
	}

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "delete", "resource": "agent_profile", "scope": "user", "id": "researcher",
		"revision": generalReceipt.Revision, "value": map[string]any{"kind": "sub_agent"},
	})); err != nil {
		t.Fatalf("delete custom SubAgent: %v", err)
	}
	afterSubAgent, _, err := config.LoadAgentProfileSettings(novaDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := findSubAgentByID(afterSubAgent.SubAgents, "researcher"); exists {
		t.Fatal("custom SubAgent was not deleted")
	}
}

func TestConfigApplyAgentProfileEnforcesActiveAgentToolCeiling(t *testing.T) {
	novaDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg := &config.Config{NovaDir: novaDir, Workspace: workspace}
	readTool := configManagerToolByName(t, cfg, "config_read")
	applyTool := configManagerToolByName(t, cfg, "config_apply")

	readRevision := func(scope string) string {
		t.Helper()
		output, err := runToolForTest(context.Background(), readTool, `{"operation":"list","resource":"agent_profile"}`)
		if err != nil {
			t.Fatal(err)
		}
		read := decodeAgentProfileListResult(t, output)
		if scope == "workspace" {
			return read.Revisions.Workspace
		}
		return read.Revisions.User
	}

	userRevision := readRevision("user")
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "agent_profile", "scope": "user", "id": config.AgentKindConfigManager,
		"revision": userRevision, "value": map[string]any{"kind": "agent", "tools": map[string]any{config.AgentToolConfigRead: true}},
	})); err == nil || !strings.Contains(err.Error(), config.AgentKindConfigManager) {
		t.Fatalf("retired Config Manager settings must not be actively editable, got %v", err)
	}

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "agent_profile", "scope": "user", "id": config.AgentKindGeneral,
		"revision": userRevision, "value": map[string]any{"kind": "agent", "tools": map[string]any{config.AgentToolEventRead: true}},
	})); err == nil || !strings.Contains(err.Error(), config.AgentToolEventRead) {
		t.Fatalf("capabilities outside the General Agent ceiling should be rejected, got %v", err)
	}
	if got, err := config.UserSettingsRevision(novaDir); err != nil || got != userRevision {
		t.Fatalf("rejected self-escalation changed user settings revision: got=%q want=%q err=%v", got, userRevision, err)
	}

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "agent_profile", "scope": "user", "id": config.AgentKindGeneral,
		"revision": userRevision, "value": map[string]any{"kind": "agent", "tools": map[string]any{"future_sensitive_capability": true}},
	})); err == nil || !strings.Contains(err.Error(), "future_sensitive_capability") {
		t.Fatalf("configuration should reject unknown enabled capabilities, got %v", err)
	}

	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "agent_profile", "scope": "user", "id": config.AgentKindGeneral,
		"revision": userRevision, "value": map[string]any{"kind": "agent", "tools": map[string]any{
			config.AgentToolFilesystemRead: true, config.AgentToolAsk: false, config.AgentToolSkills: true,
			config.AgentToolConfigRead: true, config.AgentToolConfigApply: true,
		}},
	})); err != nil {
		t.Fatalf("safe General Agent tool override failed: %v", err)
	}

}

func TestConfigSkillResourceMutatesOneReferencePerRevision(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	readTool := configManagerToolByName(t, cfg, "config_read")
	rootContent := "---\nname: research\ndescription: Research sources.\n---\n\n# Research\n"
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "skill", "scope": "user",
		"value": map[string]any{"name": "research", "content": rootContent},
	})); err != nil {
		t.Fatal(err)
	}
	createdOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "skill", "scope": "user", "id": "research/references/sources.md",
		"value": map[string]any{"content": "# Sources\n"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	created := decodeConfigMutationReceipt(t, createdOutput)
	if created.ID != "research/references/sources.md" || created.Revision == "" {
		t.Fatalf("reference create receipt = %#v", created)
	}
	readOutput, err := runToolForTest(context.Background(), readTool, mustJSON(t, map[string]any{
		"operation": "get", "resource": "skill", "scope": "user", "ids": []string{created.ID},
	}))
	if err != nil || !strings.Contains(readOutput, "# Sources") || !strings.Contains(readOutput, created.Revision) {
		t.Fatalf("reference get output=%s error=%v", readOutput, err)
	}
	updatedOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "update", "resource": "skill", "scope": "user", "id": created.ID,
		"revision": created.Revision, "value": map[string]any{"content": "# Verified sources\n"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	updated := decodeConfigMutationReceipt(t, updatedOutput)
	if updated.Revision == "" || updated.Revision == created.Revision {
		t.Fatalf("reference update receipt = %#v", updated)
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "delete", "resource": "skill", "scope": "user", "id": created.ID, "revision": updated.Revision,
	})); err != nil {
		t.Fatal(err)
	}
}

func TestConfigResourceAdaptersShareCRUDAndRevisionContract(t *testing.T) {
	tests := []struct {
		resource string
		id       string
		create   map[string]any
		update   map[string]any
	}{
		{
			resource: "narrative_style", id: "contract-narrative",
			create: map[string]any{"id": "contract-narrative", "name": "Contract narrative", "slots": []any{map[string]any{"id": "identity", "name": "Identity", "target": "system", "enabled": true, "content": "Stay coherent."}}},
			update: map[string]any{"id": "contract-narrative", "name": "Updated narrative", "slots": []any{map[string]any{"id": "identity", "name": "Identity", "target": "system", "enabled": true, "content": "Stay concise."}}},
		},
		{
			resource: "story_director", id: "contract-director",
			create: map[string]any{"id": "contract-director", "name": "Contract director"},
			update: map[string]any{"id": "contract-director", "name": "Updated director"},
		},
		{
			resource: "event_package", id: "contract-events",
			create: map[string]any{"id": "contract-events", "name": "Contract events", "events": []any{}},
			update: map[string]any{"id": "contract-events", "name": "Updated events", "events": []any{}},
		},
		{
			resource: "rule_system", id: "contract-rules",
			create: map[string]any{"id": "contract-rules", "name": "Contract rules", "trpg_system": map[string]any{"rule_templates": []any{}}},
			update: map[string]any{"id": "contract-rules", "name": "Updated rules", "trpg_system": map[string]any{"rule_templates": []any{}}},
		},
		{
			resource: "state_system", id: "contract-state",
			create: map[string]any{"id": "contract-state", "name": "Contract state", "actor_state": map[string]any{"templates": []any{map[string]any{"id": "protagonist", "name": "Protagonist"}}}},
			update: map[string]any{"id": "contract-state", "name": "Updated state", "actor_state": map[string]any{"templates": []any{map[string]any{"id": "protagonist", "name": "Protagonist"}}}},
		},
		{
			resource: "image_preset", id: "contract-image",
			create: map[string]any{"id": "contract-image", "name": "Contract image", "prompt": "soft light"},
			update: map[string]any{"id": "contract-image", "name": "Updated image", "prompt": "hard light"},
		},
	}
	for _, test := range tests {
		t.Run(test.resource, func(t *testing.T) {
			cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
			applyTool := configManagerToolByName(t, cfg, "config_apply")
			readTool := configManagerToolByName(t, cfg, "config_read")

			createdOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
				"operation": "create", "resource": test.resource, "value": test.create,
			}))
			if err != nil {
				t.Fatal(err)
			}
			created := decodeConfigMutationReceipt(t, createdOutput)
			if created.ID != test.id || created.Revision == "" {
				t.Fatalf("create receipt = %#v", created)
			}
			if output, readErr := runToolForTest(context.Background(), readTool, mustJSON(t, map[string]any{
				"operation": "get", "resource": test.resource, "ids": []string{created.ID},
			})); readErr != nil || !strings.Contains(output, created.ID) {
				t.Fatalf("get output=%s error=%v", output, readErr)
			}

			updateInput := map[string]any{
				"operation": "update", "resource": test.resource, "id": created.ID,
				"revision": created.Revision, "value": test.update,
			}
			updatedOutput, err := runToolForTest(context.Background(), applyTool, mustJSON(t, updateInput))
			if err != nil {
				t.Fatal(err)
			}
			updated := decodeConfigMutationReceipt(t, updatedOutput)
			if updated.ID != created.ID || updated.Revision == "" || updated.Revision == created.Revision {
				t.Fatalf("update receipt = %#v, created = %#v", updated, created)
			}
			if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, updateInput)); err == nil {
				t.Fatal("stale update unexpectedly succeeded")
			}
			if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
				"operation": "delete", "resource": test.resource, "id": updated.ID, "revision": updated.Revision,
			})); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConfigApplyPreservesLargeGameConfigurationCollections(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	readTool := configManagerToolByName(t, cfg, "config_read")

	events := make([]any, 30)
	for index := range events {
		events[index] = map[string]any{
			"id": fmt.Sprintf("event-%02d", index), "type_name": fmt.Sprintf("Event %02d", index),
			"description_markdown": fmt.Sprintf("Event body %02d", index), "enabled": true,
		}
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "event_package",
		"value": map[string]any{"id": "large-events", "name": "Large events", "events": events},
	})); err != nil {
		t.Fatal(err)
	}
	eventOutput, err := runToolForTest(context.Background(), readTool, `{"operation":"get","resource":"event_package","ids":["large-events"]}`)
	if err != nil {
		t.Fatal(err)
	}
	eventPackage := decodeConfigGetItem[interactive.EventPackageModule](t, eventOutput)
	if len(eventPackage.Events) != len(events) {
		t.Fatalf("persisted event cards = %d, want %d", len(eventPackage.Events), len(events))
	}

	templates := make([]any, 30)
	actors := make([]any, 30)
	for index := range templates {
		templateID := fmt.Sprintf("template-%02d", index)
		actorID := fmt.Sprintf("Actor %02d", index)
		templates[index] = map[string]any{
			"id": templateID, "name": templateID,
			"fields": []any{map[string]any{"name": "status", "type": "string"}},
		}
		actors[index] = map[string]any{"id": actorID, "template_id": templateID, "name": actorID}
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "state_system",
		"value": map[string]any{
			"id": "large-state", "name": "Large state",
			"actor_state": map[string]any{"templates": templates, "initial_actors": actors},
		},
	})); err != nil {
		t.Fatal(err)
	}
	stateOutput, err := runToolForTest(context.Background(), readTool, `{"operation":"get","resource":"state_system","ids":["large-state"]}`)
	if err != nil {
		t.Fatal(err)
	}
	stateSystem := decodeConfigGetItem[interactive.ActorStateModule](t, stateOutput)
	if len(stateSystem.ActorState.Templates) != len(templates) || len(stateSystem.ActorState.InitialActors) != len(actors) {
		t.Fatalf("persisted state collections templates=%d actors=%d", len(stateSystem.ActorState.Templates), len(stateSystem.ActorState.InitialActors))
	}

	rules := make([]any, 30)
	for index := range rules {
		rules[index] = map[string]any{
			"id": fmt.Sprintf("rule-%02d", index), "label": fmt.Sprintf("Rule %02d", index),
			"dice": "1d20", "failure_policy": "fail_forward",
			"must_check_examples": []string{fmt.Sprintf("risk-%02d", index)},
		}
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "rule_system",
		"value": map[string]any{
			"id": "large-rules", "name": "Large rules",
			"trpg_system": map[string]any{"rule_templates": rules},
		},
	})); err != nil {
		t.Fatal(err)
	}
	ruleOutput, err := runToolForTest(context.Background(), readTool, `{"operation":"get","resource":"rule_system","ids":["large-rules"]}`)
	if err != nil {
		t.Fatal(err)
	}
	ruleSystem := decodeConfigGetItem[interactive.RuleSystemModule](t, ruleOutput)
	if len(ruleSystem.TRPGSystem.RuleTemplates) != len(rules) {
		t.Fatalf("persisted rule templates = %d, want %d", len(ruleSystem.TRPGSystem.RuleTemplates), len(rules))
	}

	styleRefs := make([]string, 30)
	for index := range styleRefs {
		styleRefs[index] = fmt.Sprintf(".denova/styles/style-%02d.md", index)
	}
	if _, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "narrative_style",
		"value": map[string]any{
			"id": "large-style-refs", "name": "Large style refs", "style_refs": styleRefs,
			"style_rules": []any{map[string]any{"scene": "all", "style_refs": styleRefs}},
			"slots":       []any{map[string]any{"id": "identity", "name": "Identity", "target": "system", "enabled": true, "content": "Keep every reference."}},
		},
	})); err != nil {
		t.Fatal(err)
	}
	styleOutput, err := runToolForTest(context.Background(), readTool, `{"operation":"get","resource":"narrative_style","ids":["large-style-refs"]}`)
	if err != nil {
		t.Fatal(err)
	}
	style := decodeConfigGetItem[teller.Definition](t, styleOutput)
	if len(style.StyleRefs) != len(styleRefs) || len(style.StyleRules) != 1 || len(style.StyleRules[0].StyleRefs) != len(styleRefs) {
		t.Fatalf("persisted style references global=%d rule=%d", len(style.StyleRefs), len(style.StyleRules[0].StyleRefs))
	}
}

func TestConfigApplyRejectsOversizedEventCardWithoutSavingTruncatedData(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	applyTool := configManagerToolByName(t, cfg, "config_apply")
	readTool := configManagerToolByName(t, cfg, "config_read")
	_, err := runToolForTest(context.Background(), applyTool, mustJSON(t, map[string]any{
		"operation": "create", "resource": "event_package",
		"value": map[string]any{
			"id": "oversized-event", "name": "Oversized event",
			"events": []any{map[string]any{
				"id": "too-long", "type_name": "Too long", "enabled": true,
				"description_markdown": strings.Repeat("文", interactive.MaxEventCardDescriptionChars+1),
			}},
		},
	}))
	if err == nil || !strings.Contains(err.Error(), "events[0].description_markdown") {
		t.Fatalf("oversized event error = %v", err)
	}
	if _, err := runToolForTest(context.Background(), readTool, `{"operation":"get","resource":"event_package","ids":["oversized-event"]}`); err == nil {
		t.Fatal("rejected event package was persisted")
	}
}

func configManagerToolByName(t *testing.T, cfg *config.Config, name string) agent.Tool {
	t.Helper()
	return configManagerDefinitionByName(t, cfg, name).Tool
}

func decodeAgentProfileGetResult(t *testing.T, output string) agentProfileReadResult {
	t.Helper()
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Items) != 1 {
		t.Fatalf("agent_profile get items = %d, want 1: %s", len(envelope.Items), output)
	}
	var result agentProfileReadResult
	if err := json.Unmarshal(envelope.Items[0], &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func decodeAgentProfileListResult(t *testing.T, output string) agentProfileReadResult {
	t.Helper()
	return decodeAgentProfileGetResult(t, output)
}

func decodeConfigGetItem[T any](t *testing.T, output string) T {
	t.Helper()
	var zero T
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Items) != 1 {
		t.Fatalf("config get items = %d, want 1: %s", len(envelope.Items), output)
	}
	if err := json.Unmarshal(envelope.Items[0], &zero); err != nil {
		t.Fatal(err)
	}
	return zero
}

func configManagerDefinitionByName(t *testing.T, cfg *config.Config, name string) agent.ToolDefinition {
	t.Helper()
	definitions, err := NewTools(cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		info, infoErr := definition.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if info.Name == name {
			return definition
		}
	}
	t.Fatalf("tool %s not found", name)
	return agent.ToolDefinition{}
}

func configManagerToolNameSet(t *testing.T, tools []agent.ToolDefinition) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, tool := range tools {
		info, err := tool.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	return names
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func decodeConfigMutationReceipt(t *testing.T, output string) configMutationReceipt {
	t.Helper()
	var receipt configMutationReceipt
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}
