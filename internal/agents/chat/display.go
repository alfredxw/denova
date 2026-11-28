package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentplan "denova/internal/agents/plan"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agentschema "github.com/alfredxw/denova/agent/schema"
	agenttool "github.com/alfredxw/denova/agent/tool"
)

// displaySegmentIDEventKey carries the stable identity shared by the live
// stream adapter and the persisted display transcript for one contiguous part.
const displaySegmentIDEventKey = "display_segment_id"
const displayPhaseEventKey = "display_phase"

// PersistAgentAssistant publishes one public Agent output through the existing
// product conversation boundary. It deliberately does not own the surrounding
// Agent canonical intent/receipt; callers invoke it only from CanonicalAdapter.
func PersistAgentAssistant(conversation Conversation, content, thinking string, metadata session.MessageMetadata) error {
	if conversation == nil {
		return fmt.Errorf("persist Agent assistant: conversation is required")
	}
	if appender, ok := conversation.(interface {
		AppendAssistantWithMetadata(content, thinking string, metadata session.MessageMetadata) error
	}); ok {
		return appender.AppendAssistantWithMetadata(content, thinking, metadata)
	}
	if appender, ok := conversation.(interface {
		AppendAssistantWithThinking(content, thinking string) error
	}); ok {
		return appender.AppendAssistantWithThinking(content, thinking)
	}
	return conversation.AppendAssistant(content)
}

type displayEventAppender interface {
	AppendDisplayEvent(event session.DisplayEvent) error
}

type displayAskRecorder interface {
	RecordDisplayAsk(event session.DisplayEvent) error
}

type displayEventSourceKey struct {
	runID             string
	cycle             int
	agentName         string
	runPath           string
	subAgent          bool
	subAgentSessionID string
}

type displayTextSegment struct {
	strings.Builder
	id   string
	meta agentEventMetadata
}

type displayPendingTool struct {
	id           string
	name         string
	args         strings.Builder
	meta         agentEventMetadata
	presentation *agenttool.ToolPresentation
}

type displaySealedSegment struct {
	role    string
	segment displayTextSegment
}

type displaySourceRecorder struct {
	sealed                []displaySealedSegment
	thinking              displayTextSegment
	assistant             displayTextSegment
	pendingFinalThinking  displayTextSegment
	pendingFinalAssistant displayTextSegment
}

type displayEventRecorder struct {
	appender                      displayEventAppender
	sources                       map[displayEventSourceKey]*displaySourceRecorder
	sourceOrder                   []displayEventSourceKey
	segmentSeq                    int
	cycle                         int
	pendingTools                  map[string]*displayPendingTool
	pendingToolOrder              []string
	suppressRootAssistantSegments bool
}

type displayEventRecorderOptions struct {
	// Plan protocol output replaces root prose with structured plan cards. Keep
	// assigning live segment IDs, but do not restore that transient prose later.
	SuppressRootAssistantSegments bool
}

func newDisplayEventRecorder(conversation Conversation, options displayEventRecorderOptions) *displayEventRecorder {
	appender, _ := conversation.(displayEventAppender)
	return &displayEventRecorder{
		appender:                      appender,
		sources:                       make(map[displayEventSourceKey]*displaySourceRecorder),
		pendingTools:                  make(map[string]*displayPendingTool),
		suppressRootAssistantSegments: options.SuppressRootAssistantSegments,
	}
}

