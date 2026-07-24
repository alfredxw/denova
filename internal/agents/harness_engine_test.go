package agents

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestHarnessEngineRunCompletesAndConsumesTurnSpec(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	var legacyEvents []Event
	err := registerAcceptedHarnessTurn(engine, "turn-complete", HarnessTurnSpec{
		Runner: newRunControlTestRunner(t, &runControlFixedModel{message: &agent.Message{
			Role:             agent.Assistant,
			Content:          "finished answer",
			ReasoningContent: "final thought",
		}}, true),
		Conversation: &runControlConversation{},
		Request:      ChatRequest{Message: "write"},
		Options:      RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
		Emit:         func(event Event) { legacyEvents = append(legacyEvents, event) },
	})
	if err != nil {
		t.Fatalf("register turn spec: %v", err)
	}

	var events []runstate.EngineEvent
	result, err := runHarnessEngine(engine, context.Background(), harnessEngineRequest("turn-complete", nil), func(event runstate.EngineEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("run harness engine: %v", err)
	}
	if result.Status != runstate.EngineCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	var content, thinking strings.Builder
	var final runstate.EngineAssistantFinal
	for _, event := range events {
		switch event := event.(type) {
		case runstate.EngineAssistantDelta:
			content.WriteString(event.Delta)
		case runstate.EngineThinkingDelta:
			thinking.WriteString(event.Delta)
		case runstate.EngineAssistantFinal:
			final = event
		}
	}
	if content.String() != "finished answer" || thinking.String() != "final thought" {
		t.Fatalf("mapped deltas = content %q thinking %q", content.String(), thinking.String())
	}
	if final.Content != "finished answer" || final.Thinking != "final thought" {
		t.Fatalf("final event = %#v", final)
	}
	if countEventType(legacyEvents, "done") != 0 {
		t.Fatalf("cycle-level done must wait for durable operation settlement: %#v", legacyEvents)
	}

	_, err = runHarnessEngine(engine, context.Background(), harnessEngineRequest("turn-complete", nil), func(runstate.EngineEvent) error { return nil })
	if !errors.Is(err, ErrHarnessTurnSpecNotFound) {
		t.Fatalf("second run error = %v, want missing one-shot spec", err)
	}
}

func TestHarnessEngineFactoryRejectsMismatchedBindingBeforeConsumingTurnSpec(t *testing.T) {
	owner := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	if err := registerAcceptedHarnessTurn(owner, "turn-binding-mismatch", HarnessTurnSpec{
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("done", nil)}, true),
		Conversation: &runControlConversation{},
		Request:      ChatRequest{Message: "write"},
		Options:      testHarnessRunOptions(),
	}); err != nil {
		t.Fatal(err)
	}
	bound, err := owner.NewEngine(context.Background(), testHarnessBindingRef())
	if err != nil {
		t.Fatal(err)
	}

	mismatched := harnessEngineRequest("turn-binding-mismatch", nil)
	mismatched.Binding = mustRuntimeBinding(RuntimeBinding{AgentKind: AgentKindIDE, Workspace: "/test/workspace", SessionID: "another-session"})
	mismatched.Snapshot.Binding = mismatched.Binding
	if _, err := bound.Run(context.Background(), mismatched, func(runstate.EngineEvent) error { return nil }); !errors.Is(err, ErrHarnessBindingMismatch) {
		t.Fatalf("mismatched request error = %v, want ErrHarnessBindingMismatch", err)
	}

	if _, err := bound.Run(context.Background(), harnessEngineRequest("turn-binding-mismatch", nil), func(runstate.EngineEvent) error { return nil }); err != nil {
		t.Fatalf("matching request could not consume preserved turn spec: %v", err)
	}
}

func TestHarnessEngineRejectsTurnSpecWhoseProfileDoesNotMatchBinding(t *testing.T) {
	owner := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	options := testHarnessRunOptions()
	options.AgentKind = AgentKindConfigManager
	if err := registerAcceptedHarnessTurn(owner, "turn-profile-mismatch", HarnessTurnSpec{
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("must not run", nil)}, true),
		Conversation: &runControlConversation{},
		Request:      ChatRequest{Message: "write"},
		Options:      options,
	}); err != nil {
		t.Fatal(err)
	}
	bound, err := owner.NewEngine(context.Background(), testHarnessBindingRef())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Run(context.Background(), harnessEngineRequest("turn-profile-mismatch", nil), func(runstate.EngineEvent) error { return nil }); !errors.Is(err, ErrHarnessBindingMismatch) {
		t.Fatalf("profile mismatch error = %v, want ErrHarnessBindingMismatch", err)
	}
}

