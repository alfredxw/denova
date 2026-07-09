package agentui

import (
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/session"
)

const (
	DataTypeActivity          = "data-agent-activity"
	DataTypeClear             = "data-agent-clear"
	DataTypeContextCompaction = "data-agent-context-compaction"
	DataTypeError             = "data-agent-error"
	DataTypeInteractiveImage  = "data-agent-interactive-image"
	DataTypePlanQuestion      = "data-agent-plan-question"
	DataTypeProposedPlan      = "data-agent-proposed-plan"
	DataTypeRuleRoll          = "data-agent-rule-roll"
	DataTypeSystem            = "data-agent-system"
	DataTypeTokenUsage        = "data-agent-token-usage"
	DataTypeToolResult        = "data-agent-tool-result"
)

// Message is the backend JSON shape for AI SDK UI messages used by the web app.
// It intentionally mirrors the wire fields without depending on TypeScript types.
type Message struct {
	ID       string           `json:"id"`
	Role     string           `json:"role"`
	Metadata map[string]any   `json:"metadata,omitempty"`
	Parts    []map[string]any `json:"parts"`
}

// MessagesFromHistory converts the existing session history into AI SDK UI
// messages at read time. It does not mutate stored session data.
func MessagesFromHistory(entries []session.HistoryEntry) []Message {
	result := make([]Message, 0, len(entries))
	for index, entry := range entries {
		msg, ok := messageFromHistoryEntry(entry, index)
		if ok {
			result = append(result, msg)
		}
	}
	return result
}

func messageFromHistoryEntry(entry session.HistoryEntry, index int) (Message, bool) {
	if entry.Type == "clear" {
		return assistantDataMessage(entry, index, DataTypeClear, map[string]any{
			"created_at": formatEntryTime(entry),
		}), true
	}
	if entry.Content == "" && entry.Role != "tool_call" {
		return Message{}, false
	}

	switch entry.Role {
	case "user":
		return Message{
			ID:       historyMessageID(entry, index),
			Role:     "user",
			Metadata: metadataFromHistoryEntry(entry),
			Parts:    []map[string]any{textPart(entry.Content, "done", nil)},
		}, true
	case "assistant":
		return Message{
			ID:       historyMessageID(entry, index),
			Role:     "assistant",
			Metadata: metadataFromHistoryEntry(entry),
			Parts:    []map[string]any{textPart(entry.Content, "done", nil)},
		}, true
	case "thinking":
		return Message{
			ID:       historyMessageID(entry, index),
			Role:     "assistant",
			Metadata: metadataFromHistoryEntry(entry),
			Parts: []map[string]any{{
				"type":  "reasoning",
				"text":  entry.Content,
				"state": "done",
			}},
		}, true
	case "tool_call":
		return Message{
			ID:       historyMessageID(entry, index),
			Role:     "assistant",
			Metadata: metadataFromHistoryEntry(entry),
			Parts:    []map[string]any{toolPartFromHistory(entry)},
		}, true
	case "tool_result":
		return assistantDataMessage(entry, index, DataTypeToolResult, entryPayload(entry)), true
	case "rule_roll":
		return assistantDataMessage(entry, index, DataTypeRuleRoll, entryPayload(entry)), true
	case "interactive_image":
		return assistantDataMessage(entry, index, DataTypeInteractiveImage, entryPayload(entry)), true
	case "context_compaction":
		return assistantDataMessage(entry, index, DataTypeContextCompaction, entryPayload(entry)), true
	case "token_usage":
		return assistantDataMessage(entry, index, DataTypeTokenUsage, entryPayload(entry)), true
	case "plan_question":
		return assistantDataMessage(entry, index, DataTypePlanQuestion, entryPayload(entry)), true
	case "proposed_plan":
		return assistantDataMessage(entry, index, DataTypeProposedPlan, entryPayload(entry)), true
	case "system":
		return assistantDataMessage(entry, index, DataTypeSystem, entryPayload(entry)), true
	case "error":
		return assistantDataMessage(entry, index, DataTypeError, entryPayload(entry)), true
	default:
		return assistantDataMessage(entry, index, DataTypeActivity, entryPayload(entry)), true
	}
}

func assistantDataMessage(entry session.HistoryEntry, index int, dataType string, data map[string]any) Message {
	return Message{
		ID:       historyMessageID(entry, index),
		Role:     "assistant",
		Metadata: metadataFromHistoryEntry(entry),
		Parts: []map[string]any{{
			"type": dataType,
			"id":   historyMessageID(entry, index),
			"data": data,
		}},
	}
}

func toolPartFromHistory(entry session.HistoryEntry) map[string]any {
	input := parseJSONValue(entry.Args)
	state := "input-available"
	part := map[string]any{
		"type":       "dynamic-tool",
		"toolName":   firstNonEmpty(entry.Name, "unknown_tool"),
		"toolCallId": historyToolCallID(entry),
		"state":      state,
		"input":      input,
	}
	if entry.Status == "error" {
		part["state"] = "output-error"
		part["errorText"] = firstNonEmpty(entry.Result, entry.Content, "tool failed")
		return part
	}
	if entry.Result != "" || entry.Status == "success" {
		part["state"] = "output-available"
		part["output"] = entry.Result
	}
	if entry.Illustration != nil {
		part["toolMetadata"] = map[string]any{"illustration": entry.Illustration}
	}
	return part
}

