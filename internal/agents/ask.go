package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"denova/internal/agents/session"
	producttools "denova/internal/agents/tools"
)

type askConversation interface {
	AwaitAskWithPending(context.Context, session.AskInteraction, func(session.AskInteraction)) (session.AskResolution, error)
}

type runAskInteraction struct {
	conversation askConversation
	taskID       string
	agentKind    string
	emit         func(Event)
}

// newRunAskInteraction deliberately accepts only top-level UI-backed Agent
// kinds. SubAgents and background Agents never inherit the parent's waiter.
func newRunAskInteraction(conversation Conversation, options RunOptions, emit func(Event)) producttools.AskInteraction {
	backend, ok := conversation.(askConversation)
	if !ok || backend == nil {
		return nil
	}
	agentKind := strings.TrimSpace(options.AgentKind)
	if agentKind != AgentKindGeneral && agentKind != AgentKindIDE && agentKind != AgentKindConfigManager {
		return nil
	}
	return &runAskInteraction{
		conversation: backend,
		taskID:       strings.TrimSpace(options.TaskID),
		agentKind:    agentKind,
		emit:         emit,
	}
}

func (interaction *runAskInteraction) Ask(ctx context.Context, request session.AskInteraction) (session.AskResolution, error) {
	request.TaskID = interaction.taskID
	request.AgentKind = interaction.agentKind
	request.ID = persistentAskID(request.TaskID, request.ToolCallID)
	var presentation session.AskInteraction
	presented := false
	resolution, err := interaction.conversation.AwaitAskWithPending(ctx, request, func(pending session.AskInteraction) {
		presentation = pending
		presented = true
		if interaction.emit != nil {
			interaction.emit(Event{Type: "ask_pending", Data: pending})
		}
	})
	if err != nil {
		return session.AskResolution{}, err
	}
	// A fully resolved replay returns its durable result immediately without
	// publishing a second, context-free presentation event. Pending replays do
	// invoke the callback above, so refresh/recovery still receives both edges.
	if interaction.emit != nil && presented {
		presentation.Status = resolution.Status
		presentation.Answers = resolution.Answers
		presentation.CancelReason = resolution.CancelReason
		interaction.emit(Event{Type: "ask_resolved", Data: presentation})
	}
	return resolution, nil
}

func persistentAskID(taskID, executionID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(taskID) + "\x00" + strings.TrimSpace(executionID)))
	return "ask-" + hex.EncodeToString(digest[:16])
}

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
	if !validHarnessCycleIdentity(identity) {
		return request
	}
	request.AgentCommandID = string(identity.CommandID)
	request.AgentOperationID = string(identity.OperationID)
	request.AgentCycle = identity.Cycle
	return request
}