func TestHarnessEngineRejectsIncompleteExecutableBeforeModelEffects(t *testing.T) {
	owner := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	if err := registerAcceptedHarnessTurn(owner, "turn-incomplete", HarnessTurnSpec{
		Request: ChatRequest{Message: "write"},
		Options: testHarnessRunOptions(),
	}); err != nil {
		t.Fatal(err)
	}
	bound, err := owner.NewEngine(context.Background(), testHarnessBindingRef())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Run(context.Background(), harnessEngineRequest("turn-incomplete", nil), func(runstate.EngineEvent) error { return nil }); !errors.Is(err, ErrHarnessTurnSpecInvalid) {
		t.Fatalf("incomplete turn error = %v, want ErrHarnessTurnSpecInvalid", err)
	}
}

func TestHarnessEngineReportsLegacyOutcome(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	outcomes := make(chan RunOutcome, 1)
	if err := registerAcceptedHarnessTurn(engine, "turn-outcome", HarnessTurnSpec{
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("reported answer", nil)}, true),
		Conversation: &runControlConversation{},
		Request:      ChatRequest{Message: "write"},
		Options:      RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
		Outcome:      outcomes,
	}); err != nil {
		t.Fatalf("register turn spec: %v", err)
	}

	if _, err := runHarnessEngine(engine, context.Background(), harnessEngineRequest("turn-outcome", nil), func(runstate.EngineEvent) error { return nil }); err != nil {
		t.Fatalf("run harness engine: %v", err)
	}
	select {
	case outcome := <-outcomes:
		if outcome.Status != RunOutcomeCompleted || outcome.Content != "reported answer" {
			t.Fatalf("reported outcome = %#v", outcome)
		}
	default:
		t.Fatal("legacy outcome was not reported")
	}
}

