package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

type canonicalProbe struct {
	mu     sync.Mutex
	events []string
}

type identityCanonicalAdapter struct {
	identity         CapabilityIdentity
	materializeCalls atomic.Int32
	reconcileCalls   atomic.Int32
}

func (adapter *identityCanonicalAdapter) Identity() CapabilityIdentity { return adapter.identity }

func (adapter *identityCanonicalAdapter) MaterializeInput(context.Context, InputCommitRequest) (CommitReceipt, error) {
	adapter.materializeCalls.Add(1)
	return CommitReceipt{Revision: "input:1"}, nil
}

func (*identityCanonicalAdapter) CommitOutput(context.Context, OutputCommitRequest) (OutputCommitReceipt, error) {
	return OutputCommitReceipt{Revision: "output:1"}, nil
}

func (adapter *identityCanonicalAdapter) Reconcile(context.Context, ReconcileRequest) (ReconcileResult, error) {
	adapter.reconcileCalls.Add(1)
	return ReconcileResult{Found: true, Revision: "input:1"}, nil
}

func (*identityCanonicalAdapter) ApplyEffects(context.Context, []EffectRequest) ([]EffectResult, error) {
	return nil, nil
}

type canonicalIdentityDriftSource struct {
	definition Definition

	mu       sync.Mutex
	adapters []CanonicalAdapter
	next     int
	constant CanonicalAdapter
}

func (source *canonicalIdentityDriftSource) Prepare(context.Context, PrepareRequest) (Definition, error) {
	return source.definition, nil
}

func (source *canonicalIdentityDriftSource) CanonicalInput(context.Context, PrepareRequest) (CanonicalAdapter, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.next < len(source.adapters) {
		adapter := source.adapters[source.next]
		source.next++
		return adapter, nil
	}
	return source.constant, nil
}

type canonicalOrderContext struct{ committed *atomic.Bool }

func (canonicalOrderContext) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "context.canonical-order-test", Version: 1}
}

func (source canonicalOrderContext) Materialize(context.Context, ContextRequest) ([]ContextFragment, error) {
	if !source.committed.Load() {
		return nil, errors.New("context materialized before canonical input")
	}
	return nil, nil
}

type effectTool struct{ Tool }

func (tool effectTool) Run(ctx context.Context, arguments string, options ...ToolOption) (ToolResult, error) {
	result, err := tool.Tool.Run(ctx, arguments, options...)
	result.Effects = []Effect{{Kind: "test.effect", Data: []byte(`{"value":1}`)}}
	return result, err
}

func (*canonicalProbe) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "canonical.test", Version: 1}
}

func (probe *canonicalProbe) append(event string) {
	probe.mu.Lock()
	probe.events = append(probe.events, event)
	probe.mu.Unlock()
}

func (probe *canonicalProbe) MaterializeInput(_ context.Context, request InputCommitRequest) (CommitReceipt, error) {
	if request.Identity.Stage != CommitInput || request.Identity.RunID == "" || request.Hash == "" {
		return CommitReceipt{}, ErrDefinitionMismatch
	}
	probe.append("input")
	return CommitReceipt{Revision: "input:1"}, nil
}

func (probe *canonicalProbe) CommitOutput(_ context.Context, request OutputCommitRequest) (OutputCommitReceipt, error) {
	if request.Identity.Stage != CommitOutput || request.Message.Content != "final" || request.Hash == "" {
		return OutputCommitReceipt{}, ErrDefinitionMismatch
	}
	probe.append("output")
	return OutputCommitReceipt{Revision: "output:1"}, nil
}

func (*canonicalProbe) Reconcile(context.Context, ReconcileRequest) (ReconcileResult, error) {
	return ReconcileResult{}, nil
}

func (probe *canonicalProbe) ApplyEffects(_ context.Context, requests []EffectRequest) ([]EffectResult, error) {
	results := make([]EffectResult, len(requests))
	for index, request := range requests {
		probe.append("effect")
		results[index] = EffectResult{ID: request.ID, Revision: "effect:1"}
	}
	return results, nil
}

func (probe *canonicalProbe) snapshot() []string {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	return append([]string(nil), probe.events...)
}

