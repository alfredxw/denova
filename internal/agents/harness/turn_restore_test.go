package harness

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestDurableChatServiceRestoresAcceptedNextTurnAcrossReopen(t *testing.T) {
	dataDir := t.TempDir()
	first, err := NewDurableService(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, RootAgentName: "restore-test",
		Workspace: "/book", SessionID: "restore-session", Mode: "writing",
	}
	blocking := newBlockingPreparationConversation()
	started, err := first.SubmitCommand(context.Background(), CommandSpec{
		Kind: CommandStartTurn, CommandID: "restore-start",
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("unused", nil)}, true),
		Conversation: blocking, Request: agentchat.ChatRequest{Message: "first"}, Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitHarnessEngineSignal(t, blocking.started, "initial turn preparation")
	nextRequest := agentchat.ChatRequest{Message: "second", Locale: "zh-CN"}
	next, err := first.SubmitCommand(context.Background(), CommandSpec{
		Kind: CommandNextTurn, CommandID: "restore-next", AfterOperationID: started.OperationID,
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("unused next", nil)}, true),
		Conversation: &runControlConversation{}, Request: nextRequest, Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	restoredRequest := make(chan TurnRestoreRequest, 1)
	restoredConversation := &runControlConversation{}
	second, err := NewDurableService(
		context.Background(),
		dataDir,
		WithTurnRestorer(func(_ context.Context, request TurnRestoreRequest) (TurnSpec, error) {
			restoredRequest <- request
			return TurnSpec{
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
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := second.coordinator.runtime.Open(context.Background(), binding)
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
	replaySpec := CommandSpec{
		Kind: CommandNextTurn, CommandID: "restore-next", AfterOperationID: started.OperationID,
		Request: nextRequest, Options: options.Normalize(""),
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
	if !replayed.Replayed || agentrun.OperationID(replayed.OperationID) != next.OperationID {
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
		if observation.Snapshot.LastOperation != nil && agentrun.OperationID(observation.Snapshot.LastOperation.OperationID) == next.OperationID {
			if observation.Snapshot.LastOperation.Status != runstate.OperationSucceeded {
				t.Fatalf("restored NextTurn status = %q", observation.Snapshot.LastOperation.Status)
			}
			break
		}
		select {
		case event := <-observation.Events:
			settled, ok := event.Payload.(runstate.OperationSettledEvent)
			if ok && agentrun.OperationID(settled.OperationID) == next.OperationID {
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
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, RootAgentName: "restore-test",
		Workspace: "/book", SessionID: "restore-session", Mode: "writing",
	}.Normalize("")
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		kind     CommandKind
		delivery runstate.DeliveryKind
	}{
		{name: "steer", kind: CommandSteer, delivery: runstate.DeliverySteer},
		{name: "follow-up", kind: CommandFollowUp, delivery: runstate.DeliveryFollowUp},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := CommandSpec{
				Kind: test.kind, CommandID: test.name + "-command", OperationID: "operation-1",
				Request: agentchat.ChatRequest{Message: "continue", Locale: "en-US"}, Options: options,
				Prepare: func(context.Context) (TurnExecution, error) { return TurnExecution{}, nil },
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
			if request.Kind != test.kind || request.CommandID != agentrun.CommandID(queued.CommandID) || request.OperationID != agentrun.OperationID(queued.OperationID) || !request.Deferred {
				t.Fatalf("restore request = %#v", request)
			}
			command, err := restoredQueuedCommand(request, input)
			if err != nil {
				t.Fatal(err)
			}
			switch typed := command.(type) {
			case runstate.Steer:
				if test.kind != CommandSteer || typed.OperationID != queued.OperationID {
					t.Fatalf("restored steer = %#v", typed)
				}
			case runstate.FollowUp:
				if test.kind != CommandFollowUp || typed.OperationID != queued.OperationID {
					t.Fatalf("restored follow-up = %#v", typed)
				}
			default:
				t.Fatalf("restored command type = %T", command)
			}
		})
	}
}

func TestDecodeHarnessTurnRestoreRequestPreservesProjectIdentity(t *testing.T) {
	t.Parallel()

	options := agentrun.Options{
		AgentKind: agentrun.AgentKindGeneral, ProjectID: "project-general", StateRoot: "/state/project-general",
		Workspace: "/workspace/original", SessionID: "general-session", Mode: agentrun.ModeAgentChat,
	}.Normalize("")
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{
		Kind: CommandFollowUp, CommandID: "general-follow-up", OperationID: "general-operation",
		Request: agentchat.ChatRequest{Message: "continue"}, Options: options,
		Prepare: func(context.Context) (TurnExecution, error) { return TurnExecution{}, nil },
	}
	input := runstate.UserInput{
		Text: spec.Request.Message,
		TurnSpecRef: harnessCommandTurnRef(
			binding, spec.CommandID, harnessTurnSpecSemanticFingerprint(spec),
		),
	}
	input, err = withHarnessInputMaterializationDescriptor(input, spec)
	if err != nil {
		t.Fatal(err)
	}
	request, err := decodeHarnessTurnRestoreRequest(bindingRef, runstate.QueuedInput{
		CommandID: runstate.CommandID(spec.CommandID), OperationID: runstate.OperationID(spec.OperationID),
		Delivery: runstate.DeliveryFollowUp, Input: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Binding.ProjectID != options.ProjectID || request.Binding.Workspace != "" ||
		request.Options.ProjectID != options.ProjectID {
		t.Fatalf("restored General Project identity = binding %#v options %#v", request.Binding, request.Options)
	}
}
