package agentruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
)

// abortWaitCycle blocks until the controller interrupts it, so a stop request
// is the only way to settle the cycle.
func abortWaitCycle(started chan struct{}) ExternalCycleFactory {
	return func(context.Context, ExternalCycleInput, func(agentrun.Event), func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		close(started)
		return externalCycleFunc(func(ctx context.Context) agentrun.Outcome {
			<-ctx.Done()
			return agentrun.Outcome{Status: agentrun.OutcomeAborted, Error: ctx.Err()}
		}), nil
	}
}

func awaitSignal(t *testing.T, signal chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal(message)
	}
}

func TestAbortReplayKeepsNextPausedRound(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}

	firstStarted := make(chan struct{})
	first, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-1", Message: "Round one"}}, abortWaitCycle(firstStarted), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstOutcome := make(chan agentrun.Outcome, 1)
	go func() { firstOutcome <- first.Wait(t.Context()) }()
	awaitSignal(t, firstStarted, "round one did not start")
	abort := Command{Kind: agentexecution.CommandAbort, CommandID: "stop-1", OperationID: first.Receipt().OperationID}
	receipt, err := control.Submit(t.Context(), abort)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-firstOutcome; outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("abort outcome: %+v", outcome)
	}
	if binding, err := control.AbortedInputCommandID(t.Context(), "stop-1"); err != nil || binding != "round-1" {
		t.Fatalf("binding after abort = %q, %v", binding, err)
	}
	if replayed, err := control.Submit(t.Context(), abort); err != nil || replayed != receipt {
		t.Fatalf("replayed receipt: %+v %v", replayed, err)
	}

	// The next round starts and pauses before the client retries the old stop.
	secondStarted := make(chan struct{})
	second, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-2", Message: "Round two"}}, abortWaitCycle(secondStarted), nil)
	if err != nil {
		t.Fatal(err)
	}
	secondOutcome := make(chan agentrun.Outcome, 1)
	go func() { secondOutcome <- second.Wait(t.Context()) }()
	awaitSignal(t, secondStarted, "round two did not start")
	if _, err := control.Submit(t.Context(), Command{Kind: agentexecution.CommandSuspend, CommandID: "pause-2", OperationID: second.Receipt().OperationID}); err != nil {
		t.Fatal(err)
	}
	if outcome := <-secondOutcome; outcome.Status != agentrun.OutcomeSuspended {
		t.Fatalf("second round outcome: %+v", outcome)
	}
	if _, err := control.Submit(t.Context(), abort); err != nil {
		t.Fatalf("replay after pause: %v", err)
	}
	status, err := control.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseSuspended || status.ActiveCommandID != "round-2" {
		t.Fatalf("replay changed the paused round: %+v", status)
	}
	if binding, err := control.AbortedInputCommandID(t.Context(), "stop-1"); err != nil || binding != "round-1" {
		t.Fatalf("binding after replay = %q, %v", binding, err)
	}
}

// A queued input that starts running becomes Current without updating
// state.CommandID; the stop request must bind the input it actually discards.
func TestAbortBindsRunningQueuedInput(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	started := make(chan string, 2)
	factory := func(_ context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		commandID := input.Request.CommandID
		started <- commandID
		return externalCycleFunc(func(ctx context.Context) agentrun.Outcome {
			if commandID == "round-1" {
				<-released
				return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
			}
			<-ctx.Done()
			return agentrun.Outcome{Status: agentrun.OutcomeAborted, Error: ctx.Err()}
		}), nil
	}
	run, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-1", Message: "Round one"}}, factory, nil)
	if err != nil {
		t.Fatal(err)
	}
	runOutcome := make(chan agentrun.Outcome, 1)
	go func() { runOutcome <- run.Wait(t.Context()) }()
	if commandID := <-started; commandID != "round-1" {
		t.Fatalf("unexpected first input %q", commandID)
	}
	if _, err := control.Submit(t.Context(), Command{Kind: agentexecution.CommandFollowUp, CommandID: "queued-2", OperationID: run.Receipt().OperationID, Input: agentchat.ChatRequest{Message: "Queued"}}); err != nil {
		t.Fatal(err)
	}
	close(released)
	select {
	case commandID := <-started:
		if commandID != "queued-2" {
			t.Fatalf("queued input did not start: %q", commandID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued input did not start")
	}
	if _, err := control.Submit(t.Context(), Command{Kind: agentexecution.CommandAbort, CommandID: "stop-2", OperationID: run.Receipt().OperationID}); err != nil {
		t.Fatal(err)
	}
	if outcome := <-runOutcome; outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("queued abort outcome: %+v", outcome)
	}
	if binding, err := control.AbortedInputCommandID(t.Context(), "stop-2"); err != nil || binding != "queued-2" {
		t.Fatalf("binding = %q, %v", binding, err)
	}
}

// The recovery abort path must bind the input it discards too; its replay is
// reached with the same recovery action, so it must not pick up a newer round.
func TestRecoveryAbortBindsStoppedInputAndReplays(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-1", Message: "Round one"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	// Process loss: the durable input is running but has no live runner.
	control.mu.Lock()
	control.active = nil
	control.mu.Unlock()
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
	aborted, err := control.Resume(t.Context(), abortAction, abortWaitCycle(make(chan struct{})), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := aborted.Wait(t.Context()); outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("recovery abort outcome: %+v", outcome)
	}
	if binding, err := control.AbortedRecoveryInputCommandID(t.Context(), abortAction); err != nil || binding != "round-1" {
		t.Fatalf("recovery binding = %q, %v", binding, err)
	}
	// Replaying the same recovery action must not change the durable binding.
	replayed, err := control.Resume(t.Context(), abortAction, abortWaitCycle(make(chan struct{})), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := replayed.Wait(t.Context()); outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("replayed recovery abort outcome: %+v", outcome)
	}
	if binding, err := control.AbortedRecoveryInputCommandID(t.Context(), abortAction); err != nil || binding != "round-1" {
		t.Fatalf("recovery binding after replay = %q, %v", binding, err)
	}
}

func TestAbortBindingSurvivesRestart(t *testing.T) {
	store, _, state, options, dir := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	run, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "round-1", Message: "Round one"}}, abortWaitCycle(started), nil)
	if err != nil {
		t.Fatal(err)
	}
	runOutcome := make(chan agentrun.Outcome, 1)
	go func() { runOutcome <- run.Wait(t.Context()) }()
	awaitSignal(t, started, "round one did not start")
	if _, err := control.Submit(t.Context(), Command{Kind: agentexecution.CommandAbort, CommandID: "stop-1", OperationID: run.Receipt().OperationID}); err != nil {
		t.Fatal(err)
	}
	if outcome := <-runOutcome; outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("abort outcome: %+v", outcome)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "control.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	moved := t.TempDir()
	if err := os.WriteFile(filepath.Join(moved, "control.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.NewStore(moved)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	sess, err := reopened.Get("control")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := SessionState(options, sess)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewEngines()
	defer restarted.Close()
	control, err = restarted.ExternalControl(options, restored)
	if err != nil {
		t.Fatal(err)
	}
	if binding, err := control.AbortedInputCommandID(t.Context(), "stop-1"); err != nil || binding != "round-1" {
		t.Fatalf("binding after restart = %q, %v", binding, err)
	}
}