func (r *displayEventRecorder) Record(ev agentrun.Event) {
	if r == nil || r.appender == nil {
		return
	}
	switch ev.Type {
	case "thinking", interactiveContentReclassifiedEvent:
		meta := eventMetadataFromData(ev.Data)
		source := r.source(meta)
		r.promotePendingFinal(source)
		sealAssistant(source)
		content := eventDataString(ev.Data, "content")
		if content == "" {
			return
		}
		if source.thinking.Len() == 0 {
			source.thinking.id = r.nextTextSegmentID(meta, "thinking")
		}
		setEventDataString(ev.Data, displaySegmentIDEventKey, source.thinking.id)
		source.thinking.meta = meta
		source.thinking.WriteString(content)
	case "chunk":
		meta := eventMetadataFromData(ev.Data)
		source := r.source(meta)
		r.promotePendingFinal(source)
		if meta.SubAgent {
			sealThinking(source)
		}
		content := eventDataString(ev.Data, "content")
		if content == "" {
			return
		}
		if source.assistant.Len() == 0 {
			source.assistant.id = r.nextTextSegmentID(meta, "assistant")
		}
		setEventDataString(ev.Data, displaySegmentIDEventKey, source.assistant.id)
		if !meta.SubAgent {
			setEventDataString(ev.Data, displayPhaseEventKey, session.DisplayPhaseCandidate)
		}
		source.assistant.meta = meta
		source.assistant.WriteString(content)
	case "tool_call":
		meta := eventMetadataFromData(ev.Data)
		source := r.source(meta)
		r.promotePendingFinal(source)
		// Seal open text in memory instead of flushing to disk: ToolInputStarted
		// arrives while the model response is still streaming and may still be
		// retracted by ModelRetry before ModelCompleted accepts the step.
		sealThinking(source)
		sealAssistant(source)
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		args := eventDataString(ev.Data, "args")
		if agentplan.IsToolName(name) {
			if handled, _ := agentplan.EmitToolCall(name, args, meta.planMetadata(), planEventEmitter(r.Record)); handled {
				return
			}
		}
		if strings.TrimSpace(name) == "" {
			name = "unknown_tool"
		}
		pending := &displayPendingTool{
			id:           id,
			name:         name,
			meta:         meta,
			presentation: eventDataToolPresentation(ev.Data),
		}
		if args != "" {
			pending.args.WriteString(args)
		}
		if id != "" {
			if _, exists := r.pendingTools[id]; !exists {
				r.pendingToolOrder = append(r.pendingToolOrder, id)
			}
			r.pendingTools[id] = pending
		}
	case "tool_args_delta":
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		delta := eventDataString(ev.Data, "delta")
		if agentplan.IsToolName(name) || delta == "" || id == "" {
			return
		}
		if pending := r.pendingTools[id]; pending != nil {
			pending.args.WriteString(delta)
		}
	case "tool_result":
		meta := eventMetadataFromData(ev.Data)
		source := r.source(meta)
		r.promotePendingFinal(source)
		phase := session.DisplayPhaseProgress
		if meta.SubAgent {
			phase = ""
		}
		r.flushSourceSegments(source, phase)
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		result := eventDataString(ev.Data, "content")
		status := eventDataString(ev.Data, "status")
		if status == "" {
			status = "success"
		}
		if agentplan.IsToolName(name) {
			return
		}
		var args string
		presentation := eventDataToolPresentation(ev.Data)
		if id != "" {
			if pending := r.pendingTools[id]; pending != nil {
				args = pending.args.String()
				if strings.TrimSpace(name) == "" {
					name = pending.name
				}
				if presentation == nil {
					presentation = pending.presentation
				}
				delete(r.pendingTools, id)
			}
		}
		if strings.TrimSpace(name) == "" {
			name = "unknown_tool"
		}
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                id,
			Role:              "tool_call",
			Content:           name,
			Name:              name,
			Args:              args,
			Status:            status,
			Result:            result,
			ToolPresentation:  presentation,
			Illustration:      eventDataChapterIllustration(ev.Data, "illustration"),
			RunID:             meta.RunID,
			AgentCycle:        meta.AgentCycle,
			AgentKind:         meta.AgentKind,
			AgentName:         meta.AgentName,
			RootAgentName:     meta.RootAgentName,
			RunPath:           append([]string(nil), meta.RunPath...),
			SubAgent:          meta.SubAgent,
			SubAgentSessionID: meta.SubAgentSessionID,
			SubAgentType:      meta.SubAgentType,
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool_result failed name=%s id=%s err=%v", name, id, err))
		}
	case "ask_pending", "ask_resolved":
		meta := eventMetadataFromData(ev.Data)
		source := r.source(meta)
		r.promotePendingFinal(source)
		phase := session.DisplayPhaseProgress
		if meta.SubAgent {
			phase = ""
		}
		r.flushSourceSegments(source, phase)
		ask := eventDataAskInteraction(ev.Data)
		if ask == nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] ignore invalid display Ask event type=%s", ev.Type))
			return
		}
		recorder, ok := r.appender.(displayAskRecorder)
		if !ok {
			return
		}
		if err := recorder.RecordDisplayAsk(session.DisplayEvent{
			ID:                ask.ID,
			Role:              "ask",
			Status:            ask.Status,
			Ask:               ask,
			CreatedAt:         ask.CreatedAt,
			RunID:             meta.RunID,
			AgentCycle:        meta.AgentCycle,
			AgentKind:         meta.AgentKind,
			AgentName:         meta.AgentName,
			RootAgentName:     meta.RootAgentName,
			RunPath:           append([]string(nil), meta.RunPath...),
			SubAgent:          meta.SubAgent,
			SubAgentSessionID: meta.SubAgentSessionID,
			SubAgentType:      meta.SubAgentType,
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display Ask failed id=%s status=%s err=%v", ask.ID, ask.Status, err))
		}
	case "subagent_settled":
		r.flushSubAgentSource(eventMetadataFromData(ev.Data))
	case "context_compaction":
		status := eventDataString(ev.Data, "status")
		if status != "completed" && status != "failed" {
			return
		}
		r.flushAllIntermediateText()
		displayStatus := "success"
		if status != "completed" {
			displayStatus = "error"
		}
		meta := eventMetadataFromData(ev.Data)
		tokensBefore := eventDataInt(ev.Data, "tokens_before")
		if tokensBefore == 0 {
			tokensBefore = eventDataInt(ev.Data, "observed_prompt_tokens")
		}
		if tokensBefore == 0 {
			tokensBefore = eventDataInt(ev.Data, "estimated_tokens_before")
		}
		tokensAfter := eventDataInt(ev.Data, "tokens_after")
		if tokensAfter == 0 {
			tokensAfter = eventDataInt(ev.Data, "estimated_tokens_after")
		}
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                       eventDataString(ev.Data, "id"),
			Role:                     "context_compaction",
			Status:                   displayStatus,
			Phase:                    eventDataString(ev.Data, "phase"),
			RuntimeManaged:           eventDataBool(ev.Data, "runtime_managed"),
			Content:                  eventDataString(ev.Data, "summary"),
			RunID:                    meta.RunID,
			AgentKind:                meta.AgentKind,
			AgentCycle:               meta.AgentCycle,
			TokensBefore:             tokensBefore,
			TokensAfter:              tokensAfter,
			ProjectedTokensBefore:    eventDataInt(ev.Data, "projected_tokens_before"),
			ProjectedTokensAfter:     eventDataInt(ev.Data, "projected_tokens_after"),
			ReservedCompletionTokens: eventDataInt(ev.Data, "reserved_completion_tokens"),
			ReservedToolResultTokens: eventDataInt(ev.Data, "reserved_tool_result_tokens"),
			ContextWindowTokens:      eventDataInt(ev.Data, "context_window_tokens"),
			Threshold:                eventDataFloat(ev.Data, "threshold"),
			TargetRatio:              eventDataFloat(ev.Data, "target_ratio"),
			CompactionRevision:       eventDataInt(ev.Data, "revision"),
			SourceMessageCount:       eventDataInt(ev.Data, "source_message_count"),
			MessageCountBefore:       eventDataInt(ev.Data, "message_count_before"),
			MessageCountAfter:        eventDataInt(ev.Data, "message_count_after"),
			CompactionReason:         eventDataString(ev.Data, "reason"),
		}); err != nil {
			slog.ErrorContext(context.Background(), "Persist compaction display failed", "error", err)
		}
	case "token_usage":
		r.flushActiveTextWithoutPromotingFinal()
		stats := runTokenUsage{
			RunID:                  eventDataString(ev.Data, "run_id"),
			AgentKind:              eventDataString(ev.Data, "agent_kind"),
			PromptTokens:           eventDataInt(ev.Data, "prompt_tokens"),
			CachedPromptTokens:     eventDataInt(ev.Data, "cached_prompt_tokens"),
			CacheWritePromptTokens: eventDataInt(ev.Data, "cache_write_prompt_tokens"),
			UncachedPromptTokens:   eventDataInt(ev.Data, "uncached_prompt_tokens"),
			CacheHitRate:           eventDataFloat(ev.Data, "cache_hit_rate"),
			CompletionTokens:       eventDataInt(ev.Data, "completion_tokens"),
			ReasoningTokens:        eventDataInt(ev.Data, "reasoning_tokens"),
			TotalTokens:            eventDataInt(ev.Data, "total_tokens"),
			ModelCalls:             eventDataInt(ev.Data, "model_calls"),
			GeneratedBytes:         eventDataInt(ev.Data, "generated_bytes"),
			Calls:                  eventDataUsageCalls(ev.Data, "usage_calls"),
		}
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                     stats.RunID,
			Role:                   "token_usage",
			Content:                tokenUsageContent(stats),
			Name:                   "token_usage",
			CreatedAt:              eventDataTime(ev.Data, "created_at"),
			RunID:                  stats.RunID,
			AgentKind:              stats.AgentKind,
			PromptTokens:           stats.PromptTokens,
			CachedPromptTokens:     stats.CachedPromptTokens,
			CacheWritePromptTokens: stats.CacheWritePromptTokens,
			UncachedPromptTokens:   stats.UncachedPromptTokens,
			CacheHitRate:           stats.CacheHitRate,
			CompletionTokens:       stats.CompletionTokens,
			ReasoningTokens:        stats.ReasoningTokens,
			TotalTokens:            stats.TotalTokens,
			ModelCalls:             stats.ModelCalls,
			GeneratedBytes:         stats.GeneratedBytes,
			UsageCalls:             usageCallsForSession(stats.Calls),
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist token_usage failed run_id=%s err=%v", stats.RunID, err))
		}
	case "execution_summary":
		r.flushActiveTextWithoutPromotingFinal()
		meta := eventMetadataFromData(ev.Data)
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:            eventDataString(ev.Data, "run_id"),
			Role:          "execution_summary",
			RunID:         meta.RunID,
			AgentKind:     meta.AgentKind,
			RunStartedAt:  eventDataString(ev.Data, "run_started_at"),
			RunFinishedAt: eventDataString(ev.Data, "run_finished_at"),
			DurationMS:    eventDataInt64(ev.Data, "duration_ms"),
			RunStatus:     eventDataString(ev.Data, "status"),
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist execution summary failed run_id=%s err=%v", meta.RunID, err))
		}
	case "proposed_plan":
		meta := eventMetadataFromData(ev.Data)
		source := r.source(meta)
		r.promotePendingFinal(source)
		r.flushSealedSegments(source, session.DisplayPhaseProgress)
		r.flushThinking(source)
		content := eventDataString(ev.Data, "content")
		if strings.TrimSpace(content) == "" {
			return
		}
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                eventDataString(ev.Data, "id"),
			Role:              ev.Type,
			Content:           content,
			Status:            eventDataString(ev.Data, "status"),
			RunID:             meta.RunID,
			AgentCycle:        meta.AgentCycle,
			AgentKind:         meta.AgentKind,
			AgentName:         meta.AgentName,
			RootAgentName:     meta.RootAgentName,
			RunPath:           append([]string(nil), meta.RunPath...),
			SubAgent:          meta.SubAgent,
			SubAgentSessionID: meta.SubAgentSessionID,
			SubAgentType:      meta.SubAgentType,
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display plan event failed role=%s bytes=%d err=%v", ev.Type, len(content), err))
		}
	case "error", "aborted", "suspended":
		r.flushAllPartialText()
		status := "error"
		if ev.Type == "aborted" && ev.DataString("reason") == agentrun.AbortReasonUserRequested {
			status = "cancelled"
		}
		r.flushPendingTools(status)
	case "done":
		r.discardRootPendingFinal()
		r.flushActiveTextWithoutPromotingFinal()
		r.flushPendingTools("success")
	}
}

