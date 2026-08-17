package chat

import (
	"context"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentplan "denova/internal/agents/plan"
	"denova/internal/agents/session"
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
	UpdateDisplayToolStatus(id, name, status string) error
}

type displayToolArgsAppender interface {
	AppendDisplayToolArgs(id, name, delta string) error
}

type displayToolResultUpdater interface {
	UpdateDisplayToolResult(id, name, status, result string, presentation *agent.ToolPresentation) error
}

type displayToolIllustrationUpdater interface {
	UpdateDisplayToolIllustration(id, name string, illustration *session.ChapterIllustration) error
}

type displayAskRecorder interface {
	RecordDisplayAsk(event session.DisplayEvent) error
}

type displayEventContentAppender interface {
	AppendDisplayEventContent(id, role, delta string) error
}

type displayEventContentFlusher interface {
	FlushDisplayEventContent(id, role string) error
}

type displayAssistantRunFinalizer interface {
	FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase string) error
}

type displayEventRecorder struct {
	appender                      displayEventAppender
	thinking                      strings.Builder
	thinkingID                    string
	thinkingMeta                  agentEventMetadata
	assistant                     strings.Builder
	assistantID                   string
	assistantMeta                 agentEventMetadata
	assistantPersisted            bool
	segmentSeq                    int
	rootRunID                     string
	rootAssistantSegmentIDs       []string
	pendingToolIDs                map[string]string
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
		pendingToolIDs:                make(map[string]string),
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
		r.flushAssistant()
		if r.thinking.Len() > 0 && !r.thinkingMeta.sameSource(meta) {
			r.flushThinking()
		}
		if r.thinking.Len() == 0 {
			r.thinkingID = r.nextTextSegmentID(meta, "thinking")
		}
		setEventDataString(ev.Data, displaySegmentIDEventKey, r.thinkingID)
		r.thinkingMeta = meta
		r.thinking.WriteString(eventDataString(ev.Data, "content"))
	case "chunk":
		meta := eventMetadataFromData(ev.Data)
		r.flushThinking()
		content := eventDataString(ev.Data, "content")
		if content == "" {
			return
		}
		if r.assistant.Len() > 0 && !r.assistantMeta.sameSource(meta) {
			r.flushAssistant()
		}
		if r.assistant.Len() == 0 {
			r.assistantID = r.nextTextSegmentID(meta, "assistant")
			if !meta.SubAgent {
				r.rootRunID = meta.RunID
				r.rootAssistantSegmentIDs = append(r.rootAssistantSegmentIDs, r.assistantID)
			}
		}
		setEventDataString(ev.Data, displaySegmentIDEventKey, r.assistantID)
		if !meta.SubAgent {
			setEventDataString(ev.Data, displayPhaseEventKey, session.DisplayPhaseCandidate)
		}
		r.assistantMeta = meta
		r.assistant.WriteString(content)
		if r.suppressRootAssistantSegments && !meta.SubAgent {
			return
		}
		contentAppender, ok := r.appender.(displayEventContentAppender)
		if !ok {
			return
		}
		if !r.assistantPersisted {
			if strings.TrimSpace(r.assistant.String()) == "" {
				return
			}
			if err := r.appender.AppendDisplayEvent(r.assistantDisplayEvent(r.assistant.String())); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist initial display assistant segment failed bytes=%d err=%v", r.assistant.Len(), err))
				return
			}
			r.assistantPersisted = true
			return
		}
		if err := contentAppender.AppendDisplayEventContent(r.assistantID, "assistant", content); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] append display assistant segment failed id=%s bytes=%d err=%v", r.assistantID, len(content), err))
		}
	case "tool_call":
		r.flushThinking()
		r.flushAssistant()
		meta := eventMetadataFromData(ev.Data)
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
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                id,
			Role:              "tool_call",
			Content:           name,
			Name:              name,
			Args:              args,
			Status:            "running",
			ToolPresentation:  eventDataToolPresentation(ev.Data),
			RunID:             meta.RunID,
			AgentKind:         meta.AgentKind,
			AgentName:         meta.AgentName,
			RootAgentName:     meta.RootAgentName,
			RunPath:           append([]string(nil), meta.RunPath...),
			SubAgent:          meta.SubAgent,
			SubAgentSessionID: meta.SubAgentSessionID,
			SubAgentType:      meta.SubAgentType,
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool_call failed name=%s id=%s err=%v", name, id, err))
			return
		}
		if id != "" {
			r.pendingToolIDs[id] = name
		}
	case "tool_args_delta":
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		delta := eventDataString(ev.Data, "delta")
		if agentplan.IsToolName(name) {
			return
		}
		argsAppender, ok := r.appender.(displayToolArgsAppender)
		if !ok {
			return
		}
		if err := argsAppender.AppendDisplayToolArgs(id, name, delta); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool_args_delta failed name=%s id=%s err=%v", name, id, err))
		}
	case "tool_result":
		r.flushThinking()
		r.flushAssistant()
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
		if resultUpdater, ok := r.appender.(displayToolResultUpdater); ok {
			if err := resultUpdater.UpdateDisplayToolResult(id, name, status, result, eventDataToolPresentation(ev.Data)); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool_result failed name=%s id=%s err=%v", name, id, err))
			}
		} else if err := r.appender.UpdateDisplayToolStatus(id, name, status); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool_result status failed name=%s id=%s err=%v", name, id, err))
		}
		if illustration := eventDataChapterIllustration(ev.Data, "illustration"); illustration != nil {
			if updater, ok := r.appender.(displayToolIllustrationUpdater); ok {
				if err := updater.UpdateDisplayToolIllustration(id, name, illustration); err != nil {
					slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display illustration failed name=%s id=%s err=%v", name, id, err))
				}
			}
		}
		if id != "" {
			delete(r.pendingToolIDs, id)
		}
	case "ask_pending", "ask_resolved":
		r.flushThinking()
		r.flushAssistant()
		ask := eventDataAskInteraction(ev.Data)
		if ask == nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] ignore invalid display Ask event type=%s", ev.Type))
			return
		}
		recorder, ok := r.appender.(displayAskRecorder)
		if !ok {
			return
		}
		meta := eventMetadataFromData(ev.Data)
		if err := recorder.RecordDisplayAsk(session.DisplayEvent{
			ID:                ask.ID,
			Role:              "ask",
			Status:            ask.Status,
			Ask:               ask,
			CreatedAt:         ask.CreatedAt,
			RunID:             meta.RunID,
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
	case "token_usage":
		r.flushThinking()
		r.flushAssistant()
		stats := runTokenUsage{
			RunID:                eventDataString(ev.Data, "run_id"),
			AgentKind:            eventDataString(ev.Data, "agent_kind"),
			PromptTokens:         eventDataInt(ev.Data, "prompt_tokens"),
			CachedPromptTokens:   eventDataInt(ev.Data, "cached_prompt_tokens"),
			UncachedPromptTokens: eventDataInt(ev.Data, "uncached_prompt_tokens"),
			CacheHitRate:         eventDataFloat(ev.Data, "cache_hit_rate"),
			CompletionTokens:     eventDataInt(ev.Data, "completion_tokens"),
			ReasoningTokens:      eventDataInt(ev.Data, "reasoning_tokens"),
			TotalTokens:          eventDataInt(ev.Data, "total_tokens"),
			ModelCalls:           eventDataInt(ev.Data, "model_calls"),
			GeneratedBytes:       eventDataInt(ev.Data, "generated_bytes"),
			Calls:                eventDataUsageCalls(ev.Data, "usage_calls"),
		}
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                   stats.RunID,
			Role:                 "token_usage",
			Content:              tokenUsageContent(stats),
			Name:                 "token_usage",
			CreatedAt:            eventDataTime(ev.Data, "created_at"),
			RunID:                stats.RunID,
			AgentKind:            stats.AgentKind,
			PromptTokens:         stats.PromptTokens,
			CachedPromptTokens:   stats.CachedPromptTokens,
			UncachedPromptTokens: stats.UncachedPromptTokens,
			CacheHitRate:         stats.CacheHitRate,
			CompletionTokens:     stats.CompletionTokens,
			ReasoningTokens:      stats.ReasoningTokens,
			TotalTokens:          stats.TotalTokens,
			ModelCalls:           stats.ModelCalls,
			GeneratedBytes:       stats.GeneratedBytes,
			UsageCalls:           usageCallsForSession(stats.Calls),
		}); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist token_usage failed run_id=%s err=%v", stats.RunID, err))
		}
	case "execution_summary":
		r.flushThinking()
		r.flushAssistant()
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
		r.flushThinking()
		r.flushAssistant()
		content := eventDataString(ev.Data, "content")
		if strings.TrimSpace(content) == "" {
			return
		}
		meta := eventMetadataFromData(ev.Data)
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:                eventDataString(ev.Data, "id"),
			Role:              ev.Type,
			Content:           content,
			Status:            eventDataString(ev.Data, "status"),
			RunID:             meta.RunID,
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
	case "error", "aborted":
		r.flushThinking()
		r.flushAssistant()
		r.finalizeRootAssistantSegments(session.DisplayPhasePartial)
		for id, name := range r.pendingToolIDs {
			if err := r.appender.UpdateDisplayToolStatus(id, name, "error"); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool_error failed name=%s id=%s err=%v", name, id, err))
			}
		}
		r.pendingToolIDs = make(map[string]string)
	case "done":
		r.flushThinking()
		r.flushAssistant()
		r.finalizeRootAssistantSegments(session.DisplayPhaseFinal)
		for id, name := range r.pendingToolIDs {
			if err := r.appender.UpdateDisplayToolStatus(id, name, "success"); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display tool_done failed name=%s id=%s err=%v", name, id, err))
			}
		}
		r.pendingToolIDs = make(map[string]string)
	}
}

