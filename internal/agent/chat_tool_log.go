package agent

import (
	"encoding/json"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

func logToolCall(name, id string, argsBytes int, source string) {
	log.Printf("[agent-tool] call source=%s name=%s id=%s args_bytes=%d", source, name, id, argsBytes)
}

func logToolPath(name, id, path string) {
	log.Printf("[agent-tool] target_path name=%s id=%s path=%s", name, id, path)
}

func logToolResult(name, id, content string) {
	if looksLikeToolFailure(content) {
		log.Printf("[agent-tool] result suspected_failure=true name=%s id=%s bytes=%d preview=%q", name, id, len(content), safeLogPreview(content, 300))
		return
	}
	log.Printf("[agent-tool] result name=%s id=%s bytes=%d", name, id, len(content))
}

func toolPathFromArgs(args string) string {
	args = strings.TrimSpace(args)
	if args == "" || !strings.HasPrefix(args, "{") {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return toolPathFromPartialArgs(args)
	}
	for _, key := range []string{"path", "file_path", "filename", "file", "pattern"} {
		value, _ := payload[key].(string)
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func toolPathFromPartialArgs(args string) string {
	const scanLimit = 8 * 1024
	if len(args) > scanLimit {
		args = args[:scanLimit]
	}
	for _, key := range []string{"path", "file_path", "filename", "file", "pattern"} {
		value, ok := partialToolStringField(args, key)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return value
		}
	}
	return ""
}

func partialToolStringField(args, key string) (string, bool) {
	needle := `"` + key + `"`
	searchFrom := 0
	for {
		index := strings.Index(args[searchFrom:], needle)
		if index < 0 {
			return "", false
		}
		index += searchFrom
		afterKey := strings.TrimLeft(args[index+len(needle):], " \n\r\t")
		if !strings.HasPrefix(afterKey, ":") {
			searchFrom = index + len(needle)
			continue
		}
		afterColon := strings.TrimLeft(afterKey[1:], " \n\r\t")
		if !strings.HasPrefix(afterColon, `"`) {
			searchFrom = index + len(needle)
			continue
		}
		end := partialJSONStringLiteralEnd(afterColon)
		if end < 0 {
			return "", false
		}
		var value string
		if err := json.Unmarshal([]byte(afterColon[:end+1]), &value); err != nil {
			return "", false
		}
		return value, true
	}
}

func partialJSONStringLiteralEnd(input string) int {
	escaped := false
	for i := 1; i < len(input); i++ {
		switch {
		case escaped:
			escaped = false
		case input[i] == '\\':
			escaped = true
		case input[i] == '"':
			return i
		}
	}
	return -1
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

func safeLogPreview(content string, limit int) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	content = strings.ReplaceAll(content, "\r", "\\r")
	if len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "..."
}

// mergeToolCalls 合并流式 frame 中分散的 tool call 信息。
func mergeToolCalls(existing []schema.ToolCall, incoming []schema.ToolCall) []schema.ToolCall {
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
			existing = append(existing, schema.ToolCall{})
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
