package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"encoding/json"
	"errors"
	"testing"

	filejournal "github.com/alfredxw/denova/agent/runtime/filejournal"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestInitialStartRejectsMissingCallerCommandID(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	accepted, err := startCycle(service,
		context.Background(), nil, nil, nil, agentchat.ChatRequest{Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "missing-command"}, nil,
	)
	if !errors.Is(err, runstate.ErrInvalidCommand) || accepted != nil {
		t.Fatalf("missing command id = accepted=%v err=%v", accepted, err)
	}
}

func TestInitialStartColdReplayReturnsDurableOutcomeWithoutEngine(t *testing.T) {
	journalRoot := t.TempDir()
	firstStore, err := filejournal.NewStore(journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	firstService, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), firstStore)
	if err != nil {
		t.Fatal(err)
	}
	request := agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{CommandID: "initial-cold-replay", Message: "write"})
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "cold-replay-session",
		TaskID: "first-display-task", Mode: "ide",
	}
	firstOutcome := runCycle(firstService,
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("durable answer", nil)}, true),
		&runControlConversation{}, nil, request, options, nil,
	)
	if firstOutcome.Status != agentrun.OutcomeCompleted || firstOutcome.Content != "durable answer" {
		t.Fatalf("first outcome = %#v", firstOutcome)
	}
	if err := firstService.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	secondStore, err := filejournal.NewStore(journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	secondService, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), secondStore)
	if err != nil {
		t.Fatal(err)
	}
	defer secondService.Close(context.Background())
	var replayEvents []agentrun.Event
	replayOptions := options
	replayOptions.TaskID = "new-display-task-after-restart"
	accepted, err := startCycle(secondService,
		context.Background(), nil, nil, nil, request, replayOptions,
		func(event agentrun.Event) { replayEvents = append(replayEvents, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Receipt().Replayed || accepted.Receipt().CommandID != "initial-cold-replay" {
		t.Fatalf("cold receipt = %#v", accepted.Receipt())
	}
	outcome := accepted.Wait(context.Background())
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "durable answer" {
		t.Fatalf("cold replay outcome = %#v", outcome)
	}
	if got := countEventType(replayEvents, "chunk"); got != 1 {
		t.Fatalf("cold replay chunk count = %d, events=%#v", got, replayEvents)
	}
	if got := countEventType(replayEvents, "done"); got != 1 {
		t.Fatalf("cold replay done count = %d, events=%#v", got, replayEvents)
	}
}

func TestOlderSettledInitialStartColdReplayDoesNotWaitForFutureEvents(t *testing.T) {
	journalRoot := t.TempDir()
	store, err := filejournal.NewStore(journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), store)
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test",
		Workspace: "/book", SessionID: "older-cold-replay", Mode: "ide",
	}
	requests := []agentchat.ChatRequest{
		agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{CommandID: "older-start", Message: "first"}),
		agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{CommandID: "newer-start", Message: "second"}),
	}
	answers := []string{"first durable answer", "second durable answer"}
	for index := range requests {
		outcome := runCycle(service,
			context.Background(),
			newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage(answers[index], nil)}, true),
			&runControlConversation{}, nil, requests[index], options, nil,
		)
		if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != answers[index] {
			t.Fatalf("run %d outcome = %#v", index, outcome)
		}
	}
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := filejournal.NewStore(journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), reopenedStore)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(context.Background())
	accepted, err := startCycle(reopened, context.Background(), nil, nil, nil, requests[0], options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Receipt().Replayed {
		t.Fatalf("older receipt = %#v", accepted.Receipt())
	}
	outcome := accepted.Wait(context.Background())
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != answers[0] {
		t.Fatalf("older cold replay outcome = %#v", outcome)
	}
}

func TestInterruptedInitialStartColdReplayRequiresExplicitRecoveryWithoutEngine(t *testing.T) {
	store := runstate.NewMemoryJournalStore()
	request := agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{CommandID: "interrupted-start", Message: "write"})
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test",
		Workspace: "/book", SessionID: "interrupted-cold-replay", Mode: "ide",
	}.Normalize("")
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := newStartTurn(binding, "interrupted-start", request, options)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	key, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	operationID := runstate.OperationID("interrupted-operation")
	_, err = journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: command.ID, CommandKind: "start_turn", OperationID: operationID, Fingerprint: commandSemanticFingerprint(command)},
		runstate.OperationStartedEvent{OperationID: operationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "interrupted-user", Role: runstate.RoleUser, Content: command.Input.Text,
			Input: command.Input, Operation: operationID,
		}},
		runstate.CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "interrupted-snapshot"},
	})
	if err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), store)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	accepted, err := startCycle(service, context.Background(), nil, nil, nil, request, options, nil)
	if !errors.Is(err, ErrRecoveryRequired) || accepted != nil {
		t.Fatalf("interrupted cold replay = accepted=%v err=%v, want recovery required", accepted, err)
	}
	status, err := service.RuntimeRecoveryStatusProjection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseRunning || !status.RecoveryPaused || status.ActiveOperation != agentrun.OperationID(operationID) {
		t.Fatalf("interrupted cold projection = %#v", status)
	}
}
