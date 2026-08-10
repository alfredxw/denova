package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestExecutionRuntimeRejectsOversizedCommandBeforeOpeningBinding(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	limits := runstate.DefaultInputLimits()
	_, err = service.SubmitCommand(context.Background(), CommandRequest{
		Kind: CommandFollowUp, CommandID: strings.Repeat("x", limits.MaxCommandIDBytes+1),
		Request: agentchat.ChatRequest{Message: "must not be fingerprinted or registered"},
	})
	if !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized command error = %v", err)
	}
}

func TestExecutionRuntimeStartRejectsOversizedCommandBeforeOpeningBinding(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	limits := runstate.DefaultInputLimits()
	_, err = startCycle(service, context.Background(), nil, nil, nil, agentchat.ChatRequest{
		CommandID: strings.Repeat("x", limits.MaxCommandIDBytes+1), Message: "must not open a binding",
	}, agentrun.Options{}, nil)
	if !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized start command error = %v", err)
	}
}

func TestExecutionRuntimeSubmitSteerTargetsActiveOperation(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("draft", "thought")
	firstDone := make(chan agentrun.Outcome, 1)
	var eventMu sync.Mutex
	var events []agentrun.Event
	emit := func(event agentrun.Event) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	}
	runOutcomeTestGoroutine(firstDone, "steer root run", func() agentrun.Outcome {
		return runCycle(service,
			context.Background(), newRunControlTwoPhaseRunner(t, model), &runControlConversation{}, nil,
			agentchat.ChatRequest{CommandID: "steer-root", Message: "first"},
			agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "commands"}, emit,
		)
	})
	waitEngineSignal(t, model.blocked, "active model safe point")

	binding, err := agentrun.BindingForOptions(agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "commands"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.coordinator.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	active, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.SubmitCommand(context.Background(), CommandRequest{
		Kind: CommandSteer, CommandID: "steer-1",
		OperationID: agentrun.OperationID(active.Snapshot.ActiveOperation),
		cycle: &Cycle{
			Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("steered answer", nil)}, true),
			Conversation: &runControlConversation{},
		},
		Request: agentchat.ChatRequest{Message: "change direction"},
		Options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "commands"},
		Emit:    emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID != agentrun.OperationID(active.Snapshot.ActiveOperation) {
		t.Fatalf("steer operation = %q, want %q", receipt.OperationID, active.Snapshot.ActiveOperation)
	}
	close(model.release)

	select {
	case outcome := <-firstDone:
		if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "steered answer" {
			t.Fatalf("operation outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("steered operation did not settle")
	}
	settled, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settled.Snapshot.Phase != runstate.PhaseIdle {
		t.Fatalf("phase = %q, want idle", settled.Snapshot.Phase)
	}
	if got := settled.Snapshot.Messages[len(settled.Snapshot.Messages)-1].Content; got != "steered answer" {
		t.Fatalf("final assistant = %q", got)
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	if got := countEventType(events, "done"); got != 1 {
		t.Fatalf("operation emitted %d done events, want one: %#v", got, events)
	}
}

func TestExecutionRuntimeOperationFollowsNextCycleAndRemainsControllable(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	parentModel := newRunControlTwoPhaseModel("parent answer", "parent thought")
	successorModel := newRunControlTwoPhaseModel("successor answer", "successor thought")
	var eventMu sync.Mutex
	var events []agentrun.Event
	emit := func(event agentrun.Event) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	}
	done := make(chan agentrun.Outcome, 1)
	runOutcomeTestGoroutine(done, "next-turn root run", func() agentrun.Outcome {
		return runCycle(service,
			context.Background(), newRunControlTwoPhaseRunner(t, parentModel), &runControlConversation{}, nil,
			agentchat.ChatRequest{CommandID: "next-turn-root", Message: "parent"},
			agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"}, emit,
		)
	})
	waitEngineSignal(t, parentModel.blocked, "parent model safe point")

	binding, err := agentrun.BindingForOptions(agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.coordinator.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	active, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nextReceipt, err := service.SubmitCommand(context.Background(), CommandRequest{
		Kind: CommandNextTurn, CommandID: "accepted-next-turn", AfterOperationID: agentrun.OperationID(active.ActiveOperation),
		cycle:   &Cycle{Runner: newRunControlTwoPhaseRunner(t, successorModel), Conversation: &runControlConversation{}},
		Request: agentchat.ChatRequest{Message: "successor"}, Emit: emit,
		Options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"},
	})
	if err != nil {
		t.Fatal(err)
	}
	close(parentModel.release)
	waitEngineSignal(t, successorModel.blocked, "successor model safe point")
	select {
	case outcome := <-done:
		t.Fatalf("display run finished before accepted successor: %#v", outcome)
	default:
	}

	followUpReceipt, err := service.SubmitCommand(context.Background(), CommandRequest{
		Kind: CommandFollowUp, CommandID: "successor-follow-up", OperationID: nextReceipt.OperationID,
		cycle: &Cycle{
			Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("follow-up after successor", nil)}, true),
			Conversation: &runControlConversation{},
		},
		Request: agentchat.ChatRequest{Message: "continue successor"}, Emit: emit,
		Options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if followUpReceipt.OperationID != nextReceipt.OperationID {
		t.Fatalf("follow-up operation=%q successor=%q", followUpReceipt.OperationID, nextReceipt.OperationID)
	}
	close(successorModel.release)
	select {
	case outcome := <-done:
		if outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("successor chain outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted successor chain did not settle")
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	streamedFollowUp := false
	for _, event := range events {
		if event.Type == "chunk" {
			switch data := event.Data.(type) {
			case map[string]string:
				streamedFollowUp = data["content"] == "follow-up after successor"
			case map[string]any:
				streamedFollowUp = fmt.Sprint(data["content"]) == "follow-up after successor"
			}
			if streamedFollowUp {
				break
			}
		}
	}
	if !streamedFollowUp {
		t.Fatalf("successor follow-up output was not streamed through the original display: %#v", events)
	}
	if got := countEventType(events, "done"); got != 1 {
		t.Fatalf("successor chain emitted %d done events, want one: %#v", got, events)
	}
}

func TestExecutionRuntimeCommitsInitialAndFollowUpCyclesExactlyOnce(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("initial answer", "initial thought")
	initial := &commitConversation{runControlConversation: &runControlConversation{}}
	followUp := &commitConversation{runControlConversation: &runControlConversation{}}
	var cycleEventsMu sync.Mutex
	var cycleEvents []agentrun.Event
	emit := func(event agentrun.Event) {
		if event.Type != "agent_cycle_started" {
			return
		}
		cycleEventsMu.Lock()
		cycleEvents = append(cycleEvents, event)
		cycleEventsMu.Unlock()
	}
	done := make(chan agentrun.Outcome, 1)
	runOutcomeTestGoroutine(done, "follow-up root run", func() agentrun.Outcome {
		return runCycle(service,
			context.Background(), newRunControlTwoPhaseRunner(t, model), initial, nil,
			agentchat.ChatRequest{CommandID: "follow-up-root", Message: "initial"},
			agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"}, emit,
		)
	})
	waitEngineSignal(t, model.blocked, "initial model safe point")

	binding, err := agentrun.BindingForOptions(agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.coordinator.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	active, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	followUpSpec := CommandRequest{
		Kind: CommandFollowUp, CommandID: "follow-up-1", OperationID: agentrun.OperationID(active.Snapshot.ActiveOperation),
		cycle: &Cycle{
			Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("follow-up answer", nil)}, true),
			Conversation: followUp,
		},
		Request: agentchat.ChatRequest{Message: "continue"},
		Options: agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"},
		Emit:    emit,
	}
	firstReceipt, err := service.SubmitCommand(context.Background(), followUpSpec)
	if err != nil {
		t.Fatal(err)
	}
	queuedRetryConversation := &commitConversation{runControlConversation: &runControlConversation{}}
	queuedRetry := followUpSpec
	queuedRetry.cycle = &Cycle{
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("must not run", nil)}, true),
		Conversation: queuedRetryConversation,
	}
	replayedReceipt, err := service.SubmitCommand(context.Background(), queuedRetry)
	if err != nil {
		t.Fatalf("retry queued follow-up: %v", err)
	}
	if !replayedReceipt.Replayed || replayedReceipt.CommandID != firstReceipt.CommandID || replayedReceipt.OperationID != firstReceipt.OperationID || replayedReceipt.Cursor != firstReceipt.Cursor {
		t.Fatalf("queued retry receipt = %#v, first = %#v", replayedReceipt, firstReceipt)
	}
	conflict := queuedRetry
	conflict.Request.Message = "different input"
	if _, err := service.SubmitCommand(context.Background(), conflict); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("same command id with different input error = %v", err)
	}
	hiddenConflict := queuedRetry
	hiddenConflict.Request.PlanMode = !queuedRetry.Request.PlanMode
	if _, err := service.SubmitCommand(context.Background(), hiddenConflict); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("same command id with different adapter payload error = %v", err)
	}
	close(model.release)

	select {
	case outcome := <-done:
		if outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("operation outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("follow-up operation did not settle")
	}
	if initial.commits != 1 || followUp.commits != 1 {
		t.Fatalf("cycle commits initial=%d follow_up=%d, want one each", initial.commits, followUp.commits)
	}
	if queuedRetryConversation.commits != 0 {
		t.Fatalf("queued retry executed %d cycles", queuedRetryConversation.commits)
	}
	cycleEventsMu.Lock()
	if len(cycleEvents) != 2 {
		t.Fatalf("cycle-start events = %#v, want one per initial/follow-up cycle", cycleEvents)
	}
	for index, event := range cycleEvents {
		data, ok := event.Data.(map[string]any)
		if !ok || data["operation_id"] != string(firstReceipt.OperationID) || data["cycle"] != index+1 {
			t.Fatalf("cycle-start event[%d] = %#v", index, event)
		}
		commandID, _ := data["command_id"].(string)
		if commandID == "" || (index == 1 && commandID != followUpSpec.CommandID) {
			t.Fatalf("cycle-start command[%d] = %q event=%#v", index, commandID, event)
		}
		wantDelivery, wantMessage := string(CommandStartTurn), "initial"
		if index == 1 {
			wantDelivery, wantMessage = string(CommandFollowUp), "continue"
		}
		if data["delivery"] != wantDelivery || data["message"] != wantMessage {
			t.Fatalf("cycle-start payload[%d] = %#v, want delivery=%q message=%q", index, data, wantDelivery, wantMessage)
		}
	}
	cycleEventsMu.Unlock()
	consumedRetryConversation := &commitConversation{runControlConversation: &runControlConversation{}}
	consumedRetry := queuedRetry
	consumedRetry.cycle = &Cycle{Runner: queuedRetry.cycle.Runner, Conversation: consumedRetryConversation}
	consumedReceipt, err := service.SubmitCommand(context.Background(), consumedRetry)
	if err != nil {
		t.Fatalf("retry consumed follow-up: %v", err)
	}
	if !consumedReceipt.Replayed || consumedReceipt.Cursor != firstReceipt.Cursor {
		t.Fatalf("consumed retry receipt = %#v, first = %#v", consumedReceipt, firstReceipt)
	}
	turnRef := commandCycleRef(binding, followUpSpec.CommandID, cycleSemanticFingerprint(followUpSpec))
	if _, err := service.coordinator.engine.take(turnRef); !errors.Is(err, ErrCycleSpecNotFound) {
		t.Fatalf("consumed retry leaked turn spec %q: %v", turnRef, err)
	}
}

