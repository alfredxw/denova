package documentreview

import (
	"strings"
	"unicode/utf8"
)

const (
	maxAnchorBytes       = 128 * 1024
	maxDisplayQuoteBytes = 32 * 1024
)

// ValidateAnchor proves that a client-created anchor belongs to the exact
// canonical file snapshot the author reviewed.
func ValidateAnchor(snapshot Snapshot, anchor Anchor) error {
	anchor = normalizeAnchor(anchor)
	if anchor.Kind != AnchorKindTextRange && anchor.Kind != AnchorKindTextBlock {
		return newError(ErrorCodeInvalid, "document comment anchor kind is invalid", map[string]any{"kind": anchor.Kind})
	}
	if anchor.Encoding != AnchorEncodingUTF8 {
		return newError(ErrorCodeInvalid, "document comment anchor encoding is invalid", map[string]any{"encoding": anchor.Encoding})
	}
	if anchor.Revision == "" || anchor.Revision != strings.TrimSpace(snapshot.Revision) {
		return newError(ErrorCodeConflict, "document changed before the comment was created", map[string]any{
			"expected_revision": strings.TrimSpace(snapshot.Revision),
			"actual_revision":   anchor.Revision,
		})
	}
	if !utf8.ValidString(snapshot.Content) || anchor.Start < 0 || anchor.End < anchor.Start || anchor.End > len(snapshot.Content) ||
		!utf8ByteBoundary(snapshot.Content, anchor.Start) || !utf8ByteBoundary(snapshot.Content, anchor.End) {
		return newError(ErrorCodeInvalid, "document comment anchor range is invalid", map[string]any{
			"start": anchor.Start, "end": anchor.End, "content_bytes": len(snapshot.Content),
		})
	}
	if snapshot.Content[anchor.Start:anchor.End] != anchor.Quote {
		return newError(ErrorCodeConflict, "document comment quote no longer matches the file", map[string]any{
			"start": anchor.Start, "end": anchor.End,
		})
	}
	if anchor.Prefix != "" && (anchor.Start < len(anchor.Prefix) || snapshot.Content[anchor.Start-len(anchor.Prefix):anchor.Start] != anchor.Prefix) {
		return newError(ErrorCodeConflict, "document comment prefix no longer matches the file", nil)
	}
	if anchor.Suffix != "" && (anchor.End+len(anchor.Suffix) > len(snapshot.Content) || snapshot.Content[anchor.End:anchor.End+len(anchor.Suffix)] != anchor.Suffix) {
		return newError(ErrorCodeConflict, "document comment suffix no longer matches the file", nil)
	}
	anchorBytes := len(anchor.Kind) + len(anchor.Encoding) + len(anchor.Revision) + len(anchor.Quote) + len(anchor.Prefix) + len(anchor.Suffix)
	if anchorBytes > maxAnchorBytes || len(anchor.DisplayQuote) > maxDisplayQuoteBytes {
		return newError(ErrorCodeInvalid, "document comment anchor is too large", map[string]any{
			"max_anchor_bytes": maxAnchorBytes, "max_display_quote_bytes": maxDisplayQuoteBytes,
		})
	}
	if strings.TrimSpace(anchor.DisplayQuote) == "" {
		return newError(ErrorCodeInvalid, "document comment display quote is empty", nil)
	}
	if anchor.EditorFrom < 0 || anchor.EditorTo < anchor.EditorFrom {
		return newError(ErrorCodeInvalid, "document comment editor range is invalid", nil)
	}
	return nil
}

// ProjectAnchor relocates a stored source anchor onto the current canonical
// file only when the source quote and its surrounding context identify one
// unambiguous range. The stored anchor is never mutated by projection.
func ProjectAnchor(content, revision string, anchor Anchor) (Anchor, bool) {
	anchor = normalizeAnchor(anchor)
	revision = strings.TrimSpace(revision)
	if anchor.Revision == revision && exactAnchorMatch(content, anchor) {
		return anchor, false
	}

	start, ok := uniqueAnchorStart(content, anchor)
	if !ok {
		return anchor, true
	}
	projected := anchor
	projected.Revision = revision
	projected.Start = start
	projected.End = start + len(anchor.Quote)
	return projected, false
}

func uniqueAnchorStart(content string, anchor Anchor) (int, bool) {
	if anchor.Quote != "" {
		match := -1
		for offset := 0; offset <= len(content); {
			index := strings.Index(content[offset:], anchor.Quote)
			if index < 0 {
				break
			}
			start := offset + index
			end := start + len(anchor.Quote)
			if contextMatches(content, start, end, anchor) {
				if match >= 0 {
					return 0, false
				}
				match = start
			}
			offset = start + 1
		}
		return match, match >= 0
	}

	// Point-like anchors are rare because line comments normally quote the
	// whole text block. Retain a safe fallback for legacy/empty source ranges.
	if anchor.Prefix == "" && anchor.Suffix == "" {
		return 0, false
	}
	needle := anchor.Prefix + anchor.Suffix
	first := strings.Index(content, needle)
	if first < 0 || strings.LastIndex(content, needle) != first {
		return 0, false
	}
	return first + len(anchor.Prefix), true
}

func exactAnchorMatch(content string, anchor Anchor) bool {
	return anchor.Start >= 0 && anchor.End >= anchor.Start && anchor.End <= len(content) &&
		utf8ByteBoundary(content, anchor.Start) && utf8ByteBoundary(content, anchor.End) &&
		content[anchor.Start:anchor.End] == anchor.Quote && contextMatches(content, anchor.Start, anchor.End, anchor)
}

func contextMatches(content string, start, end int, anchor Anchor) bool {
	if anchor.Prefix != "" && (start < len(anchor.Prefix) || content[start-len(anchor.Prefix):start] != anchor.Prefix) {
		return false
	}
	if anchor.Suffix != "" && (end+len(anchor.Suffix) > len(content) || content[end:end+len(anchor.Suffix)] != anchor.Suffix) {
		return false
	}
	return true
}

func normalizeAnchor(anchor Anchor) Anchor {
	anchor.Kind = strings.TrimSpace(anchor.Kind)
	anchor.Encoding = strings.TrimSpace(anchor.Encoding)
	anchor.Revision = strings.TrimSpace(anchor.Revision)
	return anchor
}

func utf8ByteBoundary(content string, offset int) bool {
	return offset == 0 || offset == len(content) || (offset > 0 && offset < len(content) && utf8.RuneStart(content[offset]))
}
