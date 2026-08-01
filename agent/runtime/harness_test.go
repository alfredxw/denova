package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	filejournal "github.com/alfredxw/denova/agent/runtime/filejournal"
	"reflect"
	"sync"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestStartTurnIsDurableBeforeReceiptAndReplaysAfterOpen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	journals := runstate.NewMemoryJournalStore()
	engine := runstate.NewScriptedEngine(runstate.EngineScript{
		Events: []runstate.EngineEvent{
			runstate.EngineAssistantFinal{Content: "写好了。"},
		},
		Result: runstate.EngineResult{Status: runstate.EngineCompleted},
	})
	runtime, err := runstate.NewRuntime(engine, journals, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	binding := testBindingAt("/book", "session-1")
	harness, err := runtime.Open(ctx, binding)
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}

	receipt, err := harness.Submit(ctx, runstate.StartTurn{
		ID:    "command-1",
		Input: runstate.UserInput{Text: "继续写"},
	})
	if err != nil {
		t.Fatalf("submit turn: %v", err)
	}
	if receipt.Cursor == 0 || receipt.OperationID == "" {
		t.Fatalf("receipt must identify its durable commit: %+v", receipt)
	}

	waitForSettled(t, harness, receipt.Cursor)
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("close first runtime: %v", err)
	}

	reopenedRuntime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		journals,
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatalf("new reopened runtime: %v", err)
	}
	reopened, err := reopenedRuntime.Open(ctx, binding)
	if err != nil {
		t.Fatalf("reopen harness: %v", err)
	}
	observation, err := reopened.Observe(ctx, 0)
	if err != nil {
		t.Fatalf("observe reopened harness: %v", err)
	}
	if observation.Snapshot.Phase != runstate.PhaseIdle {
		t.Fatalf("reopened phase = %q, want idle", observation.Snapshot.Phase)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 2 || got[0] != "继续写" || got[1] != "写好了。" {
		t.Fatalf("replayed messages = %#v", got)
	}
}

func TestSubmitReturnsOriginalReceiptForIdempotentRetry(t *testing.T) {
	t.Parallel()
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(runstate.EngineScript{Result: runstate.EngineResult{Status: runstate.EngineCompleted}}),
		runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "retry"))
	if err != nil {
		t.Fatal(err)
	}
	command := runstate.StartTurn{ID: "same-command", Input: runstate.UserInput{Text: "same input", TurnSpecRef: "stable-ref"}}
	first, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.CommandID != first.CommandID || replayed.OperationID != first.OperationID || replayed.Cursor != first.Cursor {
		t.Fatalf("replayed receipt = %#v, first = %#v", replayed, first)
	}
	changed := command
	changed.Input.Text = "different input"
	if _, err := harness.Submit(context.Background(), changed); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("changed retry error = %v", err)
	}
}

func TestBindingTaxonomyIsApplicationOwnedAndBounded(t *testing.T) {
	t.Parallel()

	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testBindingWithProfile("/book", "config", "custom-profile"))
	if err != nil {
		t.Fatalf("open config-manager profile: %v", err)
	}
	observation, err := harness.Observe(context.Background(), 0)
	if err != nil {
		t.Fatalf("observe config-manager profile: %v", err)
	}
	if observation.Snapshot.Binding.Profile != "custom-profile" {
		t.Fatalf("profile = %q, want custom-profile", observation.Snapshot.Binding.Profile)
	}
	if _, err := runtime.Open(context.Background(), runstate.BindingRef{Kind: "custom", Key: " bad "}); !errors.Is(err, runstate.ErrInvalidBinding) {
		t.Fatalf("open malformed binding error = %v, want ErrInvalidBinding", err)
	}
	if _, err := runtime.Open(context.Background(), testGameBinding("/book", "story", "branch")); err != nil {
		t.Fatalf("application-defined game binding should be accepted: %v", err)
	}
	if _, err := runtime.Open(context.Background(), runstate.BindingRef{Kind: "external-host", Key: "global-task"}); err != nil {
		t.Fatalf("application-defined external host binding should be accepted: %v", err)
	}
}

