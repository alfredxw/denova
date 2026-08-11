package chat

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

type contextNormalizerMiddleware struct {
	*agent.BaseMiddleware
}

// NewModelContextMiddlewares returns the provider-neutral context boundaries.
// Agent.Definition.Compaction is the sole root checkpoint authority.
func NewModelContextMiddlewares() []agent.Middleware {
	return []agent.Middleware{NewContextNormalizerMiddleware()}
}

// NewContextNormalizerMiddleware creates the provider-neutral repair boundary
// that runs immediately before each model call.
func NewContextNormalizerMiddleware() agent.Middleware {
	return &contextNormalizerMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
}

func (m *contextNormalizerMiddleware) BeforeModelCall(
	ctx context.Context,
	call *agent.ModelCall,
	_ *agent.ModelContext,
) (context.Context, *agent.ModelCall, error) {
	if call == nil {
		return ctx, call, nil
	}
	normalized, err := agentcontext.NormalizeModelContextMessages(call.Messages)
	if err != nil {
		return ctx, call, fmt.Errorf("规范化供应商无关模型上下文失败 / normalize provider-neutral model context: %w", err)
	}
	next := *call
	next.Messages = normalized
	return ctx, &next, nil
}
