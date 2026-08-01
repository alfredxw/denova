package lore

import (
	"fmt"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"sort"
	"strings"
	"unicode/utf8"
)

type loreIndexEntry struct {
	Item         Item
	MatchedTerms []string
	MatchSources []string
	Score        int
}

func filterLoreIndexEntries(items []Item, options IndexOptions) ([]loreIndexEntry, int, int) {
	keywords := normalizeLoreIndexKeywords(options.Keywords)
	types := normalizeLoreIndexTypes(options.Types)
	loadModes := normalizeLoreIndexLoadModes(options.LoadModes)
	match := normalizeLoreIndexMatch(options.Match)
	shouldLimit := options.Paginate || len(keywords) > 0 || len(types) > 0 || len(loadModes) > 0
	limit := normalizeLoreIndexLimit(options.Limit)
	matched := make([]loreIndexEntry, 0, len(items))
	libraryTotal := 0
	for _, item := range items {
		if options.ExcludeResident && item.LoadMode == LoadModeResident {
			continue
		}
		libraryTotal++
		if len(types) > 0 && !types[item.Type] {
			continue
		}
		if len(loadModes) > 0 && !loadModes[item.LoadMode] {
			continue
		}
		entry := matchLoreIndexEntry(item, keywords)
		if len(keywords) > 0 && !loreIndexEntrySatisfies(entry, len(keywords), match) {
			continue
		}
		matched = append(matched, entry)
	}
	sortLoreIndexEntries(matched, len(keywords) > 0)
	matchedTotal := len(matched)
	if !shouldLimit {
		return matched, matchedTotal, libraryTotal
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matched) {
		return nil, matchedTotal, libraryTotal
	}
	end := len(matched)
	if limit < len(matched)-offset {
		end = offset + limit
	}
	return matched[offset:end], matchedTotal, libraryTotal
}

func normalizeLoreIndexLoadModes(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		switch value = strings.TrimSpace(value); value {
		case LoadModeResident, LoadModeAuto, LoadModeManual:
			result[value] = true
		}
	}
	return result
}

func normalizeLoreIndexKeywords(keywords []string) []string {
	result := make([]string, 0, len(keywords))
	seen := map[string]bool{}
	for _, keyword := range keywords {
		keyword = normalizeLoreSearchText(keyword)
		if keyword == "" || seen[keyword] {
			continue
		}
		seen[keyword] = true
		result = append(result, keyword)
	}
	return result
}

func normalizeLoreIndexTypes(types []string) map[string]bool {
	result := map[string]bool{}
	for _, itemType := range types {
		itemType = strings.TrimSpace(itemType)
		if itemType != "" {
			result[NormalizeType(itemType)] = true
		}
	}
	return result
}

func normalizeLoreIndexMatch(match string) string {
	if strings.EqualFold(strings.TrimSpace(match), IndexMatchAll) {
		return IndexMatchAll
	}
	return IndexMatchAny
}

func normalizeLoreSearchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func matchLoreIndexEntry(item Item, keywords []string) loreIndexEntry {
	entry := loreIndexEntry{Item: item}
	for _, keyword := range keywords {
		score, sources := matchLoreIndexTerm(item, keyword)
		if score <= 0 {
			continue
		}
		entry.MatchedTerms = append(entry.MatchedTerms, keyword)
		entry.Score += score
		for _, source := range sources {
			entry.MatchSources = appendUniqueString(entry.MatchSources, source)
		}
	}
	return entry
}

