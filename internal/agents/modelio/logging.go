package modelio

import (
	"strings"
	"unicode/utf8"
)

// LogPreview escapes line breaks and returns a UTF-8-safe bounded diagnostic
// preview. Callers must still avoid passing secrets or unrestricted user data.
func LogPreview(content string, limit int) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	content = strings.ReplaceAll(content, "\r", "\\r")
	if limit <= 0 {
		return ""
	}
	if len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "..."
}
