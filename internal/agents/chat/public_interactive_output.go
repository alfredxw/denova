package chat

import (
	"strings"

	agentinteractive "denova/internal/agents/interactive"
	agentplan "denova/internal/agents/plan"
	agentrun "denova/internal/agents/run"

	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

// ProjectCanonicalOutput deterministically derives the product-approved final
// canonical message from the synchronous Agent turn transcript. It never reads
// from the asynchronous display event stream, so consumer lag or delta drops
// cannot truncate persisted story prose or plan output.
func ProjectCanonicalOutput(
	request ChatRequest,
	options agentrun.Options,
	output agentcanonical.OutputCommitRequest,
) (*agentschema.Message, *agentcanonical.OutputProjection) {
	projected := output.Message.Clone()
	if projected == nil {
		return nil, nil
	}
	if request.PlanMode {
		meta := agentEventMetadata{
			RunID:         output.Identity.RunID,
			AgentCycle:    output.Identity.Cycle,
			AgentKind:     options.AgentKind,
			AgentName:     options.RootAgentName,
			RootAgentName: options.RootAgentName,
		}
		if options.RootAgentName != "" {
			meta.RunPath = []string{options.RootAgentName}
		}
		parser := agentplan.NewParser(meta.planMetadata(), nil)
		visible := parser.Push(projected.Content) + parser.Flush()
		if !parser.HasSuccessfulBlock() {
			projected.Content = visible
			return projected, nil
		}
		projected.Content = ""
		projected.ReasoningContent = ""
		projected.ToolCalls = nil
		return projected, &agentcanonical.OutputProjection{}
	}
	if options.AgentKind != agentrun.AgentKindInteractiveStory {
		return projected, nil
	}
	content, thinking := projectInteractiveTurnTranscript(output.ContextMessages, output.ActiveUserIndex, projected)
	if content == "" {
		return projected, nil
	}
	projected.Content = content
	projected.ReasoningContent = thinking
	projected.ToolCalls = nil
	return projected, &agentcanonical.OutputProjection{Content: content, Thinking: thinking}
}

func projectInteractiveTurnTranscript(
	messages []*agentschema.Message,
	activeUserIndex int,
	final *agentschema.Message,
) (string, string) {
	turnMessages := messages
	if len(turnMessages) == 0 {
		if final != nil {
			turnMessages = []*agentschema.Message{final}
		}
	} else if activeUserIndex >= 0 && activeUserIndex < len(turnMessages) {
		turnMessages = turnMessages[activeUserIndex+1:]
	}
	var narrative string
	var thinking strings.Builder
	seenOrdinals := make(map[int]bool, len(turnMessages))
	for _, msg := range turnMessages {
		if msg == nil || msg.Role != agentschema.Assistant {
			continue
		}
		// When a submission tool requests early completion, definition_engine
		// appends a synthetic final clone of the last assistant response with
		// the same ModelResponseOrdinal and cleared ToolCalls. Skip that clone
		// so its narrative and reasoning are not double-counted as thinking.
		if msg.AgentMeta != nil && msg.AgentMeta.ModelResponseOrdinal > 0 {
			if seenOrdinals[msg.AgentMeta.ModelResponseOrdinal] {
				continue
			}
			seenOrdinals[msg.AgentMeta.ModelResponseOrdinal] = true
		}
		if msg.ReasoningContent != "" {
			thinking.WriteString(msg.ReasoningContent)
		}
		if msg.Content == "" {
			continue
		}
		if hasNonSubmissionInteractiveTool(msg.ToolCalls) {
			thinking.WriteString(msg.Content)
			continue
		}
		if narrative == "" {
			narrative = msg.Content
		} else {
			thinking.WriteString(msg.Content)
		}
	}
	return narrative, thinking.String()
}

func hasNonSubmissionInteractiveTool(calls []agentschema.ToolCall) bool {
	for _, call := range calls {
		if !agentinteractive.IsInteractiveTurnSubmissionTool(call.Function.Name) {
			return true
		}
	}
	return false
}

// interactiveContentReclassifiedEvent retracts provisional Game narrative
// after a non-submission tool proves that the prose was reasoning. It is a
// Denova display projection event, not part of the reusable Agent lifecycle.
const interactiveContentReclassifiedEvent = "interactive_content_reclassified"

// publicInteractiveOutput owns Game's provisional narrative classification at
// the Denova projection seam. The reusable Agent package publishes facts about
// assistant text and tool input; only the product knows that prose preceding a
// non-submission tool is reasoning rather than canonical story narrative.
type publicInteractiveOutput struct {
	active       bool
	candidate    bool
	reclassified bool
	provisional  strings.Builder
	meta         agentEventMetadata
}

func (projector *PublicEventProjector) projectInteractiveAssistantDeltaLocked(
	meta agentEventMetadata,
	displayOnly bool,
	content string,
) bool {
	if projector == nil || displayOnly || meta.SubAgent || meta.AgentKind != agentrun.AgentKindInteractiveStory {
		return false
	}
	projector.beginInteractiveResponseLocked(meta)
	state := &projector.interactive
	if state.candidate && !state.reclassified {
		state.provisional.WriteString(content)
		projector.emitEvent(agentrun.Event{Type: "chunk", Data: meta.appendTo(map[string]any{"content": content})})
		return true
	}
	projector.thinking.WriteString(content)
	projector.emitEvent(agentrun.Event{Type: "thinking", Data: meta.appendTo(map[string]any{"content": content})})
	return true
}

func (projector *PublicEventProjector) observeInteractiveToolLocked(meta agentEventMetadata, name string) {
	if projector == nil || meta.SubAgent || meta.AgentKind != agentrun.AgentKindInteractiveStory {
		return
	}
	// A tool can only reclassify provisional text from its own active model
	// response. Tool-only responses have nothing to retract and must not create
	// state that leaks into the next response.
	if !projector.interactive.active {
		return
	}
	if agentinteractive.IsInteractiveTurnSubmissionTool(name) || projector.interactive.reclassified {
		return
	}
	state := &projector.interactive
	state.reclassified = true
	if state.provisional.Len() == 0 {
		return
	}
	content := state.provisional.String()
	state.provisional.Reset()
	projector.thinking.WriteString(content)
	projector.emitEvent(agentrun.Event{
		Type: interactiveContentReclassifiedEvent,
		Data: state.meta.appendTo(map[string]any{"content": content}),
	})
}

func (projector *PublicEventProjector) beginInteractiveResponseLocked(meta agentEventMetadata) {
	state := &projector.interactive
	if state.active {
		return
	}
	state.active = true
	state.candidate = projector.content.Len() == 0
	state.reclassified = false
	state.provisional.Reset()
	state.meta = meta
}

func (projector *PublicEventProjector) finishInteractiveResponseLocked(requestedTools []string) {
	if projector == nil || !projector.interactive.active {
		return
	}
	for _, name := range requestedTools {
		projector.observeInteractiveToolLocked(projector.interactive.meta, name)
	}
	state := &projector.interactive
	if state.candidate && !state.reclassified && state.provisional.Len() > 0 {
		projector.content.WriteString(state.provisional.String())
	}
	state.active = false
	state.candidate = false
	state.reclassified = false
	state.provisional.Reset()
	state.meta = agentEventMetadata{}
}
