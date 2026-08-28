package interactiveapp

import (
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"fmt"
	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"strings"

	agents "denova/internal/agents"
	"denova/internal/interactive"
)

// ModelContextProjection is the single Game model-history view
// shared by normal assembly and checkpoint maintenance. SourceMessages is a
// contiguous completed-turn interval; everything after SourceTurnCount is a
// non-compactable tail and must survive a checkpoint verbatim.
type ModelContextProjection struct {
	Messages             []*agents.Message
	SourceMessages       []*agents.Message
	ExistingCheckpoint   string
	SourceTurnCount      int
	PendingInputMessages []string
}

type interactiveProjectedTurn struct {
	turn     interactive.StoryModelTurn
	messages []*agents.Message
}

// interactiveResolvedContext is an interrupted input made durable by the Turn
// that eventually closed it. displayBoundary restores the original transcript
// order. A valid checkpoint never bisects the interval between acceptance and
// the owner Turn.
type interactiveResolvedContext struct {
	context          interactive.ResolvedPlayerInputContext
	acceptedBoundary int
	displayBoundary  int
	ownerTurn        int
	messages         []*agents.Message
}

type interactivePendingContext struct {
	turnBoundary int
	messages     []*agents.Message
}

func BuildModelContextProjection(
	history interactive.StoryModelHistory,
	compaction *interactive.ContextCompactionProjection,
	snapshot interactive.Snapshot,
	policy toolresult.ContextPolicy,
	current agentrun.CycleIdentity,
) (ModelContextProjection, error) {
	if history.StartTurn < 0 || history.EndTurn < history.StartTurn ||
		history.EndTurn > history.TotalTurns || len(history.Turns) != history.EndTurn-history.StartTurn {
		return ModelContextProjection{}, fmt.Errorf(
			"invalid Game model-history range: start=%d end=%d total=%d turns=%d",
			history.StartTurn, history.EndTurn, history.TotalTurns, len(history.Turns),
		)
	}

	projection := ModelContextProjection{SourceTurnCount: history.EndTurn}
	sourceStart := 0
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		sourceStart = compaction.SourceTurnCount
		projection.ExistingCheckpoint = strings.TrimSpace(compaction.Summary)
	}
	if sourceStart < history.StartTurn || sourceStart > history.EndTurn {
		return ModelContextProjection{}, fmt.Errorf(
			"Game checkpoint boundary is outside loaded model history: source=%d history=[%d,%d)",
			sourceStart, history.StartTurn, history.EndTurn,
		)
	}

	turns, resolved, checkpointMessages, err := projectInteractiveCompletedContext(
		history, compaction, policy, sourceStart,
	)
	if err != nil {
		return ModelContextProjection{}, err
	}
	pending, pendingMessages, earliestPending, err := projectInteractivePendingContext(history, snapshot, policy, current)
	if err != nil {
		return ModelContextProjection{}, err
	}
	projection.PendingInputMessages = pendingMessages
	if earliestPending < projection.SourceTurnCount {
		projection.SourceTurnCount = earliestPending
	}
	if projection.SourceTurnCount < sourceStart {
		return ModelContextProjection{}, fmt.Errorf(
			"Game checkpoint bisects a pending player input: checkpoint=%d input=%d",
			sourceStart, projection.SourceTurnCount,
		)
	}
	projection.SourceTurnCount = interactiveAtomicSourceTurnCount(
		projection.SourceTurnCount, resolved,
	)

	projection.Messages = append(projection.Messages, checkpointMessages...)
	resolvedAt := make(map[int][]*agents.Message, len(resolved))
	for _, entry := range resolved {
		resolvedAt[entry.displayBoundary] = append(resolvedAt[entry.displayBoundary], entry.messages...)
	}
	pendingAt := make(map[int][]*agents.Message, len(pending))
	for _, entry := range pending {
		boundary := entry.turnBoundary
		if boundary < history.StartTurn {
			boundary = history.StartTurn
		}
		pendingAt[boundary] = append(pendingAt[boundary], entry.messages...)
	}
	for boundary := history.StartTurn; boundary <= history.EndTurn; boundary++ {
		// A canonical owner Turn consumes every pending input that existed at its
		// commit, so a surviving pending input must have been accepted at a later
		// boundary than every context resolved by that owner. Resolved-first is
		// therefore the stable order.
		projection.Messages = append(projection.Messages, resolvedAt[boundary]...)
		projection.Messages = append(projection.Messages, pendingAt[boundary]...)
		if boundary == history.EndTurn {
			break
		}
		turn := turns[boundary-history.StartTurn]
		projection.Messages = append(projection.Messages, turn.messages...)
	}

	// Build checkpoint source independently from the display projection. This
	// keeps resolved inputs at their acceptance position while enforcing that a
	// source either includes the entire acceptance -> owner interval or none of
	// it. Pending inputs have already shortened SourceTurnCount above.
	resolvedSourceAt := make(map[int][]interactiveResolvedContext, len(resolved))
	for _, entry := range resolved {
		if entry.acceptedBoundary < projection.SourceTurnCount && entry.ownerTurn < projection.SourceTurnCount {
			resolvedSourceAt[entry.acceptedBoundary] = append(resolvedSourceAt[entry.acceptedBoundary], entry)
		}
	}
	for boundary := sourceStart; boundary < projection.SourceTurnCount; boundary++ {
		for _, entry := range resolvedSourceAt[boundary] {
			projection.SourceMessages = append(
				projection.SourceMessages,
				interactiveLocatedResolvedContextMessages(entry)...,
			)
		}
		turn := turns[boundary-history.StartTurn]
		projection.SourceMessages = append(
			projection.SourceMessages,
			interactiveLocatedTurnMessages(turn.turn, turn.messages)...,
		)
	}
	return projection, nil
}

