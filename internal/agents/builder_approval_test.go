package agents

import (
	"context"
	"encoding/json"
	"testing"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

func TestPermissionModeIsSnapshottedWhenDefinitionIsBuilt(t *testing.T) {
	cfg := &config.Config{
		Workspace:            t.TempDir(),
		OpenAIBaseURL:        "https://example.invalid",
		OpenAIModel:          "test-model",
		AgentApprovalMode:    config.AgentApprovalWrite,
		ShellEnvironmentMode: config.ShellEnvironmentProcess,
	}
	definition, err := buildAgentDefinition(context.Background(), cfg, agentBuildSpec{
		Kind: config.AgentKindIDE, Name: "DenovaAgent", Description: "test",
		Composition: mustTestPromptComposition(t, config.AgentKindIDE, "test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if definition.Permission == nil {
		t.Fatal("Definition.Permission must be the only tool approval authority")
	}
	if definition.ResultProcessor == nil {
		t.Fatal("Definition.ResultProcessor must be the only post-tool result processor")
	}
	if identity := definition.ResultProcessor.Identity(); identity.Kind != "tool_result_processor.standard" {
		t.Fatalf("unexpected ResultProcessor identity: %#v", identity)
	}
	initialIdentity := definition.Permission.Identity()
	cfg.AgentApprovalMode = config.AgentApprovalFullAccess
	decision, err := definition.Permission.Evaluate(context.Background(), agent.PermissionRequest{
		Tool: "write", CallID: "write-1", Arguments: json.RawMessage(`{"path":"chapter.md"}`),
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceWrite, MutationScope: agent.ToolMutationWorkspace,
			Recovery: agent.ToolRecoveryIdempotent,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Details.Mode != string(config.AgentApprovalWrite) {
		t.Fatalf("permission mode changed after Definition assembly: %q", decision.Details.Mode)
	}
	if got := definition.Permission.Identity(); got != initialIdentity {
		t.Fatalf("permission identity changed with caller config mutation: before=%#v after=%#v", initialIdentity, got)
	}
}
