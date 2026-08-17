package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
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

func TestDenovaPermissionRememberIsVisibleImmediatelyAndDoesNotChangeIdentity(t *testing.T) {
	workspace := t.TempDir()
	var durable []config.AgentApprovalRule
	load := func(context.Context) ([]config.AgentApprovalRule, error) {
		return append([]config.AgentApprovalRule(nil), durable...), nil
	}
	persist := func(_ context.Context, rule config.AgentApprovalRule) error {
		durable = config.NormalizeAgentApprovalRules(append(durable, rule))
		return nil
	}
	policy, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalAsk, ProjectID: "project-1", Workspace: workspace, GOOS: "linux",
		LoadRules: load, PersistRule: persist,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := permissionTestRequest(`{"command":"go test ./..."}`)
	before := policy.Identity()
	decision, err := policy.Evaluate(context.Background(), request)
	if err != nil || decision.Kind != agent.PermissionAsk {
		t.Fatalf("initial decision=%#v err=%v", decision, err)
	}
	resolved, err := policy.Resolve(context.Background(), agent.PermissionResolveRequest{
		Request: request, Resolution: agent.InteractionResolution{Permission: agent.PermissionRemember},
	})
	if err != nil || !resolved.Allowed || !resolved.Remembered {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	decision, err = policy.Evaluate(context.Background(), request)
	if err != nil || decision.Kind != agent.PermissionAllow {
		t.Fatalf("same-run decision=%#v err=%v", decision, err)
	}
	if after := policy.Identity(); after != before {
		t.Fatalf("remember changed Definition identity: before=%#v after=%#v", before, after)
	}

	reopened, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalAsk, ProjectID: "project-1", Workspace: workspace, GOOS: "linux",
		Rules: durable, LoadRules: load, PersistRule: persist,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Identity() != before {
		t.Fatalf("persisted rules changed cold Definition identity: before=%#v reopened=%#v", before, reopened.Identity())
	}
	decision, err = reopened.Evaluate(context.Background(), request)
	if err != nil || decision.Kind != agent.PermissionAllow {
		t.Fatalf("reopened decision=%#v err=%v", decision, err)
	}
}

func TestDenovaPermissionPersistFailureDoesNotAuthorizeProcessState(t *testing.T) {
	policy, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalAsk, ProjectID: "project-1", Workspace: t.TempDir(), GOOS: "linux",
		PersistRule: func(context.Context, config.AgentApprovalRule) error { return errors.New("disk unavailable") },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := permissionTestRequest(`{"command":"go test ./..."}`)
	if _, err := policy.Resolve(context.Background(), agent.PermissionResolveRequest{
		Request: request, Resolution: agent.InteractionResolution{Permission: agent.PermissionRemember},
	}); err == nil {
		t.Fatal("remember unexpectedly succeeded")
	}
	decision, err := policy.Evaluate(context.Background(), request)
	if err != nil || decision.Kind != agent.PermissionAsk {
		t.Fatalf("decision after failed persist=%#v err=%v", decision, err)
	}
}

func TestDenovaPermissionRejectsConflictingRememberedRuleBeforePersistence(t *testing.T) {
	workspace := t.TempDir()
	request := permissionTestRequest(`{"command":"go test ./..."}`)
	var generated config.AgentApprovalRule
	seed, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalAsk, ProjectID: "project-1", Workspace: workspace, GOOS: "linux",
		PersistRule: func(_ context.Context, rule config.AgentApprovalRule) error {
			generated = rule
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Resolve(context.Background(), agent.PermissionResolveRequest{
		Request: request, Resolution: agent.InteractionResolution{Permission: agent.PermissionRemember},
	}); err != nil {
		t.Fatal(err)
	}
	generated.ProjectID = "another-project"
	persisted := false
	policy, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalAsk, ProjectID: "project-1", Workspace: workspace, GOOS: "linux",
		Rules: []config.AgentApprovalRule{generated},
		PersistRule: func(context.Context, config.AgentApprovalRule) error {
			persisted = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Resolve(context.Background(), agent.PermissionResolveRequest{
		Request: request, Resolution: agent.InteractionResolution{Permission: agent.PermissionRemember},
	}); err == nil {
		t.Fatal("conflicting deterministic rule id unexpectedly succeeded")
	}
	if persisted {
		t.Fatal("conflicting rule reached durable store")
	}
}

func TestHarnessStateUsesDedicatedCrossProjectApproval(t *testing.T) {
	userPolicy, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalFullAccess, AgentKind: config.AgentKindGeneral,
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := []struct {
		name string
		want agent.PermissionDecisionKind
	}{
		{name: `manifest`, want: agent.PermissionAllow},
		{name: `file`, want: agent.PermissionAsk},
		{name: `update`, want: agent.PermissionAsk},
	}
	for _, test := range requests {
		var request agent.PermissionRequest
		switch test.name {
		case "manifest":
			request = agent.PermissionRequest{Tool: "read", Arguments: json.RawMessage(`{"path":"harness://state/current"}`)}
		case "file":
			request = agent.PermissionRequest{Tool: "read", Arguments: json.RawMessage(`{"path":"harness://state/tools/a.js"}`)}
		case "update":
			request = agent.PermissionRequest{Tool: "update_harness_state", Arguments: json.RawMessage(`{"base_revision":"r","changes":[]}`)}
		}
		decision, err := userPolicy.Evaluate(context.Background(), request)
		if err != nil || decision.Kind != test.want {
			t.Fatalf("%s decision=%#v err=%v", test.name, decision, err)
		}
	}

	optimizer, err := NewPermissionPolicy(PermissionConfig{
		Mode: config.AgentApprovalAsk, AgentKind: config.AgentKindHarnessOptimizer,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := optimizer.Evaluate(context.Background(), agent.PermissionRequest{
		Tool: "update_harness_state", Arguments: json.RawMessage(`{"base_revision":"r","changes":[]}`),
	})
	if err != nil || decision.Kind != agent.PermissionAllow {
		t.Fatalf("optimizer decision=%#v err=%v", decision, err)
	}
}

func permissionTestRequest(arguments string) agent.PermissionRequest {
	return agent.PermissionRequest{
		Tool: "bash", Arguments: json.RawMessage(arguments),
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceShell, MutationScope: agent.ToolMutationWorkspace,
			Recovery: agent.ToolRecoveryNonIdempotent,
		},
	}
}
