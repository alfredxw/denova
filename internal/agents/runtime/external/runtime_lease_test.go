package external

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"testing"

	"denova/config"

	agentschema "github.com/alfredxw/denova/agent/schema"
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

// A cache rebuild replays canonical media into the fresh provider conversation:
// historical images must reach the adapter instead of being dropped with the
// lost binding (engine switch or an expired CLI conversation store).
func TestRuntimeRebuildCarriesHistoryMedia(t *testing.T) {
	image := checkpointImage(t, png.NoCompression)
	var received []Input
	attempts := 0
	adapter := adapterFunc(func(_ context.Context, input Input, _ Host) (Result, error) {
		received = append(received, input)
		attempts++
		if attempts == 2 {
			return Result{}, ErrSessionUnavailable
		}
		return Result{SessionID: fmt.Sprintf("provider-%d", attempts), Settled: true}, nil
	})
	runtime := leaseRuntime(t, adapter)
	request := leaseRequest()
	request.Prepare = func(_ context.Context, input Input, _ Adapter) (Input, error) {
		input.History = []Message{{Role: "user", Text: "Recent reference", Cursor: 1,
			Attachments: []agentschema.Attachment{image}, ToolImages: []agentschema.Attachment{image}}}
		return input, nil
	}
	first, err := runtime.Run(t.Context(), request, maintenanceHost{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Session.Accept(t.Context(), "boundary-1"); err != nil {
		t.Fatal(err)
	}
	if err := first.Session.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Run(t.Context(), request, maintenanceHost{})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Session.Close(); err != nil {
		t.Fatal(err)
	}
	if len(received) != 3 {
		t.Fatalf("attempts: %d", len(received))
	}
	if received[0].SessionID != "" || received[1].SessionID != "provider-1" || received[2].SessionID != "" {
		t.Fatalf("session sequence: %q %q %q", received[0].SessionID, received[1].SessionID, received[2].SessionID)
	}
	rebuilt := received[2]
	if len(rebuilt.History) != 1 || len(rebuilt.History[0].Attachments) != 1 || len(rebuilt.History[0].ToolImages) != 1 {
		t.Fatalf("rebuilt history lost media: %+v", rebuilt.History)
	}
}

// Switching engines changes the runtime selection inside the binding key, so
// the old provider cache misses and the fresh conversation replays full
// history — this is the engine-switch rebuild path users actually hit.
func TestRuntimeEngineSwitchRebuildsCarryingHistory(t *testing.T) {
	image := checkpointImage(t, png.NoCompression)
	root := t.TempDir()
	var received []Input
	adapter := adapterFunc(func(_ context.Context, input Input, _ Host) (Result, error) {
		received = append(received, input)
		return Result{SessionID: "provider-1", Settled: true}, nil
	})
	request := leaseRequest()
	request.Prepare = func(_ context.Context, input Input, _ Adapter) (Input, error) {
		input.History = []Message{{Role: "user", Text: "Recent reference", Cursor: 1, Attachments: []agentschema.Attachment{image}}}
		return input, nil
	}
	runtime := func(selection config.RuntimeSelection) *Runtime {
		return &Runtime{Selection: selection, CacheRoot: root,
			Acquire: func(context.Context) (Adapter, func(), error) { return adapter, func() {}, nil }}
	}
	first, err := runtime(config.RuntimeSelection{Kind: config.RuntimeClaude}).Run(t.Context(), request, maintenanceHost{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Session.Accept(t.Context(), "boundary-1"); err != nil {
		t.Fatal(err)
	}
	if err := first.Session.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := runtime(config.RuntimeSelection{Kind: config.RuntimeAntigravity}).Run(t.Context(), request, maintenanceHost{})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Session.Close(); err != nil {
		t.Fatal(err)
	}
	if len(received) != 2 {
		t.Fatalf("attempts: %d", len(received))
	}
	if received[0].SessionID != "" || received[1].SessionID != "" {
		t.Fatalf("expected fresh conversations: %q %q", received[0].SessionID, received[1].SessionID)
	}
	if len(received[1].History) != 1 || len(received[1].History[0].Attachments) != 1 {
		t.Fatalf("engine switch lost history media: %+v", received[1].History)
	}
}
