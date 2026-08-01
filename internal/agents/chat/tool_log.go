package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

func logToolCall(name, id string, argsBytes int, source string) {
	slog.InfoContext(context.Background(), fmt.Sprintf("[agent-tool] call source=%s name=%s id=%s args_bytes=%d", source, name, id, argsBytes))
}

func logToolPath(name, id, path string) {
	slog.InfoContext(context.Background(), fmt.Sprintf("[agent-tool] target_path name=%s id=%s path=%s", name, id, path))
}

func logToolResult(name, id, content string) {
	if looksLikeToolFailure(content) {
		// Tool bodies may contain credentials, private file contents, or user
		// text. Failure logs retain classification and size only; recoverable
		// diagnostics belong in the bounded result/artifact path, not process logs.
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-tool] result suspected_failure=true name=%s id=%s bytes=%d", name, id, len(content)))
		return
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[agent-tool] result name=%s id=%s bytes=%d", name, id, len(content)))
}

func looksLikeToolFailure(content string) bool {
	text := strings.ToLower(content)
	failureKeywords := []string{
		"error", "failed", "failure", "panic", "exception", "traceback",
		"permission denied", "not found", "timeout", "timed out",
		"失败", "错误", "异常", "拒绝", "超时", "不存在",
	}
	for _, keyword := range failureKeywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// mergeToolCalls 合并流式 frame 中分散的 tool call 信息。
func mergeToolCalls(existing []agent.ToolCall, incoming []agent.ToolCall) []agent.ToolCall {
	for _, tc := range incoming {
		idx := tc.Index
		if idx == nil {
			if tc.Function.Name != "" {
				existing = append(existing, tc)
			}
			continue
		}

		i := *idx
		for len(existing) <= i {
			existing = append(existing, agent.ToolCall{})
		}
		if tc.Function.Name != "" {
			existing[i].Function.Name = tc.Function.Name
		}
		existing[i].Function.Arguments += tc.Function.Arguments
		if tc.ID != "" {
			existing[i].ID = tc.ID
		}
		existing[i].Index = tc.Index
	}
	return existing
}
