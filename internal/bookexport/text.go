// Package bookexport renders a book.Manuscript into downloadable file formats.
//
// The core book package builds the format-independent Manuscript model; this
// package owns the per-format rendering (plain text, EPUB, …) so heavier
// dependencies such as the EPUB writer stay out of the core package and each
// output format has a single, isolated place to evolve.
package bookexport

import (
	"strings"

	"denova/internal/book"
)

// RenderText assembles a manuscript into a single plain-text UTF-8 document:
// an optional title/author header, each named volume heading, then every
// chapter's title and body separated by blank lines.
func RenderText(m book.Manuscript) string {
	blocks := make([]string, 0, m.ChapterCount()*2+len(m.Volumes)+1)

	if header := textHeader(m); header != "" {
		blocks = append(blocks, header)
	}
	for _, volume := range m.Volumes {
		if title := strings.TrimSpace(volume.Title); title != "" {
			blocks = append(blocks, title)
		}
		for _, chapter := range volume.Chapters {
			blocks = append(blocks, chapter.Title, chapter.Body)
		}
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n")) + "\n"
}

func textHeader(m book.Manuscript) string {
	lines := make([]string, 0, 2)
	if title := strings.TrimSpace(m.Title); title != "" {
		lines = append(lines, title)
	}
	if author := strings.TrimSpace(m.Author); author != "" {
		lines = append(lines, "作者: "+author)
	}
	return strings.Join(lines, "\n")
}
