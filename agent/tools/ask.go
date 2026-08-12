package tools

import (
	"context"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

type askInput struct {
	Questions []agent.InteractionQuestion `json:"questions" jsonschema:"minItems=1,maxItems=3"`
}

// Ask returns the standard durable user-interaction Toolset. The tool has no
// terminal/HTTP dependency; it suspends through the current Run interaction
// client and resumes with a validated typed answer.
func Ask() (agent.Toolset, error) {
	tool, err := agent.InferTool(
		"ask",
		"Ask the user one to three bilingual questions when required information cannot be inferred safely.\n\n当无法安全推断必要信息时，向用户提出一到三个双语问题。",
		func(ctx context.Context, input askInput) (agent.ToolResult, error) {
			if !agent.IsRootInvocation(ctx) {
				return agent.ToolResult{}, errors.New("ask is available only in a root Agent invocation")
			}
			executionID := agent.CurrentToolExecutionID(ctx)
			if executionID == "" {
				return agent.ToolResult{}, errors.New("ask requires a durable tool execution ID")
			}
			questions := append([]agent.InteractionQuestion(nil), input.Questions...)
			for index := range questions {
				questions[index].Options = append([]agent.InteractionOption(nil), questions[index].Options...)
				if len(questions[index].Options) == 0 {
					questions[index].AllowFreeText = true
				}
			}
			resolution, err := agent.RequestInteraction(ctx, agent.InteractionRequest{
				ID: "ask-" + executionID, Kind: agent.InteractionAsk,
				Questions: questions,
				// Other is host-provided on every standard Ask. Models should offer
				// concise choices without guessing whether free text is needed.
				AllowOther: true,
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
		Source: agent.ToolSourceOther, Capability: "ask", Execution: agent.ToolExecutionInteractiveWait,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringInterruptibleWait,
		MaxResultBytes: 256 << 10,
	}}
	return agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.ask", Version: 2}, definition)
}
