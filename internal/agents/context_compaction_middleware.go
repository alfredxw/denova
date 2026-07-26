package agents

import (
	"context"
	"log/slog"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
	"denova/internal/observability"
)

type contextCompactionMiddleware struct {
	*agent.BaseMiddleware
	agentKind string
}

func (m *contextCompactionMiddleware) BeforeModelRewriteState(ctx context.Context, state *agent.RunState, _ *agent.ModelContext) (context.Context, *agent.RunState, error) {
	if state == nil || !agent.IsRootInvocation(ctx) {
		return ctx, state, nil
	}
	controller := compactionControllerFromContext(ctx)
	if controller == nil || controller.conversation == nil {
		return ctx, state, nil
	}
	messages := append([]*agent.Message(nil), state.Messages...)
	newMessages, result, err := controller.conversation.CompactContextIfNeeded(ctx, ContextCompactionInput{
		Messages: messages,
		Tools:    state.ToolInfos,
		Phase:    contextCompactionPhaseMidRun,
	})
	if err != nil {
		observability.Logger("agent-run").Warn("mid_run_context_compaction_failed", slog.String("agent_kind", m.agentKind), slog.Any("error", err))
		return ctx, state, nil
	}
	if !result.Triggered {
		return ctx, state, nil
	}
	next := *state
	next.Messages = newMessages
	return ctx, &next, nil
}

type contextCompactionUsage struct {
	PromptTokens           int `json:"prompt_tokens,omitempty"`
	CachedPromptTokens     int `json:"cached_prompt_tokens,omitempty"`
	CompletionTokens       int `json:"completion_tokens,omitempty"`
	ReasoningTokens        int `json:"reasoning_tokens,omitempty"`
	TotalTokens            int `json:"total_tokens,omitempty"`
	ContextWindowTokens    int `json:"context_window_tokens,omitempty"`
	EstimatedContextTokens int `json:"estimated_context_tokens,omitempty"`
}

func contextCompactionRecordFromResult(result ContextCompactionResult, agentKind string, sourceStart, sourceEnd, retainedTurns int, summary string) session.ContextCompaction {
	return session.ContextCompaction{
		Type:                "context_compaction",
		AgentKind:           agentKind,
		Epoch:               result.Epoch,
		Summary:             summary,
		SourceStartIndex:    sourceStart,
		SourceEndIndex:      sourceEnd,
		SourceMessageCount:  sourceEnd - sourceStart,
		RetainedTurns:       retainedTurns,
		TokensBefore:        result.TokensBefore,
		TokensAfter:         result.TokensAfter,
		TargetRatio:         result.TargetRatio,
		ContextWindowTokens: result.ContextWindowTokens,
		Strategy:            result.Strategy,
		Threshold:           result.Threshold,
		Reason:              contextCompactionReasonLimit,
		Phase:               result.Phase,
		CreatedAt:           time.Now().UTC(),
	}
}
