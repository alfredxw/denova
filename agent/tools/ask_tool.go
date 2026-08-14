package tools

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
	"github.com/invopop/jsonschema"
)

type askInput struct {
	Questions []agent.InteractionQuestion `json:"questions" jsonschema:"minItems=1,maxItems=3" jsonschema_description:"Bilingual questions shown together in one interaction."`
}

type askFreeTextQuestionSchema struct {
	ID            string              `json:"id" jsonschema:"required,minLength=1,maxLength=256,pattern=^[A-Za-z0-9][A-Za-z0-9._:-]*$" jsonschema_description:"Stable question ID used to correlate the answer."`
	Prompt        agent.LocalizedText `json:"prompt" jsonschema:"required" jsonschema_description:"The same user-facing question in Chinese and English."`
	AllowFreeText bool                `json:"allow_free_text" jsonschema:"required" jsonschema_description:"Always true for a free-text question."`
}

type askChoiceQuestionSchema struct {
	ID       string                    `json:"id" jsonschema:"required,minLength=1,maxLength=256,pattern=^[A-Za-z0-9][A-Za-z0-9._:-]*$" jsonschema_description:"Stable question ID used to correlate the answer."`
	Prompt   agent.LocalizedText       `json:"prompt" jsonschema:"required" jsonschema_description:"The same user-facing question in Chinese and English."`
	Options  []agent.InteractionOption `json:"options" jsonschema:"required,minItems=2,maxItems=3" jsonschema_description:"Mutually exclusive choices. Mark exactly one recommended; the host adds Other automatically."`
	Multiple bool                      `json:"multiple,omitempty" jsonschema_description:"Allow selection of more than one listed option."`
}

type askRecommendedOptionSchema struct {
	Recommended bool `json:"recommended" jsonschema:"required"`
}

// Ask returns the standard durable user-interaction Toolset. The tool has no
// terminal/HTTP dependency; it suspends through the current Run interaction
// client and resumes with a validated typed answer.
func Ask() agent.Toolset {
	return defineToolset(func(context.Context) (agent.Toolset, error) {
		return buildAsk()
	})
}

func buildAsk() (agent.Toolset, error) {
	rootSchema, err := askToolSchema()
	if err != nil {
		return nil, err
	}
	tool, err := newSchemaTool(
		"ask", "Ask one to three bilingual questions when required input cannot be inferred.", rootSchema,
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
		Presentation:   agent.UniformToolPresentation(agent.ToolPresentationInteraction),
	}}
	return agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.ask", Version: 2}, definition)
}

func askToolSchema() (*jsonschema.Schema, error) {
	root, err := reflectedToolSchema[askInput]()
	if err != nil {
		return nil, fmt.Errorf("build ask schema: %w", err)
	}
	questions, ok := root.Properties.Get("questions")
	if !ok || questions == nil {
		return nil, errors.New("build ask schema: questions property is missing")
	}
	freeText, err := reflectedToolSchema[askFreeTextQuestionSchema]()
	if err != nil {
		return nil, err
	}
	allowFreeText, ok := freeText.Properties.Get("allow_free_text")
	if !ok || allowFreeText == nil {
		return nil, errors.New("build ask schema: allow_free_text property is missing")
	}
	allowFreeText.Const = true

	choice, err := reflectedToolSchema[askChoiceQuestionSchema]()
	if err != nil {
		return nil, err
	}
	options, ok := choice.Properties.Get("options")
	if !ok || options == nil {
		return nil, errors.New("build ask schema: options property is missing")
	}
	recommended, err := reflectedToolSchema[askRecommendedOptionSchema]()
	if err != nil {
		return nil, err
	}
	recommendedFlag, ok := recommended.Properties.Get("recommended")
	if !ok || recommendedFlag == nil {
		return nil, errors.New("build ask schema: recommended property is missing")
	}
	recommendedFlag.Const = true
	one := uint64(1)
	options.Contains = recommended
	options.MinContains = &one
	options.MaxContains = &one
	questions.Items = &jsonschema.Schema{
		OneOf:       []*jsonschema.Schema{freeText, choice},
		Description: "Choose a free-text question or a choice question. Do not mix their fields.",
	}
	return root, nil
}
