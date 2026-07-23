package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestAbortSettlesBeforeAcceptedNextTurnContinues(t *testing.T) {
	t.Parallel()

	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{WaitForControl: agentruntime.EngineControlAbort},
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{
			agentruntime.EngineAssistantFinal{Content: "next completed"},
		}},
	)
	runtime, harness := openNextTurnHarness(t, engine, "next-after-abort")
	defer runtime.Close(context.Background())

	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "start", Input: agentruntime.UserInput{Text: "first"},
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := harness.Submit(context.Background(), agentruntime.NextTurn{
		ID: "next", AfterOperationID: started.OperationID,
		Input: agentruntime.UserInput{Text: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "abort", OperationID: started.OperationID, Reason: "caller stopped first",
	}); err != nil {
		t.Fatal(err)
	}

	waitForOperationStatus(t, harness, started.OperationID, agentruntime.OperationAborted)
	waitForNextTurnSettlement(t, harness, next.OperationID, agentruntime.OperationSucceeded)
	assertNextTurnMessages(t, harness, []string{"first", "second", "next completed"})
}

func TestFailureSettlesBeforeAcceptedNextTurnContinues(t *testing.T) {
	t.Parallel()

	releaseFailure := make(chan struct{})
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{Continue: releaseFailure, Err: errors.New("first engine failed")},
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{
			agentruntime.EngineAssistantFinal{Content: "next recovered"},
		}},
	)
	runtime, harness := openNextTurnHarness(t, engine, "next-after-failure")
	defer runtime.Close(context.Background())

	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "start", Input: agentruntime.UserInput{Text: "first"},
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := harness.Submit(context.Background(), agentruntime.NextTurn{
		ID: "next", AfterOperationID: started.OperationID,
		Input: agentruntime.UserInput{Text: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	close(releaseFailure)

	waitForOperationStatus(t, harness, started.OperationID, agentruntime.OperationFailed)
	waitForNextTurnSettlement(t, harness, next.OperationID, agentruntime.OperationSucceeded)
	assertNextTurnMessages(t, harness, []string{"first", "second", "next recovered"})
}

func TestCrashRecoveryKeepsAcceptedNextTurnPendingUntilExactReplay(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "restore-next"}
	nextOperation := seedUnfinishedOperationWithNextTurn(t, store, binding)
	engine := &nextTurnRestoreEngine{
		restored: make(chan agentruntime.QueuedInput, 1),
		ran:      make(chan agentruntime.EngineRequest, 1),
	}
	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
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
	if status.Phase != agentruntime.PhaseRunning || !status.RecoveryPaused ||
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
	waitForNextTurnSettlement(t, harness, nextOperation, agentruntime.OperationSucceeded)
}

func TestCrashRecoveryKeepsNextTurnPendingWhenDependencyCannotRestore(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "pending-next"}
	nextOperation := seedUnfinishedOperationWithNextTurn(t, store, binding)
	engine := &nextTurnRestoreEngine{restoreErr: errors.New("host turn dependency unavailable")}
	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
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
	if status.Phase != agentruntime.PhaseRunning || !status.RecoveryPaused || status.ActiveOperation != "unfinished-operation" || len(status.Queue) != 1 {
		t.Fatalf("pending restore projection = %#v", status)
	}
	pending := status.Queue[0]
	if pending.Delivery != agentruntime.DeliveryNextTurn || pending.OperationID != nextOperation || pending.CommandID != "next" {
		t.Fatalf("pending NextTurn = %#v", pending)
	}
	if _, err := harness.Submit(context.Background(), seededNextTurnCommand()); err == nil {
		t.Fatal("exact replay unexpectedly succeeded without its host dependency")
	}
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseRunning || !status.RecoveryPaused || len(status.Queue) != 0 || status.InputRecovery == nil ||
		status.InputRecovery.CommandID != "next" || status.InputRecovery.OperationID != nextOperation || status.InputRecovery.Delivery != agentruntime.DeliveryNextTurn {
		t.Fatalf("failed restore did not preserve the explicit recovery boundary: %#v", status)
	}
	if len(engine.requests()) != 0 {
		t.Fatal("runtime executed NextTurn without its required host dependency")
	}
}

