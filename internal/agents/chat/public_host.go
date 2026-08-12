package chat

import (
	"context"

	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"

	agent "github.com/alfredxw/denova/agent"
)

// PublicHostMiddleware installs Denova-only trace, review scope, and plan-mode
// context around the public Agent lifecycle. Interaction and artifact storage
// are bound through their dedicated Definition capabilities; the Agent package
// remains independent from Denova session types.
type PublicHostMiddleware struct {
	*agent.BaseMiddleware
	request ChatRequest
	options agentrun.Options
	trace   PublicRunTraceBinder
}

// PublicRunTraceBinder attaches the Denova run ledger to the generic Agent
// context after the public lifecycle has assigned its durable Run ID.
type PublicRunTraceBinder interface {
	BindPublicRunTrace(context.Context, string) context.Context
}

func NewPublicHostMiddleware(
	request ChatRequest,
	options agentrun.Options,
	trace PublicRunTraceBinder,
) *PublicHostMiddleware {
	return &PublicHostMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{}, request: request, options: options, trace: trace,
	}
}

func (middleware *PublicHostMiddleware) BeforeAgent(
	ctx context.Context,
	run *agent.RunContext,
) (context.Context, *agent.RunContext, error) {
	runID := ""
	if scope, ok := agent.InvocationScopeFromContext(ctx); ok {
		runID = scope.OperationID
	}
	if runID == "" {
		if identity, ok := agent.InvocationIdentityFromContext(ctx); ok {
			runID = identity.OperationID
			if runID == "" {
				runID = identity.RunID
			}
		}
	}
	if middleware.trace != nil {
		ctx = middleware.trace.BindPublicRunTrace(ctx, runID)
	}
	ctx = producttools.ContextWithWorkspaceChangeScope(ctx, producttools.WorkspaceChangeScope{
		RunID: runID, SessionID: middleware.options.SessionID,
		ReviewThreadID: middleware.options.ReviewThreadID,
	})
	if middleware.request.PlanMode {
		ctx = agenttoolruntime.ContextWithToolAccessMode(ctx, agenttoolruntime.ToolAccessModePlanReadOnly)
	}
	return ctx, run, nil
}

var _ agent.Middleware = (*PublicHostMiddleware)(nil)
