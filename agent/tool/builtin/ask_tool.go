package builtin

import (
	"context"
	"errors"

	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentinteraction "github.com/alfredxw/denova/agent/lifecycle/interaction"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agenttool "github.com/alfredxw/denova/agent/tool"
)

type askInput struct {
	Questions []askQuestionInput `json:"questions" jsonschema:"minItems=1,maxItems=3" jsonschema_description:"Questions shown together. Write visible text in the user's language."`
}

// The host derives the interaction's free-text flag from the optional choices.
// Keeping it out of model input avoids two competing ways to select a question type.
type askQuestionInput struct {
	ID       string                               `json:"id" jsonschema:"minLength=1,maxLength=256,pattern=^[A-Za-z0-9][A-Za-z0-9._:-]*$" jsonschema_description:"Stable question ID used to correlate the answer."`
	Prompt   string                               `json:"prompt" jsonschema:"minLength=1,maxLength=8192" jsonschema_description:"User-facing question in the user's language."`
	Options  []agentinteraction.InteractionOption `json:"options,omitempty" jsonschema:"maxItems=4" jsonschema_description:"Omit or use an empty array for free text. Otherwise provide two to four distinct choices, with exactly one recommended. The host adds Other automatically."`
	Multiple bool                                 `json:"multiple,omitempty" jsonschema_description:"Allow multiple listed choices. Must be false or omitted for a free-text question."`
}

// Ask returns the standard durable user-interaction Toolset. The tool has no
// terminal/HTTP dependency; it suspends through the current Run interaction
// client and resumes with a validated typed answer.
func Ask() agenttool.Toolset {
	return defineToolset(func(context.Context) (agenttool.Toolset, error) {
		return buildAsk()
	})
}

func buildAsk() (agenttool.Toolset, error) {
	tool, err := agenttool.InferTool(
		"ask", "Ask one to three questions when required input cannot be inferred. For choice questions, prefer two or three concise options; use four only when every option is materially different. Set recommended=true on exactly one option per choice question. Write questions, options, and descriptions in the same language as the user's current input.",
		func(ctx context.Context, input askInput) (agentschema.ToolResult, error) {
			if !agentexecution.IsRootInvocation(ctx) {
				return agentschema.ToolResult{}, errors.New("ask is available only in a root Agent invocation")
			}
			executionID := agentexecution.CurrentToolExecutionID(ctx)
			if executionID == "" {
				return agentschema.ToolResult{}, errors.New("ask requires a durable tool execution ID")
			}
			questions := make([]agentinteraction.InteractionQuestion, len(input.Questions))
			for index, question := range input.Questions {
				questions[index] = agentinteraction.InteractionQuestion{
					ID: question.ID, Prompt: question.Prompt,
					Options: question.Options, Multiple: question.Multiple,
					AllowFreeText: len(question.Options) == 0,
				}
			}
			resolution, err := agentinteraction.RequestInteraction(ctx, agentinteraction.InteractionRequest{
				ID: "ask-" + executionID, Kind: agentinteraction.InteractionAsk,
				Questions: questions,
				// Other is host-provided on every standard Ask. Models should offer
				// concise choices without guessing whether free text is needed.
				AllowOther: true,
			})
			if err != nil {
				return agentschema.ToolResult{}, err
			}
			return JSONResult(resolution)
		},
	)
	if err != nil {
		return nil, err
	}
	definition := agenttool.ToolDefinition{Tool: tool, Descriptor: agenttool.ToolDescriptor{
		Source: agenttool.ToolSourceOther, Capability: "ask", Execution: agenttool.ToolExecutionInteractiveWait,
		MutationScope: agenttool.ToolMutationNone, PostCheck: agenttool.ToolPostCheckNone,
		Recovery: agenttool.ToolRecoveryReadOnly, ResultProjection: agentschema.ToolResultBoundedModelContext,
		ResultRetention: agentschema.ToolResultProtected, Steering: agenttool.SteeringInterruptibleWait,
		MaxResultBytes: 256 << 10,
		Presentation:   agenttool.UniformToolPresentation(agenttool.ToolPresentationInteraction),
	}}
	return agenttool.StaticToolsIdentified(agentschema.CapabilityIdentity{Kind: "tools.ask", Version: 4}, definition)
}