func entryPayload(entry session.HistoryEntry) map[string]any {
	payload := map[string]any{
		"type":       entry.Type,
		"id":         entry.ID,
		"role":       entry.Role,
		"content":    entry.Content,
		"name":       entry.Name,
		"args":       entry.Args,
		"status":     entry.Status,
		"result":     entry.Result,
		"created_at": formatEntryTime(entry),
	}
	if entry.Illustration != nil {
		payload["illustration"] = entry.Illustration
	}
	addUsagePayload(payload, entry)
	addMetadataPayload(payload, entry)
	return payload
}

func metadataFromHistoryEntry(entry session.HistoryEntry) map[string]any {
	metadata := map[string]any{}
	addMetadataPayload(metadata, entry)
	if createdAt := formatEntryTime(entry); createdAt != "" {
		metadata["created_at"] = createdAt
	}
	if entry.Type != "" {
		metadata["history_type"] = entry.Type
	}
	if entry.Role != "" {
		metadata["display_role"] = entry.Role
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func addMetadataPayload(target map[string]any, entry session.HistoryEntry) {
	if entry.RunID != "" {
		target["run_id"] = entry.RunID
	}
	if entry.AgentKind != "" {
		target["agent_kind"] = entry.AgentKind
	}
	if entry.AgentName != "" {
		target["agent_name"] = entry.AgentName
	}
	if entry.RootAgentName != "" {
		target["root_agent_name"] = entry.RootAgentName
	}
	if len(entry.RunPath) > 0 {
		target["run_path"] = append([]string(nil), entry.RunPath...)
	}
	if entry.SubAgent {
		target["subagent"] = true
	}
	if entry.SubAgentSessionID != "" {
		target["subagent_session_id"] = entry.SubAgentSessionID
	}
	if entry.SubAgentType != "" {
		target["subagent_type"] = entry.SubAgentType
	}
	if len(entry.SSEHiddenFields) > 0 {
		target["sse_hidden_fields"] = append([]string(nil), entry.SSEHiddenFields...)
	}
	if entry.SSEHiddenReason != "" {
		target["sse_hidden_reason"] = entry.SSEHiddenReason
	}
	if entry.SSEDisplayNotice != "" {
		target["sse_display_notice"] = entry.SSEDisplayNotice
	}
	if entry.SSEGeneratedChars > 0 {
		target["sse_generated_chars"] = entry.SSEGeneratedChars
	}
}

func addUsagePayload(target map[string]any, entry session.HistoryEntry) {
	if entry.PromptTokens > 0 {
		target["prompt_tokens"] = entry.PromptTokens
	}
	if entry.CachedPromptTokens > 0 {
		target["cached_prompt_tokens"] = entry.CachedPromptTokens
	}
	if entry.UncachedPromptTokens > 0 {
		target["uncached_prompt_tokens"] = entry.UncachedPromptTokens
	}
	if entry.CacheHitRate > 0 {
		target["cache_hit_rate"] = entry.CacheHitRate
	}
	if entry.CompletionTokens > 0 {
		target["completion_tokens"] = entry.CompletionTokens
	}
	if entry.ReasoningTokens > 0 {
		target["reasoning_tokens"] = entry.ReasoningTokens
	}
	if entry.TotalTokens > 0 {
		target["total_tokens"] = entry.TotalTokens
	}
	if entry.ModelCalls > 0 {
		target["model_calls"] = entry.ModelCalls
	}
	if entry.GeneratedBytes > 0 {
		target["generated_bytes"] = entry.GeneratedBytes
	}
	if len(entry.UsageCalls) > 0 {
		target["usage_calls"] = entry.UsageCalls
	}
}

func historyMessageID(entry session.HistoryEntry, index int) string {
	if entry.ID != "" {
		return entry.ID
	}
	if !entry.CreatedAt.IsZero() {
		return fmt.Sprintf("history-%s-%d", entry.CreatedAt.Format("20060102150405.000000000"), index)
	}
	return fmt.Sprintf("history-%d", index)
}

func historyToolCallID(entry session.HistoryEntry) string {
	if entry.ID != "" {
		return entry.ID
	}
	return "tool-" + firstNonEmpty(entry.Name, "unknown")
}

func textPart(text, state string, providerMetadata map[string]any) map[string]any {
	part := map[string]any{
		"type": "text",
		"text": text,
	}
	if state != "" {
		part["state"] = state
	}
	if len(providerMetadata) > 0 {
		part["providerMetadata"] = providerMetadata
	}
	return part
}

func parseJSONValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatEntryTime(entry session.HistoryEntry) string {
	if entry.CreatedAt.IsZero() {
		return ""
	}
	return entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
}
