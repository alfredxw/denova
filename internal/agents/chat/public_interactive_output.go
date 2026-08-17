package chat

import (
	"strings"

	agentinteractive "denova/internal/agents/interactive"
	agentrun "denova/internal/agents/run"
)

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