func (r *displayEventRecorder) completeModelStep(meta agentEventMetadata, hasTools bool) {
	if r == nil || r.appender == nil {
		return
	}
	source := r.sources[r.sourceKey(meta)]
	if source == nil {
		return
	}
	if meta.SubAgent {
		r.flushSourceSegments(source, "")
		return
	}
	if meta.AgentKind == agentrun.AgentKindInteractiveStory {
		// Interactive stories store chronological display events around the
		// narrative anchor instead of a Writing Session canonical messageRecord.
		// Flush thinking at step completion so post-narrative reasoning stays
		// anchored after submit_interactive_turn instead of falling back above
		// the narrative.
		r.promotePendingFinal(source)
		r.flushSealedSegments(source, "")
		r.flushThinking(source)
		source.assistant = displayTextSegment{}
		return
	}
	if hasTools {
		r.promotePendingFinal(source)
		r.flushSourceSegments(source, session.DisplayPhaseProgress)
		return
	}
	if len(source.sealed) == 0 && source.thinking.Len() == 0 && source.assistant.Len() == 0 {
		// Usage-only ModelCompleted (e.g. Goal evaluation) carries no new assistant
		// output and must not disturb pendingFinal from the actual response.
		return
	}
	// Terminal step without tool calls: Engine.Run commits canonical
	// messageRecord (with both ReasoningContent and Content) synchronously.
	// Hold this step in memory only in case the run aborts before CommitOutput
	// or enters a continuation step.
	r.promotePendingFinal(source)
	r.flushSealedSegments(source, session.DisplayPhaseProgress)
	source.pendingFinalThinking = source.thinking
	source.thinking = displayTextSegment{}
	source.pendingFinalAssistant = source.assistant
	source.assistant = displayTextSegment{}
}

