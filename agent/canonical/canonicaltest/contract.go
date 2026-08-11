// Package canonicaltest provides reusable idempotency and partial-result
// checks for product CanonicalAdapter implementations.
package canonicaltest

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type Factory func(testing.TB) agent.CanonicalAdapter

func RunAdapterContract(t *testing.T, factory Factory) {
	t.Helper()
	adapter := factory(t)
	identity := adapter.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		t.Fatalf("identity = %#v", identity)
	}
	commit := agent.CommitIdentity{
		Session: agent.NamedSession("canonical-contract"), CommandID: "command-1",
		RunID: "run-1", Cycle: 1, Stage: agent.CommitInput,
	}
	input := agent.InputCommitRequest{Identity: commit, Hash: "input-hash", Input: agent.Text("input")}
	first, err := adapter.MaterializeInput(context.Background(), input)
	if err != nil || strings.TrimSpace(first.Revision) == "" {
		t.Fatalf("first input receipt = %#v error = %v", first, err)
	}
	replayed, err := adapter.MaterializeInput(context.Background(), input)
	if err != nil || replayed.Revision != first.Revision {
		t.Fatalf("replayed input receipt = %#v error = %v", replayed, err)
	}
	reconciled, err := adapter.Reconcile(context.Background(), agent.ReconcileRequest{Identity: commit, Hash: input.Hash})
	if err != nil || !reconciled.Found || reconciled.Revision != first.Revision {
		t.Fatalf("input reconciliation = %#v error = %v", reconciled, err)
	}

	outputIdentity := commit
	outputIdentity.Stage = agent.CommitOutput
	output := agent.OutputCommitRequest{
		Identity: outputIdentity, Hash: "output-hash",
		Message: *agent.AssistantMessage("output", nil),
	}
	outputReceipt, err := adapter.CommitOutput(context.Background(), output)
	if err != nil || strings.TrimSpace(outputReceipt.Revision) == "" {
		t.Fatalf("output receipt = %#v error = %v", outputReceipt, err)
	}
	effects := []agent.EffectRequest{
		{ID: "effect-1", Identity: outputIdentity, CallID: "call", Index: 0, Effect: agent.Effect{Kind: "contract", Data: []byte(`{"index":0}`)}},
		{ID: "effect-2", Identity: outputIdentity, CallID: "call", Index: 1, Effect: agent.Effect{Kind: "contract", Data: []byte(`{"index":1}`)}},
	}
	results, err := adapter.ApplyEffects(context.Background(), effects)
	if err != nil || len(results) != len(effects) {
		t.Fatalf("effect results = %#v error = %v", results, err)
	}
	for index, result := range results {
		if result.ID != effects[index].ID || result.Error == "" && strings.TrimSpace(result.Revision) == "" {
			t.Fatalf("effect result %d = %#v", index, result)
		}
	}
}
