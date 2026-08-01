package chat

import (
	"context"
	"crypto/sha256"
	"denova/internal/agents/run"
	"encoding/hex"
	"strings"
	"sync"

	"denova/internal/agents/session"
)

type askConversation interface {
	AwaitAskWithPending(context.Context, session.AskInteraction, func(session.AskInteraction)) (session.AskResolution, error)
}

type runAskInteraction struct {
	conversation askConversation
	taskID       string
	agentKind    string
	emit         func(agentrun.Event)
	mu           sync.Mutex
}

// newRunAskInteraction deliberately accepts only top-level UI-backed Agent
// kinds. SubAgents and background Agents never inherit the parent's waiter.
func newRunAskInteraction(conversation Conversation, options agentrun.Options, emit func(agentrun.Event)) *runAskInteraction {
	backend, ok := conversation.(askConversation)
	if !ok || backend == nil {
		return nil
	}
	agentKind := strings.TrimSpace(options.AgentKind)
	if agentKind != agentrun.AgentKindGeneral && agentKind != agentrun.AgentKindIDE && agentKind != agentrun.AgentKindConfigManager &&
		agentKind != agentrun.AgentKindInteractiveStory && agentKind != agentrun.AgentKindImage {
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
	// Session has one durable pending-interaction slot. Tool batches may reach
	// approval checks concurrently, so serialize only the interactive wait;
	// ordinary non-interactive tool execution remains parallel.
	interaction.mu.Lock()
	defer interaction.mu.Unlock()
	request.TaskID = interaction.taskID
	request.AgentKind = interaction.agentKind
	request.ID = persistentAskID(request.TaskID, request.ToolCallID)
	var presentation session.AskInteraction
	presented := false
	resolution, err := interaction.conversation.AwaitAskWithPending(ctx, request, func(pending session.AskInteraction) {
		presentation = pending
		presented = true
		if interaction.emit != nil {
			interaction.emit(agentrun.Event{Type: "ask_pending", Data: pending})
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
		interaction.emit(agentrun.Event{Type: "ask_resolved", Data: presentation})
	}
	return resolution, nil
}

func persistentAskID(taskID, executionID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(taskID) + "\x00" + strings.TrimSpace(executionID)))
	return "ask-" + hex.EncodeToString(digest[:16])
}