func projectInteractiveCompletedContext(
	history interactive.StoryModelHistory,
	compaction *interactive.ContextCompactionProjection,
	policy toolresult.ContextPolicy,
	sourceStart int,
) ([]interactiveProjectedTurn, []interactiveResolvedContext, []*agents.Message, error) {
	raw := make([]*agents.Message, 0, len(history.Turns)*3+1)
	checkpointCount := 0
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		raw = append(raw, agentcontext.NewCompactionSummaryMessage(compaction.Epoch, compaction.Summary))
		checkpointCount = 1
	}
	type messageSpan struct{ start, end int }
	turns := make([]interactiveProjectedTurn, len(history.Turns))
	turnSpans := make([]messageSpan, len(history.Turns))
	resolved := make([]interactiveResolvedContext, 0)
	resolvedAt := make(map[int][]int)
	for index, turn := range history.Turns {
		ownerTurn := history.StartTurn + index
		turns[index].turn = turn
		for _, context := range turn.ResolvedPlayerInputContexts {
			// The active checkpoint already owns every context whose closing Turn
			// is before its source boundary. Retained raw Turns intentionally remain
			// visible, but replaying their historical tool suffix would duplicate
			// evidence already represented by the checkpoint.
			if ownerTurn < sourceStart {
				continue
			}
			boundary, err := interactivePlayerInputTurnBoundary(history, context.Input)
			if err != nil {
				return nil, nil, nil, err
			}
			if boundary > ownerTurn {
				return nil, nil, nil, fmt.Errorf(
					"resolved player input %s was accepted after its owner Turn: accepted=%d owner=%d",
					context.Input.ID, boundary, ownerTurn,
				)
			}
			if boundary < sourceStart {
				return nil, nil, nil, fmt.Errorf(
					"Game checkpoint bisects resolved player input %s: accepted=%d checkpoint=%d owner=%d",
					context.Input.ID, boundary, sourceStart, ownerTurn,
				)
			}
			resolved = append(resolved, interactiveResolvedContext{
				context: context, acceptedBoundary: boundary, displayBoundary: boundary,
				ownerTurn: ownerTurn, messages: interactivePlayerInputContextMessages(context.Input, context.ModelContextBatches),
			})
			resolvedAt[boundary] = append(resolvedAt[boundary], len(resolved)-1)
		}
	}
	resolvedSpans := make([]messageSpan, len(resolved))
	for boundary := history.StartTurn; boundary <= history.EndTurn; boundary++ {
		for _, index := range resolvedAt[boundary] {
			start := len(raw)
			raw = append(raw, resolved[index].messages...)
			resolvedSpans[index] = messageSpan{start: start, end: len(raw)}
		}
		if boundary == history.EndTurn {
			break
		}
		index := boundary - history.StartTurn
		turn := history.Turns[index]
		start := len(raw)
		raw = append(raw, agent.UserMessageWithAttachments(turn.User, turn.Attachments))
		raw = append(raw, settledTurnToolContextMessages(turn.ModelContextMessages)...)
		assistant := agents.AssistantMessage(turn.Narrative, nil)
		assistant.Extra = providers.ContinuationExtra(turn.ProviderContinuation)
		raw = append(raw, assistant)
		turnSpans[index] = messageSpan{start: start, end: len(raw)}
	}
	checkpoint := append([]*agents.Message(nil), raw[:checkpointCount]...)
	for index := range turns {
		span := turnSpans[index]
		turns[index].messages = toolresult.ApplyContextPolicy(raw[span.start:span.end], policy)
	}
	for index := range resolved {
		span := resolvedSpans[index]
		resolved[index].messages = toolresult.ApplyContextPolicy(raw[span.start:span.end], policy)
	}
	return turns, resolved, checkpoint, nil
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
) ([]interactivePendingContext, []string, int, error) {
	batches := make(map[string][]interactive.ModelContextBatchEvent, len(snapshot.PendingPlayerInputs))
	for _, batch := range snapshot.PendingModelContextBatches {
		batches[batch.PlayerInputID] = append(batches[batch.PlayerInputID], batch)
	}
	pending := make([]interactivePendingContext, 0, len(snapshot.PendingPlayerInputs))
	pendingInputMessages := make([]string, 0, len(snapshot.PendingPlayerInputs))
	earliest := history.EndTurn
	for _, input := range snapshot.PendingPlayerInputs {
		if interactivePendingInputMatchesCycle(input, current) {
			// The live cycle is already represented by the final user instruction
			// and its in-run tool suffix in the provider request.
			continue
		}
		boundary, err := interactivePlayerInputTurnBoundary(history, input)
		if err != nil {
			return nil, nil, 0, err
		}
		if boundary < earliest {
			earliest = boundary
		}
		user := interruptedPlayerInputModelMessage(input)
		messages := toolresult.ApplyContextPolicy(
			interactivePlayerInputContextMessages(input, batches[input.ID]), policy,
		)
		pending = append(pending, interactivePendingContext{turnBoundary: boundary, messages: messages})
		pendingInputMessages = append(pendingInputMessages, user)
	}
	return pending, pendingInputMessages, earliest, nil
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
) []*agents.Message {
	messages := []*agents.Message{agents.UserMessage(interruptedPlayerInputModelMessage(input))}
	for _, batch := range batches {
		messages = append(messages, schemaMessagesFromInteractiveContext(batch.Messages)...)
	}
	return messages
}

