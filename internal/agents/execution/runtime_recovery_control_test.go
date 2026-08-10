package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"fmt"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestRecoveryWaitCancellationAbortsAcceptedSuccessorWithFreshCommandIdentity(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	parentModel := newRunControlTwoPhaseModel("parent answer", "parent thought")
	successorModel := newRunControlTwoPhaseModel("successor answer", "successor thought")
	defer releaseRecoveryControlModel(parentModel)
	defer releaseRecoveryControlModel(successorModel)

	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test",
		Workspace: "/book", SessionID: "recovery-cancel-successor",
	}
	accepted, err := startCycle(service,
		context.Background(), newRunControlTwoPhaseRunner(t, parentModel), &runControlConversation{}, nil,
		agentchat.ChatRequest{CommandID: "recovery-parent", Message: "parent"}, options, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	waitEngineSignal(t, parentModel.blocked, "recovery parent model safe point")

	next, err := service.SubmitCommand(context.Background(), CommandRequest{
		Kind: CommandNextTurn, CommandID: "recovery-successor", AfterOperationID: accepted.Receipt().OperationID,
		cycle:   &Cycle{Runner: newRunControlTwoPhaseRunner(t, successorModel), Conversation: &runControlConversation{}},
		Request: agentchat.ChatRequest{Message: "successor"}, Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}

	recovery, err := service.OpenRecoveryObservation(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	auditContext, stopAudit := context.WithCancel(context.Background())
	defer stopAudit()
	audit, err := recovery.harness.ObserveFromNow(auditContext)
	if err != nil {
		t.Fatal(err)
	}

	waitContext, cancelWait := context.WithCancel(context.Background())
	waitDone := make(chan agentrun.Outcome, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				waitDone <- agentrun.NewOutcome(agentrun.OutcomeFailed, fmt.Errorf("recovery wait panic: %v", recovered), "recovery wait panic", "", "")
			}
		}()
		waitDone <- recovery.Wait(waitContext, nil)
	}()
	cancelWait()

	parentAbortID := waitForRecoveryAbortCommand(t, audit, runstate.OperationID(accepted.Receipt().OperationID))
	releaseRecoveryControlModel(parentModel)
	successorAbortID := waitForRecoveryAbortCommand(t, audit, runstate.OperationID(next.OperationID))
	if successorAbortID == parentAbortID {
		t.Fatalf("successor reused parent Abort command_id %q", successorAbortID)
	}
	releaseRecoveryControlModel(successorModel)

	select {
	case outcome := <-waitDone:
		if outcome.Status != agentrun.OutcomeAborted || outcome.Error != nil {
			t.Fatalf("recovery cancellation outcome = %#v", outcome)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("recovery cancellation did not settle the accepted successor")
	}
	if outcome := accepted.Wait(context.Background()); outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("original accepted run did not observe successor Abort: %#v", outcome)
	}
}

func TestRecoveryCancellationAbortIdentityReusesCommandForFailOnceRetry(t *testing.T) {
	var identity recoveryCancellationAbortIdentity
	first := identity.forOperation("operation-a")
	retry := identity.forOperation("operation-a")
	successor := identity.forOperation("operation-b")
	if first == "" || retry != first {
		t.Fatalf("same-operation retry identities = %q and %q", first, retry)
	}
	if successor == "" || successor == first {
		t.Fatalf("successor identity = %q, parent identity = %q", successor, first)
	}
}

func waitForRecoveryAbortCommand(
	t *testing.T,
	observation runstate.Observation,
	operationID runstate.OperationID,
) runstate.CommandID {
	t.Helper()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("recovery audit observation closed before Abort was accepted")
			}
			accepted, ok := event.Payload.(runstate.CommandAcceptedEvent)
			if ok && accepted.CommandKind == "abort" && accepted.OperationID == operationID {
				return accepted.CommandID
			}
		case err, ok := <-observation.Errors:
			if ok && err != nil {
				t.Fatalf("recovery audit observation: %v", err)
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for Abort on operation %s", operationID)
		}
	}
}

func releaseRecoveryControlModel(model *runControlTwoPhaseModel) {
	if model == nil {
		return
	}
	select {
	case <-model.release:
	default:
		close(model.release)
	}
}