func TestCommandsRejectStaleOperationTargets(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(runstate.EngineScript{Continue: release})
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "stale-target"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := harness.Submit(context.Background(), runstate.Steer{
		ID: "stale", OperationID: "old-operation", Input: runstate.UserInput{Text: "redirect"},
	}); !errors.Is(err, runstate.ErrStaleOperation) {
		t.Fatalf("stale steer error = %v, want ErrStaleOperation", err)
	}
	if _, err := harness.Submit(context.Background(), runstate.Abort{
		ID: "stale-abort", OperationID: "old-operation",
	}); !errors.Is(err, runstate.ErrStaleOperation) {
		t.Fatalf("stale abort error = %v, want ErrStaleOperation", err)
	}
	close(release)
	waitForSettled(t, harness, started.Cursor)
}

func TestCommandsEnforceConfiguredDurableInputEnvelope(t *testing.T) {
	t.Parallel()

	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{InputLimits: runstate.InputLimits{
			MaxTextBytes:            8,
			MaxContextRefs:          1,
			MaxContextRefFieldBytes: 8,
			MaxContextRefBytes:      16,
			MaxDeclaredContextBytes: 16,
			MaxTurnSpecRefBytes:     8,
		}},
	)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "bounded-input"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	tests := []runstate.UserInput{
		{Text: "123456789"},
		{Text: "ok", TurnSpecRef: "123456789"},
		{Text: "ok", ContextRefs: []runstate.ContextRef{{Source: "file", Resource: "one", ByteLimit: 1}, {Source: "file", Resource: "two", ByteLimit: 1}}},
		{Text: "ok", ContextRefs: []runstate.ContextRef{{Source: "workspace", Resource: "one", ByteLimit: 1}}},
		{Text: "ok", ContextRefs: []runstate.ContextRef{{Source: "file", Resource: "one", ByteLimit: 17}}},
	}
	for index, input := range tests {
		if _, err := harness.Submit(context.Background(), runstate.StartTurn{
			ID: runstate.CommandID(fmt.Sprintf("invalid-%d", index)), Input: input,
		}); !errors.Is(err, runstate.ErrInvalidCommand) {
			t.Fatalf("case %d error = %v, want ErrInvalidCommand", index, err)
		}
	}
}

func TestActiveOutputIsDisplayOnlyReconnectState(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(runstate.EngineScript{
		Events: []runstate.EngineEvent{
			runstate.EngineThinkingDelta{Delta: "reasoning"},
			runstate.EngineAssistantDelta{Delta: "partial"},
		},
		Continue: release,
	})
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "active-output"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	deltas := 0
	for deltas < 2 {
		event := <-observation.Events
		switch event.Payload.(type) {
		case runstate.ThinkingDeltaEvent, runstate.AssistantDeltaEvent:
			deltas++
		}
	}
	reconnected, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if got := reconnected.Snapshot.ActiveOutput; got.OperationID != receipt.OperationID || got.Cycle != 1 || got.Content != "partial" || got.Thinking != "reasoning" {
		t.Fatalf("active output = %+v", got)
	}
	if requests := engine.Requests(); len(requests) != 1 || requests[0].Snapshot.ContextCursor == 0 {
		t.Fatalf("engine snapshot must carry only a context cursor: %+v", requests)
	}
	close(release)
	waitForSettled(t, harness, receipt.Cursor)
}

