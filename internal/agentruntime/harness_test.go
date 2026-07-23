package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestStartTurnIsDurableBeforeReceiptAndReplaysAfterOpen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	journals := agentruntime.NewMemoryJournalStore()
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{
		Events: []agentruntime.EngineEvent{
			agentruntime.EngineAssistantFinal{Content: "写好了。"},
		},
		Result: agentruntime.EngineResult{Status: agentruntime.EngineCompleted},
	})
	runtime, err := agentruntime.NewRuntime(engine, journals, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "session-1"}
	harness, err := runtime.Open(ctx, binding)
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}

	receipt, err := harness.Submit(ctx, agentruntime.StartTurn{
		ID:    "command-1",
		Input: agentruntime.UserInput{Text: "继续写"},
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

	reopenedRuntime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(),
		journals,
		agentruntime.RuntimeConfig{},
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
	if observation.Snapshot.Phase != agentruntime.PhaseIdle {
		t.Fatalf("reopened phase = %q, want idle", observation.Snapshot.Phase)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 2 || got[0] != "继续写" || got[1] != "写好了。" {
		t.Fatalf("replayed messages = %#v", got)
	}
}

func TestSubmitReturnsOriginalReceiptForIdempotentRetry(t *testing.T) {
	t.Parallel()
	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(agentruntime.EngineScript{Result: agentruntime.EngineResult{Status: agentruntime.EngineCompleted}}),
		agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "retry"})
	if err != nil {
		t.Fatal(err)
	}
	command := agentruntime.StartTurn{ID: "same-command", Input: agentruntime.UserInput{Text: "same input", TurnSpecRef: "stable-ref"}}
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
	if _, err := harness.Submit(context.Background(), changed); !errors.Is(err, agentruntime.ErrInvalidCommand) {
		t.Fatalf("changed retry error = %v", err)
	}
}

func TestBindingProfileIsExplicitAndValidated(t *testing.T) {
	t.Parallel()

	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "config", Profile: agentruntime.ProfileConfigManager,
	})
	if err != nil {
		t.Fatalf("open config-manager profile: %v", err)
	}
	observation, err := harness.Observe(context.Background(), 0)
	if err != nil {
		t.Fatalf("observe config-manager profile: %v", err)
	}
	if observation.Snapshot.Binding.Profile != agentruntime.ProfileConfigManager {
		t.Fatalf("profile = %q, want %q", observation.Snapshot.Binding.Profile, agentruntime.ProfileConfigManager)
	}
	if _, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "bad", Profile: agentruntime.ProfileDirector,
	}); !errors.Is(err, agentruntime.ErrInvalidBinding) {
		t.Fatalf("open invalid profile error = %v, want ErrInvalidBinding", err)
	}
	if _, err := runtime.Open(context.Background(), agentruntime.GameBinding{
		Workspace: "/book", StoryID: "story", BranchID: "branch",
	}); err != nil {
		t.Fatalf("game binding should use story/branch identity without a session: %v", err)
	}
	if _, err := runtime.Open(context.Background(), agentruntime.AutomationBinding{
		SessionID: "global-session", TaskID: "global-task",
	}); err != nil {
		t.Fatalf("user-scoped automation should not require a book workspace: %v", err)
	}
}

func TestCommandsRejectStaleOperationTargets(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{Continue: release})
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "stale-target"})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Steer{
		ID: "stale", OperationID: "old-operation", Input: agentruntime.UserInput{Text: "redirect"},
	}); !errors.Is(err, agentruntime.ErrStaleOperation) {
		t.Fatalf("stale steer error = %v, want ErrStaleOperation", err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "stale-abort", OperationID: "old-operation",
	}); !errors.Is(err, agentruntime.ErrStaleOperation) {
		t.Fatalf("stale abort error = %v, want ErrStaleOperation", err)
	}
	close(release)
	waitForSettled(t, harness, started.Cursor)
}

func TestCommandsEnforceConfiguredDurableInputEnvelope(t *testing.T) {
	t.Parallel()

	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{InputLimits: agentruntime.InputLimits{
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
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "bounded-input"})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	tests := []agentruntime.UserInput{
		{Text: "123456789"},
		{Text: "ok", TurnSpecRef: "123456789"},
		{Text: "ok", ContextRefs: []agentruntime.ContextRef{{Source: "file", Resource: "one", ByteLimit: 1}, {Source: "file", Resource: "two", ByteLimit: 1}}},
		{Text: "ok", ContextRefs: []agentruntime.ContextRef{{Source: "workspace", Resource: "one", ByteLimit: 1}}},
		{Text: "ok", ContextRefs: []agentruntime.ContextRef{{Source: "file", Resource: "one", ByteLimit: 17}}},
	}
	for index, input := range tests {
		if _, err := harness.Submit(context.Background(), agentruntime.StartTurn{
			ID: agentruntime.CommandID(fmt.Sprintf("invalid-%d", index)), Input: input,
		}); !errors.Is(err, agentruntime.ErrInvalidCommand) {
			t.Fatalf("case %d error = %v, want ErrInvalidCommand", index, err)
		}
	}
}

