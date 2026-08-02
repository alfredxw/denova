package context

import (
	"strings"
	"unicode/utf8"
)

// TrimUTF8Bytes trims surrounding whitespace and bounds text without splitting
// a UTF-8 code point. The boolean reports whether non-empty input was cut.
func TrimUTF8Bytes(value string, limit int) (string, bool) {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return "", value != ""
	}
	if len(value) <= limit {
		return value, false
	}
	used := 0
	for index, r := range value {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = len(string(r))
		}
		if used+size > limit {
			return strings.TrimSpace(value[:index]), true
		}
		used += size
	}
	return value, false
}