func (r *displayEventRecorder) flushThinking() {
	if r == nil || r.appender == nil || r.thinking.Len() == 0 {
		return
	}
	content := r.thinking.String()
	r.thinking.Reset()
	if strings.TrimSpace(content) == "" {
		r.thinkingMeta = agentEventMetadata{}
		return
	}
	if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
		ID:                r.thinkingID,
		Role:              "thinking",
		Content:           content,
		RunID:             r.thinkingMeta.RunID,
		AgentKind:         r.thinkingMeta.AgentKind,
		AgentName:         r.thinkingMeta.AgentName,
		RootAgentName:     r.thinkingMeta.RootAgentName,
		RunPath:           append([]string(nil), r.thinkingMeta.RunPath...),
		SubAgent:          r.thinkingMeta.SubAgent,
		SubAgentSessionID: r.thinkingMeta.SubAgentSessionID,
		SubAgentType:      r.thinkingMeta.SubAgentType,
	}); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display thinking failed bytes=%d err=%v", len(content), err))
	}
	r.thinkingID = ""
	r.thinkingMeta = agentEventMetadata{}
}

func (r *displayEventRecorder) flushAssistant() {
	if r == nil || r.appender == nil || r.assistant.Len() == 0 {
		return
	}
	content := r.assistant.String()
	defer r.resetAssistantSegment()
	if strings.TrimSpace(content) == "" {
		return
	}
	if r.suppressRootAssistantSegments && !r.assistantMeta.SubAgent {
		return
	}
	if r.assistantPersisted {
		if flusher, ok := r.appender.(displayEventContentFlusher); ok {
			if err := flusher.FlushDisplayEventContent(r.assistantID, "assistant"); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] flush display assistant segment failed id=%s err=%v", r.assistantID, err))
			}
		}
		return
	}
	if err := r.appender.AppendDisplayEvent(r.assistantDisplayEvent(content)); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] persist display assistant segment failed bytes=%d err=%v", len(content), err))
	}
}

