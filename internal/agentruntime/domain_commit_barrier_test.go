package agentruntime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"denova/internal/agentruntime"
)

func TestInputCommitReceiptStillAllowsAbort(t *testing.T) {
	t.Parallel()

	inputCommitted := make(chan struct{})
	engine := agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
		return engineFunc(func(_ context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
			identity := domainIdentity(request, agentruntime.DomainCommitInput)
			if err := emit(agentruntime.EngineDomainCommitIntent{Identity: identity, Hash: "input-hash"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			if err := emit(agentruntime.EngineDomainCommitReceipt{Identity: identity, Hash: "input-hash", Revision: "session:1"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			close(inputCommitted)
			for control := range request.Controls {
				if control.Kind == agentruntime.EngineControlAbort {
					return agentruntime.EngineResult{Status: agentruntime.EngineAborted}, nil
				}
			}
			return agentruntime.EngineResult{Status: agentruntime.EngineAborted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "input-abort")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	<-inputCommitted
	abort, err := harness.Submit(context.Background(), agentruntime.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatalf("Abort after input receipt: %v", err)
	}
	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, abort.Cursor)
	if settled.Status != agentruntime.OperationAborted {
		t.Fatalf("status = %q, want aborted", settled.Status)
	}
}

func TestOutputIntentWinsOverLaterAbort(t *testing.T) {
	t.Parallel()

	authorized := make(chan struct{})
	receiptAllowed := make(chan struct{})
	engine := agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
		return engineFunc(func(_ context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
			identity := domainIdentity(request, agentruntime.DomainCommitOutput)
			if err := emit(agentruntime.EngineDomainCommitIntent{Identity: identity, Hash: "output-hash"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			close(authorized)
			<-receiptAllowed
			if err := emit(agentruntime.EngineDomainCommitReceipt{Identity: identity, Hash: "output-hash", Revision: "turn:9"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			if err := emit(agentruntime.EngineAssistantFinal{Content: "committed"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			return agentruntime.EngineResult{Status: agentruntime.EngineCompleted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "output-wins")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	<-authorized
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{ID: "too-late", OperationID: started.OperationID}); !errors.Is(err, agentruntime.ErrDomainCommitRejected) {
		t.Fatalf("late Abort error = %v, want ErrDomainCommitRejected", err)
	}
	close(receiptAllowed)
	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, started.Cursor)
	if settled.Status != agentruntime.OperationSucceeded {
		t.Fatalf("status = %q, want succeeded", settled.Status)
	}
}

func TestAcknowledgedOutputReceiptWinsOverLateEngineError(t *testing.T) {
	t.Parallel()

	lateErr := errors.New("display sink failed after canonical output was acknowledged")
	engine := agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
		return engineFunc(func(_ context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
			identity := domainIdentity(request, agentruntime.DomainCommitOutput)
			if err := emit(agentruntime.EngineDomainCommitIntent{Identity: identity, Hash: "output-hash"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			if err := emit(agentruntime.EngineDomainCommitReceipt{Identity: identity, Hash: "output-hash", Revision: "turn:10"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			if err := emit(agentruntime.EngineAssistantFinal{Content: "canonical output"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			return agentruntime.EngineResult{}, lateErr
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "receipt-wins-late-error")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "start", Input: agentruntime.UserInput{Text: "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, started.Cursor)
	if settled.Status != agentruntime.OperationSucceeded {
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
	engine := agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
		return engineFunc(func(_ context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
			<-tryCommit
			err := emit(agentruntime.EngineDomainCommitIntent{Identity: domainIdentity(request, agentruntime.DomainCommitOutput), Hash: "output-hash"})
			commitResult <- err
			return agentruntime.EngineResult{Status: agentruntime.EngineAborted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "abort-wins")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	abort, err := harness.Submit(context.Background(), agentruntime.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	close(tryCommit)
	if err := <-commitResult; !errors.Is(err, agentruntime.ErrDomainCommitRejected) {
		t.Fatalf("output intent error = %v, want ErrDomainCommitRejected", err)
	}
	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, abort.Cursor)
	if settled.Status != agentruntime.OperationAborted {
		t.Fatalf("status = %q, want aborted", settled.Status)
	}
}

func TestAbortBeforeInputIntentStillAllowsUserProjection(t *testing.T) {
	t.Parallel()

	projectInput := make(chan struct{})
	inputResult := make(chan error, 1)
	engine := agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
		return engineFunc(func(_ context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
			<-projectInput
			identity := domainIdentity(request, agentruntime.DomainCommitInput)
			if err := emit(agentruntime.EngineDomainCommitIntent{Identity: identity, Hash: "input-hash"}); err != nil {
				inputResult <- err
				return agentruntime.EngineResult{Status: agentruntime.EngineAborted}, nil
			}
			err := emit(agentruntime.EngineDomainCommitReceipt{Identity: identity, Hash: "input-hash", Revision: "session:1"})
			inputResult <- err
			return agentruntime.EngineResult{Status: agentruntime.EngineAborted}, nil
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "abort-before-input")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	abort, err := harness.Submit(context.Background(), agentruntime.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	close(projectInput)
	if err := <-inputResult; err != nil {
		t.Fatalf("input projection after Abort: %v", err)
	}
	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, abort.Cursor)
	if settled.Status != agentruntime.OperationAborted {
		t.Fatalf("status = %q, want aborted", settled.Status)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.DomainCommits) != 1 || status.DomainCommits[0].Identity.Stage != agentruntime.DomainCommitInput || status.DomainCommits[0].Revision == "" {
		t.Fatalf("input receipt missing after Abort: %#v", status.DomainCommits)
	}
}

func TestDomainReceiptMustMatchAndPrecedeAssistantFinal(t *testing.T) {
	t.Parallel()

	engine := agentruntime.EngineFactoryFunc(func(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
		return engineFunc(func(_ context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
			identity := domainIdentity(request, agentruntime.DomainCommitOutput)
			if err := emit(agentruntime.EngineDomainCommitIntent{Identity: identity, Hash: "expected"}); err != nil {
				return agentruntime.EngineResult{}, err
			}
			if err := emit(agentruntime.EngineDomainCommitReceipt{Identity: identity, Hash: "wrong", Revision: "turn:1"}); !errors.Is(err, agentruntime.ErrDomainCommitRejected) {
				return agentruntime.EngineResult{}, errors.New("mismatched receipt was accepted")
			}
			if err := emit(agentruntime.EngineAssistantFinal{Content: "must not settle"}); !errors.Is(err, agentruntime.ErrDomainCommitRejected) {
				return agentruntime.EngineResult{}, errors.New("assistant final before receipt was accepted")
			}
			return agentruntime.EngineResult{}, errors.New("domain receipt missing")
		}), nil
	})
	runtime, harness := openBarrierHarness(t, engine, "receipt-order")
	defer runtime.Close(context.Background())
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, started.Cursor)
	if settled.Status != agentruntime.OperationFailed {
		t.Fatalf("status = %q, want failed", settled.Status)
	}
}

type engineFunc func(context.Context, agentruntime.EngineRequest, agentruntime.EngineEventSink) (agentruntime.EngineResult, error)

func (fn engineFunc) Run(ctx context.Context, request agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	return fn(ctx, request, emit)
}

func domainIdentity(request agentruntime.EngineRequest, stage agentruntime.DomainCommitStage) agentruntime.DomainCommitIdentity {
	return agentruntime.DomainCommitIdentity{
		CommandID: request.Snapshot.CommandID, OperationID: request.Snapshot.OperationID,
		Cycle: request.Snapshot.Cycle, Stage: stage,
	}
}

func openBarrierHarness(t *testing.T, factory agentruntime.EngineFactory, sessionID string) (*agentruntime.Runtime, *agentruntime.Harness) {
	t.Helper()
	runtime, err := agentruntime.NewRuntime(factory, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, harness
}
