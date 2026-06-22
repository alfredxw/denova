package interactive

import (
	"fmt"
	"strings"
)

func sanitizeDisplayEvents(events []DisplayEvent) []DisplayEvent {
	if len(events) == 0 {
		return nil
	}
	result := make([]DisplayEvent, 0, len(events))
	for _, event := range events {
		role := strings.TrimSpace(event.Role)
		if role == "" {
			continue
		}
		if role != "tool_call" && role != "tool_result" && role != "thinking" {
			continue
		}
		name := strings.TrimSpace(event.Name)
		content := strings.TrimSpace(event.Content)
		status := strings.TrimSpace(event.Status)
		if role == "tool_call" {
			if name == "" {
				name = content
			}
			if name == "" {
				name = "unknown_tool"
			}
			content = name
			if status == "" {
				status = "running"
			}
		}
		next := DisplayEvent{
			ID:        strings.TrimSpace(event.ID),
			Role:      role,
			Content:   content,
			Name:      name,
			Args:      event.Args,
			Status:    status,
			Result:    event.Result,
			CreatedAt: strings.TrimSpace(event.CreatedAt),
		}
		result = append(result, next)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func boundedRate(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func sanitizeTokenUsageEvent(event TokenUsageEvent) TokenUsageEvent {
	next := TokenUsageEvent{
		V:                    schemaVersion,
		Type:                 TokenUsageEventType,
		ID:                   strings.TrimSpace(event.ID),
		StoryID:              strings.TrimSpace(event.StoryID),
		BranchID:             strings.TrimSpace(event.BranchID),
		CreatedAt:            strings.TrimSpace(event.CreatedAt),
		RunID:                strings.TrimSpace(event.RunID),
		AgentKind:            strings.TrimSpace(event.AgentKind),
		PromptTokens:         nonNegativeInt(event.PromptTokens),
		CachedPromptTokens:   nonNegativeInt(event.CachedPromptTokens),
		UncachedPromptTokens: nonNegativeInt(event.UncachedPromptTokens),
		CacheHitRate:         boundedRate(event.CacheHitRate),
		CompletionTokens:     nonNegativeInt(event.CompletionTokens),
		ReasoningTokens:      nonNegativeInt(event.ReasoningTokens),
		TotalTokens:          nonNegativeInt(event.TotalTokens),
		ModelCalls:           nonNegativeInt(event.ModelCalls),
		GeneratedBytes:       nonNegativeInt(event.GeneratedBytes),
		UsageCalls:           sanitizeTokenUsageCalls(event.UsageCalls),
	}
	if next.ID == "" {
		if next.RunID != "" {
			next.ID = next.RunID
		} else {
			next.ID = newID("usage")
		}
	}
	return next
}

func sanitizeTokenUsageCalls(calls []TokenUsageCall) []TokenUsageCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]TokenUsageCall, 0, len(calls))
	for _, call := range calls {
		next := TokenUsageCall{
			Index:                nonNegativeInt(call.Index),
			CreatedAt:            call.CreatedAt,
			FinishReason:         call.FinishReason,
			RequestedTools:       append([]string(nil), call.RequestedTools...),
			AfterTools:           append([]string(nil), call.AfterTools...),
			PromptTokens:         nonNegativeInt(call.PromptTokens),
			CachedPromptTokens:   nonNegativeInt(call.CachedPromptTokens),
			UncachedPromptTokens: nonNegativeInt(call.UncachedPromptTokens),
			CacheHitRate:         boundedRate(call.CacheHitRate),
			CompletionTokens:     nonNegativeInt(call.CompletionTokens),
			ReasoningTokens:      nonNegativeInt(call.ReasoningTokens),
			TotalTokens:          nonNegativeInt(call.TotalTokens),
		}
		if next.PromptTokens == 0 && next.CompletionTokens == 0 && next.TotalTokens == 0 {
			continue
		}
		result = append(result, next)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func appendDisplayEvent(existing []DisplayEvent, next DisplayEvent) []DisplayEvent {
	events := sanitizeDisplayEvents(append(append([]DisplayEvent(nil), existing...), next))
	if len(events) == 0 {
		return nil
	}
	key := displayEventKey(next)
	if key == "" {
		return events
	}
	last := events[len(events)-1]
	for i := 0; i < len(events)-1; i++ {
		if displayEventKey(events[i]) != key {
			continue
		}
		events[i] = last
		return events[:len(events)-1]
	}
	return events
}

func displayEventKey(event DisplayEvent) string {
	role := strings.TrimSpace(event.Role)
	if id := strings.TrimSpace(event.ID); id != "" {
		return role + ":" + id
	}
	return ""
}

func applyStateOp(state map[string]any, op StateOp) {
	switch op.Op {
	case "set":
		setPath(state, op.Path, op.Value)
	case "merge":
		current, _ := getPath(state, op.Path).(map[string]any)
		if current == nil {
			current = map[string]any{}
		}
		if value, ok := op.Value.(map[string]any); ok {
			for k, v := range value {
				current[k] = v
			}
		}
		setPath(state, op.Path, current)
	case "push":
		current, _ := getPath(state, op.Path).([]any)
		setPath(state, op.Path, append(current, op.Value))
	case "pull":
		current, _ := getPath(state, op.Path).([]any)
		next := current[:0]
		for _, item := range current {
			if fmt.Sprint(item) != fmt.Sprint(op.Value) {
				next = append(next, item)
			}
		}
		setPath(state, op.Path, next)
	case "inc":
		current, _ := getPath(state, op.Path).(float64)
		by := 1.0
		if value, ok := op.Value.(float64); ok {
			by = value
		}
		setPath(state, op.Path, current+by)
	case "unset":
		unsetPath(state, op.Path)
	}
}

func getPath(root map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var current any = root
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[part]
	}
	return current
}

func setPath(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func unsetPath(root map[string]any, path string) {
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, _ := current[part].(map[string]any)
		if next == nil {
			return
		}
		current = next
	}
	delete(current, parts[len(parts)-1])
}