func (r *displayEventRecorder) assistantDisplayEvent(content string) session.DisplayEvent {
	displayPhase := ""
	if !r.assistantMeta.SubAgent {
		displayPhase = session.DisplayPhaseCandidate
	}
	return session.DisplayEvent{
		ID:                r.assistantID,
		Role:              "assistant",
		DisplayPhase:      displayPhase,
		Content:           content,
		RunID:             r.assistantMeta.RunID,
		AgentKind:         r.assistantMeta.AgentKind,
		AgentName:         r.assistantMeta.AgentName,
		RootAgentName:     r.assistantMeta.RootAgentName,
		RunPath:           append([]string(nil), r.assistantMeta.RunPath...),
		SubAgent:          r.assistantMeta.SubAgent,
		SubAgentSessionID: r.assistantMeta.SubAgentSessionID,
		SubAgentType:      r.assistantMeta.SubAgentType,
	}
}

func (r *displayEventRecorder) finalizeRootAssistantSegments(terminalPhase string) {
	if r == nil || r.appender == nil || len(r.rootAssistantSegmentIDs) == 0 {
		return
	}
	finalizer, ok := r.appender.(displayAssistantRunFinalizer)
	if !ok {
		return
	}
	finalSegmentID := r.rootAssistantSegmentIDs[len(r.rootAssistantSegmentIDs)-1]
	if err := finalizer.FinalizeDisplayAssistantRun(r.rootRunID, finalSegmentID, terminalPhase); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] finalize display assistant phases failed run_id=%s final_segment_id=%s phase=%s err=%v", r.rootRunID, finalSegmentID, terminalPhase, err))
	}
}

func (r *displayEventRecorder) resetAssistantSegment() {
	r.assistant.Reset()
	r.assistantID = ""
	r.assistantMeta = agentEventMetadata{}
	r.assistantPersisted = false
}

func (r *displayEventRecorder) nextTextSegmentID(meta agentEventMetadata, role string) string {
	r.segmentSeq++
	runID := sanitizeSubAgentSessionPart(meta.RunID)
	if runID == "" {
		runID = "run"
	}
	return fmt.Sprintf("%s-display-%03d-%s", runID, r.segmentSeq, role)
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
				PromptTokens:         eventDataInt(callMap, "prompt_tokens"),
				CachedPromptTokens:   eventDataInt(callMap, "cached_prompt_tokens"),
				UncachedPromptTokens: eventDataInt(callMap, "uncached_prompt_tokens"),
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

func eventDataToolPresentation(data interface{}) *agent.ToolPresentation {
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
	var presentation agent.ToolPresentation
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
			PromptTokens:         call.PromptTokens,
			CachedPromptTokens:   call.CachedPromptTokens,
			UncachedPromptTokens: call.UncachedPromptTokens,
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
