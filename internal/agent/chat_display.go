package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"nova/internal/session"
)

// appendAssistantIfAny 将已生成的正文持久化，避免异常中断后刷新丢失输出。
func appendAssistantIfAny(conversation Conversation, content, thinking *strings.Builder) string {
	if content == nil || content.Len() == 0 {
		return ""
	}
	generated := content.String()
	reasoning := ""
	if thinking != nil && thinking.Len() > 0 {
		reasoning = thinking.String()
	}
	if appender, ok := conversation.(interface {
		AppendAssistantWithThinking(content, thinking string) error
	}); ok {
		if err := appender.AppendAssistantWithThinking(generated, reasoning); err != nil {
			log.Printf("[agent-run] persist assistant message failed err=%v", err)
		}
	} else if err := conversation.AppendAssistant(generated); err != nil {
		log.Printf("[agent-run] persist assistant message failed err=%v", err)
	}
	log.Printf("[agent-run] persisted assistant message bytes=%d thinking_bytes=%d", len(generated), len(reasoning))
	content.Reset()
	if thinking != nil {
		thinking.Reset()
	}
	return generated
}

type displayEventAppender interface {
	AppendDisplayEvent(event session.DisplayEvent) error
	UpdateDisplayToolStatus(id, name, status string) error
}

type displayToolArgsAppender interface {
	AppendDisplayToolArgs(id, name, delta string) error
}

type displayToolResultUpdater interface {
	UpdateDisplayToolResult(id, name, status, result string) error
}

type displayEventContentAppender interface {
	AppendDisplayEventContent(id, role, delta string) error
}

type displayEventRecorder struct {
	appender             displayEventAppender
	thinking             strings.Builder
	thinkingMeta         agentEventMetadata
	pendingToolIDs       map[string]string
	subAgentAssistantIDs map[string]bool
}

func newDisplayEventRecorder(conversation Conversation) *displayEventRecorder {
	appender, _ := conversation.(displayEventAppender)
	return &displayEventRecorder{
		appender:             appender,
		pendingToolIDs:       make(map[string]string),
		subAgentAssistantIDs: make(map[string]bool),
	}
}

func (r *displayEventRecorder) Record(ev Event) {
	if r == nil || r.appender == nil {
		return
	}
	switch ev.Type {
	case "thinking":
		meta := eventMetadataFromData(ev.Data)
		if r.thinking.Len() > 0 && !r.thinkingMeta.sameSource(meta) {
			r.flushThinking()
		}
		r.thinkingMeta = meta
		r.thinking.WriteString(eventDataString(ev.Data, "content"))
	case "chunk":
		meta := eventMetadataFromData(ev.Data)
		if meta.SubAgent {
			r.flushThinking()
			r.recordSubAgentAssistantChunk(meta, eventDataString(ev.Data, "content"))
			return
		}
		r.flushThinking()
	case "tool_call":
		r.flushThinking()
		meta := eventMetadataFromData(ev.Data)
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		args := eventDataString(ev.Data, "args")
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
			RunID:             meta.RunID,
			AgentName:         meta.AgentName,
			RootAgentName:     meta.RootAgentName,
			RunPath:           append([]string(nil), meta.RunPath...),
			SubAgent:          meta.SubAgent,
			SubAgentSessionID: meta.SubAgentSessionID,
			SubAgentType:      meta.SubAgentType,
		}); err != nil {
			log.Printf("[agent-run] persist display tool_call failed name=%s id=%s err=%v", name, id, err)
			return
		}
		if id != "" {
			r.pendingToolIDs[id] = name
		}
	case "tool_args_delta":
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		delta := eventDataString(ev.Data, "delta")
		argsAppender, ok := r.appender.(displayToolArgsAppender)
		if !ok {
			return
		}
		if err := argsAppender.AppendDisplayToolArgs(id, name, delta); err != nil {
			log.Printf("[agent-run] persist display tool_args_delta failed name=%s id=%s err=%v", name, id, err)
		}
	case "tool_result":
		r.flushThinking()
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		result := eventDataString(ev.Data, "content")
		if resultUpdater, ok := r.appender.(displayToolResultUpdater); ok {
			if err := resultUpdater.UpdateDisplayToolResult(id, name, "success", result); err != nil {
				log.Printf("[agent-run] persist display tool_result failed name=%s id=%s err=%v", name, id, err)
			}
		} else if err := r.appender.UpdateDisplayToolStatus(id, name, "success"); err != nil {
			log.Printf("[agent-run] persist display tool_result status failed name=%s id=%s err=%v", name, id, err)
		}
		if id != "" {
			delete(r.pendingToolIDs, id)
		}
	case "token_usage":
		r.flushThinking()
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
			log.Printf("[agent-run] persist token_usage failed run_id=%s err=%v", stats.RunID, err)
		}
	case "error", "aborted":
		r.flushThinking()
		for id, name := range r.pendingToolIDs {
			if err := r.appender.UpdateDisplayToolStatus(id, name, "error"); err != nil {
				log.Printf("[agent-run] persist display tool_error failed name=%s id=%s err=%v", name, id, err)
			}
		}
		r.pendingToolIDs = make(map[string]string)
	case "done":
		r.flushThinking()
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
		Role:              "thinking",
		Content:           content,
		RunID:             r.thinkingMeta.RunID,
		AgentName:         r.thinkingMeta.AgentName,
		RootAgentName:     r.thinkingMeta.RootAgentName,
		RunPath:           append([]string(nil), r.thinkingMeta.RunPath...),
		SubAgent:          r.thinkingMeta.SubAgent,
		SubAgentSessionID: r.thinkingMeta.SubAgentSessionID,
		SubAgentType:      r.thinkingMeta.SubAgentType,
	}); err != nil {
		log.Printf("[agent-run] persist display thinking failed bytes=%d err=%v", len(content), err)
	}
	r.thinkingMeta = agentEventMetadata{}
}

func (r *displayEventRecorder) recordSubAgentAssistantChunk(meta agentEventMetadata, content string) {
	if r == nil || r.appender == nil || strings.TrimSpace(content) == "" {
		return
	}
	id := strings.TrimSpace(meta.SubAgentSessionID)
	if id == "" {
		id = buildSubAgentSessionID(meta.RunID, meta.AgentName, 0)
	}
	if r.subAgentAssistantIDs[id] {
		appender, ok := r.appender.(displayEventContentAppender)
		if !ok {
			return
		}
		if err := appender.AppendDisplayEventContent(id, "assistant", content); err != nil {
			log.Printf("[agent-run] persist subagent assistant chunk failed id=%s err=%v", id, err)
		}
		return
	}
	if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
		ID:                id,
		Role:              "assistant",
		Content:           content,
		Status:            "running",
		RunID:             meta.RunID,
		AgentName:         meta.AgentName,
		RootAgentName:     meta.RootAgentName,
		RunPath:           append([]string(nil), meta.RunPath...),
		SubAgent:          true,
		SubAgentSessionID: id,
		SubAgentType:      meta.SubAgentType,
	}); err != nil {
		log.Printf("[agent-run] persist subagent assistant failed id=%s err=%v", id, err)
		return
	}
	r.subAgentAssistantIDs[id] = true
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
				RequestedTools:       eventDataStringSlice(callMap, "requested_tools"),
				AfterTools:           eventDataStringSlice(callMap, "after_tools"),
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
