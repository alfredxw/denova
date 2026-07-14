package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
)

// State update paths are RFC 6901 JSON Pointers. These helpers keep pointer
// parsing and nested-object mutation separate from schema compilation.
func parseStateUpdatePath(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") || path == "/" {
		return nil, fmt.Errorf("path 必须是以 / 开头的非空 JSON Pointer")
	}
	rawSegments := strings.Split(path[1:], "/")
	segments := make([]string, 0, len(rawSegments))
	for _, raw := range rawSegments {
		segment, err := decodeStateUpdatePathSegment(raw)
		if err != nil {
			return nil, err
		}
		if segment == "" {
			return nil, fmt.Errorf("状态路径不能包含空段")
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

func decodeStateUpdatePathSegment(value string) (string, error) {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			result.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) {
			return "", fmt.Errorf("JSON Pointer 包含无效的 ~ 转义")
		}
		index++
		switch value[index] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", fmt.Errorf("JSON Pointer 只允许 ~0 和 ~1 转义")
		}
	}
	return result.String(), nil
}

func formatStateUpdatePath(segments []string) string {
	escaped := make([]string, len(segments))
	for index, segment := range segments {
		segment = strings.ReplaceAll(segment, "~", "~0")
		escaped[index] = strings.ReplaceAll(segment, "/", "~1")
	}
	return "/" + strings.Join(escaped, "/")
}

func overlappingStateUpdatePath(existing [][]string, candidate []string) string {
	for _, current := range existing {
		limit := len(current)
		if len(candidate) < limit {
			limit = len(candidate)
		}
		matches := true
		for index := 0; index < limit; index++ {
			if current[index] != candidate[index] {
				matches = false
				break
			}
		}
		if matches {
			return formatStateUpdatePath(current)
		}
	}
	return ""
}

func cloneStateUpdateObject(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	data, err := json.Marshal(object)
	if err != nil {
		return nil, false
	}
	result := map[string]any{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}
	return result, true
}

func stateUpdateNestedValue(root map[string]any, path []string) (any, bool) {
	current := any(root)
	for _, segment := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setStateUpdateNestedValue(root map[string]any, path []string, value any, requireExisting bool) error {
	current := root
	for index, segment := range path {
		if index == len(path)-1 {
			if requireExisting {
				if _, exists := current[segment]; !exists {
					return fmt.Errorf("目标叶子不存在")
				}
			}
			current[segment] = value
			return nil
		}
		next, exists := current[segment]
		if !exists {
			if requireExisting {
				return fmt.Errorf("目标子路径不存在: %s", segment)
			}
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("目标子路径不是 object: %s", segment)
		}
		current = child
	}
	return fmt.Errorf("目标子路径不能为空")
}
