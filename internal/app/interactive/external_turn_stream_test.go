package interactiveapp

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	apptask "denova/internal/app/task"
	agent "github.com/alfredxw/denova/agent"
)

func TestExternalPlanSurvivesInterruptedGameDraft(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "fixture"}}
	turn := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "plan", Message: "Explore"}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
		if err := host.Emit(external.PlanEvent([]agent.TodoItem{{ID: "1", Text: "Inspect the gate", Status: agent.TodoInProgress}})); err != nil {
			return external.Result{}, err
		}
		return external.Result{}, errors.New("fixture disconnect")
	}))
	if result := turn.Wait(t.Context()); result.Status != agentrun.OutcomeFailed {
		t.Fatalf("outcome=%+v", result)
	}
	snapshot, err := store.Snapshot(story, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingDisplayEvents) != 1 || snapshot.PendingDisplayEvents[0].Role != "todo_updated" {
		t.Fatalf("missing draft plan: %+v", snapshot.PendingDisplayEvents)
	}
}

func TestExternalGameCycleStartedReplaysAcceptedMessage(t *testing.T) {
	for _, selection := range []config.RuntimeSelection{
		{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "test"}},
		{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}},
	} {
		t.Run(string(selection.Kind), func(t *testing.T) {
			store, story, cfg := externalGameFixture(t)
			cfg.ActiveAgentRuntime = &selection
			const message = "Open the gate"
			request := agentchat.ChatRequest{CommandID: "start", Message: message}
			var operationID agentrun.OperationID
			for _, phase := range []string{"start", "resume"} {
				t.Run(phase, func(t *testing.T) {
					task, err := apptask.NewDeferred(nil)
					if err != nil {
						t.Fatal(err)
					}
					defer task.Finish()
					var turn *ExternalTurn
					turn = externalGameForTest(t, store, story, cfg, request, gameAdapterFunc(func(context.Context, external.Input, external.Host) (external.Result, error) {
						// Check the actual Task replay before the provider produces any
						// output, including the original player input after a resume.
						replay, subscription, err := task.SubscribeDisplayAfter(0)
						if err != nil {
							t.Fatal(err)
						}
						defer task.Unsubscribe(subscription)
						if len(replay.Events) != 1 || replay.Events[0].Event.Type != "agent_cycle_started" {
							t.Fatalf("missing cycle start: %+v", replay)
						}
						start := replay.Events[0].Event
						if _, err := time.Parse(time.RFC3339Nano, start.DataString("run_started_at")); err != nil {
							t.Fatalf("invalid cycle start timestamp: %v", err)
						}
						id := string(turn.OperationID())
						want := map[string]any{
							"id": id + "-output", "command_id": request.CommandID,
							"delivery": "start_turn", "message": message,
							"run_id": id, "operation_id": id, "cycle": 1,
							"run_started_at": start.DataString("run_started_at"),
						}
						if !reflect.DeepEqual(start.Data, want) {
							t.Errorf("cycle start payload = %#v, want %#v", start.Data, want)
						}
						return external.Result{}, errors.New("provider disconnected")
					}))
					turn.config.Emit = task.Emit
					if phase == "resume" && turn.OperationID() != operationID {
						t.Fatalf("resume replaced operation %q with %q", operationID, turn.OperationID())
					}
					operationID = turn.OperationID()
					if outcome := turn.Wait(t.Context()); outcome.Status != agentrun.OutcomeFailed {
						t.Fatalf("expected provider interruption: %+v", outcome)
					}
				})
				pending, err := ExternalTurnInterruption(store, story, "main")
				if err != nil || pending == nil {
					t.Fatalf("missing interrupted turn: %+v, %v", pending, err)
				}
				request = agentchat.ChatRequest{CommandID: "resume", Message: "Continue", ResumeInterruptionID: pending.ID}
			}
		})
	}
}