func TestCanonicalAdapterOwnsInputOutputAndToolEffectBarriers(t *testing.T) {
	probe := &canonicalProbe{}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{
			ID: "effect-call", Type: "function", Function: FunctionCall{Name: "effect", Arguments: `{}`},
		}}),
		AssistantMessage("final", nil),
	}}
	tool := &functionTool{name: "effect", run: func(context.Context, string) (string, error) { return "effect result", nil }}
	definition := testToolDefinition(tool)
	definition.Tool = effectTool{Tool: definition.Tool}
	owner, err := New(context.Background(), Definition{
		Model: model, Tools: mustStaticTools(t, definition), Canonical: probe,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("commit this"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	got := probe.snapshot()
	want := []string{"input", "output", "effect"}
	if len(got) != len(want) {
		t.Fatalf("canonical events=%v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("canonical events=%v", got)
		}
	}
}

func TestCanonicalInputRejectsAdapterIdentityDriftBetweenPlanAndWrite(t *testing.T) {
	planned := &identityCanonicalAdapter{identity: CapabilityIdentity{
		Kind: "canonical.identity-drift-test", Version: 1, ConfigHash: "planned",
	}}
	materialized := &identityCanonicalAdapter{identity: CapabilityIdentity{
		Kind: "canonical.identity-drift-test", Version: 1, ConfigHash: "materialized",
	}}
	source := &canonicalIdentityDriftSource{
		definition: Definition{Model: &lifecycleModel{responses: []*Message{AssistantMessage("must not run", nil)}}},
		adapters:   []CanonicalAdapter{planned, materialized},
		constant:   materialized,
	}
	owner, err := New(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("canonical-plan-write-drift"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Run(context.Background(), Input{Text: "commit once", IdempotencyKey: "canonical-drift"}); !errors.Is(err, runstate.ErrDomainCommitRejected) {
		t.Fatalf("Run error=%v, want ErrDomainCommitRejected", err)
	}
	if got := planned.materializeCalls.Load() + materialized.materializeCalls.Load(); got != 0 {
		t.Fatalf("canonical write calls=%d, want 0 after identity drift", got)
	}
}

func TestCanonicalInputReconcileRejectsAdapterIdentityDriftAfterWrite(t *testing.T) {
	written := &identityCanonicalAdapter{identity: CapabilityIdentity{
		Kind: "canonical.reconcile-drift-test", Version: 1, ConfigHash: "written",
	}}
	current := &identityCanonicalAdapter{identity: CapabilityIdentity{
		Kind: "canonical.reconcile-drift-test", Version: 1, ConfigHash: "current",
	}}
	source := &canonicalIdentityDriftSource{constant: current}
	key := NamedSession("canonical-reconcile-drift")
	engine := &definitionEngine{source: source, key: key}
	input := Input{Text: "already written", IdempotencyKey: "command"}
	_, runtimeInput, err := encodeInput(input)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := canonicalInputHash(input, written.Identity())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := runstate.TurnSnapshot{
		CommandID: "command", OperationID: "operation", Cycle: 1,
		Delivery: runstate.DeliveryStart, Input: runtimeInput,
	}
	_, err = engine.ReconcileDomainCommit(context.Background(), runstate.DomainCommitReconcileRequest{
		Snapshot: snapshot,
		Commit: runstate.DomainCommitState{
			Identity: runtimeCommitIdentity(canonicalCommitIdentity(key, snapshot, CommitInput)),
			Hash:     hash,
		},
	})
	if !errors.Is(err, ErrDefinitionMismatch) {
		t.Fatalf("ReconcileDomainCommit error=%v, want ErrDefinitionMismatch", err)
	}
	if got := current.reconcileCalls.Load(); got != 0 {
		t.Fatalf("drifted canonical store was queried %d times", got)
	}
}

func TestCanonicalInputReceiptRejectsPreparedAdapterIdentityDriftBeforeModel(t *testing.T) {
	admitted := &identityCanonicalAdapter{identity: CapabilityIdentity{
		Kind: "canonical.prepared-drift-test", Version: 1, ConfigHash: "admitted",
	}}
	prepared := &identityCanonicalAdapter{identity: CapabilityIdentity{
		Kind: "canonical.prepared-drift-test", Version: 1, ConfigHash: "prepared",
	}}
	model := &lifecycleModel{responses: []*Message{AssistantMessage("must not run", nil)}}
	source := &canonicalIdentityDriftSource{
		definition: Definition{Model: model, Canonical: prepared},
		constant:   admitted,
	}
	owner, err := New(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("canonical-prepared-drift"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "freeze adapter", IdempotencyKey: "prepared-drift"})
	if err != nil {
		t.Fatal(err)
	}
	result, waitErr := run.Wait(context.Background())
	if result.Status != ResultFailed || waitErr == nil || !strings.Contains(result.Reason, ErrDefinitionMismatch.Error()) {
		t.Fatalf("result=%#v error=%v, want Definition mismatch failure", result, waitErr)
	}
	if got := admitted.materializeCalls.Load(); got != 1 {
		t.Fatalf("canonical input materialized %d times, want exactly once", got)
	}
	if got := len(model.calls()); got != 0 {
		t.Fatalf("model calls=%d after canonical identity drift", got)
	}
}

func TestCanonicalInputPrecedesDynamicContextMaterialization(t *testing.T) {
	var committed atomic.Bool
	model := &lifecycleModel{responses: []*Message{AssistantMessage("final", nil)}}
	canonical := CanonicalAdapterFuncs{
		CapabilityIdentity: CapabilityIdentity{Kind: "canonical.order-test", Version: 1},
		MaterializeInputFn: func(context.Context, InputCommitRequest) (CommitReceipt, error) {
			committed.Store(true)
			return CommitReceipt{Revision: "input:1"}, nil
		},
		CommitOutputFn: func(context.Context, OutputCommitRequest) (OutputCommitReceipt, error) {
			return OutputCommitReceipt{Revision: "output:1"}, nil
		},
		ReconcileFn: func(context.Context, ReconcileRequest) (ReconcileResult, error) {
			return ReconcileResult{}, nil
		},
		ApplyEffectsFn: func(context.Context, []EffectRequest) ([]EffectResult, error) {
			return nil, nil
		},
	}
	owner, err := New(context.Background(), Definition{
		Model: model, Context: canonicalOrderContext{committed: &committed}, Canonical: canonical,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("check ordering"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestCanonicalOutputProjectionOwnsFutureTranscript(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("raw provider protocol", nil),
		AssistantMessage("second response", nil),
	}}
	var committedOutput string
	canonical := CanonicalAdapterFuncs{
		CapabilityIdentity: CapabilityIdentity{Kind: "canonical.output-projection-test", Version: 1},
		MaterializeInputFn: func(context.Context, InputCommitRequest) (CommitReceipt, error) {
			return CommitReceipt{Revision: "input:1"}, nil
		},
		CommitOutputFn: func(_ context.Context, request OutputCommitRequest) (OutputCommitReceipt, error) {
			committedOutput = request.Message.Content
			return OutputCommitReceipt{
				Revision:   "output:1",
				Transcript: &OutputProjection{Content: "product-approved projection"},
			}, nil
		},
		ReconcileFn: func(context.Context, ReconcileRequest) (ReconcileResult, error) {
			return ReconcileResult{}, nil
		},
		ApplyEffectsFn: func(context.Context, []EffectRequest) ([]EffectResult, error) {
			return nil, nil
		},
	}
	owner, err := New(context.Background(), Definition{Model: model, Canonical: canonical})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("output-projection"))
	if err != nil {
		t.Fatal(err)
	}
	for index, input := range []string{"first request", "second request"} {
		run, runErr := session.Run(context.Background(), Input{
			Text: input, IdempotencyKey: fmt.Sprintf("projection-%d", index),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("result=%#v error=%v", result, waitErr)
		}
	}
	if committedOutput != "second response" {
		t.Fatalf("canonical adapter did not receive raw provider output: %q", committedOutput)
	}
	calls := model.calls()
	if len(calls) != 2 {
		t.Fatalf("model calls=%d, want 2", len(calls))
	}
	var sawProjection, sawRaw bool
	for _, message := range calls[1] {
		if message == nil {
			continue
		}
		sawProjection = sawProjection || message.Content == "product-approved projection"
		sawRaw = sawRaw || message.Content == "raw provider protocol"
	}
	if !sawProjection || sawRaw {
		t.Fatalf("second model transcript projection=%t raw=%t messages=%#v", sawProjection, sawRaw, calls[1])
	}
}