func (r *displayEventRecorder) discardPreview(meta agentEventMetadata, discardIDs []string, discardProse bool) {
	if r == nil {
		return
	}
	for _, id := range discardIDs {
		delete(r.pendingTools, id)
	}
	if !discardProse {
		return
	}
	if source := r.sources[r.sourceKey(meta)]; source != nil {
		source.sealed = nil
		source.thinking = displayTextSegment{}
		source.assistant = displayTextSegment{}
	}
}

func (r *displayEventRecorder) finalizeCycle(status agentschema.ResultStatus) {
	if r == nil || r.appender == nil {
		return
	}
	if status == agentschema.ResultCompleted {
		r.discardRootFinalText()
		r.flushSubAgentText()
		return
	}
	r.flushAllPartialText()
}

func (r *displayEventRecorder) source(meta agentEventMetadata) *displaySourceRecorder {
	key := r.sourceKey(meta)
	if source := r.sources[key]; source != nil {
		return source
	}
	source := &displaySourceRecorder{}
	r.sources[key] = source
	r.sourceOrder = append(r.sourceOrder, key)
	return source
}

func (r *displayEventRecorder) flushSubAgentSource(meta agentEventMetadata) {
	if r == nil {
		return
	}
	source := r.sources[r.sourceKey(meta)]
	r.flushSourceSegments(source, "")
}

