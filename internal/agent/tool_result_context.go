package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"denova/config"
)

type ToolResultContextPolicy struct {
	AgentKind    string
	Enabled      bool
	KeepRecent   int
	BudgetBytes  int
	PreviewChars int
}

func resolveToolResultContextPolicy(cfg *config.Config, agentKind string) ToolResultContextPolicy {
	settings := config.ResolveAgentContext(cfg, agentKind)
	return ToolResultContextPolicy{
		AgentKind:    strings.TrimSpace(agentKind),
		Enabled:      settings.ToolResultRetentionEnabled,
		KeepRecent:   settings.ToolResultKeepRecent,
		BudgetBytes:  settings.ToolResultContextBudgetKB * 1024,
		PreviewChars: settings.ToolResultPreviewChars,
	}
}

func ResolveToolResultContextPolicyForConversation(cfg *config.Config, agentKind string) ToolResultContextPolicy {
	return resolveToolResultContextPolicy(cfg, agentKind)
}

func (p ToolResultContextPolicy) normalized() ToolResultContextPolicy {
	if p.KeepRecent <= 0 {
		p.KeepRecent = config.DefaultToolResultKeepRecent
	}
	if p.BudgetBytes <= 0 {
		p.BudgetBytes = config.DefaultToolResultContextBudgetKB * 1024
	}
	if p.PreviewChars <= 0 {
		p.PreviewChars = config.DefaultToolResultPreviewChars
	}
	return p
}

type toolResultContextConversation interface {
	AppendContextMessage(msg *schema.Message) error
	ToolResultContextPolicy() ToolResultContextPolicy
}

type toolResultContextRecorder struct {
	conversation    toolResultContextConversation
	policy          ToolResultContextPolicy
	retainedCallIDs map[string]struct{}
}

func newToolResultContextRecorder(conversation Conversation) *toolResultContextRecorder {
	contextConversation, ok := conversation.(toolResultContextConversation)
	if !ok || contextConversation == nil {
		return &toolResultContextRecorder{}
	}
	policy := contextConversation.ToolResultContextPolicy().normalized()
	if !policy.Enabled {
		return &toolResultContextRecorder{}
	}
	return &toolResultContextRecorder{conversation: contextConversation, policy: policy}
}

func (r *toolResultContextRecorder) RecordAssistantToolCalls(msg *schema.Message, meta agentEventMetadata) {
	if r == nil || r.conversation == nil || meta.SubAgent || msg == nil || len(msg.ToolCalls) == 0 {
		return
	}
	next := assistantToolContextMessage(msg, r.policy)
	if next == nil {
		return
	}
	if err := r.conversation.AppendContextMessage(next); err != nil {
		logAgentContextPersistError("assistant_tool_calls", err)
		return
	}
	if r.retainedCallIDs == nil {
		r.retainedCallIDs = make(map[string]struct{}, len(next.ToolCalls))
	}
	for _, call := range next.ToolCalls {
		if callID := strings.TrimSpace(call.ID); callID != "" {
			r.retainedCallIDs[callID] = struct{}{}
		}
	}
}

func (r *toolResultContextRecorder) RecordToolResult(toolName, toolCallID, content string, meta agentEventMetadata) {
	if r == nil || r.conversation == nil || meta.SubAgent || isPlanProtocolToolName(toolName) || !retainToolContextAcrossTurns(toolName, r.policy) || !r.retainedCall(toolCallID) {
		return
	}
	msg := schema.ToolMessage(toolResultContextContent(toolName, toolCallID, content, r.policy), toolCallID, schema.WithToolName(toolName))
	if err := r.conversation.AppendContextMessage(msg); err != nil {
		logAgentContextPersistError("tool_result", err)
	}
}

func (r *toolResultContextRecorder) retainedCall(toolCallID string) bool {
	if r == nil {
		return false
	}
	_, ok := r.retainedCallIDs[strings.TrimSpace(toolCallID)]
	return ok
}

func logAgentContextPersistError(kind string, err error) {
	log.Printf("[agent-run] persist tool result context failed kind=%s err=%v", kind, err)
}

func assistantToolContextMessage(msg *schema.Message, policy ToolResultContextPolicy) *schema.Message {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil
	}
	calls := make([]schema.ToolCall, 0, len(msg.ToolCalls))
	for _, call := range msg.ToolCalls {
		if isPlanProtocolToolName(call.Function.Name) || !retainToolContextAcrossTurns(call.Function.Name, policy) {
			continue
		}
		next := call
		arguments, valid := retainedToolCallArguments(next.Function.Arguments, next.Function.Name, next.ID, policy.PreviewChars)
		if !valid {
			continue
		}
		next.Function.Arguments = arguments
		calls = append(calls, next)
	}
	if len(calls) == 0 {
		return nil
	}
	return schema.AssistantMessage("", calls)
}

func retainedToolCallArguments(arguments, toolName, toolCallID string, previewChars int) (string, bool) {
	arguments = strings.TrimSpace(arguments)
	if err := validateToolArgumentsJSON(arguments); err != nil {
		return "", false
	}
	if arguments == "" {
		return "{}", true
	}
	if previewChars <= 0 || countRunes(arguments) <= previewChars {
		return arguments, true
	}
	receipt, err := json.Marshal(struct {
		Schema        string `json:"schema"`
		ToolName      string `json:"tool_name"`
		ToolCallID    string `json:"tool_call_id"`
		OriginalChars int    `json:"original_chars"`
		Note          string `json:"note"`
	}{
		Schema:        "tool_call.args_omitted.v1",
		ToolName:      strings.TrimSpace(toolName),
		ToolCallID:    strings.TrimSpace(toolCallID),
		OriginalChars: countRunes(arguments),
		Note:          "Original tool arguments were omitted from retained context. Re-run the tool if exact arguments are required.",
	})
	if err != nil {
		return "", false
	}
	return string(receipt), true
}

