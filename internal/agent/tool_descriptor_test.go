package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"denova/config"
)

func TestToolDescriptorDeclaresExecutionAndRecoveryPolicy(t *testing.T) {
	tests := []struct {
		name       string
		source     ToolSource
		capability string
		execution  ToolExecutionClass
		recovery   ToolRecoveryClass
	}{
		{
			name:       "read_file",
			source:     ToolSourceRead,
			capability: config.AgentToolFileRead,
			execution:  ToolExecutionParallelRead,
			recovery:   ToolRecoveryReadOnly,
		},
		{
			name:       "write_file",
			source:     ToolSourceWrite,
			capability: config.AgentToolFileWrite,
			execution:  ToolExecutionWorkspaceExclusive,
			recovery:   ToolRecoveryReconcilable,
		},
		{
			name:       "write_lore_items",
			source:     ToolSourceLore,
			capability: config.AgentToolLoreWrite,
			execution:  ToolExecutionWorkspaceExclusive,
			recovery:   ToolRecoveryReconcilable,
		},
		{
			name:      "task",
			source:    ToolSourceOther,
			execution: ToolExecutionChild,
			recovery:  ToolRecoveryNonIdempotent,
		},
		{
			name:       "skill",
			source:     ToolSourceOther,
			capability: config.AgentToolSkills,
			execution:  ToolExecutionParallelRead,
			recovery:   ToolRecoveryReadOnly,
		},
		{
			name:       "write_todos",
			source:     ToolSourceOther,
			capability: config.AgentToolTodo,
			execution:  ToolExecutionWorkspaceExclusive,
			recovery:   ToolRecoveryIdempotent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := DescriptorForTool(tt.name)
			if descriptor.Source != tt.source {
				t.Fatalf("source = %q, want %q", descriptor.Source, tt.source)
			}
			if descriptor.Capability != tt.capability {
				t.Fatalf("capability = %q, want %q", descriptor.Capability, tt.capability)
			}
			if descriptor.Execution != tt.execution {
				t.Fatalf("execution = %q, want %q", descriptor.Execution, tt.execution)
			}
			if descriptor.Recovery != tt.recovery {
				t.Fatalf("recovery = %q, want %q", descriptor.Recovery, tt.recovery)
			}
		})
	}

	todoDescriptor := DescriptorForTool("write_todos")
	if todoDescriptor.MutatesWorkspace || todoDescriptor.RequiresPostCheck {
		t.Fatalf("write_todos must remain session-local: %+v", todoDescriptor)
	}
}

func TestUnknownToolDescriptorIsConservativeWithoutNameInference(t *testing.T) {
	for _, name := range []string{"write_custom_plugin_state", "search_private_index", "read_side_effecting_api"} {
		descriptor := DescriptorForTool(name)
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
	result := FilterToolResultForModel("write_file", `{"path":"chapters/ch01.md"}`, "ok")
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
	err := validateToolDescriptors(context.Background(), []tool.BaseTool{descriptorTestTool{name: "write_custom_plugin_state"}})
	if err == nil || !strings.Contains(err.Error(), "write_custom_plugin_state") {
		t.Fatalf("expected undeclared descriptor error, got %v", err)
	}
	if err := validateToolDescriptors(context.Background(), []tool.BaseTool{descriptorTestTool{name: "write_file"}}); err != nil {
		t.Fatalf("declared tool rejected: %v", err)
	}
	if err := validateToolDescriptorNames([]string{"ls", "skill"}); err != nil {
		t.Fatalf("declared middleware tools rejected: %v", err)
	}
	if err := validateToolDescriptorNames([]string{"dynamic_unknown"}); err == nil || !strings.Contains(err.Error(), "dynamic_unknown") {
		t.Fatalf("undeclared middleware tool error = %v", err)
	}
}

func TestValidateToolSurfaceRejectsDuplicateNames(t *testing.T) {
	if err := validateToolDescriptors(context.Background(), []tool.BaseTool{
		descriptorTestTool{name: "read_file"},
		descriptorTestTool{name: " READ_FILE "},
	}); err == nil || !strings.Contains(err.Error(), "duplicate model-visible tool") {
		t.Fatalf("duplicate static tool error = %v", err)
	}
	if err := validateToolDescriptorNames([]string{"skill", " SKILL "}); err == nil || !strings.Contains(err.Error(), "duplicate middleware") {
		t.Fatalf("duplicate middleware tool error = %v", err)
	}
	if err := validateToolSurface(
		context.Background(),
		[]tool.BaseTool{descriptorTestTool{name: "read_file"}},
		[]string{"READ_FILE"},
	); err == nil || !strings.Contains(err.Error(), "across static and middleware") {
		t.Fatalf("cross-registration duplicate error = %v", err)
	}
}

func TestToolDescriptorGuardValidatesDynamicallyInjectedTools(t *testing.T) {
	guard := newToolDescriptorGuardMiddleware()
	runCtx := &adk.ChatModelAgentContext{Tools: []tool.BaseTool{
		descriptorTestTool{name: "read_file"},
		descriptorTestTool{name: "skill"},
	}}
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

func (t descriptorTestTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
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
