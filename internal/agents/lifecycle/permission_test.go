package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

func TestDenovaPermissionPolicyPersistsRememberedRuleBeforeAllowing(t *testing.T) {
	var persisted config.AgentApprovalRule
	policy, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalAsk, ProjectID: "project-1", Workspace: t.TempDir(), GOOS: "linux",
		PersistRule: func(_ context.Context, rule config.AgentApprovalRule) error {
			persisted = rule
			return nil
		},
		clock: func() time.Time { return time.Unix(100, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"command":"go test ./..."}`)
	request := agent.PermissionRequest{
		Tool: "bash", Arguments: arguments,
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceShell, MutationScope: agent.ToolMutationWorkspace,
			Recovery: agent.ToolRecoveryNonIdempotent,
		},
	}
	decision, err := policy.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != agent.PermissionAsk || decision.Reason.Chinese == "" || decision.Reason.English == "" {
		t.Fatalf("decision=%#v", decision)
	}
	resolved, err := policy.Resolve(context.Background(), agent.PermissionResolveRequest{
		Request: request, Resolution: agent.InteractionResolution{Permission: agent.PermissionRemember},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Allowed || !resolved.Remembered || persisted.ID == "" || persisted.ToolName != "bash" || persisted.ApprovedArgsHash == "" {
		t.Fatalf("resolved=%#v persisted=%#v", resolved, persisted)
	}
}

func TestDenovaPermissionPolicyBlocksCriticalShellCommand(t *testing.T) {
	policy, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalFullAccess, Workspace: t.TempDir(), GOOS: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := policy.Evaluate(context.Background(), agent.PermissionRequest{
		Tool: "bash", Arguments: json.RawMessage(`{"command":"rm -rf /"}`),
		Descriptor: agent.ToolDescriptor{Source: agent.ToolSourceShell, MutationScope: agent.ToolMutationExternal},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != agent.PermissionBlock {
		t.Fatalf("decision=%#v", decision)
	}
}