func TestRuntimeCloseDurablyAbortsAndRejectsNewOpen(t *testing.T) {
	t.Parallel()

	journals := runstate.NewMemoryJournalStore()
	engine := runstate.NewScriptedEngine(runstate.EngineScript{WaitForControl: runstate.EngineControlAbort})
	runtime, err := runstate.NewRuntime(engine, journals, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	binding := testBindingAt("/book", "runtime-close")
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	if _, err := runtime.Open(context.Background(), binding); !errors.Is(err, runstate.ErrRuntimeClosed) {
		t.Fatalf("open after close error = %v, want ErrRuntimeClosed", err)
	}

	reopened, err := runstate.NewRuntime(runstate.NewScriptedEngine(), journals, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new reopened runtime: %v", err)
	}
	recovered, err := reopened.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	observation, err := recovered.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if observation.Snapshot.Phase != runstate.PhaseIdle {
		t.Fatalf("phase after close = %q, want idle", observation.Snapshot.Phase)
	}
}

func TestRuntimeOpenDoesNotSerializeDifferentBindings(t *testing.T) {
	t.Parallel()

	factory := newBlockingEngineFactory("slow")
	runtime, err := runstate.NewRuntime(factory, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	slowDone := make(chan error, 1)
	runExternalErrorTestGoroutine(slowDone, "slow independent binding open", func() error {
		_, err := runtime.Open(context.Background(), testBindingAt("/book", "slow"))
		return err
	})
	<-factory.blocked
	if _, err := runtime.Open(context.Background(), testBindingAt("/book", "fast")); err != nil {
		t.Fatalf("independent open was blocked or failed: %v", err)
	}
	close(factory.release)
	if err := <-slowDone; err != nil {
		t.Fatalf("slow open: %v", err)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestObserveFromNowDoesNotReplayDisplayHistory(t *testing.T) {
	t.Parallel()

	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "first"}}},
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "second"}}},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "from-now"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	first, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "first", Input: runstate.UserInput{Text: "one"}})
	if err != nil {
		t.Fatalf("submit first turn: %v", err)
	}
	waitForSettled(t, harness, first.Cursor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		t.Fatalf("observe from now: %v", err)
	}
	if observation.Snapshot.Cursor == 0 {
		t.Fatal("current snapshot cursor was not returned")
	}
	select {
	case event := <-observation.Events:
		t.Fatalf("unexpected replay event: %#v", event)
	default:
	}
	if _, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "second", Input: runstate.UserInput{Text: "two"}}); err != nil {
		t.Fatalf("submit second turn: %v", err)
	}
	for {
		select {
		case event := <-observation.Events:
			if _, ok := event.Payload.(runstate.OperationSettledEvent); ok {
				return
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation error: %v", err)
			}
		}
	}
}

func TestSteerPreemptsIntoNextCycleOfTheSameOperation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{WaitForControl: runstate.EngineControlPreempt},
		runstate.EngineScript{
			Events: []runstate.EngineEvent{
				runstate.EngineAssistantFinal{Content: "按新方向写好了。"},
			},
			Result: runstate.EngineResult{Status: runstate.EngineCompleted},
		},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(ctx, testBindingAt("/book", "steer"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}

	started, err := harness.Submit(ctx, runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "写一段雨夜"}})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	steered, err := harness.Submit(ctx, runstate.Steer{ID: "steer", OperationID: started.OperationID, Input: runstate.UserInput{Text: "改成雪夜"}})
	if err != nil {
		t.Fatalf("steer: %v", err)
	}
	if steered.OperationID != started.OperationID {
		t.Fatalf("steer operation = %q, want %q", steered.OperationID, started.OperationID)
	}

	waitForSettled(t, harness, steered.Cursor)
	observation, err := harness.Observe(ctx, 0)
	if err != nil {
		t.Fatalf("observe settled harness: %v", err)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 3 || got[0] != "写一段雨夜" || got[1] != "改成雪夜" || got[2] != "按新方向写好了。" {
		t.Fatalf("messages after steering = %#v", got)
	}
	cycles := 0
	for _, event := range rangeDurableEvents(t, observation, observation.Snapshot.Cursor) {
		if _, ok := event.Payload.(runstate.CycleStartedEvent); ok {
			cycles++
		}
	}
	if cycles != 2 {
		t.Fatalf("cycle starts = %d, want 2", cycles)
	}
}

func TestFollowUpWaitsForTheCurrentCycleToComplete(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{
			Events:   []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "第一版"}},
			Continue: release,
		},
		runstate.EngineScript{
			Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "补充完成"}},
		},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "follow-up"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "先写正文"}})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	followed, err := harness.Submit(context.Background(), runstate.FollowUp{ID: "follow", OperationID: started.OperationID, Input: runstate.UserInput{Text: "再补一句"}})
	if err != nil {
		t.Fatalf("queue follow-up: %v", err)
	}
	if followed.OperationID != started.OperationID {
		t.Fatalf("follow-up operation = %q, want %q", followed.OperationID, started.OperationID)
	}
	close(release)

	waitForSettled(t, harness, followed.Cursor)
	observation, err := harness.Observe(context.Background(), 0)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 4 || got[0] != "先写正文" || got[1] != "第一版" || got[2] != "再补一句" || got[3] != "补充完成" {
		t.Fatalf("messages after follow-up = %#v", got)
	}
}

