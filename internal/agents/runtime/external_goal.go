package agentruntime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
	publicgoal "github.com/alfredxw/denova/agent/goal"
)

func (run *ExternalRun) afterCommit(ctx context.Context, runtime *external.RuntimeSession) error {
	if !run.goal.Active() || runtime == nil {
		return nil
	}
	control := run.control
	control.mu.Lock()
	state, err := control.read(ctx)
	control.mu.Unlock()
	if err != nil {
		return err
	}
	if state.Phase != agentrun.RunPhaseRunning || len(state.Queue) != 0 || (state.Current != nil && len(state.Current.Guidance) > run.guidance) {
		return nil
	}
	goal, present, err := control.store.Goal(ctx)
	if err != nil || !present || goal.ID != run.goal.ID || goal.Revision != run.goal.Revision || !goal.Active() {
		return err
	}
	verdict, err := evaluateExternalGoal(ctx, func(ctx context.Context, prompt string) (*agent.Message, error) {
		result, err := runtime.Evaluate(ctx, prompt)
		return agent.AssistantMessage(result.Text, nil), err
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		run.goalEvaluated = true
		slog.WarnContext(ctx, "External Goal evaluation failed", "operation_id", run.receipt.OperationID, "error", err)
		run.send(agentrun.Event{Type: "goal_evaluation_failed", Data: map[string]any{"code": "goal_evaluation_failed", "goal_id": goal.ID, "goal_revision": goal.Revision}})
		return nil
	}
	control.mu.Lock()
	state, err = control.read(ctx)
	if err != nil || state.Phase != agentrun.RunPhaseRunning || len(state.Queue) != 0 || (state.Current != nil && len(state.Current.Guidance) > run.guidance) {
		control.mu.Unlock()
		return err
	}
	if verdict.Verdict == "continue" {
		err = control.update(ctx, func(current *externalControlState) error {
			current.Queue = append(current.Queue, ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "goal-" + rand.Text(), Message: verdict.Input.Text, InputVisibility: agentrun.InputModelOnly}, Delivery: agentrun.DeliveryNextTurn, GoalID: goal.ID, GoalRevision: goal.Revision})
			return nil
		})
		control.mu.Unlock()
		run.goalEvaluated = err == nil
		return err
	}
	kind := agent.GoalComplete
	if verdict.Verdict == "blocked" {
		kind = agent.GoalBlock
	}
	updated, err := control.store.UpdateGoal(ctx, agent.GoalMutation{Kind: kind, ExpectedID: goal.ID, ExpectedRevision: goal.Revision, Report: verdict.Reason, MutationID: fmt.Sprintf("%s-evaluate-%d", run.receipt.OperationID, goal.Revision)})
	control.mu.Unlock()
	if errors.Is(err, publicgoal.ErrRevisionConflict) {
		return nil
	}
	if err != nil {
		return err
	}
	run.goalEvaluated = true
	data, _ := json.Marshal(updated)
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	payload["schema"], payload["present"] = "agent.goal.v1", true
	run.send(agentrun.Event{Type: "goal_updated", Data: payload})
	return nil
}
