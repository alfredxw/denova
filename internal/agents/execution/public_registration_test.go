package execution

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	agent "github.com/alfredxw/denova/agent"
)

func TestRejectedCommandsPreserveAcceptedRoutesAndReleaseTheirOwn(t *testing.T) {
	ctx := t.Context()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("registration-lifetime")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendNextTurnModel{started: make(chan struct{}), release: make(chan struct{})}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	options := agentrun.Options{AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: t.TempDir()}
	start := StartRequest{Cycle: Cycle{
		Definition:   agent.Definition{Name: "registration", Model: model},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("accepted", "first request"), Options: options,
	}}
	invalid := start
	invalid.Cycle.Request = agentchatRequest("invalid", "")
	if _, err := runtime.Start(ctx, invalid); err == nil {
		t.Fatal("empty input was accepted")
	}
	if count := len(runtime.public.registrations); count != 0 {
		t.Fatalf("invalid input retained %d registrations", count)
	}
	operation, err := runtime.Start(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	key, err := agentrun.AgentSessionKeyForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	accepted := runtime.public.registration(key, "accepted")
	if accepted == nil {
		t.Fatal("accepted input has no route")
	}
	_, err = runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandFollowUp, CommandID: "accepted", OperationID: operation.Receipt().OperationID,
		Request: agentchatRequest("accepted", "conflicting follow up"), Options: options,
	})
	if !errors.Is(err, agent.ErrIdempotencyConflict) {
		t.Fatalf("conflicting command: %v", err)
	}
	if runtime.public.registration(key, "accepted") != accepted {
		t.Fatal("rejected retry replaced or removed an accepted route")
	}
	type admission struct {
		message string
		receipt agentrun.CommandReceipt
		err     error
	}
	results := make(chan admission, 8)
	t.Run("concurrent input retries", func(t *testing.T) {
		for i := range cap(results) {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				message := fmt.Sprintf("candidate %d", i%2)
				receipt, err := runtime.SubmitCommand(ctx, CommandRequest{
					Kind: CommandFollowUp, CommandID: "concurrent-input", OperationID: operation.Receipt().OperationID,
					Request: agentchatRequest("concurrent-input", message), Options: options,
				})
				results <- admission{message, receipt, err}
			})
		}
	})
	var admitted admission
	for range cap(results) {
		result := <-results
		if result.err != nil {
			if !errors.Is(result.err, agent.ErrIdempotencyConflict) {
				t.Fatalf("concurrent admission: %v", result.err)
			}
			continue
		}
		if admitted.message != "" && (result.message != admitted.message || result.receipt != admitted.receipt) {
			t.Fatal("one command accepted conflicting inputs")
		}
		admitted = result
	}
	route := runtime.public.registration(key, "concurrent-input")
	if admitted.message == "" || route == nil || route.request.Message != admitted.message {
		t.Fatal("routing does not match the durably accepted input")
	}
	if _, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandCancelQueued, CommandID: "cancel-concurrent", OperationID: operation.Receipt().OperationID,
		TargetCommandID: admitted.receipt.CommandID, Options: options,
	}); err != nil {
		t.Fatal(err)
	}
	close(model.release)
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "first answer" {
		t.Fatalf("accepted command: %+v", outcome)
	}
	_, err = runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandSteer, CommandID: "too-late", OperationID: operation.Receipt().OperationID,
		Request: agentchatRequest("too-late", "cannot steer a completed Run"), Options: options,
	})
	if !errors.Is(err, agent.ErrRunSettled) {
		t.Fatalf("late steering: %v", err)
	}
	runtime.public.mu.RLock()
	retained := len(runtime.public.registrations)
	runtime.public.mu.RUnlock()
	if retained != 0 {
		t.Fatalf("rejected commands retained %d registrations", retained)
	}
}