func TestExecutionRuntimeSteerPreemptsBlockingPreparationBeforeModelEffects(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	options := agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "prepare-steer"}
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.coordinator.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	initial := newBlockingPreparationConversation()
	started, err := service.Start(context.Background(), StartRequest{Cycle: Cycle{
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("must not run", nil)}, true),
		Conversation: initial,
		Request:      agentchat.ChatRequest{CommandID: "prepare-start", Message: "initial"},
		Options:      options,
	}})
	if err != nil {
		t.Fatal(err)
	}
	startReceipt := started.Receipt()
	waitEngineSignal(t, initial.started, "initial cycle preparation")

	steered := &runControlConversation{}
	steerReceipt, err := service.SubmitCommand(context.Background(), CommandRequest{
		Kind: CommandSteer, CommandID: "prepare-steer", OperationID: startReceipt.OperationID,
		cycle: &Cycle{
			Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("steered answer", nil)}, true),
			Conversation: steered,
		},
		Request: agentchat.ChatRequest{Message: "redirect"}, Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	if steerReceipt.OperationID != startReceipt.OperationID {
		t.Fatalf("steer operation = %q, want %q", steerReceipt.OperationID, startReceipt.OperationID)
	}

	settled := false
	for !settled {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("observation closed before steered operation settled")
			}
			if eventPayload, ok := event.Payload.(runstate.OperationSettledEvent); ok && agentrun.OperationID(eventPayload.OperationID) == startReceipt.OperationID {
				settled = true
			}
		case observationErr := <-observation.Errors:
			if observationErr != nil {
				t.Fatal(observationErr)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("steer did not preempt blocking preparation")
		}
	}
	if initial.runtimeCalls != 0 {
		t.Fatalf("initial model runtime started %d times", initial.runtimeCalls)
	}
	if steered.assistant != "steered answer" {
		t.Fatalf("steered assistant = %q", steered.assistant)
	}
	snapshot, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Snapshot.Phase != runstate.PhaseIdle {
		t.Fatalf("phase = %q, want idle", snapshot.Snapshot.Phase)
	}
}

