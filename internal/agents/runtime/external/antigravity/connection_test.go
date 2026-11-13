package antigravity

import (
	"context"
	"errors"
	"testing"

	"denova/internal/agents/runtime/external"
)

func TestInterruptedCheckPreservesReadinessAndClosedCheckDoesNot(t *testing.T) {
	client := fixtureClient(t, "normal")
	client.state = external.ConnectionState{Status: "ready"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	state, err := client.Check(ctx)
	if !errors.Is(err, context.Canceled) || state.Status != "ready" || client.Status() != state {
		t.Fatalf("interrupted probe lost confirmed state: %+v %+v %v", state, client.Status(), err)
	}
	_ = client.Close()
	state, err = client.Check(t.Context())
	if err == nil || state.Status != "unavailable" || client.Status() != state {
		t.Fatalf("closed probe retained readiness: %+v %+v %v", state, client.Status(), err)
	}
}

func TestAuthenticationFailureReplacesPreviouslyReadyState(t *testing.T) {
	client := fixtureClient(t, "auth")
	client.state = external.ConnectionState{Status: "ready"}
	state, err := client.Check(t.Context())
	if err != nil || state.Status != "auth_required" || client.Status() != state {
		t.Fatalf("signed-out probe retained readiness: %+v %+v %v", state, client.Status(), err)
	}
}

func TestIsolationTimeoutReplacesPreviouslyReadyState(t *testing.T) {
	client := fixtureClient(t, "tool")
	client.state = external.ConnectionState{Status: "ready"}
	verifyOSSandbox = func() error { return context.DeadlineExceeded }
	state, err := client.Check(t.Context())
	if !errors.Is(err, ErrUnsafeCustomization) || state.Status != "unavailable" ||
		state.ReasonKey != "agentRuntime.antigravityIsolationUnsupported" || client.Status() != state {
		t.Fatalf("isolation timeout probe retained readiness: %+v %+v %v", state, client.Status(), err)
	}
}
