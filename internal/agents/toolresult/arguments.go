package toolresult

import (
	"encoding/json"
	"strings"
)

// TargetFromArguments extracts the stable file-like target used by tool
// receipts, lifecycle logs, and recovery hints. It tolerates partially streamed
// JSON so callers can diagnose interrupted model output without inventing a
// second parser.
func TargetFromArguments(arguments string) string {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" || !strings.HasPrefix(arguments, "{") {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return targetFromPartialArguments(arguments)
	}
	for _, key := range targetKeys {
		value, _ := payload[key].(string)
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var targetKeys = [...]string{"path", "file_path", "filename", "file", "pattern"}

func targetFromPartialArguments(arguments string) string {
	const scanLimit = 8 * 1024
	if len(arguments) > scanLimit {
		arguments = arguments[:scanLimit]
	}
	for _, key := range targetKeys {
		value, ok := partialStringField(arguments, key)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return value
		}
	}
	return ""
}

func partialStringField(arguments, key string) (string, bool) {
	needle := `"` + key + `"`
	searchFrom := 0
	for {
		index := strings.Index(arguments[searchFrom:], needle)
		if index < 0 {
			return "", false
		}
		index += searchFrom
		afterKey := strings.TrimLeft(arguments[index+len(needle):], " \n\r\t")
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
	for index := 1; index < len(input); index++ {
		switch {
		case escaped:
			escaped = false
		case input[index] == '\\':
			escaped = true
		case input[index] == '"':
			return index
		}
	}
	return -1
}
