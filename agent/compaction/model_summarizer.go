package compaction

import (
	"context"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type ModelSummarizerConfig struct {
	Model    agent.BaseChatModel
	Identity agent.CapabilityIdentity
	Prompt   string
}

type modelSummarizer struct{ config ModelSummarizerConfig }

func ModelSummarizer(config ModelSummarizerConfig) (Summarizer, error) {
	if config.Model == nil || strings.TrimSpace(config.Identity.Kind) == "" || config.Identity.Version == 0 {
		return nil, errors.New("Model Compaction Summarizer requires Model and stable Identity")
	}
	if strings.TrimSpace(config.Prompt) == "" {
		config.Prompt = "Summarize the supplied conversation faithfully for future continuation. Preserve decisions, constraints, unresolved work, tool results, and named entities. Do not invent facts."
	}
	return &modelSummarizer{config: config}, nil
}

func (summarizer *modelSummarizer) Identity() agent.CapabilityIdentity {
	return summarizer.config.Identity
}

func (summarizer *modelSummarizer) Summarize(ctx context.Context, request SummaryRequest) (Summary, error) {
	messages := make([]*agent.Message, 0, len(request.Messages)+1)
	messages = append(messages, agent.SystemMessage(summarizer.config.Prompt))
	for _, message := range request.Messages {
		messages = append(messages, message.Clone())
	}
	result, err := summarizer.config.Model.Generate(ctx, messages)
	if err != nil {
		return Summary{}, err
	}
	if result == nil || result.Role != agent.Assistant || strings.TrimSpace(result.Content) == "" || len(result.ToolCalls) != 0 {
		return Summary{}, errors.New("Compaction summary Model returned an invalid assistant message")
	}
	tokens := 0
	if result.ResponseMeta != nil && result.ResponseMeta.Usage != nil {
		tokens = result.ResponseMeta.Usage.TotalTokens
	}
	return Summary{Content: result.Content, TokenEstimate: tokens}, nil
}

var _ Summarizer = (*modelSummarizer)(nil)
