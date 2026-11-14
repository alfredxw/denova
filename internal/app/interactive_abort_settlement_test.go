package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	"denova/internal/interactive"
)

func TestAbortSettlementIsLimitedToExactGameCommand(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Parallel accepted inputs"})
	if err != nil { t.Fatal(err) }
	before, err := store.Snapshot(story.ID, "main")
	if err != nil { t.Fatal(err) }
	makeInput := func(command string) string {
		t.Helper()
		digest := sha256.Sum256([]byte(command))
		identity := interactive.DomainCommitIdentity{
			CommandID: command,
			OperationID: "external-game-" + hex.EncodeToString(digest[:16]),
			Cycle: 1,
		}
		intent, err := interactive.NewPlayerInputIntent(identity, "main", "Input for "+command)
		if err != nil { t.Fatal(err) }
		accepted, err := store.CommitPlayerInput(story.ID, intent)
		if err != nil { t.Fatal(err) }
		return accepted.Event.ID
	}
	first := makeInput("first")
	second := makeInput("second")
	if err := settleAbortedExternalGameInput(store, story.ID, "main", "first"); err != nil { t.Fatal(err) }
	if err := settleAbortedExternalGameInput(store, story.ID, "main", "first"); err != nil { t.Fatalf("retry: %v", err) }
	cold := interactive.NewStore(root)
	defer cold.Close()
	after, err := cold.Snapshot(story.ID, "main")
	if err != nil { t.Fatal(err) }
	if after.TurnCount != before.TurnCount || !reflect.DeepEqual(after.State, before.State) {
		t.Fatal("aborting a command changed committed story state")
	}
	if len(after.PendingPlayerInputs) != 1 || after.PendingPlayerInputs[0].ID != second || after.PendingPlayerInputs[0].ID == first {
		t.Fatalf("abort settled another command: %+v", after.PendingPlayerInputs)
	}
}

// gameAbortCycle is an interactive.External-cycle stub that waits to be
// interrupted, so a stop request is the only way to settle it.
type gameAbortCycle func(context.Context) agentrun.Outcome

func (cycle gameAbortCycle) Wait(ctx context.Context) agentrun.Outcome { return cycle(ctx) }

// gameAbortWaitFactory reports when a game cycle starts, then blocks until the
// controller interrupts it.
func gameAbortWaitFactory(started chan struct{}) agentruntime.ExternalCycleFactory {
	return func(_ context.Context, _ agentruntime.ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (agentruntime.ExternalCycle, error) {
		close(started)
		return gameAbortCycle(func(ctx context.Context) agentrun.Outcome {
			<-ctx.Done()
			return agentrun.Outcome{Status: agentrun.OutcomeAborted, Error: ctx.Err()}
		}), nil
	}
}

func awaitGameStart(t *testing.T, started chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("game round did not start")
	}
}

// commitExternalGameInput records a pending player input under the same
// identity formula the interactive Game uses for an external turn.
func commitExternalGameInput(t *testing.T, store *interactive.Store, storyID, command string) string {
	t.Helper()
	digest := sha256.Sum256([]byte(command))
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID:   command,
		OperationID: "external-game-" + hex.EncodeToString(digest[:16]),
		Cycle:       1,
	}, "main", "Input for "+command)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := store.CommitPlayerInput(storyID, intent)
	if err != nil {
		t.Fatal(err)
	}
	return accepted.Event.ID
}

func assertSinglePendingGameInput(t *testing.T, store *interactive.Store, storyID, wantID string) {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 1 || snapshot.PendingPlayerInputs[0].ID != wantID {
		t.Fatalf("pending inputs = %+v, want only %q", snapshot.PendingPlayerInputs, wantID)
	}
}