func TestChatExecutionCloseClearsDurablyAcceptedTurnSpec(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	command := runstate.StartTurn{
		ID:    "accepted-before-close",
		Input: runstate.UserInput{Text: "queued", TurnSpecRef: "accepted-before-close"},
	}
	lease, err := service.coordinator.engine.register(
		"accepted-before-close",
		command,
		cycleSpec{Request: agentchat.ChatRequest{Message: "queued"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	lease.accept()
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.coordinator.engine.take("accepted-before-close"); !errors.Is(err, ErrCycleSpecNotFound) {
		t.Fatalf("closed harness retained accepted spec: %v", err)
	}
}

func TestExecutionTurnSpecSemanticFingerprintUsesCallerPayloadOnly(t *testing.T) {
	first := CommandRequest{Request: agentchat.ChatRequest{
		Message: "draw", ImagePresetID: "preset-1",
		ImagePreset: agentchat.ImagePresetContext{ID: "preset-1", AgentSystemPrompt: "resolved version one"},
	}, Options: agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, RootAgentName: "writer",
		Workspace: "/book", SessionID: "session-1", Mode: "writing",
	}}
	retry := first
	retry.Request.ImagePreset.AgentSystemPrompt = "resolved version two"
	if got, want := cycleSemanticFingerprint(retry), cycleSemanticFingerprint(first); got != want {
		t.Fatalf("server-resolved context changed retry fingerprint: got %q want %q", got, want)
	}
	retry.Request.ImagePresetID = "preset-2"
	if cycleSemanticFingerprint(retry) == cycleSemanticFingerprint(first) {
		t.Fatal("different caller payload reused the same turn fingerprint")
	}
	retry = first
	retry.Options.Mode = "review"
	if cycleSemanticFingerprint(retry) == cycleSemanticFingerprint(first) {
		t.Fatal("different stable run options reused the same turn fingerprint")
	}
}

func TestInitialStartTurnReferenceUsesCallerCommandAndFrozenPayload(t *testing.T) {
	request := agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{
		CommandID: "start-command-1", Message: "write",
		WritingSkill: "", ImagePresetID: "", TellerID: "",
	})
	resolved := request
	resolved.WritingSkill = "resolved-skill"
	resolved.ImagePresetID = "resolved-preset"
	resolved.TellerID = "resolved-teller"
	firstOptions := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "session-1",
		TaskID: "display-task-1", ReviewThreadID: "resolved-review-1",
		IdleTimeout: time.Second, ToolResultMaxBytes: 1024,
	}
	binding, err := agentrun.BindingForOptions(firstOptions)
	if err != nil {
		t.Fatal(err)
	}
	first, firstRef, err := newStartTurn(binding, "start-command-1", resolved, firstOptions)
	if err != nil {
		t.Fatal(err)
	}

	retryOptions := firstOptions
	retryOptions.TaskID = "display-task-after-restart"
	retryOptions.ReviewThreadID = ""
	retryOptions.IdleTimeout = 0
	retryOptions.ToolResultMaxBytes = 0
	retry, retryRef, err := newStartTurn(binding, "start-command-1", request, retryOptions)
	if err != nil {
		t.Fatal(err)
	}
	if firstRef != retryRef || first.Input.TurnSpecRef != retry.Input.TurnSpecRef {
		t.Fatalf("retry turn ref changed: first=%q retry=%q", firstRef, retryRef)
	}
	if first.ID != "start-command-1" || retry.ID != first.ID {
		t.Fatalf("caller command identity was replaced: first=%q retry=%q", first.ID, retry.ID)
	}

	different := agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{
		CommandID: "start-command-1", Message: "write something else",
	})
	_, differentRef, err := newStartTurn(binding, "start-command-1", different, retryOptions)
	if err != nil {
		t.Fatal(err)
	}
	if differentRef == firstRef {
		t.Fatal("different caller payload reused the same deterministic turn reference")
	}
}

type commitConversation struct {
	*runControlConversation
	commits int
}

func (c *commitConversation) CommitAgentCycle(_ context.Context, outcome agentrun.Outcome) error {
	if outcome.Status == agentrun.OutcomeCompleted || outcome.Status == agentrun.OutcomePreempted {
		c.commits++
	}
	return nil
}