func interactiveAtomicSourceTurnCount(
	sourceEnd int,
	resolved []interactiveResolvedContext,
) int {
	// A pending boundary may bisect one or more resolved acceptance intervals.
	// Repeatedly retreat to the oldest intersected acceptance boundary until
	// every included interval also contains its owner Turn.
	for {
		next := sourceEnd
		for _, entry := range resolved {
			boundary := entry.acceptedBoundary
			if sourceEnd <= boundary || sourceEnd > entry.ownerTurn {
				continue
			}
			if boundary < next {
				next = boundary
			}
		}
		if next == sourceEnd {
			return sourceEnd
		}
		sourceEnd = next
	}
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

func interactiveLocatedResolvedContextMessages(entry interactiveResolvedContext) []*agents.Message {
	result := make([]*agents.Message, 0, len(entry.messages))
	locator := fmt.Sprintf(
		"[source accepted_input_id=%s accepted_turn=%d owner_turn=%d]",
		entry.context.Input.ID, entry.acceptedBoundary, entry.ownerTurn,
	)
	for index, message := range entry.messages {
		if message == nil {
			continue
		}
		cloned := message.Clone()
		if index == 0 && cloned.Role == agents.RoleUser {
			cloned.Content = locator + "\n" + cloned.Content
		}
		result = append(result, cloned)
	}
	return result
}

func interactiveLocatedTurnMessages(turn interactive.StoryModelTurn, messages []*agents.Message) []*agents.Message {
	result := make([]*agents.Message, 0, len(messages))
	locator := fmt.Sprintf("[source turn_id=%s branch_id=%s]", turn.ID, turn.BranchID)
	for index, message := range messages {
		if message == nil {
			continue
		}
		cloned := message.Clone()
		if index == 0 && cloned.Role == agents.RoleUser ||
			index == len(messages)-1 && cloned.Role == agents.RoleAssistant && len(cloned.ToolCalls) == 0 {
			cloned.Content = locator + "\n" + cloned.Content
		}
		result = append(result, cloned)
	}
	return result
}
