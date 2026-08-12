package chat

import (
	"context"

	agentinteractive "denova/internal/agents/interactive"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"

	agent "github.com/alfredxw/denova/agent"
)

// PublicHostMiddleware installs Denova-only execution context around the
// reusable Agent loop. Product interactions and artifact storage stay here;
// the public Agent package remains independent from Denova session types.
type PublicHostMiddleware struct {
	*agent.BaseMiddleware
	conversation Conversation
	request      ChatRequest
	options      agentrun.Options
	emit         func(agentrun.Event)
}

func NewPublicHostMiddleware(
	conversation Conversation,
	request ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) *PublicHostMiddleware {
	return &PublicHostMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{}, conversation: conversation,
		request: request, options: options, emit: emit,
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
	ctx = producttools.ContextWithWorkspaceChangeScope(ctx, producttools.WorkspaceChangeScope{
		RunID: runID, SessionID: middleware.options.SessionID,
		ReviewThreadID: middleware.options.ReviewThreadID,
	})
	if provider, ok := middleware.conversation.(toolArtifactStoreProvider); ok {
		ctx = agent.ContextWithToolArtifactStore(ctx, provider.ToolArtifactStore())
	}
	if middleware.request.PlanMode {
		ctx = agenttoolruntime.ContextWithToolAccessMode(ctx, agenttoolruntime.ToolAccessModePlanReadOnly)
	}
	if interaction := newRunAskInteraction(middleware.conversation, middleware.options, middleware.emit); interaction != nil {
		ctx = producttools.ContextWithAskInteraction(ctx, interaction)
		ctx = agenttoolruntime.ContextWithApprovalHost(ctx, interaction)
	}
	// Public Agent exposes a provider-neutral completion safe point. These
	// Denova contexts remain for direct/legacy Runner tests and custom tools.
	if middleware.options.AgentKind == agentrun.AgentKindInteractiveStory {
		ctx = agentinteractive.WithTurnCancel(ctx, func(...agent.AgentCancelOption) (*agent.CancelHandle, bool) {
			return nil, agent.RequestCompletionAfterTools(ctx)
		})
	} else if agentinteractive.IsDirectorPlanRun(middleware.options.AgentKind, middleware.options.MaintenanceTask) {
		ctx = agentinteractive.WithDirectorPlanCancel(ctx, func(...agent.AgentCancelOption) (*agent.CancelHandle, bool) {
			return nil, agent.RequestCompletionAfterTools(ctx)
		})
	}
	return ctx, run, nil
}

var _ agent.Middleware = (*PublicHostMiddleware)(nil)
