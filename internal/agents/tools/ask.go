package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

type askOptionInput struct {
	ID          string `json:"id" jsonschema:"required" jsonschema_description:"Stable option ID."`
	Label       string `json:"label" jsonschema:"required" jsonschema_description:"Short display label."`
	Description string `json:"description,omitempty" jsonschema_description:"Optional one-sentence explanation of the choice and its tradeoff."`
}

type askQuestionInput struct {
	ID                  string           `json:"id" jsonschema:"required" jsonschema_description:"Stable question ID."`
	Question            string           `json:"question" jsonschema:"required" jsonschema_description:"Question shown to the user."`
	Options             []askOptionInput `json:"options,omitempty" jsonschema_description:"Provide 2-3 choices, or omit for a free-text question. Other is added automatically."`
	MultiSelect         bool             `json:"multi_select,omitempty" jsonschema_description:"Allow more than one choice for this question."`
	RecommendedOptionID string           `json:"recommended_option_id,omitempty" jsonschema_description:"ID of the recommended choice; the UI marks it automatically."`
}

type askInput struct {
	Questions []askQuestionInput `json:"questions" jsonschema:"required,minItems=1,maxItems=3" jsonschema_description:"One to three questions. IDs must be unique."`
}

// AskInteraction is the host seam behind the model-visible ask tool. A host
// implementation must persist pending state before it blocks.
type AskInteraction interface {
	Ask(context.Context, session.AskInteraction) (session.AskResolution, error)
}

type askInteractionContextKey struct{}

// ContextWithAskInteraction binds one durable interactive host to a top-level
// Agent run. Catalog registration alone never invents a UI host.
func ContextWithAskInteraction(ctx context.Context, interaction AskInteraction) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if interaction == nil {
		return ctx
	}
	return context.WithValue(ctx, askInteractionContextKey{}, interaction)
}

func askInteractionFromContext(ctx context.Context) (AskInteraction, bool) {
	if ctx == nil || !agent.IsRootInvocation(ctx) {
		return nil, false
	}
	interaction, ok := ctx.Value(askInteractionContextKey{}).(AskInteraction)
	return interaction, ok && interaction != nil
}

func newAskTool() (agent.ToolDefinition, error) {
	tool, err := agent.InferTool("ask", "Pause this top-level interactive run to collect one to three user decisions. Each choice question must provide 2-3 options; omit options for free text. The UI adds Other automatically. There is no default timeout, and cancellation is returned as a structured result.", func(ctx context.Context, input askInput) (agent.ToolResult, error) {
		// ask controls the root transcript and blocks its interactive host. It
		// must fail closed even if a child catalog accidentally registers the
		// tool or a host value is explicitly attached to a child Context.
		if !agent.IsRootInvocation(ctx) {
			return agent.ToolResult{}, errors.New("ask is available only in a root Agent invocation")
		}
		interaction, ok := askInteractionFromContext(ctx)
		if !ok {
			return agent.ToolResult{}, errors.New("ask is unavailable without an interactive host")
		}
		callID := strings.TrimSpace(agent.ToolCallID(ctx))
		if callID == "" {
			return agent.ToolResult{}, errors.New("ask requires a stable tool call id")
		}
		executionID := agent.ToolExecutionID(ctx, callID)
		if executionID == "" {
			return agent.ToolResult{}, errors.New("ask requires a stable tool execution id")
		}
		questions := make([]session.AskQuestion, 0, len(input.Questions))
		for _, question := range input.Questions {
			options := make([]session.AskOption, 0, len(question.Options))
			for _, option := range question.Options {
				options = append(options, session.AskOption{ID: option.ID, Label: option.Label, Description: option.Description})
			}
			questions = append(questions, session.AskQuestion{
				ID: question.ID, Question: question.Question, Options: options,
				MultiSelect: question.MultiSelect, RecommendedOptionID: question.RecommendedOptionID,
			})
		}
		resolution, err := interaction.Ask(ctx, session.AskInteraction{
			ID: executionID, ToolCallID: executionID, ProviderCallID: callID, Questions: questions,
		})
		if err != nil {
			return agent.ToolResult{}, err
		}
		payload, err := json.Marshal(resolution)
		if err != nil {
			return agent.ToolResult{}, err
		}
		result := agent.TextToolResult(string(payload))
		result.Details = json.RawMessage(payload)
		return result, nil
	})
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	return defineTool(tool, agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: config.AgentToolAsk,
		Execution:        agent.ToolExecutionInteractiveWait,
		MutationScope:    agent.ToolMutationSession,
		PostCheck:        agent.ToolPostCheckSessionState,
		Recovery:         agent.ToolRecoveryReconcilable,
		ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention:  agent.ToolResultProtected,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   defaultToolResultMaxBytes,
	})
}

// NewAsk builds the host-backed interactive ask tool.
func NewAsk() (agent.ToolDefinition, error) { return newAskTool() }
