package lifecycle

import (
	"context"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
	publicgoal "github.com/alfredxw/denova/agent/goal"
)

// NewGoalManager returns Denova's adapter for the public revisioned Goal
// capability. The public manager remains the sole state authority; this
// adapter only gives autonomous continuations valid product HostData.
func NewGoalManager() agent.GoalManager {
	return denovaGoalManager{delegate: publicgoal.Standard()}
}

type denovaGoalManager struct{ delegate agent.GoalManager }

func (manager denovaGoalManager) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "denova.goal.standard", Version: 1}
}

func (manager denovaGoalManager) Apply(ctx context.Context, request agent.GoalApplyRequest) (agent.GoalState, error) {
	return manager.delegate.Apply(ctx, request)
}

func (manager denovaGoalManager) Prepare(ctx context.Context, request agent.GoalPrepareRequest) (agent.GoalPreparation, error) {
	return manager.delegate.Prepare(ctx, request)
}

func (manager denovaGoalManager) AfterRun(ctx context.Context, request agent.GoalAfterRunRequest) (agent.GoalContinuation, error) {
	continuation, err := manager.delegate.AfterRun(ctx, request)
	if err != nil || !continuation.Continue {
		return continuation, err
	}
	data, err := DecodeTurnHostData(request.Input)
	if err != nil {
		return agent.GoalContinuation{}, fmt.Errorf("prepare Denova Goal continuation: %w", err)
	}
	message := strings.TrimSpace(continuation.Input.Text)
	if message == "" {
		return agent.GoalContinuation{}, fmt.Errorf("prepare Denova Goal continuation: message is empty")
	}
	// Preserve only locale and durable routing. References, selections, review
	// feedback, and explicit Skills belonged to the completed caller turn and
	// must not be replayed as a new autonomous instruction.
	next, err := TurnInput(TurnNext, agentchat.ChatRequest{
		Message: message, Locale: data.Caller.Locale,
		InputVisibility: agentrun.InputModelOnly,
	}, agentrun.Options{
		AutomationTaskID: data.AutomationID,
		TurnID:           data.TurnID,
		MaintenanceTask:  data.MaintenanceTask,
		Mode:             data.Mode,
		WriteMode:        data.WriteMode,
		WriteScope:       data.WriteScope,
		RestoreData:      data.RestoreData,
	})
	if err != nil {
		return agent.GoalContinuation{}, fmt.Errorf("prepare Denova Goal continuation input: %w", err)
	}
	// Agent assigns the durable command identity for automatic continuations.
	next.IdempotencyKey = ""
	continuation.Input = next
	return continuation, nil
}

var _ agent.GoalManager = denovaGoalManager{}
