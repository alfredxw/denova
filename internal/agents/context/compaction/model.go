package compaction

import (
	"context"
	"fmt"
	"io"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/modelio"
	agentrun "denova/internal/agents/run"
)

// GenerateSummary is the provider adapter for context compaction. Prompt
// construction and input admission remain owned by this package so every
// caller shares the same bounded, cache-aware behavior.
func GenerateSummary(
	ctx context.Context,
	cfg *config.Config,
	request SummaryRequest,
	emitDelta func(attempt int, delta string),
) (summary string, runErr error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindContextCompaction, "context_compaction", "generate", map[string]any{
		"source_agent_kind": strings.TrimSpace(request.SourceAgentKind),
		"source_messages":   request.SourceMessages,
		"source_tokens":     request.SourceTokens,
	})
	defer func() { finishTrace(runErr) }()

	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindContextCompaction)
	if err != nil {
		return "", fmt.Errorf("resolve context compaction model configuration: %w", err)
	}
	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		return "", fmt.Errorf("create context compaction model: %w", err)
	}

	const attempt = 1
	const mode = "stream"
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindContextCompaction, "context_compaction", mode, modelCfg, request.Messages, nil, true)
	message, err := streamSummaryAttempt(llmTraceCtx, chatModel, request.Messages, attempt, emitDelta)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindContextCompaction, "context_compaction", mode, modelCfg.Model, attempt, nil, err, nil)
		return "", fmt.Errorf("compact context: %w", err)
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindContextCompaction, "context_compaction", mode, modelCfg.Model, attempt, message, nil, nil)

	summary = strings.TrimSpace(message.Content)
	if summary == "" {
		return "", fmt.Errorf("context compaction result is empty")
	}
	return summary, nil
}

func streamSummaryAttempt(
	ctx context.Context,
	chatModel agent.BaseChatModel,
	input []*agent.Message,
	attempt int,
	emitDelta func(attempt int, delta string),
) (*agent.Message, error) {
	stream, err := chatModel.Stream(ctx, input)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var chunks []*agent.Message
	for {
		message, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if message == nil {
			continue
		}
		chunks = append(chunks, message)
		if message.Content != "" && emitDelta != nil {
			emitDelta(attempt, message.Content)
		}
	}
	return agent.ConcatMessages(chunks)
}