func (r *displayEventRecorder) sourceKey(meta agentEventMetadata) displayEventSourceKey {
	cycle := meta.AgentCycle
	if cycle == 0 && r != nil && r.cycle > 0 {
		cycle = r.cycle
	}
	return displayEventSourceKey{
		runID: meta.RunID, cycle: cycle, agentName: meta.AgentName,
		runPath: strings.Join(meta.RunPath, "\x00"), subAgent: meta.SubAgent, subAgentSessionID: meta.SubAgentSessionID,
	}
}

func sealThinking(source *displaySourceRecorder) {
	if source == nil || source.thinking.Len() == 0 {
		return
	}
	source.sealed = append(source.sealed, displaySealedSegment{role: "thinking", segment: source.thinking})
	source.thinking = displayTextSegment{}
}

func sealAssistant(source *displaySourceRecorder) {
	if source == nil || source.assistant.Len() == 0 {
		return
	}
	source.sealed = append(source.sealed, displaySealedSegment{role: "assistant", segment: source.assistant})
	source.assistant = displayTextSegment{}
}

func (r *displayEventRecorder) flushSealedSegments(source *displaySourceRecorder, assistantPhase string) {
	if r == nil || source == nil || len(source.sealed) == 0 {
		return
	}
	sealed := source.sealed
	source.sealed = nil
	for _, item := range sealed {
		if item.role == "thinking" {
			r.persistThinkingSegment(item.segment)
		} else {
			r.persistAssistantSegment(item.segment, assistantPhase)
		}
	}
}

func (r *displayEventRecorder) flushSourceSegments(source *displaySourceRecorder, assistantPhase string) {
	if r == nil || source == nil {
		return
	}
	r.flushSealedSegments(source, assistantPhase)
	r.flushThinking(source)
	r.flushAssistantWithPhase(source, assistantPhase)
}

func (r *displayEventRecorder) promotePendingFinal(source *displaySourceRecorder) {
	if r == nil || source == nil {
		return
	}
	if source.pendingFinalThinking.Len() > 0 {
		segment := source.pendingFinalThinking
		source.pendingFinalThinking = displayTextSegment{}
		r.persistThinkingSegment(segment)
	}
	if source.pendingFinalAssistant.Len() > 0 {
		segment := source.pendingFinalAssistant
		source.pendingFinalAssistant = displayTextSegment{}
		r.persistAssistantSegment(segment, session.DisplayPhaseProgress)
	}
}

func (r *displayEventRecorder) discardRootPendingFinal() {
	if r == nil {
		return
	}
	for _, key := range r.sourceOrder {
		if key.subAgent {
			continue
		}
		if source := r.sources[key]; source != nil {
			source.pendingFinalThinking = displayTextSegment{}
			source.pendingFinalAssistant = displayTextSegment{}
		}
	}
}

