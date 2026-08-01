package chat

import (
	"context"
	"denova/internal/agents/run"
	"fmt"
	"reflect"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

type contextNormalizerMiddleware struct {
	*agent.BaseMiddleware
}

// NewModelContextMiddlewares returns the ordered provider-neutral context
// boundaries. Normalization must remain immediately before maintenance so a
// compaction decision always sees a valid tool protocol.
func NewModelContextMiddlewares(agentKind string, includeMaintenance bool) []agent.Middleware {
	middlewares := []agent.Middleware{NewContextNormalizerMiddleware()}
	if includeMaintenance {
		middlewares = append(middlewares, NewContextMaintenanceMiddleware(agentKind))
	}
	return middlewares
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
	if !reflect.DeepEqual(call.Messages, normalized) {
		if controller := compactionControllerFromContext(ctx); controller != nil && controller.emit != nil {
			controller.emit(agentrun.Event{Type: "context_normalizer", Data: map[string]any{
				"status": "repaired", "context_normalizer_repair_count": 1,
				"messages_before": len(call.Messages), "messages_after": len(normalized),
			}})
		}
	}
	return ctx, &next, nil
}