func TestHarnessEngineMaterializesQueuedTurnAtExecutionTime(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	prepared := false
	if err := registerAcceptedHarnessTurn(engine, "turn-lazy", HarnessTurnSpec{
		Request: ChatRequest{Message: "queued request"},
		Options: RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
		Prepare: func(context.Context) (HarnessTurnExecution, error) {
			prepared = true
			return HarnessTurnExecution{
				Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("fresh context", nil)}, true),
				Conversation: &runControlConversation{},
				Request:      ChatRequest{Message: "queued request"},
				Options:      RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if prepared {
		t.Fatal("queued turn was prepared during durable admission")
	}

	var final runstate.EngineAssistantFinal
	_, err := runHarnessEngine(engine, context.Background(), harnessEngineRequest("turn-lazy", nil), func(event runstate.EngineEvent) error {
		if value, ok := event.(runstate.EngineAssistantFinal); ok {
			final = value
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared || final.Content != "fresh context" {
		t.Fatalf("prepared=%t final=%#v", prepared, final)
	}
}

func TestHarnessEngineReleasesCancelledPendingTurnSpec(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	if err := registerAcceptedHarnessTurn(engine, "queued-turn", HarnessTurnSpec{}); err != nil {
		t.Fatal(err)
	}
	engine.ReleasePendingInput(context.Background(), runstate.UserInput{TurnSpecRef: "queued-turn"})
	if _, err := engine.take("queued-turn"); !errors.Is(err, ErrHarnessTurnSpecNotFound) {
		t.Fatalf("released spec lookup error = %v, want ErrHarnessTurnSpecNotFound", err)
	}
}

func TestHarnessEngineBindsDurableCycleIdentityBeforeExecution(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	conversation := &identityBindingConversation{}
	if err := registerAcceptedHarnessTurn(engine, "turn-identity", HarnessTurnSpec{
		CommandID:    CommandID("command-1"),
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("done", nil)}, true),
		Conversation: conversation,
		Request:      ChatRequest{Message: "play"},
		Options:      RunOptions{AgentKind: AgentKindInteractiveStory, RootAgentName: "run-control-test"},
	}); err != nil {
		t.Fatal(err)
	}
	request := harnessEngineRequest("turn-identity", nil)
	request.Snapshot.OperationID = runstate.OperationID("operation-1")
	request.Snapshot.Cycle = 3
	if _, err := runHarnessEngine(engine, context.Background(), request, func(runstate.EngineEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	want := (HarnessCycleIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 3})
	if conversation.identity != want {
		t.Fatalf("bound identity = %#v, want %#v", conversation.identity, want)
	}
}

func TestHarnessEnginePreparesCycleAfterIdentityAndBeforeRuntime(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	conversation := &orderedCycleConversation{}
	if err := registerAcceptedHarnessTurn(engine, "turn-prepare-order", HarnessTurnSpec{
		CommandID:    CommandID("command-prepare"),
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("done", nil)}, true),
		Conversation: conversation,
		Request:      ChatRequest{Message: "play"},
		Options:      RunOptions{AgentKind: AgentKindInteractiveStory, RootAgentName: "run-control-test"},
	}); err != nil {
		t.Fatal(err)
	}
	request := harnessEngineRequest("turn-prepare-order", nil)
	request.Snapshot.OperationID = runstate.OperationID("operation-prepare")
	request.Snapshot.Cycle = 2
	if _, err := runHarnessEngine(engine, context.Background(), request, func(runstate.EngineEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	want := []string{"bind", "prepare", "runtime"}
	if len(conversation.order) < len(want) || strings.Join(conversation.order[:len(want)], ",") != strings.Join(want, ",") {
		t.Fatalf("cycle order = %#v, want prefix %#v", conversation.order, want)
	}
}

func TestHarnessEnginePreparationFailureStopsCycleBeforeRuntime(t *testing.T) {
	wantErr := errors.New("director outbox unavailable")
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	conversation := &orderedCycleConversation{prepareErr: wantErr}
	outcomes := make(chan RunOutcome, 1)
	if err := registerAcceptedHarnessTurn(engine, "turn-prepare-error", HarnessTurnSpec{
		CommandID:    CommandID("command-prepare-error"),
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("must not run", nil)}, true),
		Conversation: conversation,
		Request:      ChatRequest{Message: "play"},
		Options:      RunOptions{AgentKind: AgentKindInteractiveStory, RootAgentName: "run-control-test"},
		Outcome:      outcomes,
	}); err != nil {
		t.Fatal(err)
	}
	request := harnessEngineRequest("turn-prepare-error", nil)
	request.Snapshot.OperationID = runstate.OperationID("operation-prepare-error")
	request.Snapshot.Cycle = 1
	if _, err := runHarnessEngine(engine, context.Background(), request, func(runstate.EngineEvent) error { return nil }); !errors.Is(err, wantErr) {
		t.Fatalf("prepare error = %v, want %v", err, wantErr)
	}
	if got := strings.Join(conversation.order, ","); got != "bind,prepare" {
		t.Fatalf("cycle order = %q, want bind,prepare without runtime", got)
	}
	select {
	case outcome := <-outcomes:
		if outcome.Status != RunOutcomeFailed || !errors.Is(outcome.Error, wantErr) {
			t.Fatalf("preparation outcome = %#v", outcome)
		}
	default:
		t.Fatal("preparation failure did not report a terminal outcome")
	}
	if _, err := engine.take("turn-prepare-error"); !errors.Is(err, ErrHarnessTurnSpecNotFound) {
		t.Fatalf("failed preparation retained consumed spec: %v", err)
	}
}

func TestHarnessEngineControlInterruptsBlockingPreparation(t *testing.T) {
	tests := []struct {
		name        string
		control     runstate.EngineControlKind
		wantEngine  runstate.EngineStatus
		wantOutcome RunOutcomeStatus
	}{
		{
			name: "preempt", control: runstate.EngineControlPreempt,
			wantEngine: runstate.EnginePreempted, wantOutcome: RunOutcomePreempted,
		},
		{
			name: "abort", control: runstate.EngineControlAbort,
			wantEngine: runstate.EngineAborted, wantOutcome: RunOutcomeAborted,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
			conversation := newBlockingPreparationConversation()
			outcomes := make(chan RunOutcome, 1)
			ref := "turn-blocking-prepare-" + test.name
			if err := registerAcceptedHarnessTurn(engine, ref, HarnessTurnSpec{
				CommandID:    CommandID("command-" + test.name),
				Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("must not run", nil)}, true),
				Conversation: conversation,
				Request:      ChatRequest{Message: "play"},
				Options:      RunOptions{AgentKind: AgentKindInteractiveStory, RootAgentName: "run-control-test"},
				Outcome:      outcomes,
			}); err != nil {
				t.Fatal(err)
			}
			controls := make(chan runstate.EngineControl, 1)
			done := make(chan harnessEngineTestResult, 1)
			runHarnessEngineTestGoroutine(done, "controlled preparation run", func() (runstate.EngineResult, error) {
				return runHarnessEngine(engine, context.Background(), harnessEngineRequest(ref, controls), func(runstate.EngineEvent) error { return nil })
			})
			waitHarnessEngineSignal(t, conversation.started, "cycle preparation")
			controls <- runstate.EngineControl{Kind: test.control}

			select {
			case got := <-done:
				if got.err != nil || got.result.Status != test.wantEngine {
					t.Fatalf("controlled preparation result = %#v err=%v, want %q", got.result, got.err, test.wantEngine)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("control did not interrupt blocking preparation")
			}
			if conversation.runtimeCalls != 0 {
				t.Fatalf("legacy runtime started %d times after preparation control", conversation.runtimeCalls)
			}
			select {
			case outcome := <-outcomes:
				if outcome.Status != test.wantOutcome || outcome.Error != nil {
					t.Fatalf("controlled preparation outcome = %#v", outcome)
				}
			default:
				t.Fatal("controlled preparation did not report an outcome")
			}
		})
	}
}

