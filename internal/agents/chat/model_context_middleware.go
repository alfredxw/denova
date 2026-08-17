package chat

import (
	"context"
	"fmt"
	"reflect"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/toolresult"
)

type contextNormalizerMiddleware struct {
	*agent.BaseMiddleware
}

// NewModelContextMiddlewares returns Denova's model-only history policy and
// presentation normalizer. Agent retains complete raw tool batches and
// provider reasoning so Cleanup, Compaction, recovery, and audit stay lossless;
// only the request projection drops history that the product never replays.
func NewModelContextMiddlewares(policy toolresult.ContextPolicy) []agent.Middleware {
	return []agent.Middleware{NewModelHistoryProjectionMiddleware(policy), NewContextNormalizerMiddleware()}
}

// NewContextNormalizerMiddleware creates the provider-neutral repair boundary
// that runs immediately before each model call.
func NewContextNormalizerMiddleware() agent.Middleware {
	return &contextNormalizerMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
}

func (m *contextNormalizerMiddleware) BeforeModelCall(
	ctx context.Context,
	call *agent.ModelCall,
	modelContext *agent.ModelContext,
) (context.Context, *agent.ModelCall, error) {
	if call == nil {
		return ctx, call, nil
	}
	normalized, err := agentcontext.NormalizeModelContextMessages(call.Messages)
	if err != nil {
		return ctx, call, fmt.Errorf("normalize provider-neutral model context: %w", err)
	}
	if !reflect.DeepEqual(call.Messages, normalized) {
		modelContext.ReportContextNormalization(agent.ContextNormalizationMetrics{
			RepairCount: 1, MessagesBefore: len(call.Messages), MessagesAfter: len(normalized),
		})
	}
	next := *call
	next.Messages = normalized
	return ctx, &next, nil
}