func openNextTurnHarness(t *testing.T, engine agentruntime.EngineFactory, sessionID string) (*agentruntime.Runtime, *agentruntime.Harness) {
	t.Helper()
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: sessionID})
	if err != nil {
		runtime.Close(context.Background())
		t.Fatal(err)
	}
	return runtime, harness
}

func waitForNextTurnSettlement(t *testing.T, harness *agentruntime.Harness, operationID agentruntime.OperationID, want agentruntime.OperationStatus) {
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
			settled, ok := event.Payload.(agentruntime.OperationSettledEvent)
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

func assertNextTurnMessages(t *testing.T, harness *agentruntime.Harness, want []string) {
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
	store agentruntime.JournalStore,
	binding agentruntime.Binding,
) agentruntime.OperationID {
	t.Helper()
	ref, err := agentruntime.BindingReference(binding)
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
	activeOperation := agentruntime.OperationID("unfinished-operation")
	nextOperation := agentruntime.OperationID("restored-next-operation")
	nextCommand := seededNextTurnCommand()
	if _, err := journal.Append(context.Background(), 0, []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: activeOperation, Fingerprint: "seed-start"},
		agentruntime.OperationStartedEvent{OperationID: activeOperation},
		agentruntime.UserMessageCommittedEvent{Message: agentruntime.Message{
			ID: "first-user", Role: agentruntime.RoleUser, Content: "first",
			Input: agentruntime.UserInput{Text: "first", TurnSpecRef: "first-turn-spec"}, Operation: activeOperation,
		}},
		agentruntime.CycleStartedEvent{OperationID: activeOperation, Cycle: 1, SnapshotID: "first-snapshot"},
		agentruntime.CommandAcceptedEvent{CommandID: "next", CommandKind: "next_turn", OperationID: nextOperation, Fingerprint: testCommandFingerprint(nextCommand)},
		agentruntime.QueueEnqueuedEvent{Item: agentruntime.QueuedInput{
			CommandID: "next", OperationID: nextOperation, Delivery: agentruntime.DeliveryNextTurn,
			Input: agentruntime.UserInput{Text: "second", TurnSpecRef: "next-turn-spec"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	return nextOperation
}

func seededNextTurnCommand() agentruntime.NextTurn {
	return agentruntime.NextTurn{
		ID:               "next",
		AfterOperationID: "unfinished-operation",
		Input:            agentruntime.UserInput{Text: "second", TurnSpecRef: "next-turn-spec"},
	}
}

func testCommandFingerprint(command agentruntime.Command) string {
	fingerprint, err := agentruntime.CommandFingerprint(command)
	if err != nil {
		panic(err)
	}
	return fingerprint
}

type nextTurnRestoreEngine struct {
	mu         sync.Mutex
	restoreErr error
	restored   chan agentruntime.QueuedInput
	ran        chan agentruntime.EngineRequest
	seen       []agentruntime.EngineRequest
}

func (e *nextTurnRestoreEngine) NewEngine(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
	return e, nil
}

func (e *nextTurnRestoreEngine) RestorePendingInput(_ context.Context, input agentruntime.QueuedInput) error {
	if e.restoreErr != nil {
		return e.restoreErr
	}
	if e.restored != nil {
		e.restored <- input
	}
	return nil
}

func (e *nextTurnRestoreEngine) Run(_ context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	e.mu.Lock()
	e.seen = append(e.seen, request)
	e.mu.Unlock()
	if e.ran != nil {
		e.ran <- request
	}
	if err := emit(agentruntime.EngineAssistantFinal{Content: "restored"}); err != nil {
		return agentruntime.EngineResult{}, err
	}
	return agentruntime.EngineResult{Status: agentruntime.EngineCompleted}, nil
}

func (e *nextTurnRestoreEngine) requests() []agentruntime.EngineRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]agentruntime.EngineRequest(nil), e.seen...)
}
