package interactiveapp

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/interactive"
)

func TestGameSubmissionDoesNotInterruptAcceptedNativeGuidance(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "test"}}
	turn := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "opening", Message: "Open the gate"}, gameAdapterFunc(func(ctx context.Context, _ external.Input, host external.Host) (external.Result, error) {
		pending := true
		controls := external.Steering{Next: func(context.Context) (external.Guidance, bool, error) {
			return external.Guidance{Request: agentchat.ChatRequest{CommandID: "late-guidance", Message: "Check the gate"}, Count: 1}, pending, nil
		}, Delivered: func(external.Guidance) { pending = false }}
		if err := controls.Deliver(ctx, host, func(context.Context, external.Input) error { return nil }); err != nil {
			return external.Result{}, err
		}
		if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "The gate opens."}}); err != nil {
			return external.Result{}, err
		}
		gameHostCall(t, ctx, host, "state", "submit_interactive_turn", gameStateArgs)
		gameHostCall(t, ctx, host, "choices", "submit_interactive_turn", `{"choices":["Enter","Observe","Listen","Inspect","Wait"]}`)
		if ctx.Err() != nil {
			t.Error("submission interrupted native guidance before the provider could consume it")
		}
		return external.Result{Settled: true}, ctx.Err()
	}))
	if outcome := turn.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome: %+v", outcome)
	}
	snapshot, err := store.Snapshot(story, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range snapshot.CurrentTurn.DisplayEvents {
		if event.Role == "tool_call" && (event.Status != "success" || event.Result == "") {
			t.Fatalf("committed tool is still pending after reload: %+v", event)
		}
	}
}

func TestGameNativeSteeringSurvivesInterruptedTurn(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "test"}}
	guidance := agentchat.ChatRequest{CommandID: "live-guidance", Message: "Keep the gate intact"}
	first := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "opening", Message: "Open the gate"}, gameAdapterFunc(func(ctx context.Context, _ external.Input, host external.Host) (external.Result, error) {
		pending := true
		controls := external.Steering{Next: func(context.Context) (external.Guidance, bool, error) {
			return external.Guidance{Request: guidance, Count: 1}, pending, nil
		}, Delivered: func(external.Guidance) { pending = false }}
		if err := controls.Deliver(ctx, host, func(_ context.Context, input external.Input) error {
			if input.Text != guidance.Message {
				return errors.New("native Game guidance changed")
			}
			return nil
		}); err != nil {
			return external.Result{}, err
		}
		return external.Result{}, errors.New("connection lost after steering")
	}))
	if outcome := first.Wait(t.Context()); outcome.Status != agentrun.OutcomeFailed || first.ConsumedGuidance() != 1 {
		t.Fatalf("initial outcome: %+v", outcome)
	}
	interruption, err := ExternalTurnInterruption(store, story, "main")
	if err != nil || interruption == nil {
		t.Fatalf("interruption: %+v %v", interruption, err)
	}
	resumed := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "resume", ResumeInterruptionID: interruption.ID}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
		count := 0
		for _, message := range input.History {
			if message.Text == guidance.Message {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("restored guidance count=%d", count)
		}
		if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "The gate opens intact."}}); err != nil {
			return external.Result{}, err
		}
		gameHostCall(t, ctx, host, "state", "submit_interactive_turn", gameStateArgs)
		gameHostCall(t, ctx, host, "choices", "submit_interactive_turn", `{"choices":["Enter","Observe","Listen","Inspect","Wait"]}`)
		return external.Result{}, ctx.Err()
	}))
	resumed.config.Guidance = []agentchat.ChatRequest{guidance}
	if outcome := resumed.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("resumed outcome: %+v", outcome)
	}
	snapshot, err := store.Snapshot(story, "main")
	if err != nil || snapshot.TurnCount != 1 {
		t.Fatalf("turn commit: %+v %v", snapshot, err)
	}
	count := 0
	for _, message := range schemaMessagesFromInteractiveContext(snapshot.CurrentTurn.ModelContextMessages) {
		if interactive.UserGuidanceCommand(message) == guidance.CommandID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("canonical turn duplicated guidance: %d", count)
	}
}
