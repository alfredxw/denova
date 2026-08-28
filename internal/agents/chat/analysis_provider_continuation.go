package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func contextAnalysisAssistantParts(id, source string, msg *agent.Message) []ContextAnalysisPart {
	if msg == nil || msg.Role != agent.Assistant ||
		(strings.TrimSpace(msg.ReasoningContent) == "" && len(providers.ContinuationExtra(msg.Extra)) == 0) {
		return nil
	}
	parts := make([]ContextAnalysisPart, 0, 4)
	if strings.TrimSpace(msg.ReasoningContent) != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID: id + "_reasoning", Source: source, Title: "推理摘要 / Reasoning summary",
			Role: string(agent.Assistant), Kind: "reasoning", Content: msg.ReasoningContent,
		}))
	}
	if strings.TrimSpace(msg.Content) != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID: id + "_body", Source: source, Title: "助手正文 / Assistant body",
			Role: string(agent.Assistant), Kind: "body", Content: msg.Content,
		}))
	}
	if len(msg.ToolCalls) > 0 {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID: id + "_tool_calls", Source: source, Title: "工具调用 / Tool calls",
			Role: string(agent.Assistant), Kind: "tool_call",
			ToolName: contextAnalysisToolCallNames(msg.ToolCalls), ToolCallID: contextAnalysisToolCallIDs(msg.ToolCalls),
			Content: contextAnalysisToolCallsContent(msg.ToolCalls),
		}))
	}
	if continuation, ok := contextAnalysisProviderContinuationPart(id+"_provider_continuation", source, msg.Extra); ok {
		parts = append(parts, continuation)
	}
	return parts
}

// contextAnalysisProviderContinuationPart exposes only routing identity and
// payload size. Raw signed or encrypted provider state must never be copied
// into the user-facing analyzer.
func contextAnalysisProviderContinuationPart(id, source string, extra map[string]any) (ContextAnalysisPart, bool) {
	selected := providers.ContinuationExtra(extra)
	stored, ok := selected[providers.ExtraKeyContinuation]
	if !ok {
		return ContextAnalysisPart{}, false
	}
	input := ContextAnalysisPartInput{
		ID: id, Source: source, Title: "不透明模型延续状态 / Opaque model continuation",
		Role: string(agent.Assistant), Kind: "provider_continuation",
		Content: "Opaque provider payload retained; raw content hidden.\n已保留不透明模型载荷；原始内容已隐藏。",
	}
	data, err := json.Marshal(stored)
	if err != nil {
		input.Note = "metadata unavailable / 元数据不可用"
		return NewContextAnalysisPart(input), true
	}
	var continuation providers.Continuation
	if err := json.Unmarshal(data, &continuation); err != nil {
		input.Note = "metadata unavailable / 元数据不可用"
		return NewContextAnalysisPart(input), true
	}
	input.Note = fmt.Sprintf(
		"provider=%s · protocol=%s · model=%s · payload_bytes=%d",
		continuation.Provider, continuation.Protocol, strings.TrimSpace(continuation.Model), len(continuation.Payload),
	)
	return NewContextAnalysisPart(input), true
}
