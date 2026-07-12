package book

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	cardHTMLCommentPattern  = regexp.MustCompile(`(?s)<!--.*?-->`)
	cardScriptPattern       = regexp.MustCompile(`(?is)<(?:script|style)\b[^>]*>.*?</(?:script|style)>`)
	cardRuntimeBlockPattern = regexp.MustCompile(`(?is)<(?:UpdateVariable|JSONPatch|variables?|state|statusbar|initvar)\b[^>]*>.*?</(?:UpdateVariable|JSONPatch|variables?|state|statusbar|initvar)>`)
	cardRuntimeSelfClosingPattern = regexp.MustCompile(`(?is)<(?:UpdateVariable|JSONPatch|StatusPlaceHolderImpl)\b[^>]*/>`)
	cardTemplatePattern     = regexp.MustCompile(`(?s)<%.*?%>`)
	cardStatusPattern       = regexp.MustCompile(`(?is)<StatusPlaceHolderImpl\s*/?>`)
	cardCodeFencePattern    = regexp.MustCompile("(?is)```(?:json|javascript|js|typescript|ts|html|css)?\\s*.*?```")
	cardTagPattern          = regexp.MustCompile(`(?s)<[^>]+>`)
	cardGameTextPattern     = regexp.MustCompile(`(?is)<gametxt\b[^>]*>(.*?)</gametxt>`)
	cardOnlyTitlePattern    = regexp.MustCompile(`^\s*(?:[【\[《].{1,80}[】\]》]|#{1,3}\s*.{1,80})\s*$`)
)

type sanitizedTavernEntry struct {
	Content      string
	Removed      bool
	MixedCleaned bool
}

func sanitizeTavernBookEntry(entry tavernBookEntry) sanitizedTavernEntry {
	original := normalizeCardText(entry.Content)
	if original == "" {
		return sanitizedTavernEntry{Removed: true}
	}
	cleaned := sanitizeTavernNaturalLanguage(original)
	changed := cleaned != original
	if cleaned == "" || isPureTavernRuntimeEntry(entry, cleaned) {
		return sanitizedTavernEntry{Removed: true, MixedCleaned: changed}
	}
	return sanitizedTavernEntry{Content: cleaned, MixedCleaned: changed}
}

func sanitizeTavernRuntimeProneField(value string) string {
	entry := tavernBookEntry{Comment: firstCardRunes(value, 120), Content: value}
	sanitized := sanitizeTavernBookEntry(entry)
	if sanitized.Removed {
		return ""
	}
	return sanitized.Content
}

func sanitizeTavernNaturalLanguage(value string) string {
	value = normalizeCardText(value)
	value = cardHTMLCommentPattern.ReplaceAllString(value, "")
	value = cardScriptPattern.ReplaceAllString(value, "")
	value = cardRuntimeBlockPattern.ReplaceAllString(value, "")
	value = cardRuntimeSelfClosingPattern.ReplaceAllString(value, "")
	value = cardTemplatePattern.ReplaceAllString(value, "")
	value = cardStatusPattern.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "<UpdateVariable>", "")
	value = strings.ReplaceAll(value, "</UpdateVariable>", "")
	value = strings.ReplaceAll(value, "<JSONPatch>", "")
	value = strings.ReplaceAll(value, "</JSONPatch>", "")
	return compactCardBlankLines(value)
}

func isPureTavernRuntimeEntry(entry tavernBookEntry, cleaned string) bool {
	title := strings.ToLower(strings.TrimSpace(entry.Comment))
	original := strings.ToLower(entry.Content)
	titleMarkers := []string{
		"mvu", "zod", "output format", "输出格式", "变量定义", "变量初始化",
		"状态栏", "statusbar", "regex", "正则", "<updatevariable", "<jsonpatch",
		"<statusplaceholderimpl", "tavern_helper", "xiaobaix", "创意工坊", "资源预载",
	}
	titleMarked := false
	for _, marker := range titleMarkers {
		if strings.Contains(title, marker) {
			titleMarked = true
			break
		}
	}
	runtimeBody := strings.Contains(original, "<updatevariable") || strings.Contains(original, "<jsonpatch") || strings.Contains(original, "<statusplaceholderimpl")
	if !titleMarked && !runtimeBody {
		return false
	}
	withoutCode := cardCodeFencePattern.ReplaceAllString(cleaned, "")
	withoutTags := cardTagPattern.ReplaceAllString(withoutCode, "")
	withoutTags = strings.TrimSpace(withoutTags)
	return withoutTags == "" || (titleMarked && utf8.RuneCountInString(withoutTags) < 80)
}

