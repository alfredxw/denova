package agents

import (
	"context"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestDurableChatServiceRestoresAcceptedNextTurnAcrossReopen(t *testing.T) {
	dataDir := t.TempDir()
	first, err := NewDurableChatService(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	options := RunOptions{
		AgentKind: AgentKindIDE, RootAgentName: "restore-test",
		Workspace: "/book", SessionID: "restore-session", Mode: "writing",
	}
	blocking := newBlockingPreparationConversation()
	started, err := first.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandStartTurn, CommandID: "restore-start",
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("unused", nil)}, true),
		Conversation: blocking, Request: ChatRequest{Message: "first"}, Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitHarnessEngineSignal(t, blocking.started, "initial turn preparation")
	nextRequest := ChatRequest{Message: "second", Locale: "zh-CN"}
	next, err := first.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandNextTurn, CommandID: "restore-next", AfterOperationID: started.OperationID,
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("unused next", nil)}, true),
		Conversation: &runControlConversation{}, Request: nextRequest, Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	restoredRequest := make(chan HarnessTurnRestoreRequest, 1)
	restoredConversation := &runControlConversation{}
	second, err := NewDurableChatService(
		context.Background(),
		dataDir,
		WithHarnessTurnRestorer(func(_ context.Context, request HarnessTurnRestoreRequest) (HarnessTurnSpec, error) {
			restoredRequest <- request
			return HarnessTurnSpec{
				Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("restored answer", nil)}, true),
				Conversation: restoredConversation,
				Request:      request.Request,
				Options:      request.Options,
			}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close(context.Background())
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := second.harness.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || len(status.Queue) != 1 || status.Queue[0].CommandID != "restore-next" {
		t.Fatalf("reopen did not preserve accepted NextTurn: %#v", status)
	}
	select {
	case request := <-restoredRequest:
		t.Fatalf("reopen invoked host restorer before exact command replay: %#v", request)
	default:
	}
	replaySpec := AgentCommandSpec{
		Kind: AgentCommandNextTurn, CommandID: "restore-next", AfterOperationID: started.OperationID,
		Request: nextRequest, Options: options.normalized(""),
	}
	replayInput := runstate.UserInput{
		Text:        nextRequest.Message,
		ContextRefs: harnessContextRefs(nextRequest),
		TurnSpecRef: harnessCommandTurnRef(
			binding,
			replaySpec.CommandID,
			harnessTurnSpecSemanticFingerprint(replaySpec),
		),
	}
	replayInput, err = withHarnessInputMaterializationDescriptor(replayInput, replaySpec)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := harness.Submit(context.Background(), runstate.NextTurn{
		ID: "restore-next", AfterOperationID: runstate.OperationID(started.OperationID), Input: replayInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || OperationID(replayed.OperationID) != next.OperationID {
		t.Fatalf("exact replay receipt = %#v", replayed)
	}
	select {
	case request := <-restoredRequest:
		if request.CommandID != "restore-next" || request.OperationID != next.OperationID ||
			request.AfterOperationID != started.OperationID || request.Request.Message != nextRequest.Message ||
			request.Request.Locale != nextRequest.Locale {
			t.Fatalf("restored request = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("host restorer was not called")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	observation, err := harness.Observe(ctx, runstate.Cursor(next.Cursor))
	if err != nil {
		t.Fatal(err)
	}
	for {
		if observation.Snapshot.LastOperation != nil && OperationID(observation.Snapshot.LastOperation.OperationID) == next.OperationID {
			if observation.Snapshot.LastOperation.Status != runstate.OperationSucceeded {
				t.Fatalf("restored NextTurn status = %q", observation.Snapshot.LastOperation.Status)
			}
			break
		}
		select {
		case event := <-observation.Events:
			settled, ok := event.Payload.(runstate.OperationSettledEvent)
			if ok && OperationID(settled.OperationID) == next.OperationID {
				if settled.Status != runstate.OperationSucceeded {
					t.Fatalf("restored NextTurn status = %q", settled.Status)
				}
				goto settled
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatalf("restored NextTurn did not settle: %v", ctx.Err())
		}
	}

settled:
	return
}

func TestDecodeHarnessTurnRestoreRequestSupportsTransientQueueCommands(t *testing.T) {
	options := RunOptions{
		AgentKind: AgentKindIDE, RootAgentName: "restore-test",
		Workspace: "/book", SessionID: "restore-session", Mode: "writing",
	}.normalized("")
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		kind     AgentCommandKind
		delivery runstate.DeliveryKind
	}{
		{name: "steer", kind: AgentCommandSteer, delivery: runstate.DeliverySteer},
		{name: "follow-up", kind: AgentCommandFollowUp, delivery: runstate.DeliveryFollowUp},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := AgentCommandSpec{
				Kind: test.kind, CommandID: test.name + "-command", OperationID: "operation-1",
				Request: ChatRequest{Message: "continue", Locale: "en-US"}, Options: options,
				Prepare: func(context.Context) (HarnessTurnExecution, error) { return HarnessTurnExecution{}, nil },
			}
			input := runstate.UserInput{
				Text:        spec.Request.Message,
				ContextRefs: harnessContextRefs(spec.Request),
				TurnSpecRef: harnessCommandTurnRef(binding, spec.CommandID, harnessTurnSpecSemanticFingerprint(spec)),
			}
			input, err = withHarnessInputMaterializationDescriptor(input, spec)
			if err != nil {
				t.Fatal(err)
			}
			queued := runstate.QueuedInput{
				CommandID: runstate.CommandID(spec.CommandID), OperationID: runstate.OperationID(spec.OperationID),
				Delivery: test.delivery, Input: input,
			}
			request, err := decodeHarnessTurnRestoreRequest(bindingRef, queued)
			if err != nil {
				t.Fatal(err)
			}
			if request.Kind != test.kind || request.CommandID != CommandID(queued.CommandID) || request.OperationID != OperationID(queued.OperationID) || !request.Deferred {
				t.Fatalf("restore request = %#v", request)
			}
			command, err := restoredQueuedCommand(request, input)
			if err != nil {
				t.Fatal(err)
			}
			switch typed := command.(type) {
			case runstate.Steer:
				if test.kind != AgentCommandSteer || typed.OperationID != queued.OperationID {
					t.Fatalf("restored steer = %#v", typed)
				}
			case runstate.FollowUp:
				if test.kind != AgentCommandFollowUp || typed.OperationID != queued.OperationID {
					t.Fatalf("restored follow-up = %#v", typed)
				}
			default:
				t.Fatalf("restored command type = %T", command)
			}
		})
	}
}
