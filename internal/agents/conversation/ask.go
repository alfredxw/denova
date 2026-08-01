package conversation

import (
	"context"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func (c *SessionConversation) AwaitAsk(ctx context.Context, request session.AskInteraction) (session.AskResolution, error) {
	if c == nil || c.session == nil {
		return session.AskResolution{}, context.Canceled
	}
	return c.session.AwaitAsk(ctx, c.bindAskCycleIdentity(request))
}

func (c *SessionConversation) AwaitAskWithPending(ctx context.Context, request session.AskInteraction, onPending func(session.AskInteraction)) (session.AskResolution, error) {
	if c == nil || c.session == nil {
		return session.AskResolution{}, context.Canceled
	}
	return c.session.AwaitAskWithPending(ctx, c.bindAskCycleIdentity(request), onPending)
}

func (c *SessionConversation) bindAskCycleIdentity(request session.AskInteraction) session.AskInteraction {
	identity := c.agentCycleIdentitySnapshot()
	if !agentrun.ValidCycleIdentity(identity) {
		return request
	}
	request.AgentCommandID = string(identity.CommandID)
	request.AgentOperationID = string(identity.OperationID)
	request.AgentCycle = identity.Cycle
	return request
}