func matchLoreIndexTerm(item Item, keyword string) (int, []string) {
	bestScore := 0
	sources := []string{}
	recordMatch := func(label string, weight int) {
		if weight > bestScore {
			bestScore = weight
			sources = []string{label}
			return
		}
		if weight == bestScore {
			sources = appendUniqueString(sources, label)
		}
	}
	matchContains := func(label string, weight int, values ...string) {
		if bestScore > weight {
			return
		}
		for _, value := range values {
			if strings.Contains(normalizeLoreSearchText(value), keyword) {
				recordMatch(label, weight)
				return
			}
		}
	}
	matchFuzzy := func(label string, weight int, values ...string) {
		if bestScore > weight {
			return
		}
		for index, value := range values {
			if index >= 32 {
				return
			}
			normalized := normalizeLoreSearchText(value)
			if strings.Contains(normalized, keyword) {
				continue
			}
			if loreShortMetadataFuzzyMatch(keyword, normalized) {
				recordMatch(label, weight)
				return
			}
		}
	}

	matchContains("ID", 120, item.ID)
	matchContains("名称", 115, item.Name)
	matchContains("关键词", 105, item.Keywords...)
	matchContains("标签", 95, item.Tags...)
	matchFuzzy("模糊名称", 85, item.Name)
	matchFuzzy("模糊关键词", 75, item.Keywords...)
	matchFuzzy("模糊标签", 65, item.Tags...)
	matchContains("简介", 60, item.BriefDescription)
	matchContains("正文", 40, item.Content)
	return bestScore, sources
}

func loreShortMetadataFuzzyMatch(keyword, candidate string) bool {
	keywordRunes := utf8.RuneCountInString(keyword)
	candidateRunes := utf8.RuneCountInString(candidate)
	if keywordRunes < 3 || candidateRunes < 3 || keywordRunes > 48 || candidateRunes > 48 {
		return false
	}
	maxDistance := 1
	if keywordRunes >= 8 {
		maxDistance = 2
	}
	if keywordRunes >= 16 {
		maxDistance = 3
	}
	keywordChars := []rune(keyword)
	candidateChars := []rune(candidate)
	minWindow := keywordRunes - maxDistance
	if minWindow < 3 {
		minWindow = 3
	}
	maxWindow := keywordRunes + maxDistance
	if maxWindow > candidateRunes {
		maxWindow = candidateRunes
	}
	for width := minWindow; width <= maxWindow; width++ {
		for start := 0; start+width <= candidateRunes; start++ {
			if fuzzy.LevenshteinDistance(string(keywordChars), string(candidateChars[start:start+width])) <= maxDistance {
				return true
			}
		}
	}
	return false
}

func loreIndexEntrySatisfies(entry loreIndexEntry, keywordCount int, match string) bool {
	if match == IndexMatchAll {
		return len(entry.MatchedTerms) == keywordCount
	}
	return len(entry.MatchedTerms) > 0
}

func sortLoreIndexEntries(entries []loreIndexEntry, ranked bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		if ranked && entries[i].Score != entries[j].Score {
			return entries[i].Score > entries[j].Score
		}
		if ranked && len(entries[i].MatchedTerms) != len(entries[j].MatchedTerms) {
			return len(entries[i].MatchedTerms) > len(entries[j].MatchedTerms)
		}
		if rankI, rankJ := loreImportanceRank(entries[i].Item.Importance), loreImportanceRank(entries[j].Item.Importance); rankI != rankJ {
			return rankI < rankJ
		}
		if entries[i].Item.Type != entries[j].Item.Type {
			return entries[i].Item.Type < entries[j].Item.Type
		}
		if entries[i].Item.Name != entries[j].Item.Name {
			return entries[i].Item.Name < entries[j].Item.Name
		}
		return entries[i].Item.ID < entries[j].Item.ID
	})
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func normalizeLoreIndexLimit(limit int) int {
	if limit <= 0 {
		return IndexDefaultLimit
	}
	return limit
}

func renderLoreIndexMarkdown(entries []loreIndexEntry, matchedTotal, libraryTotal int, options IndexOptions) string {
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = IndexDefaultMaxBytes
	}
	if maxBytes <= 0 {
		return ""
	}

	candidates := []struct {
		briefRunes int
		nameOnly   bool
		hint       string
	}{
		{briefRunes: 180},
		{briefRunes: 72, hint: "（索引已压缩：简介已截断；可用 keywords/types 缩小范围，再调用 read_lore_items 读取正文。）"},
		{nameOnly: true, hint: "（索引过大，已降级为仅 ID 和名称；可用 keywords/types 细查，再调用 read_lore_items 读取正文。）"},
	}
	for _, candidate := range candidates {
		out := renderLoreIndexCandidate(entries, matchedTotal, libraryTotal, options, candidate.briefRunes, candidate.nameOnly, candidate.hint)
		if len([]byte(out)) <= maxBytes {
			return strings.TrimSpace(out)
		}
	}
	return renderBoundedLoreNameIndex(entries, matchedTotal, libraryTotal, options, maxBytes)
}

