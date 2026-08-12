// Package goaltest provides reusable behavioral checks for the built-in
// standard Goal protocol. A custom GoalManager owns its own mutation kinds and
// Data schema and must define a domain-specific contract instead.
package goaltest

import (
	"context"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type Factory func(testing.TB) agent.GoalManager

func RunStandardManagerContract(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("identity_and_revision_fences", func(t *testing.T) {
		manager := factory(t)
		identity := manager.Identity()
		if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
			t.Fatalf("identity = %#v", identity)
		}
		created, err := manager.Apply(context.Background(), agent.GoalApplyRequest{Mutation: agent.GoalMutation{
			Kind: agent.GoalSet, Objective: "complete the contract objective", MutationID: "contract-create",
		}})
		if err != nil || !created.Active() || created.Revision == 0 || strings.TrimSpace(created.ID) == "" {
			t.Fatalf("created = %#v error = %v", created, err)
		}
		replayed, err := manager.Apply(context.Background(), agent.GoalApplyRequest{
			Present: true, Current: created,
			Mutation: agent.GoalMutation{Kind: agent.GoalClear, MutationID: "contract-create"},
		})
		if err != nil || !reflect.DeepEqual(replayed, created) {
			t.Fatalf("idempotent replay = %#v error = %v", replayed, err)
		}
		if _, err := manager.Apply(context.Background(), agent.GoalApplyRequest{
			Present: true, Current: created,
			Mutation: agent.GoalMutation{
				Kind: agent.GoalPause, ExpectedID: created.ID,
				ExpectedRevision: created.Revision + 1, MutationID: "contract-stale",
			},
		}); err == nil {
			t.Fatal("stale Goal revision was accepted")
		}
	})

	t.Run("prepare_and_terminal_continuation", func(t *testing.T) {
		manager := factory(t)
		created, err := manager.Apply(context.Background(), agent.GoalApplyRequest{Mutation: agent.GoalMutation{
			Kind: agent.GoalSet, Objective: "prepare this objective", MutationID: "contract-prepare",
		}})
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := manager.Prepare(context.Background(), agent.GoalPrepareRequest{State: created, Present: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(prepared.Context) == 0 && len(prepared.Tools) == 0 && !prepared.StandardTool {
			t.Fatal("active Goal prepared no model-facing capability")
		}
		continuation, err := manager.AfterRun(context.Background(), agent.GoalAfterRunRequest{
			State: created, Present: true, Result: agent.Result{Status: agent.ResultCompleted},
		})
		if err != nil {
			t.Fatal(err)
		}
		if continuation.Continue && strings.TrimSpace(continuation.Input.Text) == "" {
			t.Fatal("continuation has no prompt")
		}
		completed, err := manager.Apply(context.Background(), agent.GoalApplyRequest{
			Current: created, Present: true,
			Mutation: agent.GoalMutation{
				Kind: agent.GoalComplete, ExpectedID: created.ID,
				ExpectedRevision: created.Revision, MutationID: "contract-complete",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		terminal, err := manager.AfterRun(context.Background(), agent.GoalAfterRunRequest{
			State: completed, Present: true, Result: agent.Result{Status: agent.ResultCompleted},
		})
		if err != nil || terminal.Continue {
			t.Fatalf("terminal continuation = %#v error = %v", terminal, err)
		}
	})
}
