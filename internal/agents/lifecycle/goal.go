package lifecycle

import (
	"context"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/modelio"
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
	return agent.CapabilityIdentity{Kind: "denova.goal.standard", Version: 3}
}

func (manager denovaGoalManager) Apply(ctx context.Context, request agent.GoalApplyRequest) (agent.GoalState, error) {
	return manager.delegate.Apply(ctx, request)
}

func (manager denovaGoalManager) Prepare(ctx context.Context, request agent.GoalPrepareRequest) (agent.GoalPreparation, error) {
	return manager.delegate.Prepare(ctx, request)
}

func (manager denovaGoalManager) AfterRun(ctx context.Context, request agent.GoalAfterRunRequest) (agent.GoalAfterRunDecision, error) {
	decision, err := manager.delegate.AfterRun(modelio.WithTraceSource(ctx, "goal_evaluation"), request)
	if err != nil || decision.Verdict != agent.GoalVerdictContinue {
		return decision, err
	}
	data, err := DecodeTurnHostData(request.Input)
	if err != nil {
		return decision, fmt.Errorf("prepare Denova Goal continuation: %w", err)
	}
	message := strings.TrimSpace(decision.Input.Text)
	if message == "" {
		return decision, fmt.Errorf("prepare Denova Goal continuation: message is empty")
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
		return decision, fmt.Errorf("prepare Denova Goal continuation input: %w", err)
	}
	// Agent assigns the durable command identity for automatic continuations.
	next.IdempotencyKey = ""
	decision.Input = next
	return decision, nil
}

var _ agent.GoalManager = denovaGoalManager{}