func TestActiveOutputIsDisplayOnlyReconnectState(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{
		Events: []agentruntime.EngineEvent{
			agentruntime.EngineThinkingDelta{Delta: "reasoning"},
			agentruntime.EngineAssistantDelta{Delta: "partial"},
		},
		Continue: release,
	})
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "active-output"})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	deltas := 0
	for deltas < 2 {
		event := <-observation.Events
		switch event.Payload.(type) {
		case agentruntime.ThinkingDeltaEvent, agentruntime.AssistantDeltaEvent:
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

	journals := agentruntime.NewMemoryJournalStore()
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{WaitForControl: agentruntime.EngineControlAbort})
	runtime, err := agentruntime.NewRuntime(engine, journals, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "runtime-close"}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	if _, err := runtime.Open(context.Background(), binding); !errors.Is(err, agentruntime.ErrRuntimeClosed) {
		t.Fatalf("open after close error = %v, want ErrRuntimeClosed", err)
	}

	reopened, err := agentruntime.NewRuntime(agentruntime.NewScriptedEngine(), journals, agentruntime.RuntimeConfig{})
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
	if observation.Snapshot.Phase != agentruntime.PhaseIdle {
		t.Fatalf("phase after close = %q, want idle", observation.Snapshot.Phase)
	}
}

func TestRuntimeOpenDoesNotSerializeDifferentBindings(t *testing.T) {
	t.Parallel()

	factory := newBlockingEngineFactory("slow")
	runtime, err := agentruntime.NewRuntime(factory, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	slowDone := make(chan error, 1)
	runExternalErrorTestGoroutine(slowDone, "slow independent binding open", func() error {
		_, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "slow"})
		return err
	})
	<-factory.blocked
	if _, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "fast"}); err != nil {
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

	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "first"}}},
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "second"}}},
	)
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "from-now"})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	first, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "first", Input: agentruntime.UserInput{Text: "one"}})
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
	if _, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "second", Input: agentruntime.UserInput{Text: "two"}}); err != nil {
		t.Fatalf("submit second turn: %v", err)
	}
	for {
		select {
		case event := <-observation.Events:
			if _, ok := event.Payload.(agentruntime.OperationSettledEvent); ok {
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
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{WaitForControl: agentruntime.EngineControlPreempt},
		agentruntime.EngineScript{
			Events: []agentruntime.EngineEvent{
				agentruntime.EngineAssistantFinal{Content: "按新方向写好了。"},
			},
			Result: agentruntime.EngineResult{Status: agentruntime.EngineCompleted},
		},
	)
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(ctx, agentruntime.WritingBinding{Workspace: "/book", SessionID: "steer"})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}

	started, err := harness.Submit(ctx, agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "写一段雨夜"}})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	steered, err := harness.Submit(ctx, agentruntime.Steer{ID: "steer", OperationID: started.OperationID, Input: agentruntime.UserInput{Text: "改成雪夜"}})
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
		if _, ok := event.Payload.(agentruntime.CycleStartedEvent); ok {
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
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{
			Events:   []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "第一版"}},
			Continue: release,
		},
		agentruntime.EngineScript{
			Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "补充完成"}},
		},
	)
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "follow-up"})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "先写正文"}})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	followed, err := harness.Submit(context.Background(), agentruntime.FollowUp{ID: "follow", OperationID: started.OperationID, Input: agentruntime.UserInput{Text: "再补一句"}})
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

func TestNextTurnRunsAsASeparateOperationAfterTheActiveOne(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{
			Events:   []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "第一轮完成"}},
			Continue: release,
		},
		agentruntime.EngineScript{
			Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "第二轮完成"}},
		},
	)
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.GameBinding{
		Workspace: "/game", SessionID: "next-turn", StoryID: "story", BranchID: "branch",
	})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "观察房间"}})
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	next, err := harness.Submit(context.Background(), agentruntime.NextTurn{ID: "next", AfterOperationID: started.OperationID, Input: agentruntime.UserInput{Text: "打开门"}})
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
	store, err := agentruntime.NewFileJournalStore(root)
	if err != nil {
		t.Fatalf("new file journal: %v", err)
	}
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{
		Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: "已持久化"}},
	})
	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "file-journal"}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "persist", Input: agentruntime.UserInput{Text: "保存这轮"}})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForSettled(t, harness, receipt.Cursor)
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("close first runtime: %v", err)
	}

	reopenedStore, err := agentruntime.NewFileJournalStore(root)
	if err != nil {
		t.Fatalf("reopen file journal: %v", err)
	}
	reopenedRuntime, err := agentruntime.NewRuntime(agentruntime.NewScriptedEngine(), reopenedStore, agentruntime.RuntimeConfig{})
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
	if observation.Snapshot.Phase != agentruntime.PhaseIdle {
		t.Fatalf("reopened phase = %q, want idle", observation.Snapshot.Phase)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 2 || got[0] != "保存这轮" || got[1] != "已持久化" {
		t.Fatalf("replayed file messages = %#v", got)
	}
}

