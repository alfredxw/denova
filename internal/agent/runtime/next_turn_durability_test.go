package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	runstate "denova/internal/agent/runtime"
)

func TestAbortSettlesBeforeAcceptedNextTurnContinues(t *testing.T) {
	t.Parallel()

	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{WaitForControl: runstate.EngineControlAbort},
		runstate.EngineScript{Events: []runstate.EngineEvent{
			runstate.EngineAssistantFinal{Content: "next completed"},
		}},
	)
	runtime, harness := openNextTurnHarness(t, engine, "next-after-abort")
	defer runtime.Close(context.Background())

	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "first"},
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := harness.Submit(context.Background(), runstate.NextTurn{
		ID: "next", AfterOperationID: started.OperationID,
		Input: runstate.UserInput{Text: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.Abort{
		ID: "abort", OperationID: started.OperationID, Reason: "caller stopped first",
	}); err != nil {
		t.Fatal(err)
	}

	waitForOperationStatus(t, harness, started.OperationID, runstate.OperationAborted)
	waitForNextTurnSettlement(t, harness, next.OperationID, runstate.OperationSucceeded)
	assertNextTurnMessages(t, harness, []string{"first", "second", "next completed"})
}

func TestFailureSettlesBeforeAcceptedNextTurnContinues(t *testing.T) {
	t.Parallel()

	releaseFailure := make(chan struct{})
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{Continue: releaseFailure, Err: errors.New("first engine failed")},
		runstate.EngineScript{Events: []runstate.EngineEvent{
			runstate.EngineAssistantFinal{Content: "next recovered"},
		}},
	)
	runtime, harness := openNextTurnHarness(t, engine, "next-after-failure")
	defer runtime.Close(context.Background())

	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "first"},
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := harness.Submit(context.Background(), runstate.NextTurn{
		ID: "next", AfterOperationID: started.OperationID,
		Input: runstate.UserInput{Text: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	close(releaseFailure)

	waitForOperationStatus(t, harness, started.OperationID, runstate.OperationFailed)
	waitForNextTurnSettlement(t, harness, next.OperationID, runstate.OperationSucceeded)
	assertNextTurnMessages(t, harness, []string{"first", "second", "next recovered"})
}

func TestCrashRecoveryKeepsAcceptedNextTurnPendingUntilExactReplay(t *testing.T) {
	t.Parallel()

	store := runstate.NewMemoryJournalStore()
	binding := runstate.WritingBinding{Workspace: "/book", SessionID: "restore-next"}
	nextOperation := seedUnfinishedOperationWithNextTurn(t, store, binding)
	engine := &nextTurnRestoreEngine{
		restored: make(chan runstate.QueuedInput, 1),
		ran:      make(chan runstate.EngineRequest, 1),
	}
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case restored := <-engine.restored:
		t.Fatalf("recovery restored queued input without an explicit replay: %#v", restored)
	default:
	}
	select {
	case request := <-engine.ran:
		t.Fatalf("recovery ran queued input without an explicit replay: %#v", request)
	default:
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseRunning || !status.RecoveryPaused ||
		status.ActiveOperation != "unfinished-operation" || len(status.Queue) != 1 || status.Queue[0].OperationID != nextOperation {
		t.Fatalf("recovered pending NextTurn = %#v", status)
	}

	replayed, err := harness.Submit(context.Background(), seededNextTurnCommand())
	if err != nil {
		t.Fatalf("replay accepted NextTurn: %v", err)
	}
	if !replayed.Replayed || replayed.OperationID != nextOperation {
		t.Fatalf("replayed receipt = %#v", replayed)
	}
	select {
	case restored := <-engine.restored:
		if restored.OperationID != nextOperation || restored.Input.TurnSpecRef != "next-turn-spec" {
			t.Fatalf("restored queued input = %#v", restored)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted NextTurn was not restored after reopen")
	}
	select {
	case request := <-engine.ran:
		if request.Snapshot.OperationID != nextOperation || request.Snapshot.Input.Text != "second" {
			t.Fatalf("restored engine request = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("restored NextTurn did not run")
	}
	waitForNextTurnSettlement(t, harness, nextOperation, runstate.OperationSucceeded)
}

func TestCrashRecoveryKeepsNextTurnPendingWhenDependencyCannotRestore(t *testing.T) {
	t.Parallel()

	store := runstate.NewMemoryJournalStore()
	binding := runstate.WritingBinding{Workspace: "/book", SessionID: "pending-next"}
	nextOperation := seedUnfinishedOperationWithNextTurn(t, store, binding)
	engine := &nextTurnRestoreEngine{restoreErr: errors.New("host turn dependency unavailable")}
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}

	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseRunning || !status.RecoveryPaused || status.ActiveOperation != "unfinished-operation" || len(status.Queue) != 1 {
		t.Fatalf("pending restore projection = %#v", status)
	}
	pending := status.Queue[0]
	if pending.Delivery != runstate.DeliveryNextTurn || pending.OperationID != nextOperation || pending.CommandID != "next" {
		t.Fatalf("pending NextTurn = %#v", pending)
	}
	if _, err := harness.Submit(context.Background(), seededNextTurnCommand()); err == nil {
		t.Fatal("exact replay unexpectedly succeeded without its host dependency")
	}
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseRunning || !status.RecoveryPaused || len(status.Queue) != 0 || status.InputRecovery == nil ||
		status.InputRecovery.CommandID != "next" || status.InputRecovery.OperationID != nextOperation || status.InputRecovery.Delivery != runstate.DeliveryNextTurn {
		t.Fatalf("failed restore did not preserve the explicit recovery boundary: %#v", status)
	}
	if len(engine.requests()) != 0 {
		t.Fatal("runtime executed NextTurn without its required host dependency")
	}
}

func openNextTurnHarness(t *testing.T, engine runstate.EngineFactory, sessionID string) (*runstate.Runtime, *runstate.Harness) {
	t.Helper()
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: sessionID})
	if err != nil {
		runtime.Close(context.Background())
		t.Fatal(err)
	}
	return runtime, harness
}

func waitForNextTurnSettlement(t *testing.T, harness *runstate.Harness, operationID runstate.OperationID, want runstate.OperationStatus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	observation, err := harness.Observe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshot.LastOperation != nil && observation.Snapshot.LastOperation.OperationID == operationID {
		if observation.Snapshot.LastOperation.Status != want {
			t.Fatalf("operation status = %s, want %s", observation.Snapshot.LastOperation.Status, want)
		}
		return
	}
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("observation closed before NextTurn settled")
			}
			settled, ok := event.Payload.(runstate.OperationSettledEvent)
			if !ok || settled.OperationID != operationID {
				continue
			}
			if settled.Status != want {
				t.Fatalf("operation status = %s, want %s", settled.Status, want)
			}
			return
		case err := <-observation.Errors:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatalf("NextTurn %s did not settle: %v", operationID, ctx.Err())
		}
	}
}

