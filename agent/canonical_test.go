package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

type canonicalProbe struct {
	mu     sync.Mutex
	events []string
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
		Model: model, Tools: StaticTools(definition), Canonical: probe,
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
