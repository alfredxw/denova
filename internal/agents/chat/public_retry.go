package chat

import (
	"log/slog"
	"sort"
	"strings"

	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

// A preview belongs to one source's current model response. Confirmed tool
// batches are removed at ModelCompleted, before the next preview begins.
type publicResponsePreview struct {
	ordinal                     int
	contentStart, thinkingStart int
	segments                    map[string]bool // true for tool input, false for prose
}

func (projector *PublicEventProjector) beginPreview(meta agentEventMetadata) {
	key := meta.SubAgentSessionID
	if projector.previews[key] != nil {
		if meta.ResponseOrdinal > 0 {
			projector.previews[key].ordinal = meta.ResponseOrdinal
		}
		return
	}
	content, thinking := &projector.content, &projector.thinking
	if meta.SubAgent {
		content, thinking = projector.nestedOutput(projector.nestedContent, meta), projector.nestedOutput(projector.nestedThinking, meta)
	}
	projector.previews[key] = &publicResponsePreview{
		ordinal:      meta.ResponseOrdinal,
		contentStart: content.Len(), thinkingStart: thinking.Len(), segments: make(map[string]bool),
	}
}

func (projector *PublicEventProjector) projectRetry(meta agentEventMetadata, retry agent.ModelRetry) {
	preview := projector.previews[meta.SubAgentSessionID]
	discard := []string{}
	if preview != nil {
		for id, tool := range preview.segments {
			if tool || retry.OutputState != agent.ModelOutputComplete {
				discard = append(discard, id)
				delete(projector.toolInputs, id)
				delete(projector.recorder.pendingToolIDs, id)
			}
		}
		if retry.OutputState != agent.ModelOutputComplete {
			content, thinking := &projector.content, &projector.thinking
			if meta.SubAgent {
				content, thinking = projector.nestedOutput(projector.nestedContent, meta), projector.nestedOutput(projector.nestedThinking, meta)
			}
			truncatePreview(content, preview.contentStart)
			truncatePreview(thinking, preview.thinkingStart)
			if !meta.SubAgent {
				projector.interactive = publicInteractiveOutput{}
			}
		} else if !meta.SubAgent {
			// Game may already have accepted complete prose while asking for
			// missing modules. Keep that candidate during business repair.
			projector.finishInteractiveResponseLocked(nil)
		}
	}
	projector.recorder.flushSource(meta)
	if appender, ok := projector.conversation.(interface{ DiscardDisplayEvents([]string) error }); ok && len(discard) > 0 {
		if err := appender.DiscardDisplayEvents(discard); err != nil {
			slog.Error("discard failed model display preview", "run_id", meta.RunID, "error", err)
		}
	}
	delete(projector.previews, meta.SubAgentSessionID)
	sort.Strings(discard)
	projector.emitEvent(agentrun.Event{Type: "model_retry", Data: meta.appendTo(map[string]any{
		"attempt": retry.Attempt, "max_attempts": retry.MaxAttempts, "delay_ms": retry.Delay.Milliseconds(),
		"reason": retry.Reason, "output_state": string(retry.OutputState), "response_ordinal": retry.ResponseOrdinal,
		"discard_ids": discard, "accepted_content": projector.content.String(),
	})})
}

func truncatePreview(builder *strings.Builder, length int) {
	prefix := builder.String()[:min(length, builder.Len())]
	builder.Reset()
	builder.WriteString(prefix)
}
