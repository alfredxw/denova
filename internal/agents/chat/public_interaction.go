package chat

import (
	"denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
	"time"
)

// ProjectPendingInteraction is a read-only UI projection. The product
// transport carries the public Run identity but owns no interaction state.
func ProjectPendingInteraction(request agent.InteractionRequest, status agentrun.RuntimeStatus) *session.AskInteraction {
	projected := session.ProjectInteractionRequest(request, time.Now().UTC())
	if projected == nil {
		return nil
	}
	projected.AgentKind = status.Binding.AgentKind
	projected.AgentCommandID = string(status.ActiveCommandID)
	projected.AgentOperationID = string(status.ActiveOperation)
	projected.AgentCycle = status.ActiveCycle
	return projected
}
