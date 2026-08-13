package agent

import (
	"context"
	"encoding/json"
	"errors"
)

type goalToolInput struct {
	Action           GoalMutationKind `json:"action" jsonschema:"enum=complete,enum=block"`
	ExpectedID       string           `json:"expected_id"`
	ExpectedRevision uint64           `json:"expected_revision"`
	Report           string           `json:"report,omitempty" jsonschema:"maxLength=1048576"`
}

func standardGoalTool(manager GoalManager, session SessionView, run RunView) (ToolDefinition, error) {
	tool, err := InferTool("goal", "Complete the exact active Session goal only after the entire objective is achieved, or block it only when progress genuinely requires user input or an external state change. Never use it for an intermediate milestone. Goal creation, pause, resume, and clear remain host-owned.", func(ctx context.Context, input goalToolInput) (ToolResult, error) {
		if !IsRootInvocation(ctx) {
			return ToolResult{}, errors.New("Goal tool is available only in a root Agent invocation")
		}
		client := capabilityStateFromContext(ctx)
		if client == nil {
			return ToolResult{}, errors.New("Goal state client is unavailable")
		}
		if input.Action != GoalComplete && input.Action != GoalBlock {
			return ToolResult{}, errors.New("Goal tool may only complete or block the active Goal")
		}
		state, err := client.updateGoal(ctx, manager, session, run, GoalMutation{
			Kind: input.Action, ExpectedID: input.ExpectedID,
			ExpectedRevision: input.ExpectedRevision, Report: input.Report,
		})
		if err != nil {
			return ToolResult{}, err
		}
		stateHash, err := hashCanonical(state)
		if err != nil {
			return ToolResult{}, err
		}
		encoded, err := json.Marshal(struct {
			Status    GoalStatus `json:"status"`
			Revision  uint64     `json:"revision"`
			StateHash string     `json:"state_hash"`
		}{Status: state.Status, Revision: state.Revision, StateHash: stateHash})
		if err != nil {
			return ToolResult{}, err
		}
		return TextToolResult(string(encoded)), nil
	})
	if err != nil {
		return ToolDefinition{}, err
	}
	return ToolDefinition{Tool: tool, Descriptor: ToolDescriptor{
		Source: ToolSourceWrite, Execution: ToolExecutionSessionExclusive,
		MutationScope: ToolMutationSession, PostCheck: ToolPostCheckSessionState,
		Recovery: ToolRecoveryIdempotent, ResultProjection: ToolResultBoundedModelContext,
		ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent, MaxResultBytes: 128 << 10,
	}}, nil
}
