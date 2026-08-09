package toolruntime

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
)

func TestPlanReadOnlyAccessFiltersModelToolSurface(t *testing.T) {
	definitions := []agent.ToolDefinition{
		accessPolicyDefinition(t, "read", producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolWorkspaceRead)),
		accessPolicyDefinition(t, "task", accessPolicyDescriptor(agent.ToolExecutionChild, agent.ToolMutationNone, config.AgentToolDelegation)),
		accessPolicyDefinition(t, "ask", accessPolicyDescriptor(agent.ToolExecutionInteractiveWait, agent.ToolMutationSession, config.AgentToolAsk)),
		accessPolicyDefinition(t, "todo", accessPolicyDescriptor(agent.ToolExecutionSessionExclusive, agent.ToolMutationSession, config.AgentToolTodo)),
		accessPolicyDefinition(t, "submit_domain_state", accessPolicyDescriptor(agent.ToolExecutionSessionExclusive, agent.ToolMutationSession, "domain_commit")),
		accessPolicyDefinition(t, "write", producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)),
		accessPolicyDefinition(t, "apply_config", accessPolicyDescriptor(agent.ToolExecutionConfigExclusive, agent.ToolMutationConfig, config.AgentToolConfigApply)),
		accessPolicyDefinition(t, "browser", accessPolicyDescriptor(agent.ToolExecutionSessionExclusive, agent.ToolMutationExternal, config.AgentToolBrowser)),
	}
	original := &agent.RunContext{Instruction: "keep", Tools: definitions}
	middleware := NewOrchestratorMiddleware(OrchestratorConfig{})

	_, filtered, err := middleware.BeforeAgent(
		ContextWithToolAccessMode(context.Background(), ToolAccessModePlanReadOnly),
		original,
	)
	if err != nil {
		t.Fatal(err)
	}
	if filtered == original {
		t.Fatal("access filtering must return a run snapshot instead of mutating the caller")
	}
	if len(original.Tools) != len(definitions) {
		t.Fatalf("original tool surface was mutated: got %d tools, want %d", len(original.Tools), len(definitions))
	}
	if got, want := accessPolicyToolNames(t, filtered.Tools), []string{"read", "task", "ask", "todo"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("filtered tools = %v, want %v", got, want)
	}
}

func TestPlanReadOnlyAccessBlocksForgedMutationAtExecution(t *testing.T) {
	middleware := NewOrchestratorMiddleware(OrchestratorConfig{})
	planCtx := ContextWithToolAccessMode(context.Background(), ToolAccessModePlanReadOnly)

	for _, test := range []struct {
		name       string
		descriptor agent.ToolDescriptor
		allowed    bool
	}{
		{name: "read", descriptor: producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolWorkspaceRead), allowed: true},
		{name: "ask", descriptor: accessPolicyDescriptor(agent.ToolExecutionInteractiveWait, agent.ToolMutationSession, config.AgentToolAsk), allowed: true},
		{name: "submit_domain_state", descriptor: accessPolicyDescriptor(agent.ToolExecutionSessionExclusive, agent.ToolMutationSession, "domain_commit")},
		{name: "write", descriptor: producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := accessPolicyDefinition(t, test.name, test.descriptor)
			info, err := definition.Tool.Info(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			called := false
			wrapped, err := middleware.WrapToolCall(context.Background(), func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
				called = true
				return agent.TextToolResult("executed"), nil
			}, &agent.ToolContext{
				Name: test.name,
				Definition: agent.ToolDefinitionSnapshot{
					Info: info, Descriptor: definition.Descriptor,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := wrapped(planCtx, `{}`)
			if err != nil {
				t.Fatal(err)
			}
			if test.allowed {
				if !called || result.ModelContent != "executed" {
					t.Fatalf("allowed tool did not execute: called=%t result=%#v", called, result)
				}
				return
			}
			if called {
				t.Fatal("blocked tool reached its endpoint")
			}
			if !strings.Contains(result.ModelContent, "Plan Mode is read-only") || !strings.Contains(result.ModelContent, "规划模式为只读") {
				t.Fatalf("blocked result must explain the policy bilingually: %q", result.ModelContent)
			}
		})
	}
}

type accessPolicyTool struct{ name string }

func (tool accessPolicyTool) Info(context.Context) (*agent.ToolInfo, error) {
	return &agent.ToolInfo{Name: tool.name}, nil
}

func (accessPolicyTool) Run(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
	return agent.TextToolResult("executed"), nil
}

func accessPolicyDefinition(t *testing.T, name string, descriptor agent.ToolDescriptor) agent.ToolDefinition {
	t.Helper()
	definition, err := producttools.Define(accessPolicyTool{name: name}, descriptor)
	if err != nil {
		t.Fatalf("define %s: %v", name, err)
	}
	return definition
}

func accessPolicyDescriptor(execution agent.ToolExecutionClass, mutation agent.ToolMutationScope, capability string) agent.ToolDescriptor {
	descriptor := agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: capability,
		Execution: execution, MutationScope: mutation,
		ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention:  agent.ToolResultProtected,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   1024,
	}
	switch mutation {
	case agent.ToolMutationNone:
		descriptor.PostCheck = agent.ToolPostCheckNone
		descriptor.Recovery = agent.ToolRecoveryReadOnly
	case agent.ToolMutationSession:
		descriptor.PostCheck = agent.ToolPostCheckSessionState
		descriptor.Recovery = agent.ToolRecoveryReconcilable
	case agent.ToolMutationConfig:
		descriptor.PostCheck = agent.ToolPostCheckConfigRevision
		descriptor.Recovery = agent.ToolRecoveryReconcilable
	case agent.ToolMutationExternal:
		descriptor.PostCheck = agent.ToolPostCheckExternalReceipt
		descriptor.Recovery = agent.ToolRecoveryNonIdempotent
	}
	return descriptor
}

func accessPolicyToolNames(t *testing.T, definitions []agent.ToolDefinition) []string {
	t.Helper()
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, info.Name)
	}
	return names
}