func TestHarnessEngineCycleCommitRunsOnceBeforeSettlementAndFailsOperation(t *testing.T) {
	wantErr := errors.New("game projection unavailable")
	commits := 0
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	if err := registerAcceptedHarnessTurn(engine, "turn-commit-error", HarnessTurnSpec{
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("persisted answer", nil)}, true),
		Conversation: &runControlConversation{},
		Request:      ChatRequest{Message: "play"},
		Options:      RunOptions{AgentKind: AgentKindInteractiveStory, RootAgentName: "run-control-test"},
		CycleCommit: func(_ context.Context, outcome RunOutcome) error {
			commits++
			if outcome.Status != RunOutcomeCompleted || outcome.Content != "persisted answer" {
				t.Fatalf("commit outcome = %#v", outcome)
			}
			return wantErr
		},
	}); err != nil {
		t.Fatalf("register turn spec: %v", err)
	}

	_, err := runHarnessEngine(engine, context.Background(), harnessEngineRequest("turn-commit-error", nil), func(runstate.EngineEvent) error { return nil })
	if !errors.Is(err, wantErr) {
		t.Fatalf("run error = %v, want commit error", err)
	}
	if commits != 1 {
		t.Fatalf("cycle commits = %d, want one", commits)
	}
}

func TestHarnessEngineBridgesPreemptControl(t *testing.T) {
	model := newRunControlTwoPhaseModel("draft before preempt", "thinking before preempt")
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	if err := registerAcceptedHarnessTurn(engine, "turn-preempt", HarnessTurnSpec{
		Runner:       newRunControlTwoPhaseRunner(t, model),
		Conversation: &runControlConversation{},
		Request:      ChatRequest{Message: "write"},
		Options:      RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
	}); err != nil {
		t.Fatalf("register turn spec: %v", err)
	}

	controls := make(chan runstate.EngineControl)
	done := make(chan harnessEngineTestResult, 1)
	runHarnessEngineTestGoroutine(done, "preempt bridge run", func() (runstate.EngineResult, error) {
		return runHarnessEngine(engine, context.Background(), harnessEngineRequest("turn-preempt", controls), func(runstate.EngineEvent) error { return nil })
	})

	waitHarnessEngineSignal(t, model.blocked, "second model call")
	controls <- runstate.EngineControl{Kind: runstate.EngineControlPreempt}
	select {
	case got := <-done:
		t.Fatalf("preempt returned before the model safe point: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
	close(model.release)

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("preempt run: %v", got.err)
		}
		if got.result.Status != runstate.EnginePreempted {
			t.Fatalf("preempt status = %q, want preempted", got.result.Status)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for preempted run")
	}
}

func TestHarnessEngineRejectsMissingOrConflictingTurnSpec(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	if err := registerAcceptedHarnessTurn(engine, "", HarnessTurnSpec{}); !errors.Is(err, ErrHarnessTurnSpecRefRequired) {
		t.Fatalf("empty ref error = %v", err)
	}
	first := runstate.StartTurn{ID: "duplicate", Input: runstate.UserInput{Text: "same", TurnSpecRef: "duplicate"}}
	firstLease, err := engine.register("duplicate", first, HarnessTurnSpec{Request: ChatRequest{Message: "same"}})
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}
	defer firstLease.release()
	retryLease, err := engine.register("duplicate", first, HarnessTurnSpec{Request: ChatRequest{Message: "same"}})
	if err != nil {
		t.Fatalf("equal retry registration: %v", err)
	}
	retryLease.release()
	conflict := runstate.StartTurn{ID: "duplicate", Input: runstate.UserInput{Text: "different", TurnSpecRef: "duplicate"}}
	if _, err := engine.register("duplicate", conflict, HarnessTurnSpec{}); !errors.Is(err, ErrHarnessTurnSpecConflict) || !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("conflicting registration error = %v", err)
	}

	_, err = runHarnessEngine(engine, context.Background(), harnessEngineRequest("missing", nil), func(runstate.EngineEvent) error { return nil })
	if !errors.Is(err, ErrHarnessTurnSpecNotFound) {
		t.Fatalf("missing ref error = %v", err)
	}
}

