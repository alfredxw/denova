package agents

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

type approvalHostStub struct {
	granted  bool
	requests []toolApprovalRequest
}

func (host *approvalHostStub) ApproveTool(_ context.Context, request toolApprovalRequest) (bool, error) {
	host.requests = append(host.requests, request)
	return host.granted, nil
}

func TestToolApprovalPolicyAllowsSafeAskReadWithoutPrompt(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	middleware := approvalTestMiddleware(config.AgentApprovalAsk, workspace)
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware, func(context.Context, string, ...agent.ToolOption) (string, error) {
		called = true
		return "ok", nil
	}, testToolContext("bash", "safe-read"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"command":"git status --short"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !called || result != "ok" {
		t.Fatalf("called=%t result=%q", called, result)
	}
}

func TestToolApprovalPolicyPromptsBeforeUnknownAskCommand(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	host := &approvalHostStub{granted: true}
	ctx := contextWithToolApprovalHost(context.Background(), host)
	middleware := approvalTestMiddleware(config.AgentApprovalAsk, workspace)
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware, func(context.Context, string, ...agent.ToolOption) (string, error) {
		called = true
		return "ok", nil
	}, testToolContext("bash", "unknown-command"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint(ctx, `{"command":"custom-project-command --fix"}`); err != nil {
		t.Fatal(err)
	}
	if !called || len(host.requests) != 1 {
		t.Fatalf("called=%t approval requests=%d", called, len(host.requests))
	}
	if host.requests[0].Decision.Command != "custom-project-command --fix" {
		t.Fatalf("approval command = %q", host.requests[0].Decision.Command)
	}
}

func TestToolApprovalPolicyDenialReturnsBlockedResult(t *testing.T) {
	t.Parallel()
	host := &approvalHostStub{granted: false}
	ctx := contextWithToolApprovalHost(context.Background(), host)
	middleware := approvalTestMiddleware(config.AgentApprovalWrite, t.TempDir())
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware, func(context.Context, string, ...agent.ToolOption) (string, error) {
		called = true
		return "unexpected", nil
	}, testToolContext("bash", "remote-mutation"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"command":"git push origin main"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called || len(host.requests) != 1 || !strings.Contains(result, "用户拒绝") {
		t.Fatalf("called=%t requests=%d result=%q", called, len(host.requests), result)
	}
}

func TestToolApprovalPolicyFailsClosedWithoutInteractiveHost(t *testing.T) {
	t.Parallel()
	middleware := approvalTestMiddleware(config.AgentApprovalAsk, t.TempDir())
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware, func(context.Context, string, ...agent.ToolOption) (string, error) {
		called = true
		return "unexpected", nil
	}, testToolContext("bash", "headless"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"command":"npm test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called || !strings.Contains(result, "没有可恢复的交互主机") {
		t.Fatalf("called=%t result=%q", called, result)
	}
}

func TestToolApprovalPolicyDeniesCriticalCommandInFullAccess(t *testing.T) {
	t.Parallel()
	middleware := approvalTestMiddleware(config.AgentApprovalFullAccess, t.TempDir())
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware, func(context.Context, string, ...agent.ToolOption) (string, error) {
		called = true
		return "unexpected", nil
	}, testToolContext("bash", "critical"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"command":"rm -rf /"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called || !strings.Contains(result, "Recursive forced deletion") {
		t.Fatalf("called=%t result=%q", called, result)
	}
}

func TestToolApprovalModeIsSnapshottedWhenRunAssemblyIsBuilt(t *testing.T) {
	cfg := &config.Config{
		Workspace:            t.TempDir(),
		AgentApprovalMode:    config.AgentApprovalWrite,
		ShellEnvironmentMode: config.ShellEnvironmentProcess,
	}
	assembly, err := buildChatModelAgentAssembly(context.Background(), cfg, chatModelAgentAssemblySpec{
		Kind: AgentKindIDE,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.AgentApprovalMode = config.AgentApprovalFullAccess
	for _, middleware := range assembly.Middlewares {
		orchestrator, ok := middleware.(*toolOrchestratorMiddleware)
		if !ok {
			continue
		}
		if orchestrator.approvalMode != config.AgentApprovalWrite {
			t.Fatalf("approval mode changed after assembly: %q", orchestrator.approvalMode)
		}
		return
	}
	t.Fatal("tool approval middleware was not installed")
}

func TestToolApprovalDetailsExposeBoundedStructuredArguments(t *testing.T) {
	t.Parallel()
	request := toolApprovalRequest{Arguments: `{"action":"run","command":"click","selector":"button.save"}`}
	if got := toolApprovalDetails(request); got != request.Arguments {
		t.Fatalf("details = %q, want %q", got, request.Arguments)
	}
	request.Decision.Command = "git push"
	if got := toolApprovalDetails(request); got != "" {
		t.Fatalf("shell command details = %q, want empty", got)
	}
	request.Decision.Command = ""
	request.Arguments = strings.Repeat("界", toolApprovalDetailsMax)
	got := toolApprovalDetails(request)
	if len(got) > toolApprovalDetailsMax || !strings.Contains(got, "详情已截断") {
		t.Fatalf("bounded details bytes=%d marker=%t", len(got), strings.Contains(got, "详情已截断"))
	}
}

func approvalTestMiddleware(mode config.AgentApprovalMode, workspace string) *toolOrchestratorMiddleware {
	return &toolOrchestratorMiddleware{
		agentKind: AgentKindIDE, enforceApprovalPolicy: true,
		approvalMode: mode, workspace: workspace,
	}
}
