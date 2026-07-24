package agents

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
)

func TestToolDescriptorDeclaresExecutionAndRecoveryPolicy(t *testing.T) {
	tool, err := newWriteTodosTool()
	if err != nil {
		t.Fatal(err)
	}
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := agenttools.DescriptorFromInfo(info)
	if !ok || descriptor.Capability != config.AgentToolTodo || descriptor.Execution != agenttools.ExecutionWorkspaceExclusive || descriptor.Recovery != agenttools.RecoveryIdempotent {
		t.Fatalf("todo descriptor = %+v, %t", descriptor, ok)
	}
	if descriptor.MutatesWorkspace || descriptor.RequiresPostCheck {
		t.Fatalf("write_todos must remain session-local: %+v", descriptor)
	}
}

func TestUnknownToolDescriptorIsConservativeWithoutNameInference(t *testing.T) {
	for _, name := range []string{"write_custom_plugin_state", "search_private_index", "read_side_effecting_api"} {
		descriptor := unknownToolManifest(name)
		if descriptor.Source != ToolSourceOther {
			t.Fatalf("unknown %q source = %q, want other", name, descriptor.Source)
		}
		if descriptor.Capability != "" {
			t.Fatalf("unknown %q capability = %q, want empty", name, descriptor.Capability)
		}
		if descriptor.Execution != ToolExecutionWorkspaceExclusive {
			t.Fatalf("unknown %q execution = %q, want exclusive", name, descriptor.Execution)
		}
		if descriptor.Recovery != ToolRecoveryNonIdempotent {
			t.Fatalf("unknown %q recovery = %q, want non-idempotent", name, descriptor.Recovery)
		}
	}
}

func TestToolResultMetadataIncludesRecoveryContract(t *testing.T) {
	descriptor := workspaceWriteDescriptor(agenttools.SourceWrite, config.AgentToolFileWrite, agenttools.RecoveryReconcilable)
	result := filterToolResultForModelWithDescriptor("write_file", descriptor, `{"path":"chapters/ch01.md"}`, "ok", 0)
	for _, want := range []string{
		"execution: workspace_exclusive",
		"recovery: reconcilable",
		"result_projection: bounded_model_context",
	} {
		if !containsLine(result.Content, want) {
			t.Fatalf("filtered result missing %q:\n%s", want, result.Content)
		}
	}
}

func TestValidateToolDescriptorsRejectsUndeclaredTools(t *testing.T) {
	err := validateToolDescriptors(context.Background(), []agent.BaseTool{descriptorTestTool{name: "write_custom_plugin_state"}})
	if err == nil || !strings.Contains(err.Error(), "write_custom_plugin_state") {
		t.Fatalf("expected undeclared descriptor error, got %v", err)
	}
	declared, bindErr := defineTool(descriptorTestTool{name: "write_file"}, workspaceWriteDescriptor(agenttools.SourceWrite, config.AgentToolFileWrite, agenttools.RecoveryReconcilable))
	if bindErr != nil {
		t.Fatal(bindErr)
	}
	if err := validateToolDescriptors(context.Background(), []agent.BaseTool{declared}); err != nil {
		t.Fatalf("declared tool rejected: %v", err)
	}
}

func TestValidateToolSurfaceRejectsDuplicateNames(t *testing.T) {
	first, _ := defineTool(descriptorTestTool{name: "read_file"}, boundedReadDescriptor(agenttools.SourceRead, config.AgentToolFileRead))
	second, _ := defineTool(descriptorTestTool{name: "read_file"}, boundedReadDescriptor(agenttools.SourceRead, config.AgentToolFileRead))
	if err := validateToolDescriptors(context.Background(), []agent.BaseTool{
		first,
		second,
	}); err == nil || !strings.Contains(err.Error(), "duplicate model-visible tool") {
		t.Fatalf("duplicate static tool error = %v", err)
	}
}

func TestToolDescriptorGuardValidatesDynamicallyInjectedTools(t *testing.T) {
	guard := newToolDescriptorGuardMiddleware()
	read, _ := defineTool(descriptorTestTool{name: "read_file"}, boundedReadDescriptor(agenttools.SourceRead, config.AgentToolFileRead))
	skill, _ := defineTool(descriptorTestTool{name: "skill"}, boundedReadDescriptor(agenttools.SourceOther, config.AgentToolSkills))
	runCtx := &agent.RunContext{Tools: []agent.BaseTool{read, skill}}
	_, guarded, err := guard.BeforeAgent(context.Background(), runCtx)
	if err != nil || guarded != runCtx {
		t.Fatalf("declared dynamic tools rejected: context=%p want=%p err=%v", guarded, runCtx, err)
	}

	runCtx.Tools = append(runCtx.Tools, descriptorTestTool{name: "dynamic_unknown"})
	if _, _, err := guard.BeforeAgent(context.Background(), runCtx); err == nil || !strings.Contains(err.Error(), "dynamic_unknown") {
		t.Fatalf("dynamic undeclared tool error = %v", err)
	}
}

type descriptorTestTool struct{ name string }

func (t descriptorTestTool) Info(context.Context) (*agent.ToolInfo, error) {
	return &agent.ToolInfo{Name: t.name}, nil
}

func testToolContext(name, callID string) *agent.ToolContext {
	var descriptor agenttools.Descriptor
	switch name {
	case "read_file", "grep", "search_story_history":
		descriptor = boundedReadDescriptor(agenttools.SourceRead, config.AgentToolFileRead)
		if name == "search_story_history" {
			descriptor = boundedReadDescriptor(ToolSourceHistory, "")
		}
	case "write_file", "edit_file":
		descriptor = workspaceWriteDescriptor(agenttools.SourceWrite, config.AgentToolFileWrite, agenttools.RecoveryReconcilable)
	case "execute", "execute_shell":
		descriptor = workspaceWriteDescriptor(agenttools.SourceShell, config.AgentToolShellExecute, agenttools.RecoveryNonIdempotent)
	default:
		return &agent.ToolContext{Name: name, CallID: callID}
	}
	definition := agenttools.Definition{Tool: descriptorTestTool{name: name}, Descriptor: descriptor}
	info, _ := definition.ToolInfo(context.Background())
	return &agent.ToolContext{Name: name, CallID: callID, Info: info}
}

func containsLine(content, line string) bool {
	for start := 0; start <= len(content); {
		end := start
		for end < len(content) && content[end] != '\n' {
			end++
		}
		if content[start:end] == line {
			return true
		}
		if end == len(content) {
			break
		}
		start = end + 1
	}
	return false
}