func (r *displayEventRecorder) discardRootFinalText() {
	if r == nil {
		return
	}
	for _, key := range r.sourceOrder {
		if key.subAgent {
			continue
		}
		if source := r.sources[key]; source != nil {
			source.pendingFinalThinking = displayTextSegment{}
			source.pendingFinalAssistant = displayTextSegment{}
			source.sealed = nil
			source.thinking = displayTextSegment{}
			source.assistant = displayTextSegment{}
		}
	}
}

func (r *displayEventRecorder) flushSubAgentText() {
	if r == nil {
		return
	}
	for _, key := range r.sourceOrder {
		if !key.subAgent {
			continue
		}
		r.flushSourceSegments(r.sources[key], "")
	}
}

func (r *displayEventRecorder) flushActiveTextWithoutPromotingFinal() {
	if r == nil {
		return
	}
	for _, key := range r.sourceOrder {
		phase := session.DisplayPhaseProgress
		if key.subAgent {
			phase = ""
		}
		r.flushSourceSegments(r.sources[key], phase)
	}
}

func (r *displayEventRecorder) flushAllIntermediateText() {
	if r == nil {
		return
	}
	for _, key := range r.sourceOrder {
		source := r.sources[key]
		r.promotePendingFinal(source)
		phase := session.DisplayPhaseProgress
		if key.subAgent {
			phase = ""
		}
		r.flushSourceSegments(source, phase)
	}
}

func (r *displayEventRecorder) flushAllPartialText() {
	if r == nil {
		return
	}
	for _, key := range r.sourceOrder {
		source := r.sources[key]
		if source == nil {
			continue
		}
		if source.pendingFinalThinking.Len() > 0 {
			segment := source.pendingFinalThinking
			source.pendingFinalThinking = displayTextSegment{}
			r.persistThinkingSegment(segment)
		}
		if source.pendingFinalAssistant.Len() > 0 {
			segment := source.pendingFinalAssistant
			source.pendingFinalAssistant = displayTextSegment{}
			if key.subAgent {
				r.persistAssistantSegment(segment, "")
			} else {
				r.persistAssistantSegment(segment, session.DisplayPhasePartial)
			}
		}
		phase := session.DisplayPhasePartial
		if key.subAgent {
			phase = ""
		}
		r.flushSourceSegments(source, phase)
	}
}

func (r *displayEventRecorder) flushPendingTools(status string) {
	if r == nil || r.appender == nil || len(r.pendingTools) == 0 {
		return
	}
	for _, id := range r.pendingToolOrder {
		pending, exists := r.pendingTools[id]
		if !exists || pending == nil {
			continue
		}
		delete(r.pendingTools, id)
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                pending.id,
			Role:              "tool_call",
			Content:           pending.name,
			Name:              pending.name,
			Args:              pending.args.String(),
			Status:            status,
			ToolPresentation:  pending.presentation,
			RunID:             pending.meta.RunID,
			AgentCycle:        pending.meta.AgentCycle,
			AgentKind:         pending.meta.AgentKind,
			AgentName:         pending.meta.AgentName,
			RootAgentName:     pending.meta.RootAgentName,
			RunPath:           append([]string(nil), pending.meta.RunPath...),
			SubAgent:          pending.meta.SubAgent,
			SubAgentSessionID: pending.meta.SubAgentSessionID,
			SubAgentType:      pending.meta.SubAgentType,
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool terminal status failed name=%s id=%s status=%s err=%v", pending.name, pending.id, status, err))
		}
	}
	r.pendingTools = make(map[string]*displayPendingTool)
	r.pendingToolOrder = nil
}

func (r *displayEventRecorder) flushThinking(source *displaySourceRecorder) {
	if r == nil || r.appender == nil || source == nil || source.thinking.Len() == 0 {
		return
	}
	segment := source.thinking
	source.thinking = displayTextSegment{}
	r.persistThinkingSegment(segment)
}

func (r *displayEventRecorder) persistThinkingSegment(segment displayTextSegment) {
	if r == nil || r.appender == nil || segment.Len() == 0 {
		return
	}
	content := segment.String()
	if strings.TrimSpace(content) == "" {
		return
	}
	if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
		ID:                segment.id,
		Role:              "thinking",
		Content:           content,
		RunID:             segment.meta.RunID,
		AgentCycle:        segment.meta.AgentCycle,
		AgentKind:         segment.meta.AgentKind,
		AgentName:         segment.meta.AgentName,
		RootAgentName:     segment.meta.RootAgentName,
		RunPath:           append([]string(nil), segment.meta.RunPath...),
		SubAgent:          segment.meta.SubAgent,
		SubAgentSessionID: segment.meta.SubAgentSessionID,
		SubAgentType:      segment.meta.SubAgentType,
	}); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display thinking failed bytes=%d err=%v", len(content), err))
	}
}

