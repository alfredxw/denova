package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

type permissionResolutionInvariantPolicy struct {
	decision PermissionResolvedDecision
	resolve  atomic.Int32
}

func (*permissionResolutionInvariantPolicy) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "permission.resolution-invariant-test", Version: 1}
}

func (*permissionResolutionInvariantPolicy) Evaluate(context.Context, PermissionRequest) (PermissionDecision, error) {
	return PermissionDecision{
		Kind: PermissionAsk,
		Reason: LocalizedText{
			Chinese: "需要确认测试权限。",
			English: "Confirm the test permission.",
		},
		Details: PermissionDetails{CanRemember: true},
	}, nil
}

func (policy *permissionResolutionInvariantPolicy) Resolve(
	_ context.Context,
	_ PermissionResolveRequest,
) (PermissionResolvedDecision, error) {
	policy.resolve.Add(1)
	return policy.decision, nil
}

func TestPermissionResolutionRequiresExactAllowedAndRememberedDecision(t *testing.T) {
	tests := []struct {
		name     string
		choice   PermissionChoice
		decision PermissionResolvedDecision
		want     string
	}{
		{
			name: "allow once cannot claim a persisted rule", choice: PermissionAllowOnce,
			decision: PermissionResolvedDecision{Allowed: true, Remembered: true}, want: "allow-once",
		},
		{
			name: "remember must be durable before success", choice: PermissionRemember,
			decision: PermissionResolvedDecision{Allowed: true}, want: "remembered rule was durable",
		},
		{
			name: "deny cannot authorize execution", choice: PermissionDeny,
			decision: PermissionResolvedDecision{Allowed: true}, want: "deny decision",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := &permissionResolutionInvariantPolicy{decision: test.decision}
			run, toolRuns := startPermissionResolutionInvariantRun(t, policy)
			interactionID := waitPermissionInteractionID(t, run)
			err := run.Respond(context.Background(), interactionID, InteractionResponse{Permission: test.choice})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Respond error=%v, want text %q", err, test.want)
			}
			if policy.resolve.Load() != 1 {
				t.Fatalf("Permission Resolve calls=%d, want 1", policy.resolve.Load())
			}
			if toolRuns.Load() != 0 {
				t.Fatalf("tool ran %d time(s) before a valid durable permission decision", toolRuns.Load())
			}
			if _, abortErr := run.Abort(context.Background(), AbortRequest{Reason: "finish invariant test"}); abortErr != nil {
				t.Fatal(abortErr)
			}
			if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultAborted {
				t.Fatalf("aborted result=%#v error=%v", result, waitErr)
			}
		})
	}
}

func TestCancelledPermissionDoesNotInvokePolicyResolution(t *testing.T) {
	policy := &permissionResolutionInvariantPolicy{decision: PermissionResolvedDecision{Allowed: true, Remembered: true}}
	run, toolRuns := startPermissionResolutionInvariantRun(t, policy)
	interactionID := waitPermissionInteractionID(t, run)
	if err := run.Respond(context.Background(), interactionID, InteractionResponse{Cancelled: true}); err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("cancelled permission result=%#v error=%v", result, err)
	}
	if policy.resolve.Load() != 0 {
		t.Fatalf("cancelled permission invoked policy Resolve %d time(s)", policy.resolve.Load())
	}
	if toolRuns.Load() != 0 {
		t.Fatalf("cancelled permission executed tool %d time(s)", toolRuns.Load())
	}
}

func startPermissionResolutionInvariantRun(
	t *testing.T,
	policy PermissionPolicy,
) (*Run, *atomic.Int32) {
	t.Helper()
	var toolRuns atomic.Int32
	tool, err := InferTool("permission_invariant", "exercise the permission fence", func(context.Context, struct{}) (string, error) {
		toolRuns.Add(1)
		return "ran", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{
			ID: "permission-invariant-call", Type: "function",
			Function: FunctionCall{Name: "permission_invariant", Arguments: `{}`},
		}}),
		AssistantMessage("done", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Model: model,
		Tools: mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "tools.permission-invariant", Version: 1}, ToolDefinition{
			Tool: tool,
			Descriptor: ToolDescriptor{
				Source: ToolSourceWrite, Execution: ToolExecutionWorkspaceExclusive,
				MutationScope: ToolMutationWorkspace, PostCheck: ToolPostCheckWorkspaceChange,
				Recovery: ToolRecoveryIdempotent, ResultProjection: ToolResultBoundedModelContext,
				ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent,
				MaxResultBytes: 64 << 10,
			},
		}),
		Permission: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("permission-resolution-invariant-"+newPublicID("test")))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("test permission resolution"))
	if err != nil {
		t.Fatal(err)
	}
	return run, &toolRuns
}

func waitPermissionInteractionID(t *testing.T, run *Run) string {
	t.Helper()
	for event := range run.Events() {
		if request, ok := event.Payload.(InteractionRequested); ok {
			return request.Request.ID
		}
	}
	t.Fatal(errors.New("Permission Interaction was not requested"))
	return ""
}