func toolResultContextContent(toolName, toolCallID, content string, policy ToolResultContextPolicy) string {
	content = stripToolResultMetadata(content)
	content = semanticToolResultContextContent(toolName, content, policy)
	content = strings.TrimRight(content, "\n")
	if content == "" {
		content = "(无返回内容)"
	}
	// JSON bodies must not be mid-cut at PreviewChars (default 2000): the "\n..."
	// marker makes providers that re-parse tool content fail with
	// "unexpected control character ... char 2000".
	if policy.PreviewChars > 0 && countRunes(content) > policy.PreviewChars && looksLikeJSONPayload(content) {
		return toolResultPlaceholderMessage(&schema.Message{
			Role: schema.Tool, Content: content, ToolName: toolName, ToolCallID: toolCallID,
		}, "tool_result_json_preview_exceeded").Content
	}
	return limitContextText(content, policy.PreviewChars, fmt.Sprintf(
		"\n[Denova tool result preview truncated for context]\ntool_name: %s\ntool_call_id: %s\noriginal_chars: %d\npreview_chars: %d",
		toolName,
		toolCallID,
		countRunes(content),
		policy.PreviewChars,
	))
}

func stripToolResultMetadata(content string) string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return ""
	}
	for _, separator := range []string{"\n\n" + toolResultMetadataHeader, "\n" + toolResultMetadataHeader} {
		if before, _, ok := strings.Cut(content, separator); ok {
			return strings.TrimRight(before, "\n")
		}
	}
	if strings.HasPrefix(strings.TrimSpace(content), toolResultMetadataHeader) {
		return ""
	}
	return content
}

func limitContextText(content string, maxRunes int, marker string) string {
	if maxRunes <= 0 || content == "" {
		return content
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return strings.TrimRight(string(runes[:maxRunes]), "\n") + marker
}

func looksLikeJSONPayload(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[")
}

func applyToolResultContextPolicy(messages []*schema.Message, policy ToolResultContextPolicy) []*schema.Message {
	if len(messages) == 0 {
		return messages
	}
	policy = policy.normalized()
	if !policy.Enabled {
		return removeToolContextMessages(messages)
	}
	messages = filterSemanticToolContextMessages(messages, policy)
	if len(messages) == 0 {
		return messages
	}
	toolIndexes := make([]int, 0)
	for i, msg := range messages {
		if msg != nil && msg.Role == schema.Tool {
			toolIndexes = append(toolIndexes, i)
		}
	}
	if len(toolIndexes) == 0 {
		return messages
	}
	keepFull := make(map[int]bool, policy.KeepRecent)
	for i := len(toolIndexes) - 1; i >= 0 && len(keepFull) < policy.KeepRecent; i-- {
		keepFull[toolIndexes[i]] = true
	}
	result := append([]*schema.Message(nil), messages...)
	used := 0
	for i := len(result) - 1; i >= 0; i-- {
		msg := result[i]
		if msg == nil || msg.Role != schema.Tool {
			continue
		}
		msg = sanitizedToolContextMessage(msg, policy)
		result[i] = msg
		size := len(msg.Content)
		if used+size <= policy.BudgetBytes {
			used += size
			continue
		}
		reason := "tool_result_context_budget_exceeded"
		if keepFull[i] {
			reason = "recent_tool_result_context_budget_exceeded"
		}
		result[i] = toolResultPlaceholderMessage(msg, reason)
	}
	return result
}

func sanitizedToolContextMessage(msg *schema.Message, policy ToolResultContextPolicy) *schema.Message {
	if msg == nil || msg.Role != schema.Tool {
		return msg
	}
	content := semanticToolResultContextContent(msg.ToolName, stripToolResultMetadata(msg.Content), policy)
	if content == msg.Content {
		return msg
	}
	next := *msg
	next.Content = content
	return &next
}

func ApplyToolResultContextPolicyForConversation(messages []*schema.Message, policy ToolResultContextPolicy) []*schema.Message {
	return applyToolResultContextPolicy(messages, policy)
}

func removeToolContextMessages(messages []*schema.Message) []*schema.Message {
	filtered := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.Tool {
			continue
		}
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			if strings.TrimSpace(msg.Content) == "" {
				continue
			}
			next := *msg
			next.ToolCalls = nil
			filtered = append(filtered, &next)
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func toolResultPlaceholderMessage(msg *schema.Message, reason string) *schema.Message {
	if msg == nil {
		return nil
	}
	content := fmt.Sprintf(`[Denova retained tool result placeholder]
schema: tool_result.placeholder.v1
reason: %s
tool_name: %s
tool_call_id: %s
omitted_bytes: %d

The full retained tool result was omitted from this model context. Re-run the tool if exact content is required.`,
		reason,
		strings.TrimSpace(msg.ToolName),
		strings.TrimSpace(msg.ToolCallID),
		len(msg.Content),
	)
	return schema.ToolMessage(content, msg.ToolCallID, schema.WithToolName(msg.ToolName))
}