func (r *displayEventRecorder) flushAssistantWithPhase(source *displaySourceRecorder, phase string) {
	if r == nil || r.appender == nil || source == nil || source.assistant.Len() == 0 {
		return
	}
	segment := source.assistant
	source.assistant = displayTextSegment{}
	r.persistAssistantSegment(segment, phase)
}

func (r *displayEventRecorder) persistAssistantSegment(segment displayTextSegment, phase string) {
	if r == nil || r.appender == nil || segment.Len() == 0 {
		return
	}
	content := segment.String()
	if strings.TrimSpace(content) == "" {
		return
	}
	if r.suppressRootAssistantSegments && !segment.meta.SubAgent {
		return
	}
	displayPhase := ""
	if !segment.meta.SubAgent {
		displayPhase = phase
	}
	if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
		ID:                segment.id,
		Role:              "assistant",
		DisplayPhase:      displayPhase,
		Content:           content,
		RunID:             segment.meta.RunID,
		AgentCycle:        segment.meta.AgentCycle,
		AgentKind:         segment.meta.AgentKind,
		AgentName:         segment.meta.AgentName,
		RootAgentName:     segment.meta.RootAgentName,
		RunPath:           append([]string(nil), segment.meta.RunPath...),
		SubAgent:          segment.meta.SubAgent,
		SubAgentSessionID: segment.meta.SubAgentSessionID,
		SubAgentType:      segment.meta.SubAgentType,
	}); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display assistant segment failed bytes=%d err=%v", len(content), err))
	}
}

func (r *displayEventRecorder) nextTextSegmentID(meta agentEventMetadata, role string) string {
	r.segmentSeq++
	runID := sanitizeSubAgentSessionPart(meta.RunID)
	if runID == "" {
		runID = "run"
	}
	if meta.ResponseOrdinal > 0 {
		return fmt.Sprintf("%s-cycle-%03d-response-%03d-display-%03d-%s", runID, r.cycle, meta.ResponseOrdinal, r.segmentSeq, role)
	}
	return fmt.Sprintf("%s-cycle-%03d-display-%03d-%s", runID, r.cycle, r.segmentSeq, role)
}

func setEventDataString(data interface{}, key, value string) {
	if key == "" || value == "" {
		return
	}
	switch typed := data.(type) {
	case map[string]string:
		typed[key] = value
	case map[string]interface{}:
		typed[key] = value
	}
}

func eventDataString(data interface{}, key string) string {
	switch typed := data.(type) {
	case map[string]string:
		return typed[key]
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func eventDataTime(data interface{}, key string) time.Time {
	raw := strings.TrimSpace(eventDataString(data, key))
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func eventDataInt(data interface{}, key string) int {
	switch typed := data.(type) {
	case map[string]int:
		return typed[key]
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			switch v := value.(type) {
			case int:
				return v
			case int64:
				return int(v)
			case float64:
				return int(v)
			case float32:
				return int(v)
			}
		}
	}
	return 0
}

func eventDataInt64(data interface{}, key string) int64 {
	switch typed := data.(type) {
	case map[string]int64:
		return typed[key]
	case map[string]int:
		return int64(typed[key])
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			switch v := value.(type) {
			case int:
				return int64(v)
			case int64:
				return v
			case float64:
				return int64(v)
			case float32:
				return int64(v)
			}
		}
	}
	return 0
}

func eventDataFloat(data interface{}, key string) float64 {
	switch typed := data.(type) {
	case map[string]float64:
		return typed[key]
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			switch v := value.(type) {
			case float64:
				return v
			case float32:
				return float64(v)
			case int:
				return float64(v)
			case int64:
				return float64(v)
			}
		}
	}
	return 0
}

func eventDataBool(data interface{}, key string) bool {
	switch typed := data.(type) {
	case map[string]bool:
		return typed[key]
	case map[string]string:
		return strings.EqualFold(typed[key], "true")
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			switch v := value.(type) {
			case bool:
				return v
			case string:
				return strings.EqualFold(v, "true")
			}
		}
	}
	return false
}