func TestMultipleFollowUpsRunInAcceptedOrder(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{
			Events:   []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "初稿完成"}},
			Continue: release,
		},
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "第一条完成"}}},
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "第二条完成"}}},
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "第三条完成"}}},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "multiple-follow-ups"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "先写正文"},
	})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}

	inputs := []string{"补充第一条", "补充第二条", "补充第三条"}
	var last runstate.Receipt
	for index, input := range inputs {
		last, err = harness.Submit(context.Background(), runstate.FollowUp{
			ID:          runstate.CommandID(fmt.Sprintf("follow-%d", index+1)),
			OperationID: started.OperationID,
			Input:       runstate.UserInput{Text: input},
		})
		if err != nil {
			t.Fatalf("queue follow-up %d: %v", index+1, err)
		}
	}
	close(release)

	waitForSettled(t, harness, last.Cursor)
	observation, err := harness.Observe(context.Background(), 0)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	want := []string{
		"先写正文", "初稿完成",
		"补充第一条", "第一条完成",
		"补充第二条", "第二条完成",
		"补充第三条", "第三条完成",
	}
	if got := messageTexts(observation.Snapshot.Messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("messages after queued follow-ups = %#v, want %#v", got, want)
	}
}

func TestNextTurnRunsAsASeparateOperationAfterTheActiveOne(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{
			Events:   []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "第一轮完成"}},
			Continue: release,
		},
		runstate.EngineScript{
			Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "第二轮完成"}},
		},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), testGameBinding("/game", "story", "branch"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "观察房间"}})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	next, err := harness.Submit(context.Background(), runstate.NextTurn{ID: "next", AfterOperationID: started.OperationID, Input: runstate.UserInput{Text: "打开门"}})
	if err != nil {
		t.Fatalf("queue next turn: %v", err)
	}
	if next.OperationID == "" || next.OperationID == started.OperationID {
		t.Fatalf("next-turn operation = %q, active operation = %q", next.OperationID, started.OperationID)
	}
	close(release)

	waitForOperationSettled(t, harness, next.Cursor, next.OperationID)
	observation, err := harness.Observe(context.Background(), 0)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 4 || got[0] != "观察房间" || got[1] != "第一轮完成" || got[2] != "打开门" || got[3] != "第二轮完成" {
		t.Fatalf("messages after next turn = %#v", got)
	}
}

func TestFileJournalReplaysAcrossStoreInstances(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := filejournal.NewStore(root)
	if err != nil {
		t.Fatalf("new file journal: %v", err)
	}
	engine := runstate.NewScriptedEngine(runstate.EngineScript{
		Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "已持久化"}},
	})
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	binding := testBindingAt("/book", "file-journal")
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "persist", Input: runstate.UserInput{Text: "保存这轮"}})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForSettled(t, harness, receipt.Cursor)
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("close first runtime: %v", err)
	}

	reopenedStore, err := filejournal.NewStore(root)
	if err != nil {
		t.Fatalf("reopen file journal: %v", err)
	}
	reopenedRuntime, err := runstate.NewRuntime(runstate.NewScriptedEngine(), reopenedStore, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new reopened runtime: %v", err)
	}
	reopened, err := reopenedRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("reopen harness: %v", err)
	}
	observation, err := reopened.Observe(context.Background(), 0)
	if err != nil {
		t.Fatalf("observe reopened harness: %v", err)
	}
	if observation.Snapshot.Phase != runstate.PhaseIdle {
		t.Fatalf("reopened phase = %q, want idle", observation.Snapshot.Phase)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 2 || got[0] != "保存这轮" || got[1] != "已持久化" {
		t.Fatalf("replayed file messages = %#v", got)
	}
}

