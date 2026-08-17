package agentrun

import (
	"math"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

func toolNamesFromCalls(calls []agent.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func uncachedPromptTokens(promptTokens, cachedPromptTokens int) int {
	if promptTokens <= 0 {
		return 0
	}
	if cachedPromptTokens <= 0 {
		return promptTokens
	}
	if cachedPromptTokens >= promptTokens {
		return 0
	}
	return promptTokens - cachedPromptTokens
}

func roundRatio(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Round(value*10000) / 10000
}