func assertNextTurnMessages(t *testing.T, harness *runstate.Harness, want []string) {
	t.Helper()
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != len(want) {
		t.Fatalf("message texts = %#v, want %#v", got, want)
	} else {
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("message texts = %#v, want %#v", got, want)
			}
		}
	}
}

func seedUnfinishedOperationWithNextTurn(
	t *testing.T,
	store runstate.JournalStore,
	binding runstate.Binding,
) runstate.OperationID {
	t.Helper()
	ref, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	activeOperation := runstate.OperationID("unfinished-operation")
	nextOperation := runstate.OperationID("restored-next-operation")
	nextCommand := seededNextTurnCommand()
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: activeOperation, Fingerprint: "seed-start"},
		runstate.OperationStartedEvent{OperationID: activeOperation},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "first-user", Role: runstate.RoleUser, Content: "first",
			Input: runstate.UserInput{Text: "first", TurnSpecRef: "first-turn-spec"}, Operation: activeOperation,
		}},
		runstate.CycleStartedEvent{OperationID: activeOperation, Cycle: 1, SnapshotID: "first-snapshot"},
		runstate.CommandAcceptedEvent{CommandID: "next", CommandKind: "next_turn", OperationID: nextOperation, Fingerprint: testCommandFingerprint(nextCommand)},
		runstate.QueueEnqueuedEvent{Item: runstate.QueuedInput{
			CommandID: "next", OperationID: nextOperation, Delivery: runstate.DeliveryNextTurn,
			Input: runstate.UserInput{Text: "second", TurnSpecRef: "next-turn-spec"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	return nextOperation
}

func seededNextTurnCommand() runstate.NextTurn {
	return runstate.NextTurn{
		ID:               "next",
		AfterOperationID: "unfinished-operation",
		Input:            runstate.UserInput{Text: "second", TurnSpecRef: "next-turn-spec"},
	}
}

func testCommandFingerprint(command runstate.Command) string {
	fingerprint, err := runstate.CommandFingerprint(command)
	if err != nil {
		panic(err)
	}
	return fingerprint
}

type nextTurnRestoreEngine struct {
	mu         sync.Mutex
	restoreErr error
	restored   chan runstate.QueuedInput
	ran        chan runstate.EngineRequest
	seen       []runstate.EngineRequest
}

func (e *nextTurnRestoreEngine) NewEngine(context.Context, runstate.BindingRef) (runstate.Engine, error) {
	return e, nil
}

func (e *nextTurnRestoreEngine) RestorePendingInput(_ context.Context, input runstate.QueuedInput) error {
	if e.restoreErr != nil {
		return e.restoreErr
	}
	if e.restored != nil {
		e.restored <- input
	}
	return nil
}

func (e *nextTurnRestoreEngine) Run(_ context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
	e.mu.Lock()
	e.seen = append(e.seen, request)
	e.mu.Unlock()
	if e.ran != nil {
		e.ran <- request
	}
	if err := emit(runstate.EngineAssistantFinal{Content: "restored"}); err != nil {
		return runstate.EngineResult{}, err
	}
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func (e *nextTurnRestoreEngine) requests() []runstate.EngineRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]runstate.EngineRequest(nil), e.seen...)
}