func TestHarnessEngineReturnsSinkError(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	if err := registerAcceptedHarnessTurn(engine, "turn-sink-error", HarnessTurnSpec{
		Runner:       newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("answer", nil)}, true),
		Conversation: &runControlConversation{},
		Request:      ChatRequest{Message: "write"},
		Options:      RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
	}); err != nil {
		t.Fatalf("register turn spec: %v", err)
	}

	wantErr := errors.New("sink unavailable")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := runHarnessEngine(engine, ctx, harnessEngineRequest("turn-sink-error", nil), func(event runstate.EngineEvent) error {
		if _, ok := event.(runstate.EngineAssistantDelta); ok {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("run error = %v, want sink error", err)
	}
}

func TestHarnessEngineMapsDescriptorRecoveryToRetrySafety(t *testing.T) {
	tests := []struct {
		name     string
		recovery ToolRecoveryClass
		want     runstate.RetrySafety
	}{
		{name: "read_file", recovery: ToolRecoveryReadOnly, want: runstate.RetrySafe},
		{name: "write_file", recovery: ToolRecoveryReconcilable, want: runstate.RetryUnknown},
		{name: "bash", recovery: ToolRecoveryNonIdempotent, want: runstate.RetryUnsafe},
		{name: "missing_descriptor", want: runstate.RetryUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := harnessToolRetrySafety(ToolExecutionRecord{ToolName: tt.name, Descriptor: agenttools.Descriptor{Recovery: tt.recovery}}); got != tt.want {
				t.Fatalf("retry safety = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHarnessEngineConcurrentRegistrationLeasesPreserveAcceptedSpec(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	const contenders = 16
	command := runstate.FollowUp{
		ID: "shared-command", OperationID: "shared-operation",
		Input: runstate.UserInput{Text: "same input", TurnSpecRef: "shared"},
	}
	leases := make(chan *harnessTurnSpecLease, contenders)
	errs := make(chan error, contenders)
	for index := 0; index < contenders; index++ {
		runErrorTestGoroutine(errs, "concurrent turn registration", func() error {
			lease, err := engine.register("shared", command, HarnessTurnSpec{Request: ChatRequest{Message: "same input"}})
			if err == nil {
				leases <- lease
			}
			return err
		})
	}

	registered := 0
	for index := 0; index < contenders; index++ {
		err := <-errs
		if err == nil {
			registered++
		} else {
			t.Fatalf("unexpected registration error: %v", err)
		}
	}
	if registered != contenders {
		t.Fatalf("registrations = %d, want %d shared leases", registered, contenders)
	}
	allLeases := make([]*harnessTurnSpecLease, 0, contenders)
	for index := 0; index < registered; index++ {
		allLeases = append(allLeases, <-leases)
	}
	allLeases[0].accept()
	releaseErrs := make(chan error, len(allLeases)-1)
	for _, lease := range allLeases[1:] {
		lease := lease
		runErrorTestGoroutine(releaseErrs, "concurrent turn registration release", func() error {
			lease.release()
			return nil
		})
	}
	for index := 1; index < len(allLeases); index++ {
		err := <-releaseErrs
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.take("shared"); err != nil {
		t.Fatalf("accepted registration was deleted by replay releases: %v", err)
	}
}

func TestHarnessEngineRegistrationRejectsDifferentCommandSemantics(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	first := runstate.FollowUp{
		ID: "shared-command", OperationID: "shared-operation",
		Input: runstate.UserInput{Text: "first payload", TurnSpecRef: "shared"},
	}
	lease, err := engine.register("shared", first, HarnessTurnSpec{Request: ChatRequest{Message: "first payload"}})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	conflict := first
	conflict.Input.Text = "different payload"
	if _, err := engine.register("shared", conflict, HarnessTurnSpec{Request: ChatRequest{Message: "different payload"}}); !errors.Is(err, runstate.ErrInvalidCommand) || !errors.Is(err, ErrHarnessTurnSpecConflict) {
		t.Fatalf("conflicting semantic registration error = %v", err)
	}
	spec, err := engine.take("shared")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Request.Message != "first payload" {
		t.Fatalf("registered payload = %q, want first payload", spec.Request.Message)
	}
}

func TestHarnessEngineRegistrationRejectsDifferentRuntimeDescriptor(t *testing.T) {
	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	command := runstate.StartTurn{
		ID:    "shared-runtime-command",
		Input: runstate.UserInput{Text: "same payload", TurnSpecRef: "shared-runtime"},
	}
	first := HarnessTurnSpec{
		Request: ChatRequest{Message: "same payload"},
		Options: testHarnessRunOptions(),
	}
	lease, err := engine.register("shared-runtime", command, first)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	conflict := first
	conflict.Options.RootAgentName = "different-profile-graph"
	if _, err := engine.register("shared-runtime", command, conflict); !errors.Is(err, runstate.ErrInvalidCommand) || !errors.Is(err, ErrHarnessTurnSpecConflict) {
		t.Fatalf("conflicting runtime descriptor error = %v", err)
	}
}

func harnessEngineRequest(ref string, controls <-chan runstate.EngineControl) runstate.EngineRequest {
	binding := testHarnessBindingRef()
	return runstate.EngineRequest{
		Binding: binding,
		Snapshot: runstate.TurnSnapshot{
			Binding: binding,
			Input:   runstate.UserInput{Text: "write", TurnSpecRef: ref},
		},
		Controls: controls,
	}
}

// runHarnessEngine exercises the shared execution core in unit tests that do
// not care about factory binding. Binding-specific behavior is covered through
// NewEngine in the dedicated factory tests above.
func runHarnessEngine(engine *harnessEngine, ctx context.Context, request runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
	return engine.run(ctx, request, emit, nil)
}

func testHarnessBindingRef() runstate.BindingRef {
	return mustRuntimeBinding(RuntimeBinding{AgentKind: AgentKindIDE, Workspace: "/test/workspace", SessionID: "test-session"})
}

func testHarnessRunOptions() RunOptions {
	return RunOptions{
		AgentKind: AgentKindIDE, Workspace: "/test/workspace", SessionID: "test-session",
		RootAgentName: "run-control-test",
	}
}

func waitHarnessEngineSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", description)
	}
}

type identityBindingConversation struct {
	runControlConversation
	identity HarnessCycleIdentity
}

func (c *identityBindingConversation) BindAgentCycleIdentity(identity HarnessCycleIdentity) {
	c.identity = identity
}

type orderedCycleConversation struct {
	runControlConversation
	order      []string
	prepareErr error
}

type blockingPreparationConversation struct {
	runControlConversation
	started      chan struct{}
	startOnce    sync.Once
	runtimeCalls int
}

func newBlockingPreparationConversation() *blockingPreparationConversation {
	return &blockingPreparationConversation{started: make(chan struct{})}
}

func (c *blockingPreparationConversation) PrepareAgentCycle(ctx context.Context) error {
	c.startOnce.Do(func() { close(c.started) })
	<-ctx.Done()
	return ctx.Err()
}

func (c *blockingPreparationConversation) AssembleModelContext(
	ctx context.Context,
	_ string,
	input ModelContextInput,
) (ModelContextResult, error) {
	c.runtimeCalls++
	return AssembleSingleUserModelContext(ctx, input)
}

func (c *orderedCycleConversation) BindAgentCycleIdentity(HarnessCycleIdentity) {
	c.order = append(c.order, "bind")
}

func (c *orderedCycleConversation) PrepareAgentCycle(context.Context) error {
	c.order = append(c.order, "prepare")
	return c.prepareErr
}

func (c *orderedCycleConversation) AssembleModelContext(
	ctx context.Context,
	_ string,
	input ModelContextInput,
) (ModelContextResult, error) {
	c.order = append(c.order, "runtime")
	return AssembleSingleUserModelContext(ctx, input)
}

func registerAcceptedHarnessTurn(engine *harnessEngine, ref string, spec HarnessTurnSpec) error {
	message := strings.TrimSpace(spec.Request.Message)
	if message == "" {
		message = "test input"
	}
	lease, err := engine.register(ref, runstate.StartTurn{
		ID: runstate.CommandID("test-" + ref),
		Input: runstate.UserInput{
			Text:        message,
			TurnSpecRef: ref,
		},
	}, spec)
	if err != nil {
		return err
	}
	lease.accept()
	return nil
}
