package agentruntime

import (
	"context"
	"encoding/json"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

type controlAdapter func(context.Context, external.Input, external.Host) (external.Result, error)

func (controlAdapter) Version() string { return "control-fixture-1" }
func (adapter controlAdapter) Run(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
	return adapter(ctx, input, host)
}

func TestExternalGoalResumesAfterPauseDuringCommittedEvaluation(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	goal, err := state.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalSet, Objective: "Finish and verify the draft"})
	if err != nil {
		t.Fatal(err)
	}
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	evaluations, cycles := 0, 0
	adapter := controlAdapter(func(ctx context.Context, input external.Input, _ external.Host) (external.Result, error) {
		if input.Mode != external.OperationEvaluate {
			return external.Result{SessionID: "primary", Settled: true}, nil
		}
		evaluations++
		if evaluations == 1 {
			status, err := control.Status(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = control.Submit(ctx, Command{Kind: agentexecution.CommandSuspend, CommandID: "pause-evaluation", OperationID: status.ActiveOperation})
			if err != nil {
				t.Fatal(err)
			}
			return external.Result{}, ctx.Err()
		}
		return external.Result{Text: `{"verdict":"complete","reason":"Verified the saved draft","next_instruction":""}`, Settled: true}, nil
	})
	runtime := &external.Runtime{CacheRoot: t.TempDir(), Acquire: func(context.Context) (external.Adapter, func(), error) { return adapter, func() {}, nil }}
	factory := func(_ context.Context, input ExternalCycleInput, _ func(agentrun.Event), after func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		cycles++
		if cycles == 2 && (input.Request.InputVisibility != agentrun.InputModelOnly || input.GoalID != goal.ID || input.Request.CommandID == "original") {
			t.Fatalf("resume repeated committed user input: %+v", input)
		}
		return externalCycleFunc(func(ctx context.Context) agentrun.Outcome {
			response, err := runtime.Run(ctx, external.SessionRequest{Key: "paused-goal", Input: external.Input{Text: input.Request.Message}}, &external.MaintenanceObserver{})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Session.Close()
			if err := after(ctx, response.Session); err != nil {
				t.Fatal(err)
			}
			return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
		}), nil
	}
	run, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "original", Message: "Write the draft"}}, factory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := run.Wait(t.Context()); outcome.Status != agentrun.OutcomeSuspended {
		t.Fatalf("pause: %+v", outcome)
	}
	status, err := control.Status(t.Context())
	if err != nil || len(status.Queue) != 1 {
		t.Fatalf("missing deferred Goal work: %+v, %v", status, err)
	}
	resumed, err := control.Resume(t.Context(), agentexecution.RuntimeRecoveryActions(status)[0], factory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := resumed.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("resume: %+v", outcome)
	}
	final, _, err := state.Goal(t.Context())
	if err != nil || final.Active() || evaluations != 2 || cycles != 2 {
		t.Fatalf("Goal not settled: %+v, evaluations=%d cycles=%d err=%v", final, evaluations, cycles, err)
	}
}

func TestExternalGoalContinuesThenAcceptsReadOnlyVerdict(t *testing.T) {
	for _, terminal := range []string{"complete", "blocked"} {
		t.Run(terminal, func(t *testing.T) {
			_, _, state, options, _ := controlFixture(t)
			if _, err := state.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalSet, Objective: "Complete and verify the draft"}); err != nil {
				t.Fatal(err)
			}
			engines := NewEngines()
			defer engines.Close()
			control, err := engines.ExternalControl(options, state)
			if err != nil {
				t.Fatal(err)
			}
			evaluations, cycles := 0, 0
			adapter := controlAdapter(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
				if input.Mode != external.OperationEvaluate {
					return external.Result{SessionID: "primary", Settled: true}, nil
				}
				evaluations++
				if len(input.Tools) != 0 || len(input.History) != 0 || input.SessionID != "primary" {
					t.Fatalf("evaluation was not an isolated context fork: %+v", input)
				}
				result, err := host.CallTool(ctx, external.ToolCall{Name: "write"})
				if err == nil && result.Success {
					t.Fatal("Goal evaluator could write product data")
				}
				verdict, next := terminal, ""
				if evaluations == 1 {
					verdict, next = "continue", "Verify the saved draft"
				}
				body, _ := json.Marshal(map[string]string{"verdict": verdict, "reason": "Checked the confirmed work", "next_instruction": next})
				return external.Result{Text: string(body), SessionID: "evaluation-fork", Settled: true}, nil
			})
			runtime := &external.Runtime{CacheRoot: t.TempDir(), Acquire: func(context.Context) (external.Adapter, func(), error) { return adapter, func() {}, nil }}
			factory := func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), after func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
				cycles++
				if cycles == 2 && (input.Request.Message != "Verify the saved draft" || input.Request.InputVisibility != agentrun.InputModelOnly) {
					t.Fatalf("invalid Goal continuation: %+v", input)
				}
				return externalCycleFunc(func(ctx context.Context) agentrun.Outcome {
					response, err := runtime.Run(ctx, external.SessionRequest{Key: "goal", Input: external.Input{Text: input.Request.Message}}, &external.MaintenanceObserver{})
					if err != nil {
						t.Fatal(err)
					}
					defer response.Session.Close()
					if err := after(ctx, response.Session); err != nil {
						t.Fatal(err)
					}
					return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
				}), nil
			}
			run, err := control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "goal-start", Message: "Write the draft"}}, factory, nil)
			if err != nil {
				t.Fatal(err)
			}
			if outcome := run.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("Goal task: %+v", outcome)
			}
			goal, _, err := state.Goal(t.Context())
			if err != nil || goal.Active() || cycles != 2 || evaluations != 2 {
				t.Fatalf("Goal did not settle: %+v cycles=%d evaluations=%d err=%v", goal, cycles, evaluations, err)
			}
		})
	}
}
