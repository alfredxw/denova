package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	adk "github.com/alfredxw/denova/adk"

	runstate "denova/internal/agent/runtime"
)

func TestChatServiceRejectsOversizedCommandBeforeOpeningBinding(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	limits := runstate.DefaultInputLimits()
	_, err = service.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandStartTurn, CommandID: strings.Repeat("x", limits.MaxCommandIDBytes+1),
		Request: ChatRequest{Message: "must not be fingerprinted or registered"},
	})
	if !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized command error = %v", err)
	}
}

func TestChatServiceStartRejectsOversizedCommandBeforeOpeningBinding(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	limits := runstate.DefaultInputLimits()
	_, err = service.StartWithOptions(context.Background(), nil, nil, nil, ChatRequest{
		CommandID: strings.Repeat("x", limits.MaxCommandIDBytes+1), Message: "must not open a binding",
	}, RunOptions{}, nil)
	if !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized start command error = %v", err)
	}
}

func TestChatServiceSubmitSteerTargetsActiveOperation(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("draft", "thought")
	firstDone := make(chan RunOutcome, 1)
	var eventMu sync.Mutex
	var events []Event
	emit := func(event Event) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	}
	runOutcomeTestGoroutine(firstDone, "steer root run", func() RunOutcome {
		return service.RunWithOptions(
			context.Background(), newRunControlTwoPhaseRunner(t, model), &runControlConversation{}, nil,
			ChatRequest{CommandID: "steer-root", Message: "first"},
			RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "commands"}, emit,
		)
	})
	waitHarnessEngineSignal(t, model.blocked, "active model safe point")

	binding, err := harnessBindingForOptions(RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "commands"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.harness.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	active, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandSteer, CommandID: "steer-1",
		OperationID:  active.Snapshot.ActiveOperation,
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("steered answer", nil)}, true),
		Conversation: &runControlConversation{}, Request: ChatRequest{Message: "change direction"},
		Options: RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "commands"},
		Emit:    emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID != active.Snapshot.ActiveOperation {
		t.Fatalf("steer operation = %q, want %q", receipt.OperationID, active.Snapshot.ActiveOperation)
	}
	close(model.release)

	select {
	case outcome := <-firstDone:
		if outcome.Status != RunOutcomeCompleted || outcome.Content != "steered answer" {
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

func TestChatServiceAcceptedRunFollowsNextTurnAndRemainsControllable(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	parentModel := newRunControlTwoPhaseModel("parent answer", "parent thought")
	successorModel := newRunControlTwoPhaseModel("successor answer", "successor thought")
	var eventMu sync.Mutex
	var events []Event
	emit := func(event Event) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	}
	done := make(chan RunOutcome, 1)
	runOutcomeTestGoroutine(done, "next-turn root run", func() RunOutcome {
		return service.RunWithOptions(
			context.Background(), newRunControlTwoPhaseRunner(t, parentModel), &runControlConversation{}, nil,
			ChatRequest{CommandID: "next-turn-root", Message: "parent"},
			RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"}, emit,
		)
	})
	waitHarnessEngineSignal(t, parentModel.blocked, "parent model safe point")

	binding, err := harnessBindingForOptions(RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.harness.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	active, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nextReceipt, err := service.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandNextTurn, CommandID: "accepted-next-turn", AfterOperationID: active.ActiveOperation,
		Runner: newRunControlTwoPhaseRunner(t, successorModel), Conversation: &runControlConversation{},
		Request: ChatRequest{Message: "successor"}, Emit: emit,
		Options: RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"},
	})
	if err != nil {
		t.Fatal(err)
	}
	close(parentModel.release)
	waitHarnessEngineSignal(t, successorModel.blocked, "successor model safe point")
	select {
	case outcome := <-done:
		t.Fatalf("display run finished before accepted successor: %#v", outcome)
	default:
	}

	followUpReceipt, err := service.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandFollowUp, CommandID: "successor-follow-up", OperationID: nextReceipt.OperationID,
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("follow-up after successor", nil)}, true),
		Conversation: &runControlConversation{}, Request: ChatRequest{Message: "continue successor"}, Emit: emit,
		Options: RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "next-turn-control"},
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
		if outcome.Status != RunOutcomeCompleted {
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

func TestChatServiceCommitsInitialAndFollowUpCyclesExactlyOnce(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("initial answer", "initial thought")
	initial := &harnessCommitConversation{runControlConversation: &runControlConversation{}}
	followUp := &harnessCommitConversation{runControlConversation: &runControlConversation{}}
	var cycleEventsMu sync.Mutex
	var cycleEvents []Event
	emit := func(event Event) {
		if event.Type != "agent_cycle_started" {
			return
		}
		cycleEventsMu.Lock()
		cycleEvents = append(cycleEvents, event)
		cycleEventsMu.Unlock()
	}
	done := make(chan RunOutcome, 1)
	runOutcomeTestGoroutine(done, "follow-up root run", func() RunOutcome {
		return service.RunWithOptions(
			context.Background(), newRunControlTwoPhaseRunner(t, model), initial, nil,
			ChatRequest{CommandID: "follow-up-root", Message: "initial"},
			RunOptions{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"}, emit,
		)
	})
	waitHarnessEngineSignal(t, model.blocked, "initial model safe point")

	binding, err := harnessBindingForOptions(RunOptions{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.harness.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	active, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	followUpSpec := AgentCommandSpec{
		Kind: AgentCommandFollowUp, CommandID: "follow-up-1", OperationID: active.Snapshot.ActiveOperation,
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("follow-up answer", nil)}, true),
		Conversation: followUp, Request: ChatRequest{Message: "continue"},
		Options: RunOptions{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"},
		Emit:    emit,
	}
	firstReceipt, err := service.SubmitCommand(context.Background(), followUpSpec)
	if err != nil {
		t.Fatal(err)
	}
	queuedRetryConversation := &harnessCommitConversation{runControlConversation: &runControlConversation{}}
	queuedRetry := followUpSpec
	queuedRetry.Runner = newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("must not run", nil)}, true)
	queuedRetry.Conversation = queuedRetryConversation
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
		if outcome.Status != RunOutcomeCompleted {
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
		wantDelivery, wantMessage := string(AgentCommandStartTurn), "initial"
		if index == 1 {
			wantDelivery, wantMessage = string(AgentCommandFollowUp), "continue"
		}
		if data["delivery"] != wantDelivery || data["message"] != wantMessage {
			t.Fatalf("cycle-start payload[%d] = %#v, want delivery=%q message=%q", index, data, wantDelivery, wantMessage)
		}
	}
	cycleEventsMu.Unlock()
	consumedRetryConversation := &harnessCommitConversation{runControlConversation: &runControlConversation{}}
	consumedRetry := queuedRetry
	consumedRetry.Conversation = consumedRetryConversation
	consumedReceipt, err := service.SubmitCommand(context.Background(), consumedRetry)
	if err != nil {
		t.Fatalf("retry consumed follow-up: %v", err)
	}
	if !consumedReceipt.Replayed || consumedReceipt.Cursor != firstReceipt.Cursor {
		t.Fatalf("consumed retry receipt = %#v, first = %#v", consumedReceipt, firstReceipt)
	}
	turnRef := harnessCommandTurnRef(binding, followUpSpec.CommandID, harnessTurnSpecSemanticFingerprint(followUpSpec))
	if _, err := service.harness.engine.take(turnRef); !errors.Is(err, ErrHarnessTurnSpecNotFound) {
		t.Fatalf("consumed retry leaked turn spec %q: %v", turnRef, err)
	}
}

func TestChatServiceSteerPreemptsBlockingPreparationBeforeModelEffects(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	options := RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "prepare-steer"}
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.harness.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	initial := newBlockingPreparationConversation()
	startReceipt, err := service.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandStartTurn, CommandID: "prepare-start",
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("must not run", nil)}, true),
		Conversation: initial, Request: ChatRequest{Message: "initial"}, Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitHarnessEngineSignal(t, initial.started, "initial cycle preparation")

	steered := &runControlConversation{}
	steerReceipt, err := service.SubmitCommand(context.Background(), AgentCommandSpec{
		Kind: AgentCommandSteer, CommandID: "prepare-steer", OperationID: startReceipt.OperationID,
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("steered answer", nil)}, true),
		Conversation: steered, Request: ChatRequest{Message: "redirect"}, Options: options,
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
			if eventPayload, ok := event.Payload.(runstate.OperationSettledEvent); ok && eventPayload.OperationID == startReceipt.OperationID {
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

func TestChatHarnessCloseClearsDurablyAcceptedTurnSpec(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	command := runstate.StartTurn{
		ID:    "accepted-before-close",
		Input: runstate.UserInput{Text: "queued", TurnSpecRef: "accepted-before-close"},
	}
	lease, err := service.harness.engine.register(
		"accepted-before-close",
		command,
		HarnessTurnSpec{Request: ChatRequest{Message: "queued"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	lease.accept()
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.harness.engine.take("accepted-before-close"); !errors.Is(err, ErrHarnessTurnSpecNotFound) {
		t.Fatalf("closed harness retained accepted spec: %v", err)
	}
}

func TestHarnessTurnSpecSemanticFingerprintUsesCallerPayloadOnly(t *testing.T) {
	first := AgentCommandSpec{Request: ChatRequest{
		Message: "draw", ImagePresetID: "preset-1",
		ImagePreset: ImagePresetContext{ID: "preset-1", AgentSystemPrompt: "resolved version one"},
	}, Options: RunOptions{
		AgentKind: AgentKindIDE, RootAgentName: "writer",
		Workspace: "/book", SessionID: "session-1", Mode: "writing",
	}}
	retry := first
	retry.Request.ImagePreset.AgentSystemPrompt = "resolved version two"
	if got, want := harnessTurnSpecSemanticFingerprint(retry), harnessTurnSpecSemanticFingerprint(first); got != want {
		t.Fatalf("server-resolved context changed retry fingerprint: got %q want %q", got, want)
	}
	retry.Request.ImagePresetID = "preset-2"
	if harnessTurnSpecSemanticFingerprint(retry) == harnessTurnSpecSemanticFingerprint(first) {
		t.Fatal("different caller payload reused the same turn fingerprint")
	}
	retry = first
	retry.Options.Mode = "review"
	if harnessTurnSpecSemanticFingerprint(retry) == harnessTurnSpecSemanticFingerprint(first) {
		t.Fatal("different stable run options reused the same turn fingerprint")
	}
}

func TestInitialStartTurnReferenceUsesCallerCommandAndFrozenPayload(t *testing.T) {
	request := CaptureChatRequestCallerInput(ChatRequest{
		CommandID: "start-command-1", Message: "write",
		WritingSkill: "", ImagePresetID: "", TellerID: "",
	})
	resolved := request
	resolved.WritingSkill = "resolved-skill"
	resolved.ImagePresetID = "resolved-preset"
	resolved.TellerID = "resolved-teller"
	firstOptions := RunOptions{
		AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "session-1",
		TaskID: "display-task-1", ReviewThreadID: "resolved-review-1",
		IdleTimeout: time.Second, ToolResultMaxBytes: 1024,
	}
	binding, err := harnessBindingForOptions(firstOptions)
	if err != nil {
		t.Fatal(err)
	}
	first, firstRef, err := newHarnessStartTurn(binding, "start-command-1", resolved, firstOptions)
	if err != nil {
		t.Fatal(err)
	}

	retryOptions := firstOptions
	retryOptions.TaskID = "display-task-after-restart"
	retryOptions.ReviewThreadID = ""
	retryOptions.IdleTimeout = 0
	retryOptions.ToolResultMaxBytes = 0
	retry, retryRef, err := newHarnessStartTurn(binding, "start-command-1", request, retryOptions)
	if err != nil {
		t.Fatal(err)
	}
	if firstRef != retryRef || first.Input.TurnSpecRef != retry.Input.TurnSpecRef {
		t.Fatalf("retry turn ref changed: first=%q retry=%q", firstRef, retryRef)
	}
	if first.ID != "start-command-1" || retry.ID != first.ID {
		t.Fatalf("caller command identity was replaced: first=%q retry=%q", first.ID, retry.ID)
	}

	different := CaptureChatRequestCallerInput(ChatRequest{
		CommandID: "start-command-1", Message: "write something else",
	})
	_, differentRef, err := newHarnessStartTurn(binding, "start-command-1", different, retryOptions)
	if err != nil {
		t.Fatal(err)
	}
	if differentRef == firstRef {
		t.Fatal("different caller payload reused the same deterministic turn reference")
	}
}

type harnessCommitConversation struct {
	*runControlConversation
	commits int
}

func (c *harnessCommitConversation) CommitAgentCycle(_ context.Context, outcome RunOutcome) error {
	if outcome.Status == RunOutcomeCompleted || outcome.Status == RunOutcomePreempted {
		c.commits++
	}
	return nil
}
