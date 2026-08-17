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
	matchContains("name", 115, item.Name)
	matchContains("keywords", 105, item.Keywords...)
	matchContains("tags", 95, item.Tags...)
	matchFuzzy("fuzzy name", 85, item.Name)
	matchFuzzy("fuzzy keywords", 75, item.Keywords...)
	matchFuzzy("fuzzy tags", 65, item.Tags...)
	matchContains("brief", 60, item.BriefDescription)
	matchContains("content", 40, item.Content)
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
		{briefRunes: 72, hint: "[Index compressed: briefs were truncated. Narrow with keywords/types, then call read_lore_items for complete bodies.]"},
		{nameOnly: true, hint: "[Index too large: showing IDs and names only. Narrow with keywords/types, then call read_lore_items for complete bodies.]"},
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
		sb.WriteString("# Lore Index\n\n")
	}
	filtered := len(normalizeLoreIndexKeywords(options.Keywords)) > 0 || len(normalizeLoreIndexTypes(options.Types)) > 0 || len(normalizeLoreIndexLoadModes(options.LoadModes)) > 0
	if libraryTotal == 0 {
		sb.WriteString("The lore library has no enabled items.\n")
		return
	}
	if filtered && matchedTotal == 0 {
		fmt.Fprintf(sb, "The lore library has %d enabled items; this search matched none. No match does not mean the library is empty. Adjust keywords/types or use empty arguments to browse the catalog.\n", libraryTotal)
		return
	}
	if options.Paginate || filtered {
		offset := options.Offset
		if offset < 0 {
			offset = 0
		}
		if filtered {
			fmt.Fprintf(sb, "The lore library has %d enabled items; this search matched %d and returned %d on this page (offset=%d).", libraryTotal, matchedTotal, returned, offset)
		} else {
			fmt.Fprintf(sb, "The lore library has %d enabled items and returned %d on this page (offset=%d).", libraryTotal, returned, offset)
		}
		if matchedTotal > offset+returned {
			fmt.Fprintf(sb, " Use offset=%d for the next page.", offset+returned)
		}
		sb.WriteString("\n\n")
		return
	}
	scope := "enabled lore items"
	if options.ExcludeResident {
		scope = "non-resident lore items"
	}
	fmt.Fprintf(sb, "Total: %d %s. The default index contains only ID, name, and brief; call read_lore_items for complete bodies.\n\n", matchedTotal, scope)
}

func formatCompactLoreIndexEntry(entry loreIndexEntry, briefRunes int, nameOnly bool) string {
	item := entry.Item
	if nameOnly {
		return fmt.Sprintf("- id: %s | name: %s\n", item.ID, item.Name)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "- id: %s\n  name: %s\n", item.ID, item.Name)
	brief := compactLoreBrief(item.BriefDescription, briefRunes)
	if brief != "" {
		fmt.Fprintf(&sb, "  brief: %s\n", brief)
	}
	if len(item.Keywords) > 0 {
		fmt.Fprintf(&sb, "  keywords: %s\n", compactLoreBrief(strings.Join(item.Keywords, ", "), 120))
	}
	if len(entry.MatchedTerms) > 0 {
		fmt.Fprintf(&sb, "  matched_terms: %s\n", strings.Join(entry.MatchedTerms, ", "))
	}
	if len(entry.MatchSources) > 0 {
		fmt.Fprintf(&sb, "  match_sources: %s\n", strings.Join(entry.MatchSources, ", "))
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
	hint := "[Index budget is limited. Only IDs and names that fit are shown; narrow with keywords/types to find omitted items.]\n\n"
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
		notice := fmt.Sprintf("\n[%d more lore items were omitted by the index budget. Narrow with keywords/types.]\n", omitted)
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
	fmt.Fprintf(&sb, "- id: %s\n  name: %s\n  type: %s\n  importance: %s\n  load_mode: %s\n", item.ID, item.Name, item.Type, item.Importance, item.LoadMode)
	if len(item.Tags) > 0 {
		fmt.Fprintf(&sb, "  tags: %s\n", strings.Join(item.Tags, ", "))
	}
	if item.BriefDescription != "" {
		fmt.Fprintf(&sb, "  brief: %s\n", item.BriefDescription)
	}
	if len(item.Keywords) > 0 {
		fmt.Fprintf(&sb, "  keywords: %s\n", strings.Join(item.Keywords, ", "))
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
	addSource("name", item.Name)
	addSource("type", item.Type, TypeLabel(item.Type))
	addSource("tags", item.Tags...)
	addSource("keywords", item.Keywords...)
	addSource("brief", item.BriefDescription)
	addSource("content", item.Content)
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
