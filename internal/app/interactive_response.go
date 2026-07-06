package app

import (
	"fmt"
	"strings"
)

const (
	thinkOpenTag  = "<think>"
	thinkCloseTag = "</think>"
)

func parseInteractiveAssistantOutput(content string) (string, error) {
	narrative := extractNarrative(content)
	if strings.TrimSpace(narrative) == "" {
		return "", fmt.Errorf("互动叙事内容为空")
	}
	return strings.TrimSpace(narrative), nil
}

func extractNarrative(content string) string {
	return stripThinkPrelude(content)
}

// stripThinkPrelude 移除叙事正文里残留的思考块，兜底模型把思考混入正文的情况：
// 配对 <think>...</think>、未闭合 <think>...，以及无开始标签、仅以 </think> 收尾的前言。
func stripThinkPrelude(s string) string {
	for {
		open := thinkIndexFold(s, thinkOpenTag)
		if open < 0 {
			break
		}
		closeIdx := thinkIndexFold(s[open:], thinkCloseTag)
		if closeIdx < 0 {
			s = s[:open]
			break
		}
		s = s[:open] + s[open+closeIdx+len(thinkCloseTag):]
	}
	if closeIdx := thinkIndexFold(s, thinkCloseTag); closeIdx >= 0 {
		s = s[closeIdx+len(thinkCloseTag):]
	}
	return strings.TrimSpace(s)
}

func thinkIndexFold(s, sub string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(sub))
}