func renderLoreIndexCandidate(entries []loreIndexEntry, matchedTotal, libraryTotal int, options IndexOptions, briefRunes int, nameOnly bool, hint string) string {
	var sb strings.Builder
	writeLoreIndexHeader(&sb, matchedTotal, libraryTotal, len(entries), options)
	if hint != "" {
		sb.WriteString(hint)
		sb.WriteString("\n\n")
	}
	for _, entry := range entries {
		sb.WriteString(formatCompactLoreIndexEntry(entry, briefRunes, nameOnly))
	}
	return strings.TrimSpace(sb.String())
}

func writeLoreIndexHeader(sb *strings.Builder, matchedTotal, libraryTotal, returned int, options IndexOptions) {
	if !options.OmitTitle {
		sb.WriteString("# 资料库索引\n\n")
	}
	filtered := len(normalizeLoreIndexKeywords(options.Keywords)) > 0 || len(normalizeLoreIndexTypes(options.Types)) > 0 || len(normalizeLoreIndexLoadModes(options.LoadModes)) > 0
	if libraryTotal == 0 {
		sb.WriteString("资料库暂无启用条目。\n")
		return
	}
	if filtered && matchedTotal == 0 {
		fmt.Fprintf(sb, "资料库共有 %d 条启用资料；本次检索匹配 0 条。未命中不代表资料库为空，可调整 keywords/types 或使用空参数浏览目录。\n", libraryTotal)
		return
	}
	if options.Paginate || filtered {
		offset := options.Offset
		if offset < 0 {
			offset = 0
		}
		if filtered {
			fmt.Fprintf(sb, "资料库共有 %d 条启用资料；本次匹配 %d 条，本页返回 %d 条（offset=%d）。", libraryTotal, matchedTotal, returned, offset)
		} else {
			fmt.Fprintf(sb, "资料库共有 %d 条启用资料；本页返回 %d 条（offset=%d）。", libraryTotal, returned, offset)
		}
		if matchedTotal > offset+returned {
			fmt.Fprintf(sb, " 下一页使用 offset=%d。", offset+returned)
		}
		sb.WriteString("\n\n")
		return
	}
	scope := "启用资料"
	if options.ExcludeResident {
		scope = "非驻留资料"
	}
	fmt.Fprintf(sb, "共 %d 条%s。默认索引只含 ID、名称和简介；需要正文时调用 read_lore_items。\n\n", matchedTotal, scope)
}