func TestRecoveryNeverRetriesAnUnfinishedToolEffect(t *testing.T) {
	t.Parallel()

	journals := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "recover"}
	bindingRef := agentruntime.BindingRef{
		Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting,
		Workspace: "/book", SessionID: "recover",
	}
	encodedBinding, err := json.Marshal(bindingRef)
	if err != nil {
		t.Fatalf("encode binding: %v", err)
	}
	journal, err := journals.OpenJournal(context.Background(), string(encodedBinding))
	if err != nil {
		t.Fatalf("open seed journal: %v", err)
	}
	operationID := agentruntime.OperationID("operation-recover")
	if _, err := journal.Append(context.Background(), 0, []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "seed"},
		agentruntime.OperationStartedEvent{OperationID: operationID},
		agentruntime.UserMessageCommittedEvent{Message: agentruntime.Message{
			ID: "message-user", Role: agentruntime.RoleUser, Content: "写入章节",
			Input: agentruntime.UserInput{Text: "写入章节"}, Operation: operationID,
		}},
		agentruntime.CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot-recover"},
		agentruntime.ToolCallStartedEvent{Call: agentruntime.ToolCallState{
			CallID: "tool-1", Name: "write_file", Arguments: []byte(`{"path":"chapter.md"}`),
			OperationID: operationID, Cycle: 1,
		}},
	}); err != nil {
		t.Fatalf("seed unfinished operation: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("release seed journal: %v", err)
	}

	recoveryEngine := agentruntime.NewScriptedEngine()
	recoveryRuntime, err := agentruntime.NewRuntime(recoveryEngine, journals, agentruntime.RuntimeConfig{})
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
	if observation.Snapshot.Phase != agentruntime.PhaseRunning || !observation.Snapshot.RecoveryPaused || len(observation.Snapshot.OpenToolCalls) != 0 {
		t.Fatalf("recovered snapshot = %+v", observation.Snapshot)
	}
	events := rangeDurableEvents(t, observation, observation.Snapshot.Cursor)
	unknownTool := false
	paused := false
	for _, event := range events {
		switch payload := event.Payload.(type) {
		case agentruntime.ToolCallFinishedEvent:
			unknownTool = payload.CallID == "tool-1" && payload.IsError && payload.RetrySafety == agentruntime.RetryUnknown &&
				payload.ResultDescriptor.Bytes == len(agentruntime.UnknownToolEffectResult) && payload.ResultDescriptor.SHA256 != ""
		case agentruntime.OperationRecoveryPausedEvent:
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

func waitForSettled(t *testing.T, harness *agentruntime.Harness, after agentruntime.Cursor) {
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
			if _, ok := event.Payload.(agentruntime.OperationSettledEvent); ok {
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

func waitForOperationSettled(t *testing.T, harness *agentruntime.Harness, after agentruntime.Cursor, operationID agentruntime.OperationID) {
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
			settled, ok := event.Payload.(agentruntime.OperationSettledEvent)
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

func messageTexts(messages []agentruntime.Message) []string {
	result := make([]string, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.Content)
	}
	return result
}

func rangeDurableEvents(t *testing.T, observation agentruntime.Observation, through agentruntime.Cursor) []agentruntime.Event {
	t.Helper()

	events := make([]agentruntime.Event, 0, through)
	for len(events) < int(through) {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatalf("event stream closed after %d of %d durable events", len(events), through)
			}
			if event.Durability == agentruntime.EventDurable {
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

func waitForEventType[T any](t *testing.T, harness *agentruntime.Harness, after agentruntime.Cursor) T {
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

func (f *blockingEngineFactory) NewEngine(ctx context.Context, binding agentruntime.BindingRef) (agentruntime.Engine, error) {
	if binding.SessionID == f.session {
		f.once.Do(func() { close(f.blocked) })
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return agentruntime.NewScriptedEngine(), nil
}
