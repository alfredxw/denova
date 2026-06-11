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
		result = append(result, DisplayEvent{
			ID:        strings.TrimSpace(event.ID),
			Role:      role,
			Content:   content,
			Name:      name,
			Status:    status,
			CreatedAt: strings.TrimSpace(event.CreatedAt),
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
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