func formatCompactLoreIndexEntry(entry loreIndexEntry, briefRunes int, nameOnly bool) string {
	item := entry.Item
	if nameOnly {
		return fmt.Sprintf("- id: %s | 名称: %s\n", item.ID, item.Name)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n", item.ID, item.Name)
	brief := compactLoreBrief(item.BriefDescription, briefRunes)
	if brief != "" {
		fmt.Fprintf(&sb, "  简介: %s\n", brief)
	}
	if len(item.Keywords) > 0 {
		fmt.Fprintf(&sb, "  关键词: %s\n", compactLoreBrief(strings.Join(item.Keywords, "、"), 120))
	}
	if len(entry.MatchedTerms) > 0 {
		fmt.Fprintf(&sb, "  匹配词: %s\n", strings.Join(entry.MatchedTerms, "、"))
	}
	if len(entry.MatchSources) > 0 {
		fmt.Fprintf(&sb, "  匹配来源: %s\n", strings.Join(entry.MatchSources, "、"))
	}
	return sb.String()
}

func compactLoreBrief(brief string, limit int) string {
	brief = strings.Join(strings.Fields(strings.TrimSpace(brief)), " ")
	if brief == "" {
		return ""
	}
	if limit <= 0 || utf8.RuneCountInString(brief) <= limit {
		return brief
	}
	if limit <= 3 {
		return truncateRunes(brief, limit)
	}
	return truncateRunes(brief, limit-3) + "..."
}

func renderBoundedLoreNameIndex(entries []loreIndexEntry, matchedTotal, libraryTotal int, options IndexOptions, maxBytes int) string {
	var sb strings.Builder
	writeLoreIndexHeader(&sb, matchedTotal, libraryTotal, len(entries), options)
	hint := "（索引预算不足，以下仅展示能放入预算的 ID 和名称；未显示条目请用 keywords/types 细查。）\n\n"
	appendLoreContextPart(&sb, hint, maxBytes)
	omitted := 0
	for idx, entry := range entries {
		line := formatCompactLoreIndexEntry(entry, 0, true)
		if sb.Len()+len([]byte(line)) > maxBytes {
			omitted = len(entries) - idx
			break
		}
		sb.WriteString(line)
	}
	if omitted > 0 {
		notice := fmt.Sprintf("\n（还有 %d 条资料因索引预算未显示；请使用 keywords/types 细查。）\n", omitted)
		if sb.Len()+len([]byte(notice)) <= maxBytes {
			sb.WriteString(notice)
		}
	}
	out := strings.TrimSpace(sb.String())
	if len([]byte(out)) <= maxBytes {
		return out
	}
	return strings.TrimSpace(truncateStringBytes(out, maxBytes))
}

func formatLoreItemIndexMarkdown(item Item) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  重要度: %s\n  加载策略: %s\n", item.ID, item.Name, TypeLabel(item.Type), loreImportanceLabel(item.Importance), loreLoadModeLabel(item.LoadMode))
	if len(item.Tags) > 0 {
		fmt.Fprintf(&sb, "  标签: %s\n", strings.Join(item.Tags, "、"))
	}
	if item.BriefDescription != "" {
		fmt.Fprintf(&sb, "  简介: %s\n", item.BriefDescription)
	}
	if len(item.Keywords) > 0 {
		fmt.Fprintf(&sb, "  关键词: %s\n", strings.Join(item.Keywords, "、"))
	}
	sb.WriteString("\n")
	return sb.String()
}

func appendLoreContextPart(sb *strings.Builder, text string, maxBytes int) bool {
	if text == "" {
		return true
	}
	if maxBytes <= 0 {
		sb.WriteString(text)
		return true
	}
	remaining := maxBytes - sb.Len()
	if remaining <= 0 {
		return false
	}
	if len([]byte(text)) <= remaining {
		sb.WriteString(text)
		return true
	}
	clipped := truncateStringBytes(text, remaining)
	if clipped == "" {
		return false
	}
	sb.WriteString(clipped)
	return false
}

func truncateStringBytes(text string, maxBytes int) string {
	if maxBytes <= 0 || text == "" {
		return ""
	}
	if len([]byte(text)) <= maxBytes {
		return text
	}
	end := 0
	for idx, r := range text {
		next := idx + utf8.RuneLen(r)
		if next > maxBytes {
			break
		}
		end = next
	}
	return text[:end]
}

func loreItemMatchesQuery(item Item, query string) bool {
	return len(loreItemMatchSources(item, query)) > 0
}

func loreItemMatchSources(item Item, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	sources := []string{}
	addSource := func(label string, values ...string) {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), query) {
				sources = append(sources, label)
				return
			}
		}
	}
	addSource("ID", item.ID)
	addSource("名称", item.Name)
	addSource("类型", item.Type, TypeLabel(item.Type))
	addSource("标签", item.Tags...)
	addSource("关键词", item.Keywords...)
	addSource("简介", item.BriefDescription)
	addSource("正文", item.Content)
	return sources
}

func loreImportanceLabel(v string) string {
	switch normalizeLoreImportance(v) {
	case "major":
		return "主要"
	case "important":
		return "重要"
	default:
		return "次要"
	}
}