func compactCardBlankLines(value string) string {
	lines := strings.Split(normalizeCardText(value), "\n")
	result := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank || len(result) == 0 {
				continue
			}
			blank = true
			result = append(result, "")
			continue
		}
		blank = false
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func sanitizeTavernOpening(value string) (string, bool) {
	original := normalizeCardText(value)
	if original == "" || strings.Contains(strings.ToLower(original), "<customized") {
		return "", false
	}
	if matches := cardGameTextPattern.FindStringSubmatch(original); len(matches) > 1 {
		original = matches[1]
	} else if strings.Contains(strings.ToLower(original), "<!doctype") ||
		strings.Contains(strings.ToLower(original), "<html") ||
		strings.Contains(strings.ToLower(original), "<script") ||
		strings.Contains(strings.ToLower(original), "<style") {
		return "", false
	}
	cleaned := sanitizeTavernNaturalLanguage(original)
	cleaned = cardTagPattern.ReplaceAllString(cleaned, "\n")
	cleaned = compactCardBlankLines(cleaned)
	if cleaned == "" || cardOnlyTitlePattern.MatchString(cleaned) {
		return "", false
	}
	runes := []rune(cleaned)
	if len(runes) <= 4000 {
		return cleaned, false
	}
	return string(runes[:4000]), true
}

func naturalLanguageBrief(content string) string {
	content = sanitizeTavernNaturalLanguage(content)
	for _, paragraph := range strings.Split(content, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if cardOnlyTitlePattern.MatchString(paragraph) {
			continue
		}
		paragraph = strings.TrimSpace(strings.TrimLeft(paragraph, "#*- "))
		if paragraph == "" || strings.HasPrefix(paragraph, "![") {
			continue
		}
		runes := []rune(paragraph)
		if len(runes) > 240 {
			paragraph = string(runes[:240])
		}
		return strings.TrimSpace(paragraph)
	}
	return ""
}

func inferTavernLoreType(title, content string) string {
	normalizedTitle := strings.ToLower(strings.TrimSpace(strings.TrimLeft(title, "# ")))
	titlePrefixes := []struct {
		typeName string
		prefixes []string
	}{
		{"character", []string{"角色:", "角色：", "人物:", "人物：", "npc:", "npc：", "[角色]", "【角色】", "[人物]", "【人物】", "[npc]", "【npc】"}},
		{"location", []string{"地点:", "地点：", "场景:", "场景：", "区域:", "区域：", "[地点]", "【地点】", "[场景]", "【场景】"}},
		{"faction", []string{"势力:", "势力：", "组织:", "组织：", "门派:", "门派：", "阵营:", "阵营：", "[势力]", "【势力】", "[组织]", "【组织】"}},
		{"rule", []string{"规则:", "规则：", "机制:", "机制：", "法则:", "法则：", "[规则]", "【规则】", "[机制]", "【机制】"}},
		{"item", []string{"物品:", "物品：", "道具:", "道具：", "装备:", "装备：", "[物品]", "【物品】", "[道具]", "【道具】"}},
	}
	for _, candidate := range titlePrefixes {
		for _, prefix := range candidate.prefixes {
			if strings.HasPrefix(normalizedTitle, prefix) {
				return candidate.typeName
			}
		}
	}
	probe := strings.TrimSpace(title) + "\n" + firstCardRunes(content, 500)
	patterns := []struct {
		typeName string
		pattern  *regexp.Regexp
	}{
		{"character", regexp.MustCompile(`(?im)^\s*(?:#{1,3}\s*)?(?:角色|人物|npc)(?:设定|资料|信息)?\s*[:：]`)},
		{"location", regexp.MustCompile(`(?im)^\s*(?:#{1,3}\s*)?(?:地点|场景|区域)(?:设定|资料|信息)?\s*[:：]`)},
		{"faction", regexp.MustCompile(`(?im)^\s*(?:#{1,3}\s*)?(?:势力|组织|门派|阵营)(?:设定|资料|信息)?\s*[:：]`)},
		{"rule", regexp.MustCompile(`(?im)^\s*(?:#{1,3}\s*)?(?:规则|机制|法则)(?:设定|资料|信息)?\s*[:：]`)},
		{"item", regexp.MustCompile(`(?im)^\s*(?:#{1,3}\s*)?(?:物品|道具|装备)(?:设定|资料|信息)?\s*[:：]`)},
	}
	for _, candidate := range patterns {
		if candidate.pattern.MatchString(probe) {
			return candidate.typeName
		}
	}
	return "world"
}

func firstCardRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}
