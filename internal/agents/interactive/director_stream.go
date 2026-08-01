package interactive

import (
	producttools "denova/internal/agents/tools"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

type directorToolTextCounter struct {
	scanBuffer        string
	countingValue     bool
	valueDone         bool
	escapedValue      bool
	unicodeEscapeLeft int
}

func (c *directorToolTextCounter) countDelta(delta string, keys []string) int {
	if c == nil || c.valueDone || delta == "" {
		return 0
	}
	input := delta
	if !c.countingValue {
		c.scanBuffer += delta
		offset, ok := jsonStringValueOffsetAny(c.scanBuffer, keys)
		if !ok {
			c.trimScanBuffer()
			return 0
		}
		input = c.scanBuffer[offset:]
		c.scanBuffer = ""
		c.countingValue = true
	}
	return c.countValue(input)
}

func (c *directorToolTextCounter) trimScanBuffer() {
	const maxScanBuffer = 256
	if len(c.scanBuffer) > maxScanBuffer {
		c.scanBuffer = c.scanBuffer[len(c.scanBuffer)-maxScanBuffer:]
	}
}

func (c *directorToolTextCounter) countValue(input string) int {
	count := 0
	for input != "" {
		r, size := utf8.DecodeRuneInString(input)
		if size <= 0 {
			size = 1
		}
		input = input[size:]
		if c.unicodeEscapeLeft > 0 {
			c.unicodeEscapeLeft--
			if c.unicodeEscapeLeft == 0 {
				count++
			}
			continue
		}
		if c.escapedValue {
			c.escapedValue = false
			if r == 'u' {
				c.unicodeEscapeLeft = 4
				continue
			}
			count++
			continue
		}
		switch r {
		case '\\':
			c.escapedValue = true
		case '"':
			c.valueDone = true
			return count
		default:
			count++
		}
	}
	return count
}

type directorToolPathArgPreview struct {
	key  string
	path string
}

func directorToolPathArgPreviewFromArgs(args string) (directorToolPathArgPreview, bool) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return directorToolPathArgPreview{}, false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		for _, key := range []string{"file_path", "path", "filename", "file"} {
			value, _ := payload[key].(string)
			value = strings.TrimSpace(value)
			if value != "" {
				return directorToolPathArgPreview{key: key, path: value}, true
			}
		}
	}
	for _, key := range []string{"file_path", "path", "filename", "file"} {
		value, ok := partialDirectorJSONStringField(trimmed, key)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return directorToolPathArgPreview{key: key, path: value}, true
		}
	}
	return directorToolPathArgPreview{}, false
}

func partialDirectorJSONStringField(args, key string) (string, bool) {
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

func jsonStringValueOffsetAny(data string, keys []string) (int, bool) {
	for _, key := range keys {
		if offset, ok := jsonStringValueOffset(data, key); ok {
			return offset, true
		}
	}
	return 0, false
}

func jsonStringValueOffset(data, key string) (int, bool) {
	needle := `"` + key + `"`
	index := strings.Index(data, needle)
	if index < 0 {
		return 0, false
	}
	afterKey := strings.TrimLeft(data[index+len(needle):], " \n\r\t")
	if afterKey == "" || !strings.HasPrefix(afterKey, ":") {
		return 0, false
	}
	afterColon := strings.TrimLeft(afterKey[1:], " \n\r\t")
	if afterColon == "" || !strings.HasPrefix(afterColon, `"`) {
		return 0, false
	}
	return len(data) - len(afterColon) + 1, true
}

func directorToolGeneratedTextKeys(name string) []string {
	switch strings.TrimSpace(name) {
	case "edit":
		return []string{"new_string", "content"}
	case producttools.SubmitDirectorPlanUpdateToolName:
		return []string{"plan", "agent_brief", "lore_context"}
	default:
		return []string{"content", "new_string"}
	}
}

func directorToolStateKey(id, name string) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	return "name:" + strings.TrimSpace(name)
}