// replayAbortGameFixture drives one real external controller over the Game
// journal: round one aborted and settled, round two paused, then the old stop
// request replayed and settled again.
func TestAbortReplayKeepsNextPausedGameRoundInput(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Abort replay"})
	if err != nil {
		t.Fatal(err)
	}
	firstInput := commitExternalGameInput(t, store, story.ID, "round-1")
	secondInput := commitExternalGameInput(t, store, story.ID, "round-2")
	options := agentrun.Options{ProjectID: "stable-project", AgentKind: agentrun.AgentKindInteractiveStory, StoryID: story.ID, BranchID: "main", Mode: "interactive"}
	state, err := agentruntime.GameState(options, store)
	if err != nil {
		t.Fatal(err)
	}
	engines := agentruntime.NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}

	firstStarted := make(chan struct{})
	first, err := control.Start(t.Context(), agentruntime.ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-1", Message: "Round one"}}, gameAbortWaitFactory(firstStarted), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstOutcome := make(chan agentrun.Outcome, 1)
	go func() { firstOutcome <- first.Wait(t.Context()) }()
	awaitGameStart(t, firstStarted)
	abort := agentruntime.Command{Kind: agentexecution.CommandAbort, CommandID: "stop-1", OperationID: first.Receipt().OperationID}
	if _, err := control.Submit(t.Context(), abort); err != nil {
		t.Fatal(err)
	}
	if outcome := <-firstOutcome; outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("abort outcome: %+v", outcome)
	}
	if err := settleBoundAbortedGameInput(t.Context(), control, store, story.ID, "main", "stop-1"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 1 || snapshot.PendingPlayerInputs[0].ID != secondInput {
		t.Fatalf("first settle left %+v, want only %q", snapshot.PendingPlayerInputs, secondInput)
	}
	for _, input := range snapshot.PendingPlayerInputs {
		if input.ID == firstInput {
			t.Fatalf("first round input was not settled: %+v", snapshot.PendingPlayerInputs)
		}
	}

	secondStarted := make(chan struct{})
	second, err := control.Start(t.Context(), agentruntime.ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-2", Message: "Round two"}}, gameAbortWaitFactory(secondStarted), nil)
	if err != nil {
		t.Fatal(err)
	}
	secondOutcome := make(chan agentrun.Outcome, 1)
	go func() { secondOutcome <- second.Wait(t.Context()) }()
	awaitGameStart(t, secondStarted)
	if _, err := control.Submit(t.Context(), agentruntime.Command{Kind: agentexecution.CommandSuspend, CommandID: "pause-2", OperationID: second.Receipt().OperationID}); err != nil {
		t.Fatal(err)
	}
	if outcome := <-secondOutcome; outcome.Status != agentrun.OutcomeSuspended {
		t.Fatalf("second round outcome: %+v", outcome)
	}

	// Retry the old stop request; the paused round must survive both the
	// controller replay and the settlement that follows it.
	if _, err := control.Submit(t.Context(), abort); err != nil {
		t.Fatalf("replay after pause: %v", err)
	}
	if err := settleBoundAbortedGameInput(t.Context(), control, store, story.ID, "main", "stop-1"); err != nil {
		t.Fatal(err)
	}
	assertSinglePendingGameInput(t, store, story.ID, secondInput)
	status, err := control.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseSuspended || status.ActiveCommandID != "round-2" {
		t.Fatalf("replay changed the paused round: %+v", status)
	}
}

// TestRecoveryAbortReplayKeepsNextPausedGameRoundInput covers the recovery
// counterpart: a retried recovery abort must settle only the input it actually
// discarded, never the paused next round.
func TestRecoveryAbortReplayKeepsNextPausedGameRoundInput(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Recovery abort replay"})
	if err != nil {
		t.Fatal(err)
	}
	firstInput := commitExternalGameInput(t, store, story.ID, "round-1")
	secondInput := commitExternalGameInput(t, store, story.ID, "round-2")
	options := agentrun.Options{ProjectID: "stable-project", AgentKind: agentrun.AgentKindInteractiveStory, StoryID: story.ID, BranchID: "main", Mode: "interactive"}
	state, err := agentruntime.GameState(options, store)
	if err != nil {
		t.Fatal(err)
	}
	engines := agentruntime.NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}

	// Round one is durably accepted, then the process loses its live runner;
	// a fresh controller over the same journal has no runner either.
	if _, err := control.Start(t.Context(), agentruntime.ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-1", Message: "Round one"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	restarted := agentruntime.NewEngines()
	defer restarted.Close()
	control, err = restarted.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	status, err := control.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var abortAction agentexecution.RuntimeRecoveryAction
	for _, action := range agentexecution.RuntimeRecoveryActions(status) {
		if action.Kind == agentexecution.RuntimeRecoveryAbort {
			abortAction = action
			break
		}
	}
	if abortAction.Kind == "" {
		t.Fatalf("no recovery abort action in %+v", status)
	}
	aborted, err := control.Resume(t.Context(), abortAction, gameAbortWaitFactory(make(chan struct{})), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := aborted.Wait(t.Context()); outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("recovery abort outcome: %+v", outcome)
	}
	if err := settleRecoveredAbortedGameInput(t.Context(), control, store, story.ID, "main", abortAction); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 1 || snapshot.PendingPlayerInputs[0].ID != secondInput {
		t.Fatalf("recovery abort settled wrong inputs: %+v", snapshot.PendingPlayerInputs)
	}
	for _, input := range snapshot.PendingPlayerInputs {
		if input.ID == firstInput {
			t.Fatalf("first round input was not settled: %+v", snapshot.PendingPlayerInputs)
		}
	}

	// The next round starts and pauses before the recovery abort is retried.
	secondStarted := make(chan struct{})
	second, err := control.Start(t.Context(), agentruntime.ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-2", Message: "Round two"}}, gameAbortWaitFactory(secondStarted), nil)
	if err != nil {
		t.Fatal(err)
	}
	secondOutcome := make(chan agentrun.Outcome, 1)
	go func() { secondOutcome <- second.Wait(t.Context()) }()
	awaitGameStart(t, secondStarted)
	if _, err := control.Submit(t.Context(), agentruntime.Command{Kind: agentexecution.CommandSuspend, CommandID: "pause-2", OperationID: second.Receipt().OperationID}); err != nil {
		t.Fatal(err)
	}
	if outcome := <-secondOutcome; outcome.Status != agentrun.OutcomeSuspended {
		t.Fatalf("second round outcome: %+v", outcome)
	}
	replayed, err := control.Resume(t.Context(), abortAction, gameAbortWaitFactory(make(chan struct{})), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := replayed.Wait(t.Context()); outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("replayed recovery abort outcome: %+v", outcome)
	}
	if err := settleRecoveredAbortedGameInput(t.Context(), control, store, story.ID, "main", abortAction); err != nil {
		t.Fatal(err)
	}
	assertSinglePendingGameInput(t, store, story.ID, secondInput)
	status, err = control.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseSuspended || status.ActiveCommandID != "round-2" {
		t.Fatalf("replayed recovery abort changed the paused round: %+v", status)
	}
}
