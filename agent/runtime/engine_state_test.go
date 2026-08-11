package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestEngineStatePersistsAcrossCyclesAndRuntimeReopen(t *testing.T) {
	t.Parallel()

	store := runstate.NewMemoryJournalStore()
	binding := testBindingAt("/workspace", "engine-state")
	firstState := json.RawMessage(`{"version":1,"messages":["first"]}`)
	secondState := json.RawMessage(`{"version":1,"messages":["first","second"]}`)
	firstEngine := runstate.NewScriptedEngine(
		runstate.EngineScript{Events: []runstate.EngineEvent{
			runstate.EngineAssistantFinal{Content: "first", State: firstState},
		}},
		runstate.EngineScript{Events: []runstate.EngineEvent{
			runstate.EngineAssistantFinal{Content: "second", State: secondState},
		}},
	)
	firstRuntime, err := runstate.NewRuntime(firstEngine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := firstRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	first, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "first", Input: runstate.UserInput{Text: "first"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSettled(t, harness, first.Cursor)
	second, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "second", Input: runstate.UserInput{Text: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSettled(t, harness, second.Cursor)

	requests := firstEngine.Requests()
	if len(requests) != 2 || len(requests[0].Snapshot.State) != 0 ||
		string(requests[1].Snapshot.State) != string(firstState) {
		t.Fatalf("cycle Engine states = %#v", requests)
	}
	if err := firstRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopenedEngine := runstate.NewScriptedEngine(runstate.EngineScript{Events: []runstate.EngineEvent{
		runstate.EngineAssistantFinal{Content: "third", State: json.RawMessage(`{"version":1,"messages":["third"]}`)},
	}})
	reopenedRuntime, err := runstate.NewRuntime(reopenedEngine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedRuntime.Close(context.Background()) })
	reopened, err := reopenedRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	third, err := reopened.Submit(context.Background(), runstate.StartTurn{
		ID: "third", Input: runstate.UserInput{Text: "third"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSettled(t, reopened, third.Cursor)
	reopenedRequests := reopenedEngine.Requests()
	if len(reopenedRequests) != 1 || string(reopenedRequests[0].Snapshot.State) != string(secondState) {
		t.Fatalf("reopened Engine state = %#v, want %s", reopenedRequests, secondState)
	}
}
