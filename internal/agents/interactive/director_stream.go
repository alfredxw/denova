package interactive

import (
	producttools "denova/internal/agents/tools"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

type directorToolTextCounter struct {
	inString          bool
	countingValue     bool
	escapedValue      bool
	unicodeEscapeLeft int
	candidateKey      string
	candidateTooLong  bool
	pendingKey        string
	expectingValue    bool
}

func (c *directorToolTextCounter) countDelta(delta string, keys []string) int {
	if c == nil || delta == "" {
		return 0
	}
	count := 0
	for delta != "" {
		r, size := utf8.DecodeRuneInString(delta)
		if size <= 0 {
			size = 1
		}
		delta = delta[size:]
		if !c.inString {
			if c.expectingValue {
				if isJSONWhitespace(r) {
					continue
				}
				c.expectingValue = false
				if r == '"' {
					c.startString(true)
					continue
				}
			}
			if c.pendingKey != "" {
				if isJSONWhitespace(r) {
					continue
				}
				pendingKey := c.pendingKey
				c.pendingKey = ""
				if r == ':' {
					c.expectingValue = containsDirectorGeneratedTextKey(keys, pendingKey)
					continue
				}
			}
			if r == '"' {
				c.startString(false)
			}
			continue
		}

		if !c.countingValue {
			if c.escapedValue {
				c.escapedValue = false
				c.candidateTooLong = true
				continue
			}
			switch r {
			case '\\':
				c.escapedValue = true
				c.candidateTooLong = true
			case '"':
				c.inString = false
				if !c.candidateTooLong && c.candidateKey != "" {
					c.pendingKey = c.candidateKey
				}
				c.candidateKey = ""
				c.candidateTooLong = false
			default:
				const maxCandidateKeyBytes = 64
				if !c.candidateTooLong && len(c.candidateKey)+size <= maxCandidateKeyBytes {
					c.candidateKey += string(r)
				} else {
					c.candidateKey = ""
					c.candidateTooLong = true
				}
			}
			continue
		}

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
			c.inString = false
			c.countingValue = false
		default:
			count++
		}
	}
	return count
}

func (c *directorToolTextCounter) startString(countingValue bool) {
	c.inString = true
	c.countingValue = countingValue
	c.escapedValue = false
	c.unicodeEscapeLeft = 0
	c.candidateKey = ""
	c.candidateTooLong = false
	if countingValue {
		c.pendingKey = ""
	}
}

func containsDirectorGeneratedTextKey(keys []string, target string) bool {
	for _, key := range keys {
		if key == target {
			return true
		}
	}
	return false
}

func isJSONWhitespace(r rune) bool {
	switch r {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
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

func directorToolGeneratedTextKeys(name string) []string {
	switch strings.TrimSpace(name) {
	case "edit":
		return []string{"new_string"}
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
