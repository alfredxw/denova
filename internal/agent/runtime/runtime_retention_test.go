package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runstate "denova/internal/agent/runtime"
)

func TestActorDisplayRetentionIsBoundedAndColdCommandReplayStaysIdempotent(t *testing.T) {
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "one"}}},
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "two"}}},
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "three"}}},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{
		RetainedEventLimit: 4, RetainedMessageLimit: 2, RetainedCommandLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "retention"})
	if err != nil {
		t.Fatal(err)
	}
	commands := []runstate.StartTurn{
		{ID: "one", Input: runstate.UserInput{Text: "first"}},
		{ID: "two", Input: runstate.UserInput{Text: "second"}},
		{ID: "three", Input: runstate.UserInput{Text: "third"}},
	}
	var first runstate.Receipt
	for index, command := range commands {
		observeCtx, stopObserving := context.WithTimeout(context.Background(), 500*time.Millisecond)
		observation, err := harness.ObserveFromNow(observeCtx)
		if err != nil {
			stopObserving()
			t.Fatal(err)
		}
		receipt, err := harness.Submit(context.Background(), command)
		if err != nil {
			stopObserving()
			t.Fatal(err)
		}
		if index == 0 {
			first = receipt
		}
		waitForRetainedTestSettlement(t, observeCtx, observation, receipt.OperationID)
		stopObserving()
	}
	if _, err := harness.Observe(context.Background(), 0); !errors.Is(err, runstate.ErrCursorExpired) {
		t.Fatalf("old cursor error = %v, want ErrCursorExpired", err)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Snapshot.MessagesTruncated || len(observation.Snapshot.Messages) != 2 || observation.Snapshot.TimelineStartCursor <= 1 {
		t.Fatalf("bounded snapshot = %#v", observation.Snapshot)
	}
	replayed, err := harness.Submit(context.Background(), commands[0])
	if err != nil {
		t.Fatalf("cold command replay: %v", err)
	}
	if !replayed.Replayed || replayed.OperationID != first.OperationID || replayed.Cursor != first.Cursor {
		t.Fatalf("cold replay receipt = %#v, want %#v", replayed, first)
	}
}

func waitForRetainedTestSettlement(t *testing.T, ctx context.Context, observation runstate.Observation, operationID runstate.OperationID) {
	t.Helper()
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("event stream closed before settlement")
			}
			settled, ok := event.Payload.(runstate.OperationSettledEvent)
			if ok && settled.OperationID == operationID {
				return
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation failed: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for operation %q: %v", operationID, ctx.Err())
		}
	}
}

func TestProjectConservativelyReportsUnfinishedJournalWithoutCreatingActor(t *testing.T) {
	t.Parallel()

	store := runstate.NewMemoryJournalStore()
	ref := runstate.BindingRef{
		Kind: runstate.BindingWriting, Profile: runstate.ProfileWriting,
		Workspace: "/book", SessionID: "unfinished-projection",
	}
	key, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	identity := runstate.DomainCommitIdentity{
		CommandID: "start", OperationID: "operation", Cycle: 1, Stage: runstate.DomainCommitOutput,
	}
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: "operation", Fingerprint: "seed"},
		runstate.OperationStartedEvent{OperationID: "operation"},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "user", Role: runstate.RoleUser, Content: "write",
			Input: runstate.UserInput{Text: "write"}, Operation: "operation",
		}},
		runstate.CycleStartedEvent{OperationID: "operation", Cycle: 1, SnapshotID: "snapshot"},
		runstate.DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "output-hash"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	var engines atomic.Int32
	runtime, err := runstate.NewRuntime(runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		engines.Add(1)
		return runstate.NewScriptedEngine(), nil
	}), store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := runtime.Project(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "unfinished-projection"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.ActiveOperation != "" || !status.RecoveryPending || status.LastOperation == nil || status.LastOperation.Status != runstate.OperationInterrupted {
		t.Fatalf("conservative projection = %#v", status)
	}
	if !strings.Contains(status.LastOperation.Reason, "recovery_pending") || len(status.DomainCommits) != 1 || status.DomainCommits[0].Identity.Stage != runstate.DomainCommitOutput {
		t.Fatalf("recovery evidence = %#v", status)
	}
	if got := engines.Load(); got != 0 {
		t.Fatalf("Project created %d Engines", got)
	}
}

func TestFileJournalStoresToolPayloadDescriptorsInsteadOfRawContent(t *testing.T) {
	root := t.TempDir()
	store, err := runstate.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	const argumentSecret = "argument-secret-must-not-be-journaled"
	const resultSecret = "result-secret-must-not-be-journaled"
	engine := runstate.NewScriptedEngine(runstate.EngineScript{Events: []runstate.EngineEvent{
		runstate.EngineToolStarted{CallID: "call", Name: "write_file", Arguments: json.RawMessage(`{"secret":"` + argumentSecret + `"}`)},
		runstate.EngineToolFinished{CallID: "call", Name: "write_file", Result: resultSecret, RetrySafety: runstate.RetryUnsafe},
		runstate.EngineAssistantFinal{Content: "done"},
	}})
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "payload-descriptor"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForSettled(t, harness, receipt.Cursor)
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var journalBytes []byte
	var manifestBytes []byte
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			journalBytes, err = os.ReadFile(root + "/" + entry.Name())
			if err != nil {
				t.Fatal(err)
			}
		}
		if strings.HasSuffix(entry.Name(), ".manifest.json") {
			manifestBytes, err = os.ReadFile(root + "/" + entry.Name())
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	encoded := string(journalBytes)
	if strings.Contains(encoded, argumentSecret) || strings.Contains(encoded, resultSecret) {
		t.Fatalf("raw tool payload leaked into journal: %s", encoded)
	}
	if !strings.Contains(encoded, "sha256") {
		t.Fatalf("tool payload descriptors missing: %s", encoded)
	}
	manifest := string(manifestBytes)
	if !strings.Contains(manifest, `"version":1`) || !strings.Contains(manifest, `"session_id":"payload-descriptor"`) || !strings.Contains(manifest, `"kind":"writing"`) {
		t.Fatalf("versioned binding manifest missing: %s", manifest)
	}
}
