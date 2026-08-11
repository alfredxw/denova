package agent

import (
	"context"
	"encoding/json"
	"errors"
)

type goalToolInput struct {
	Action           GoalMutationKind `json:"action" jsonschema:"enum=set,enum=pause,enum=resume,enum=complete,enum=block,enum=clear"`
	Objective        string           `json:"objective,omitempty" jsonschema:"maxLength=65536"`
	ExpectedID       string           `json:"expected_id,omitempty"`
	ExpectedRevision uint64           `json:"expected_revision,omitempty"`
	Report           string           `json:"report,omitempty" jsonschema:"maxLength=1048576"`
}

func standardGoalTool(manager GoalManager, session SessionView, run RunView) (ToolDefinition, error) {
	tool, err := InferTool("goal", "Set, pause, resume, complete, block, or clear the durable Session goal using exact revision fences.\n\n使用精确版本约束来设置、暂停、恢复、完成、阻塞或清除持久化会话目标。", func(ctx context.Context, input goalToolInput) (ToolResult, error) {
		client := capabilityStateFromContext(ctx)
		if client == nil {
			return ToolResult{}, errors.New("Goal state client is unavailable")
		}
		state, err := client.updateGoal(ctx, manager, session, run, GoalMutation{
			Kind: input.Action, Objective: input.Objective, ExpectedID: input.ExpectedID,
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
