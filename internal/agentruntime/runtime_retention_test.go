package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestActorDisplayRetentionIsBoundedAndColdCommandReplayStaysIdempotent(t *testing.T) {
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "one"}}},
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "two"}}},
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "three"}}},
	)
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{
		RetainedEventLimit: 4, RetainedMessageLimit: 2, RetainedCommandLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "retention"})
	if err != nil {
		t.Fatal(err)
	}
	commands := []agentruntime.StartTurn{
		{ID: "one", Input: agentruntime.UserInput{Text: "first"}},
		{ID: "two", Input: agentruntime.UserInput{Text: "second"}},
		{ID: "three", Input: agentruntime.UserInput{Text: "third"}},
	}
	var first agentruntime.Receipt
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
	if _, err := harness.Observe(context.Background(), 0); !errors.Is(err, agentruntime.ErrCursorExpired) {
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

func waitForRetainedTestSettlement(t *testing.T, ctx context.Context, observation agentruntime.Observation, operationID agentruntime.OperationID) {
	t.Helper()
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("event stream closed before settlement")
			}
			settled, ok := event.Payload.(agentruntime.OperationSettledEvent)
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

	store := agentruntime.NewMemoryJournalStore()
	ref := agentruntime.BindingRef{
		Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting,
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
	identity := agentruntime.DomainCommitIdentity{
		CommandID: "start", OperationID: "operation", Cycle: 1, Stage: agentruntime.DomainCommitOutput,
	}
	if _, err := journal.Append(context.Background(), 0, []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: "operation", Fingerprint: "seed"},
		agentruntime.OperationStartedEvent{OperationID: "operation"},
		agentruntime.UserMessageCommittedEvent{Message: agentruntime.Message{
			ID: "user", Role: agentruntime.RoleUser, Content: "write",
			Input: agentruntime.UserInput{Text: "write"}, Operation: "operation",
		}},
		agentruntime.CycleStartedEvent{OperationID: "operation", Cycle: 1, SnapshotID: "snapshot"},
		agentruntime.DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "output-hash"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	var engines atomic.Int32
	runtime, err := agentruntime.NewRuntime(agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
		engines.Add(1)
		return agentruntime.NewScriptedEngine(), nil
	}), store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := runtime.Project(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "unfinished-projection"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseIdle || status.ActiveOperation != "" || !status.RecoveryPending || status.LastOperation == nil || status.LastOperation.Status != agentruntime.OperationInterrupted {
		t.Fatalf("conservative projection = %#v", status)
	}
	if !strings.Contains(status.LastOperation.Reason, "recovery_pending") || len(status.DomainCommits) != 1 || status.DomainCommits[0].Identity.Stage != agentruntime.DomainCommitOutput {
		t.Fatalf("recovery evidence = %#v", status)
	}
	if got := engines.Load(); got != 0 {
		t.Fatalf("Project created %d Engines", got)
	}
}

func TestFileJournalStoresToolPayloadDescriptorsInsteadOfRawContent(t *testing.T) {
	root := t.TempDir()
	store, err := agentruntime.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	const argumentSecret = "argument-secret-must-not-be-journaled"
	const resultSecret = "result-secret-must-not-be-journaled"
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{Events: []agentruntime.EngineEvent{
		agentruntime.EngineToolStarted{CallID: "call", Name: "write_file", Arguments: json.RawMessage(`{"secret":"` + argumentSecret + `"}`)},
		agentruntime.EngineToolFinished{CallID: "call", Name: "write_file", Result: resultSecret, RetrySafety: agentruntime.RetryUnsafe},
		agentruntime.EngineAssistantFinal{Content: "done"},
	}})
	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "payload-descriptor"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
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
