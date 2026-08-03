package toolruntime

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

type approvalHostStub struct {
	granted  bool
	choice   ApprovalChoice
	requests []ApprovalRequest
}

func (host *approvalHostStub) ApproveTool(_ context.Context, request ApprovalRequest) (ApprovalResult, error) {
	host.requests = append(host.requests, request)
	if host.choice != "" {
		return ApprovalResult{Choice: host.choice}, nil
	}
	if host.granted {
		return ApprovalResult{Choice: ApprovalAllowOnce}, nil
	}
	return ApprovalResult{Choice: ApprovalDenied}, nil
}

func TestWorkspaceApprovalIsReusedWithinCurrentRun(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	host := &approvalHostStub{choice: ApprovalAllowWorkspace}
	ctx := ContextWithApprovalHost(context.Background(), host)
	middleware := approvalTestMiddleware(config.AgentApprovalAsk, workspace)
	called := 0
	endpoint, err := wrapTextToolCallForTest(middleware, func(context.Context, string, ...agent.ToolOption) (string, error) {
		called++
		return "ok", nil
	}, testToolContext("bash", "remembered-command"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint(ctx, `{"command":"go test ./internal/agents/..."}`); err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint(ctx, `{"command":"go test ./internal/app/..."}`); err != nil {
		t.Fatal(err)
	}
	if called != 2 || len(host.requests) != 1 {
		t.Fatalf("executions=%d approval requests=%d", called, len(host.requests))
	}
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
	ctx := ContextWithApprovalHost(context.Background(), host)
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
	ctx := ContextWithApprovalHost(context.Background(), host)
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
	if called || len(host.requests) != 1 || !strings.Contains(result, "user denied") {
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
	if called || !strings.Contains(result, "no recoverable interactive host") {
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

func approvalTestMiddleware(mode config.AgentApprovalMode, workspace string) *OrchestratorMiddleware {
	return NewOrchestratorMiddleware(OrchestratorConfig{
		AgentKind: config.AgentKindIDE, EnforceApprovalPolicy: true,
		ApprovalMode: mode, Workspace: workspace,
	})
}
