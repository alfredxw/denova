package agents

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
)

func TestToolDescriptorDeclaresExecutionAndRecoveryPolicy(t *testing.T) {
	definition, err := newToolCatalog(nil).WriteTodos()
	if err != nil {
		t.Fatal(err)
	}
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	descriptor := definition.Descriptor
	if info.Name != "write_todos" || descriptor.Capability != config.AgentToolTodo ||
		descriptor.Execution != agent.ToolExecutionWorkspaceExclusive ||
		descriptor.Recovery != agent.ToolRecoveryIdempotent {
		t.Fatalf("todo definition = info=%+v descriptor=%+v", info, descriptor)
	}
	if descriptor.MutatesWorkspace || descriptor.RequiresPostCheck {
		t.Fatalf("write_todos must remain session-local: %+v", descriptor)
	}
}

func TestUnknownToolManifestIsConservativeWithoutNameInference(t *testing.T) {
	for _, name := range []string{"write_custom_plugin_state", "search_private_index", "read_side_effecting_api"} {
		descriptor := unknownToolManifest(name)
		if descriptor.Source != ToolSourceOther || descriptor.Capability != "" ||
			descriptor.Execution != ToolExecutionWorkspaceExclusive ||
			descriptor.Recovery != ToolRecoveryNonIdempotent {
			t.Fatalf("unknown %q manifest = %+v", name, descriptor)
		}
	}
}

func TestStructuredToolResultKeepsRecoveryContractOutOfModelText(t *testing.T) {
	descriptor := producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolFileWrite, agent.ToolRecoveryReconcilable)
	filtered := filterToolResultForModelWithDescriptor("write_file", descriptor, `{"path":"chapters/ch01.md"}`, "ok", 0)
	if filtered.Content != "ok" || filtered.Result.ModelContent != "ok" {
		t.Fatalf("model content was polluted: %#v", filtered)
	}
	if strings.Contains(filtered.Content, "Denova tool result metadata") || strings.Contains(filtered.Content, "recovery:") {
		t.Fatalf("descriptor leaked into model text: %q", filtered.Content)
	}
	if filtered.Manifest.Execution != agent.ToolExecutionWorkspaceExclusive ||
		filtered.Manifest.Recovery != agent.ToolRecoveryReconcilable ||
		filtered.Manifest.ResultProjection != agent.ToolResultBoundedModelContext {
		t.Fatalf("durable manifest lost recovery contract: %+v", filtered.Manifest)
	}
	if filtered.Result.Metadata.Target != "chapters/ch01.md" || filtered.Result.Metadata.IdempotencyKey == "" {
		t.Fatalf("structured metadata missing target or idempotency key: %+v", filtered.Result.Metadata)
	}
}

func TestRegistryRejectsUnclassifiedAndDuplicateTools(t *testing.T) {
	undeclared := agent.ToolDefinition{Tool: descriptorTestTool{name: "write_custom_plugin_state"}}
	if _, err := agent.NewRegistry(context.Background(), undeclared); err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("expected unclassified definition error, got %v", err)
	}

	first, err := producttools.Define(descriptorTestTool{name: "read_file"}, producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolFileRead))
	if err != nil {
		t.Fatal(err)
	}
	second, err := producttools.Define(descriptorTestTool{name: "read_file"}, producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolFileRead))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.NewRegistry(context.Background(), first, second); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate tool error = %v", err)
	}
}

func TestRegistrySnapshotCarriesDescriptorWithoutToolInfoExtra(t *testing.T) {
	definition, err := producttools.Define(descriptorTestTool{name: "read_file"}, producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolFileRead))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := agent.NewRegistry(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := registry.Snapshot("read_file")
	if !ok || snapshot.Info == nil || snapshot.Descriptor.Execution != agent.ToolExecutionParallelRead {
		t.Fatalf("snapshot = %#v ok=%t", snapshot, ok)
	}
	if snapshot.Info.Extra != nil {
		t.Fatalf("provider ToolInfo must not carry descriptor metadata: %#v", snapshot.Info.Extra)
	}
}

type descriptorTestTool struct{ name string }

func (tool descriptorTestTool) Info(context.Context) (*agent.ToolInfo, error) {
	return &agent.ToolInfo{Name: tool.name}, nil
}

func (descriptorTestTool) Run(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
	return agent.TextToolResult("ok"), nil
}

func testToolContext(name, callID string) *agent.ToolContext {
	var descriptor agent.ToolDescriptor
	switch name {
	case "read_file", "grep", "search_story_history":
		descriptor = producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolFileRead)
		if name == "search_story_history" {
			descriptor = producttools.BoundedReadDescriptor(ToolSourceHistory, "")
		}
	case "write_file", "edit_file":
		descriptor = producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolFileWrite, agent.ToolRecoveryReconcilable)
	case "execute", "execute_shell":
		descriptor = producttools.WorkspaceWriteDescriptor(agent.ToolSourceShell, config.AgentToolShellExecute, agent.ToolRecoveryNonIdempotent)
	default:
		return &agent.ToolContext{Name: name, CallID: callID}
	}
	return &agent.ToolContext{
		Name: name, CallID: callID,
		Definition: agent.ToolDefinitionSnapshot{
			Info:       &agent.ToolInfo{Name: name},
			Descriptor: descriptor,
		},
	}
}