func TestRecoveryNeverRetriesAnUnfinishedToolEffect(t *testing.T) {
	t.Parallel()

	journals := runstate.NewMemoryJournalStore()
	binding := testBindingAt("/book", "recover")
	bindingRef := binding
	encodedBinding, err := json.Marshal(bindingRef)
	if err != nil {
		t.Fatalf("encode binding: %v", err)
	}
	journal, err := journals.OpenJournal(context.Background(), string(encodedBinding))
	if err != nil {
		t.Fatalf("open seed journal: %v", err)
	}
	operationID := runstate.OperationID("operation-recover")
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "seed"},
		runstate.OperationStartedEvent{OperationID: operationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "message-user", Role: runstate.RoleUser, Content: "写入章节",
			Input: runstate.UserInput{Text: "写入章节"}, Operation: operationID,
		}},
		runstate.CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot-recover"},
		runstate.ToolCallStartedEvent{Call: runstate.ToolCallState{
			CallID: "tool-1", Name: "write", Arguments: []byte(`{"path":"chapter.md"}`),
			OperationID: operationID, Cycle: 1,
		}},
	}); err != nil {
		t.Fatalf("seed unfinished operation: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("release seed journal: %v", err)
	}

	recoveryEngine := runstate.NewScriptedEngine()
	recoveryRuntime, err := runstate.NewRuntime(recoveryEngine, journals, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new recovery runtime: %v", err)
	}
	recovered, err := recoveryRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("recover harness: %v", err)
	}
	observation, err := recovered.Observe(context.Background(), 0)
	if err != nil {
		t.Fatalf("observe recovery: %v", err)
	}
	if observation.Snapshot.Phase != runstate.PhaseRunning || !observation.Snapshot.RecoveryPaused || len(observation.Snapshot.OpenToolCalls) != 0 {
		t.Fatalf("recovered snapshot = %+v", observation.Snapshot)
	}
	events := rangeDurableEvents(t, observation, observation.Snapshot.Cursor)
	unknownTool := false
	paused := false
	for _, event := range events {
		switch payload := event.Payload.(type) {
		case runstate.ToolCallFinishedEvent:
			unknownTool = payload.CallID == "tool-1" && payload.IsError && payload.RetrySafety == runstate.RetryUnknown &&
				payload.ResultDescriptor.Bytes == len(runstate.UnknownToolEffectResult) && payload.ResultDescriptor.SHA256 != ""
		case runstate.OperationRecoveryPausedEvent:
			paused = payload.OperationID == operationID
		}
	}
	if !unknownTool || !paused {
		t.Fatalf("recovery events missing unknown tool result or pause: %#v", events)
	}
	if got := len(recoveryEngine.Requests()); got != 0 {
		t.Fatalf("recovery started %d engine runs, want zero", got)
	}
}

func waitForSettled(t *testing.T, harness *runstate.Harness, after runstate.Cursor) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.Observe(ctx, after)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("event stream closed before settlement")
			}
			if _, ok := event.Payload.(runstate.OperationSettledEvent); ok {
				return
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation failed: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for settlement: %v", ctx.Err())
		}
	}
}

func waitForOperationSettled(t *testing.T, harness *runstate.Harness, after runstate.Cursor, operationID runstate.OperationID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.Observe(ctx, after)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("event stream closed before settlement")
			}
			settled, ok := event.Payload.(runstate.OperationSettledEvent)
			if ok && settled.OperationID == operationID {
				return
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation failed: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for operation %q: %v", operationID, ctx.Err())
		}
	}
}

func messageTexts(messages []runstate.Message) []string {
	result := make([]string, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.Content)
	}
	return result
}

func rangeDurableEvents(t *testing.T, observation runstate.Observation, through runstate.Cursor) []runstate.Event {
	t.Helper()

	events := make([]runstate.Event, 0, through)
	for len(events) < int(through) {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatalf("event stream closed after %d of %d durable events", len(events), through)
			}
			if event.Durability == runstate.EventDurable {
				events = append(events, event)
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation failed: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out after %d of %d durable events", len(events), through)
		}
	}
	return events
}

func waitForEventType[T any](t *testing.T, harness *runstate.Harness, after runstate.Cursor) T {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.Observe(ctx, after)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatalf("event stream closed before %T", *new(T))
			}
			if payload, ok := event.Payload.(T); ok {
				return payload
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation failed: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %T: %v", *new(T), ctx.Err())
		}
	}
}

type blockingEngineFactory struct {
	session string
	blocked chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingEngineFactory(session string) *blockingEngineFactory {
	return &blockingEngineFactory{
		session: session,
		blocked: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (f *blockingEngineFactory) NewEngine(ctx context.Context, binding runstate.BindingRef) (runstate.Engine, error) {
	if binding.Key == f.session {
		f.once.Do(func() { close(f.blocked) })
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return runstate.NewScriptedEngine(), nil
}
