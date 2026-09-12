package interactiveapp

import (
	"fmt"
	"strings"

	agents "denova/internal/agents"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

// ModelContextProjection renders the canonical branch in message order.
// Agent owns checkpoint eligibility and coverage, including unfinished Turns.
// This adapter only restores interrupted inputs at their accepted branch slot.
type ModelContextProjection struct {
	Messages             []*agent.Message
	PendingInputMessages []string
}

type interactivePendingContext struct {
	turnBoundary int
	messages     []*agent.Message
}

func BuildModelContextProjection(history interactive.StoryModelHistory, compaction *agent.CompactionState, snapshot interactive.Snapshot,
	policy toolresult.ContextPolicy, current agentrun.CycleIdentity,
) (ModelContextProjection, error) {
	return buildModelContextProjection(history, compaction, snapshot, policy, current, func(input interactive.PlayerInputAcceptedEvent) *agent.Message {
		return agent.UserMessageWithAttachments(interruptedPlayerInputModelMessage(input), input.Attachments)
	})
}

// Canonical storage supplies raw input text; product inspection may explain
// that an older accepted input produced no narrative. Neither edits the journal.
func buildModelContextProjection(history interactive.StoryModelHistory, compaction *agent.CompactionState, snapshot interactive.Snapshot,
	policy toolresult.ContextPolicy, current agentrun.CycleIdentity, inputMessage func(interactive.PlayerInputAcceptedEvent) *agent.Message,
) (ModelContextProjection, error) {
	if history.StartTurn != 0 || history.EndTurn < 0 || history.EndTurn > history.TotalTurns || len(history.Turns) != history.EndTurn {
		return ModelContextProjection{}, fmt.Errorf("invalid canonical Game history: start=%d end=%d total=%d turns=%d", history.StartTurn, history.EndTurn, history.TotalTurns, len(history.Turns))
	}
	resolvedAt := make(map[int][]*agent.Message)
	for owner, turn := range history.Turns {
		for _, resolved := range turn.ResolvedPlayerInputContexts {
			boundary, err := interactivePlayerInputTurnBoundary(history, resolved.Input)
			if err != nil {
				return ModelContextProjection{}, err
			}
			if boundary == owner+1 {
				// Regeneration replaces the latest visible Turn in the same branch slot.
				boundary = owner
			} else if boundary > owner {
				return ModelContextProjection{}, fmt.Errorf("resolved player input %s was accepted after its owner Turn: accepted=%d owner=%d", resolved.Input.ID, boundary, owner)
			}
			messages := interactivePlayerInputContextMessages(resolved.Input, resolved.ModelContextBatches, inputMessage)
			resolvedAt[boundary] = append(resolvedAt[boundary], toolresult.ApplyContextPolicy(messages, policy)...)
		}
	}
	pending, pendingInputs, err := projectInteractivePendingContext(history, snapshot, policy, current, inputMessage)
	if err != nil {
		return ModelContextProjection{}, err
	}
	pendingAt := make(map[int][]*agent.Message)
	for _, entry := range pending {
		pendingAt[entry.turnBoundary] = append(pendingAt[entry.turnBoundary], entry.messages...)
	}
	projection := ModelContextProjection{PendingInputMessages: pendingInputs}
	for boundary := 0; boundary <= history.EndTurn; boundary++ {
		projection.Messages = append(projection.Messages, resolvedAt[boundary]...)
		projection.Messages = append(projection.Messages, pendingAt[boundary]...)
		if boundary == history.EndTurn {
			break
		}
		turn := history.Turns[boundary]
		messages := []*agent.Message{agent.UserMessageWithAttachments(turn.User, turn.Attachments)}
		messages = append(messages, settledTurnToolContextMessages(turn.ModelContextMessages)...)
		assistant := agent.AssistantMessage(turn.Narrative, nil)
		assistant.Extra = providers.ContinuationExtra(turn.ProviderContinuation)
		messages = append(messages, assistant)
		projection.Messages = append(projection.Messages, toolresult.ApplyContextPolicy(messages, policy)...)
	}
	if compaction != nil {
		// The live suffix is owned by Agent and may extend beyond this Story view.
		// Idle inspection applies the identical raw-message coverage through Agent.
		projection.Messages, err = compaction.Project(projection.Messages, max(1, len(compaction.Summary)))
		if err != nil {
			return ModelContextProjection{}, err
		}
	}
	return projection, nil
}

// settledTurnToolContextMessages keeps historical tool calls and results while
// removing assistant prose from those protocol messages. In Game mode the
// canonical final narrative is appended separately below, so replaying the
// same prose inside submit_interactive_turn would duplicate every settled Turn.
func settledTurnToolContextMessages(messages []interactive.ModelContextMessage) []*agents.Message {
	projected := schemaMessagesFromInteractiveContext(messages)
	for _, message := range projected {
		if message != nil && message.Role == agents.RoleAssistant && len(message.ToolCalls) > 0 {
			message.Content = ""
		}
	}
	return projected
}

func projectInteractivePendingContext(
	history interactive.StoryModelHistory,
	snapshot interactive.Snapshot,
	policy toolresult.ContextPolicy,
	current agentrun.CycleIdentity,
	inputMessage func(interactive.PlayerInputAcceptedEvent) *agent.Message,
) ([]interactivePendingContext, []string, error) {
	batches := make(map[string][]interactive.ModelContextBatchEvent, len(snapshot.PendingPlayerInputs))
	for _, batch := range snapshot.PendingModelContextBatches {
		batches[batch.PlayerInputID] = append(batches[batch.PlayerInputID], batch)
	}
	pending := make([]interactivePendingContext, 0, len(snapshot.PendingPlayerInputs))
	pendingInputMessages := make([]string, 0, len(snapshot.PendingPlayerInputs))
	for _, input := range snapshot.PendingPlayerInputs {
		if interactivePendingInputMatchesCycle(input, current) {
			// The live cycle is already represented by the final user instruction
			// and its in-run tool suffix in the provider request.
			continue
		}
		boundary, err := interactivePlayerInputTurnBoundary(history, input)
		if err != nil {
			return nil, nil, err
		}
		user := inputMessage(input).Content
		messages := toolresult.ApplyContextPolicy(
			interactivePlayerInputContextMessages(input, batches[input.ID], inputMessage), policy,
		)
		pending = append(pending, interactivePendingContext{turnBoundary: boundary, messages: messages})
		pendingInputMessages = append(pendingInputMessages, user)
	}
	return pending, pendingInputMessages, nil
}

func interactivePendingInputMatchesCycle(input interactive.PlayerInputAcceptedEvent, current agentrun.CycleIdentity) bool {
	return strings.TrimSpace(input.AgentCommandID) != "" &&
		input.AgentCommandID == string(current.CommandID) &&
		input.AgentOperationID == string(current.OperationID) &&
		input.AgentCycle == current.Cycle
}

func interactivePlayerInputContextMessages(
	input interactive.PlayerInputAcceptedEvent,
	batches []interactive.ModelContextBatchEvent,
	inputMessage func(interactive.PlayerInputAcceptedEvent) *agent.Message,
) []*agents.Message {
	messages := []*agents.Message{inputMessage(input)}
	for _, batch := range batches {
		messages = append(messages, schemaMessagesFromInteractiveContext(batch.Messages)...)
	}
	return messages
}

func interactivePlayerInputTurnBoundary(history interactive.StoryModelHistory, input interactive.PlayerInputAcceptedEvent) (int, error) {
	boundary := input.AcceptedTurnCount
	if boundary < 0 || boundary > history.TotalTurns {
		return 0, fmt.Errorf(
			"invalid accepted-turn boundary for player input %s: boundary=%d total=%d",
			input.ID, boundary, history.TotalTurns,
		)
	}
	return boundary, nil
}
