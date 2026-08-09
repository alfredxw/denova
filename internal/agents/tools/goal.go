package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/goal"
)

type goalFinishInput struct {
	Outcome string `json:"outcome" jsonschema:"required,enum=completed,enum=blocked" jsonschema_description:"Use completed only when the objective is fully achieved; use blocked only when progress requires user input or an external state change."`
	Report  string `json:"report,omitempty" jsonschema_description:"Concise final result or the exact blocking condition."`
}

// NewGoalFinish creates the root-only completion seam for one captured goal
// revision. A stale runner can never finish a replacement goal.
func NewGoalFinish(store goal.Store, captured goal.State) (agent.ToolDefinition, error) {
	tool, err := agent.InferTool(
		"goal_finish",
		"Finish the active durable goal. Call this only when the complete objective is achieved, or progress is genuinely blocked on user input or an external state change. Do not call it for an intermediate step.",
		func(ctx context.Context, input goalFinishInput) (agent.ToolResult, error) {
			if !agent.IsRootInvocation(ctx) {
				return agent.ToolResult{}, errors.New("goal_finish is available only in a root Agent invocation")
			}
			if store == nil || !captured.IsActive() {
				return agent.ToolResult{}, errors.New("goal_finish is unavailable without an active goal")
			}
			outcome := goal.Status(strings.TrimSpace(input.Outcome))
			next, err := store.FinishGoal(ctx, captured.ID, captured.Revision, outcome, input.Report)
			if err != nil {
				return agent.ToolResult{}, err
			}
			payload, err := json.Marshal(next)
			if err != nil {
				return agent.ToolResult{}, err
			}
			result := agent.TextToolResult(string(payload))
			result.Details = payload
			return result, nil
		},
	)
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	return defineTool(tool, agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: config.AgentToolGoal,
		Execution:        agent.ToolExecutionSessionExclusive,
		MutationScope:    agent.ToolMutationSession,
		PostCheck:        agent.ToolPostCheckSessionState,
		Recovery:         agent.ToolRecoveryIdempotent,
		ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention:  agent.ToolResultProtected,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   defaultToolResultMaxBytes,
	})
}