func eventDataUsageCalls(data interface{}, key string) []runTokenUsageCall {
	typed, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	value, ok := typed[key]
	if !ok {
		return nil
	}
	switch calls := value.(type) {
	case []runTokenUsageCall:
		return append([]runTokenUsageCall(nil), calls...)
	case []interface{}:
		result := make([]runTokenUsageCall, 0, len(calls))
		for _, item := range calls {
			callMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			result = append(result, runTokenUsageCall{
				Index:                eventDataInt(callMap, "index"),
				CreatedAt:            eventDataString(callMap, "created_at"),
				FinishReason:         eventDataString(callMap, "finish_reason"),
				RequestedTools:       agenttoolruntime.EventDataStringSlice(callMap, "requested_tools"),
				AfterTools:           agenttoolruntime.EventDataStringSlice(callMap, "after_tools"),
				PromptTokens:           eventDataInt(callMap, "prompt_tokens"),
				CachedPromptTokens:     eventDataInt(callMap, "cached_prompt_tokens"),
				CacheWritePromptTokens: eventDataInt(callMap, "cache_write_prompt_tokens"),
				UncachedPromptTokens:   eventDataInt(callMap, "uncached_prompt_tokens"),
				CacheHitRate:         eventDataFloat(callMap, "cache_hit_rate"),
				CompletionTokens:     eventDataInt(callMap, "completion_tokens"),
				ReasoningTokens:      eventDataInt(callMap, "reasoning_tokens"),
				TotalTokens:          eventDataInt(callMap, "total_tokens"),
			})
		}
		return result
	default:
		return nil
	}
}

func eventDataChapterIllustration(data interface{}, key string) *session.ChapterIllustration {
	typed, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	value, ok := typed[key]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result session.ChapterIllustration
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	if strings.TrimSpace(result.Schema) == "" || strings.TrimSpace(result.ImagePath) == "" {
		return nil
	}
	return &result
}

func eventDataAskInteraction(data interface{}) *session.AskInteraction {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var interaction session.AskInteraction
	if err := json.Unmarshal(raw, &interaction); err != nil {
		return nil
	}
	interaction.ID = strings.TrimSpace(interaction.ID)
	interaction.ToolCallID = strings.TrimSpace(interaction.ToolCallID)
	interaction.AgentKind = strings.TrimSpace(interaction.AgentKind)
	interaction.Status = strings.TrimSpace(interaction.Status)
	if interaction.ID == "" || interaction.ToolCallID == "" || interaction.AgentKind == "" {
		return nil
	}
	if interaction.Status != session.AskPending && interaction.Status != session.AskAnswered && interaction.Status != session.AskCancelled {
		return nil
	}
	return &interaction
}

func eventDataToolPresentation(data interface{}) *agenttool.ToolPresentation {
	typed, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	value, ok := typed["tool_presentation"]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var presentation agenttool.ToolPresentation
	if err := json.Unmarshal(raw, &presentation); err != nil {
		return nil
	}
	normalized, err := presentation.Normalize()
	if err != nil {
		return nil
	}
	return &normalized
}

func usageCallsForSession(calls []runTokenUsageCall) []session.TokenUsageCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]session.TokenUsageCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, session.TokenUsageCall{
			Index:                call.Index,
			CreatedAt:            call.CreatedAt,
			FinishReason:         call.FinishReason,
			RequestedTools:       append([]string(nil), call.RequestedTools...),
			AfterTools:           append([]string(nil), call.AfterTools...),
			PromptTokens:           call.PromptTokens,
			CachedPromptTokens:     call.CachedPromptTokens,
			CacheWritePromptTokens: call.CacheWritePromptTokens,
			UncachedPromptTokens:   call.UncachedPromptTokens,
			CacheHitRate:         call.CacheHitRate,
			CompletionTokens:     call.CompletionTokens,
			ReasoningTokens:      call.ReasoningTokens,
			TotalTokens:          call.TotalTokens,
		})
	}
	return result
}

func parseWriteLoreItemsToolResult(toolName, content string) ([]string, []string) {
	if toolName != "write_lore_items" {
		return nil, nil
	}
	var itemIDs []string
	var deletedIDs []string
	var structured struct {
		ItemIDs    []string `json:"item_ids"`
		DeletedIDs []string `json:"deleted_ids"`
	}
	if json.Unmarshal([]byte(content), &structured) == nil && (structured.ItemIDs != nil || structured.DeletedIDs != nil) {
		return structured.ItemIDs, structured.DeletedIDs
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if raw, ok := strings.CutPrefix(line, "item_ids:"); ok {
			_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &itemIDs)
			continue
		}
		if raw, ok := strings.CutPrefix(line, "deleted_ids:"); ok {
			_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &deletedIDs)
		}
	}
	return itemIDs, deletedIDs
}
