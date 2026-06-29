package middleware

import (
	"encoding/json"
	"strconv"
	"strings"
)

type toolPathArgPreview struct {
	key  string
	path string
}

func toolPathArgPreviewFromArgs(args string) (toolPathArgPreview, bool) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return toolPathArgPreview{}, false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		for _, key := range []string{"file_path", "path", "filename", "file"} {
			value, _ := payload[key].(string)
			value = strings.TrimSpace(value)
			if value != "" {
				return toolPathArgPreview{key: displayToolPathKey(key), path: value}, true
			}
		}
	}
	for _, key := range []string{"file_path", "path", "filename", "file"} {
		value, ok := partialJSONStringField(trimmed, key)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return toolPathArgPreview{key: displayToolPathKey(key), path: value}, true
		}
	}
	return toolPathArgPreview{}, false
}

func marshalToolPathArgPreview(preview toolPathArgPreview) string {
	key := preview.key
	if key == "" {
		key = "path"
	}
	keyData, err := json.Marshal(key)
	if err != nil {
		return ""
	}
	pathData, err := json.Marshal(preview.path)
	if err != nil {
		return ""
	}
	return "{" + string(keyData) + ":" + string(pathData) + "}"
}

func displayToolPathKey(key string) string {
	switch key {
	case "file_path", "path":
		return key
	default:
		return "path"
	}
}

func partialJSONStringField(args, key string) (string, bool) {
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
		value := afterColon[1:]
		escaped := false
		for i := 0; i < len(value); i++ {
			switch value[i] {
			case '\\':
				escaped = !escaped
			case '"':
				if escaped {
					escaped = false
					continue
				}
				decoded, err := strconv.Unquote(`"` + value[:i] + `"`)
				if err != nil {
					return value[:i], true
				}
				return decoded, true
			default:
				escaped = false
			}
		}
		return "", false
	}
}

func isNovelChapterBodyPath(path string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	for strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}
	if strings.HasPrefix(normalized, "chapters/") || strings.HasPrefix(normalized, "drafts/") {
		return true
	}
	parts := strings.Split(normalized, "/")
	for index, part := range parts {
		if part != ".nova" || index+2 >= len(parts) {
			continue
		}
		if parts[index+2] == "chapters" || parts[index+2] == "drafts" {
			return true
		}
	}
	return false
}

func toolArgsDisplayDelta(previous, current string) string {
	if current == previous {
		return ""
	}
	if strings.HasPrefix(current, previous) {
		return strings.TrimPrefix(current, previous)
	}
	return current
}
