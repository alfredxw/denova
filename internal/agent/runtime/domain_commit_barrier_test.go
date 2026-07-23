package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	runstate "denova/internal/agent/runtime"
)

func TestInputCommitReceiptStillAllowsAbort(t *testing.T) {
	t.Parallel()

	inputCommitted := make(chan struct{})
	engine := runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		return engineFunc(func(_ context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
			identity := domainIdentity(request, runstate.DomainCommitInput)
			if err := emit(runstate.EngineDomainCommitIntent{Identity: identity, Hash: "input-hash"}); err != nil {
				return runstate.EngineResult{}, err
			}
			if err := emit(runstate.EngineDomainCommitReceipt{Identity: identity, Hash: "input-hash", Revision: "session:1"}); err != nil {
				return runstate.EngineResult{}, err
			}
			close(inputCommitted)
			for control := range request.Controls {
				if control.Kind == runstate.EngineControlAbort {
					return runstate.EngineResult{Status: runstate.EngineAborted}, nil
				}
			}
			return runstate.EngineResult{Status: runstate.EngineAborted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "input-abort")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	<-inputCommitted
	abort, err := harness.Submit(context.Background(), runstate.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatalf("Abort after input receipt: %v", err)
	}
	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, abort.Cursor)
	if settled.Status != runstate.OperationAborted {
		t.Fatalf("status = %q, want aborted", settled.Status)
	}
}

func TestOutputIntentWinsOverLaterAbort(t *testing.T) {
	t.Parallel()

	authorized := make(chan struct{})
	receiptAllowed := make(chan struct{})
	engine := runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		return engineFunc(func(_ context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
			identity := domainIdentity(request, runstate.DomainCommitOutput)
			if err := emit(runstate.EngineDomainCommitIntent{Identity: identity, Hash: "output-hash"}); err != nil {
				return runstate.EngineResult{}, err
			}
			close(authorized)
			<-receiptAllowed
			if err := emit(runstate.EngineDomainCommitReceipt{Identity: identity, Hash: "output-hash", Revision: "turn:9"}); err != nil {
				return runstate.EngineResult{}, err
			}
			if err := emit(runstate.EngineAssistantFinal{Content: "committed"}); err != nil {
				return runstate.EngineResult{}, err
			}
			return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "output-wins")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	<-authorized
	if _, err := harness.Submit(context.Background(), runstate.Abort{ID: "too-late", OperationID: started.OperationID}); !errors.Is(err, runstate.ErrDomainCommitRejected) {
		t.Fatalf("late Abort error = %v, want ErrDomainCommitRejected", err)
	}
	close(receiptAllowed)
	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, started.Cursor)
	if settled.Status != runstate.OperationSucceeded {
		t.Fatalf("status = %q, want succeeded", settled.Status)
	}
}

func TestAcknowledgedOutputReceiptWinsOverLateEngineError(t *testing.T) {
	t.Parallel()

	lateErr := errors.New("display sink failed after canonical output was acknowledged")
	engine := runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		return engineFunc(func(_ context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
			identity := domainIdentity(request, runstate.DomainCommitOutput)
			if err := emit(runstate.EngineDomainCommitIntent{Identity: identity, Hash: "output-hash"}); err != nil {
				return runstate.EngineResult{}, err
			}
			if err := emit(runstate.EngineDomainCommitReceipt{Identity: identity, Hash: "output-hash", Revision: "turn:10"}); err != nil {
				return runstate.EngineResult{}, err
			}
			if err := emit(runstate.EngineAssistantFinal{Content: "canonical output"}); err != nil {
				return runstate.EngineResult{}, err
			}
			return runstate.EngineResult{}, lateErr
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "receipt-wins-late-error")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, started.Cursor)
	if settled.Status != runstate.OperationSucceeded {
		t.Fatalf("status = %q, want succeeded", settled.Status)
	}
	if !strings.Contains(settled.Reason, lateErr.Error()) {
		t.Fatalf("late adapter warning was not retained: %#v", settled)
	}
}

func TestAbortBeforeOutputIntentRejectsCanonicalAdmission(t *testing.T) {
	t.Parallel()

	tryCommit := make(chan struct{})
	commitResult := make(chan error, 1)
	engine := runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		return engineFunc(func(_ context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
			<-tryCommit
			err := emit(runstate.EngineDomainCommitIntent{Identity: domainIdentity(request, runstate.DomainCommitOutput), Hash: "output-hash"})
			commitResult <- err
			return runstate.EngineResult{Status: runstate.EngineAborted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "abort-wins")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	abort, err := harness.Submit(context.Background(), runstate.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	close(tryCommit)
	if err := <-commitResult; !errors.Is(err, runstate.ErrDomainCommitRejected) {
		t.Fatalf("output intent error = %v, want ErrDomainCommitRejected", err)
	}
	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, abort.Cursor)
	if settled.Status != runstate.OperationAborted {
		t.Fatalf("status = %q, want aborted", settled.Status)
	}
}

func TestAbortBeforeInputIntentStillAllowsUserProjection(t *testing.T) {
	t.Parallel()

	projectInput := make(chan struct{})
	inputResult := make(chan error, 1)
	engine := runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		return engineFunc(func(_ context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
			<-projectInput
			identity := domainIdentity(request, runstate.DomainCommitInput)
			if err := emit(runstate.EngineDomainCommitIntent{Identity: identity, Hash: "input-hash"}); err != nil {
				inputResult <- err
				return runstate.EngineResult{Status: runstate.EngineAborted}, nil
			}
			err := emit(runstate.EngineDomainCommitReceipt{Identity: identity, Hash: "input-hash", Revision: "session:1"})
			inputResult <- err
			return runstate.EngineResult{Status: runstate.EngineAborted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "abort-before-input")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	abort, err := harness.Submit(context.Background(), runstate.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	close(projectInput)
	if err := <-inputResult; err != nil {
		t.Fatalf("input projection after Abort: %v", err)
	}
	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, abort.Cursor)
	if settled.Status != runstate.OperationAborted {
		t.Fatalf("status = %q, want aborted", settled.Status)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.DomainCommits) != 1 || status.DomainCommits[0].Identity.Stage != runstate.DomainCommitInput || status.DomainCommits[0].Revision == "" {
		t.Fatalf("input receipt missing after Abort: %#v", status.DomainCommits)
	}
}

func TestDomainReceiptMustMatchAndPrecedeAssistantFinal(t *testing.T) {
	t.Parallel()

	engine := runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		return engineFunc(func(_ context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
			identity := domainIdentity(request, runstate.DomainCommitOutput)
			if err := emit(runstate.EngineDomainCommitIntent{Identity: identity, Hash: "expected"}); err != nil {
				return runstate.EngineResult{}, err
			}
			if err := emit(runstate.EngineDomainCommitReceipt{Identity: identity, Hash: "wrong", Revision: "turn:1"}); !errors.Is(err, runstate.ErrDomainCommitRejected) {
				return runstate.EngineResult{}, errors.New("mismatched receipt was accepted")
			}
			if err := emit(runstate.EngineAssistantFinal{Content: "must not settle"}); !errors.Is(err, runstate.ErrDomainCommitRejected) {
				return runstate.EngineResult{}, errors.New("assistant final before receipt was accepted")
			}
			return runstate.EngineResult{}, errors.New("domain receipt missing")
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "receipt-order")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, started.Cursor)
	if settled.Status != runstate.OperationFailed {
		t.Fatalf("status = %q, want failed", settled.Status)
	}
}

type engineFunc func(context.Context, runstate.EngineRequest, runstate.EngineEventSink) (runstate.EngineResult, error)

func (fn engineFunc) Run(ctx context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
	return fn(ctx, request, emit)
}

func domainIdentity(request runstate.EngineRequest, stage runstate.DomainCommitStage) runstate.DomainCommitIdentity {
	return runstate.DomainCommitIdentity{
		CommandID: request.Snapshot.CommandID, OperationID: request.Snapshot.OperationID,
		Cycle: request.Snapshot.Cycle, Stage: stage,
	}
}

func openBarrierHarness(t *testing.T, factory runstate.EngineFactory, sessionID string) (*runstate.Runtime, *runstate.Harness) {
	t.Helper()
	runtime, err := runstate.NewRuntime(factory, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, harness
}
