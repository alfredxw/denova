// Package permissiontest provides reusable behavioral checks for Permission
// Policy implementations.
package permissiontest

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type Factory func(testing.TB) agent.PermissionPolicy

func RunPolicyContract(t *testing.T, factory Factory) {
	t.Helper()
	policy := factory(t)
	identity := policy.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		t.Fatalf("identity = %#v", identity)
	}
	requests := []agent.PermissionRequest{
		{Tool: "read", Descriptor: agent.ToolDescriptor{Source: agent.ToolSourceRead, MutationScope: agent.ToolMutationNone}},
		{Tool: "write", Descriptor: agent.ToolDescriptor{Source: agent.ToolSourceWrite, MutationScope: agent.ToolMutationWorkspace}},
	}
	for _, request := range requests {
		decision, err := policy.Evaluate(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		switch decision.Kind {
		case agent.PermissionAllow, agent.PermissionAsk, agent.PermissionBlock:
		default:
			t.Fatalf("decision = %#v", decision)
		}
	}
	allowed, err := policy.Resolve(context.Background(), agent.PermissionResolveRequest{
		Request: requests[1], Resolution: agent.InteractionResolution{Permission: agent.PermissionAllowOnce},
	})
	if err != nil || !allowed.Allowed || allowed.Remembered {
		t.Fatalf("allow-once = %#v error = %v", allowed, err)
	}
	denied, err := policy.Resolve(context.Background(), agent.PermissionResolveRequest{
		Request: requests[1], Resolution: agent.InteractionResolution{Permission: agent.PermissionDeny},
	})
	if err != nil || denied.Allowed {
		t.Fatalf("deny = %#v error = %v", denied, err)
	}
}
