package external

import (
	"context"
	"errors"
	"testing"

	"denova/config"
)

func leaseRuntime(t *testing.T, adapter Adapter) *Runtime {
	t.Helper()
	return &Runtime{Selection: config.RuntimeSelection{Kind: config.RuntimeClaude}, CacheRoot: t.TempDir(),
		Acquire: func(context.Context) (Adapter, func(), error) { return adapter, func() {}, nil }}
}

func leaseRequest() SessionRequest {
	return SessionRequest{Key: "project/session", Boundary: "boundary-1", Input: Input{Text: "Continue."}}
}

// A failed attempt must not destroy the acknowledged binding: the alternative is
// replaying the whole canonical source into a fresh provider conversation on
// every later turn.
func TestRuntimeRestoresBindingAfterFailedAttempt(t *testing.T) {
	var resumed []string
	adapter := adapterFunc(func(_ context.Context, input Input, _ Host) (Result, error) {
		resumed = append(resumed, input.SessionID)
		if input.SessionID == "" {
			return Result{SessionID: "provider-1", Settled: true}, nil
		}
		return Result{}, errors.New("attempt failed")
	})
	runtime := leaseRuntime(t, adapter)

	first, err := runtime.Run(t.Context(), leaseRequest(), maintenanceHost{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Session.Accept(t.Context(), "boundary-1"); err != nil {
		t.Fatal(err)
	}
	if err := first.Session.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := runtime.Run(t.Context(), leaseRequest(), maintenanceHost{})
	if err == nil {
		t.Fatal("failing attempt succeeded")
	}
	if err := second.Session.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.Run(t.Context(), leaseRequest(), maintenanceHost{}); err == nil {
		t.Fatal("third attempt succeeded")
	}
	if len(resumed) != 3 || resumed[0] != "" || resumed[1] != "provider-1" || resumed[2] != "provider-1" {
		t.Fatalf("provider reuse after failure: %v", resumed)
	}
}

// A binding is leased to exactly one attempt at a time, so a concurrent attempt
// starts a fresh conversation instead of appending to an in-flight one.
func TestRuntimeLeasesBindingToSingleAttempt(t *testing.T) {
	runtime := leaseRuntime(t, adapterFunc(func(context.Context, Input, Host) (Result, error) {
		return Result{SessionID: "provider-1", Settled: true}, nil
	}))
	first, err := runtime.prepare(t.Context(), leaseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if first.input.SessionID != "" {
		t.Fatalf("unexpected cached session: %q", first.input.SessionID)
	}
	first.input.SessionID, first.cache.SessionID = "provider-1", "provider-1"
	if err := first.Accept(t.Context(), "boundary-1"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	holder, err := runtime.prepare(t.Context(), leaseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if holder.input.SessionID != "provider-1" {
		t.Fatalf("acknowledged binding was not resumed: %q", holder.input.SessionID)
	}
	second, err := runtime.prepare(t.Context(), leaseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if second.input.SessionID != "" {
		t.Fatalf("second attempt shared the leased conversation: %q", second.input.SessionID)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
}
