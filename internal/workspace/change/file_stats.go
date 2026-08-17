package change

import (
	"unicode"
	"unicode/utf8"
)

// FileStats describes the complete text file after a successful mutation.
// Characters counts Unicode code points including whitespace, while
// NonWhitespaceCharacters matches Denova's language-neutral writing count.
type FileStats struct {
	Bytes                   int `json:"bytes"`
	Characters              int `json:"characters"`
	NonWhitespaceCharacters int `json:"non_whitespace_characters"`
	Lines                   int `json:"lines"`
}

func measureFileStats(content []byte) FileStats {
	stats := FileStats{Bytes: len(content)}
	if len(content) > 0 {
		stats.Lines = 1
	}
	for len(content) > 0 {
		character, size := utf8.DecodeRune(content)
		content = content[size:]
		stats.Characters++
		if character == '\n' {
			stats.Lines++
		}
		if !unicode.IsSpace(character) {
			stats.NonWhitespaceCharacters++
		}
	}
	return stats
}
