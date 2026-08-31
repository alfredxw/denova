package interactive

import "strings"

const (
	// StoryTitleSourcePending marks the temporary title of a story whose first
	// playable narrative has not been committed yet.
	StoryTitleSourcePending   = "pending"
	StoryTitleSourceGenerated = "generated"
	StoryTitleSourceUser      = "user"
)

const maxGeneratedStoryTitleRunes = 32

func normalizeStoryTitleSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case StoryTitleSourcePending:
		return StoryTitleSourcePending
	case StoryTitleSourceGenerated:
		return StoryTitleSourceGenerated
	case StoryTitleSourceUser:
		return StoryTitleSourceUser
	default:
		// Stories created by released versions predate title_source. Treat their
		// existing names as user-owned so loading them can never rename them.
		return StoryTitleSourceUser
	}
}

func generatedStoryTitle(narrative string) string {
	candidate := strings.TrimSpace(narrative)
	if candidate == "" {
		return ""
	}
	if end := strings.IndexAny(candidate, "。！？.!?…\r\n"); end >= 0 {
		candidate = candidate[:end]
	}
	candidate = strings.Join(strings.Fields(candidate), " ")
	candidate = strings.Trim(candidate, " \t\r\n#*_~'\"“”‘’「」『』《》〈〉[]【】（）()")
	if candidate == "" {
		return ""
	}
	runes := []rune(candidate)
	if len(runes) <= maxGeneratedStoryTitleRunes {
		return candidate
	}
	return strings.TrimSpace(string(runes[:maxGeneratedStoryTitleRunes-1])) + "…"
}

func generatePendingStoryTitle(meta *StoryMeta, narrative string) bool {
	if meta == nil || normalizeStoryTitleSource(meta.TitleSource) != StoryTitleSourcePending {
		return false
	}
	title := generatedStoryTitle(narrative)
	if title == "" {
		return false
	}
	meta.Title = title
	meta.TitleSource = StoryTitleSourceGenerated
	return true
}
