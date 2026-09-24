package toolruntime

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	agent "github.com/alfredxw/denova/agent"
)

type hostInvocationTool struct {
	run func(context.Context) (agent.ToolResult, error)
}

func (hostInvocationTool) Info(context.Context) (*agent.ToolInfo, error) {
	return &agent.ToolInfo{Name: "write"}, nil
}

func (tool hostInvocationTool) Run(ctx context.Context, _ string, _ ...agent.ToolOption) (agent.ToolResult, error) {
	return tool.run(ctx)
}

func TestHostToolInvocationTracksRunEntry(t *testing.T) {
	for _, scenario := range []string{"cancel_before_run", "cancel_in_run", "policy_block", "invalid_identity", "success"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			called := false
			tool := hostInvocationTool{run: func(context.Context) (agent.ToolResult, error) {
				called = true
				if scenario == "cancel_in_run" {
					cancel()
					return agent.ToolResult{}, context.Canceled
				}
				return agent.TextToolResult("done"), nil
			}}
			definition := agent.ToolDefinition{Tool: tool, Descriptor: testToolContext("write", "").Definition.Descriptor}
			policy := OrchestratorConfig{Workspace: t.TempDir(), ToolSettings: config.ResolvedAgentToolSettings{config.AgentToolWorkspaceWrite: scenario != "policy_block"}, EnforceToolSettings: true}
			identity := HostToolIdentity{OperationID: "operation", ExecutionID: "execution", SessionID: "session"}
			if scenario == "cancel_before_run" {
				cancel()
			}
			if scenario == "invalid_identity" {
				identity.ExecutionID = ""
			}
			outcome, err := InvokeHostTool(ctx, policy, identity, definition, `{"path":"chapter.md","content":"text"}`)
			wantInvocation := HostToolNotInvoked
			if scenario == "success" || scenario == "cancel_in_run" {
				wantInvocation = HostToolInvoked
			}
			if outcome.Invocation != wantInvocation || called != (wantInvocation == HostToolInvoked) {
				t.Fatalf("invocation=%v called=%t want=%v", outcome.Invocation, called, wantInvocation)
			}
			switch scenario {
			case "cancel_before_run", "cancel_in_run":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("lost cancellation error: %v", err)
				}
			case "invalid_identity":
				if err == nil {
					t.Fatal("invalid identity was accepted")
				}
			case "policy_block":
				if err != nil || outcome.Result.Status != agent.ToolResultBlocked {
					t.Fatalf("policy result=%+v err=%v", outcome.Result, err)
				}
			case "success":
				if err != nil || outcome.Result.Status != agent.ToolResultSuccess || outcome.Result.ModelContent != "done" {
					t.Fatalf("success result=%+v err=%v", outcome.Result, err)
				}
			}
		})
	}
}
