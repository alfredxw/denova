package tools

import (
	"context"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

type askInput struct {
	Questions  []agent.InteractionQuestion `json:"questions" jsonschema:"minItems=1,maxItems=3"`
	AllowOther bool                        `json:"allow_other,omitempty"`
}

// Ask returns the standard durable user-interaction Toolset. The tool has no
// terminal/HTTP dependency; it suspends through the current Run interaction
// client and resumes with a validated typed answer.
func Ask() (agent.Toolset, error) {
	tool, err := agent.InferTool(
		"ask",
		"Ask the user one to three bilingual questions when required information cannot be inferred safely.\n\n当无法安全推断必要信息时，向用户提出一到三个双语问题。",
		func(ctx context.Context, input askInput) (agent.ToolResult, error) {
			executionID := agent.CurrentToolExecutionID(ctx)
			if executionID == "" {
				return agent.ToolResult{}, errors.New("ask requires a durable tool execution ID")
			}
			resolution, err := agent.RequestInteraction(ctx, agent.InteractionRequest{
				ID: "ask-" + executionID, Kind: agent.InteractionAsk,
				Questions:  append([]agent.InteractionQuestion(nil), input.Questions...),
				AllowOther: input.AllowOther,
			})
			if err != nil {
				return agent.ToolResult{}, err
			}
			return JSONResult(resolution)
		},
	)
	if err != nil {
		return nil, err
	}
	definition := agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Execution: agent.ToolExecutionInteractiveWait,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringInterruptibleWait,
		MaxResultBytes: 256 << 10,
	}}
	return agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.ask", Version: 1}, definition), nil
}
