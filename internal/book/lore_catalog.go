package book

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// LoreNameCatalogOptions controls the compact, model-visible name catalog.
// The catalog is derived from lore/items.json and is never persisted separately.
type LoreNameCatalogOptions struct {
	Offset          int
	MaxBytes        int
	ExcludeResident bool
	OmitWhenEmpty   bool
}

// LoreNameCatalogMarkdown returns a bounded catalog for candidate discovery.
// Entries are added as complete lines so the result is never cut mid-entry.
func (s *LoreStore) LoreNameCatalogMarkdown(options LoreNameCatalogOptions) (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	filtered := make([]LoreItem, 0, len(items))
	for _, item := range items {
		if options.ExcludeResident && item.LoadMode == LoreLoadModeResident {
			continue
		}
		filtered = append(filtered, item)
	}
	if options.OmitWhenEmpty && len(filtered) == 0 {
		return "", nil
	}
	entries := make([]loreIndexEntry, 0, len(filtered))
	for _, item := range filtered {
		entries = append(entries, loreIndexEntry{Item: item})
	}
	sortLoreIndexEntries(entries, false)

	maxBytes := options.MaxBytes
	if maxBytes <= 0 || maxBytes > LoreIndexDefaultMaxBytes {
		maxBytes = LoreIndexDefaultMaxBytes
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(entries) {
		offset = len(entries)
	}
	revision, err := s.Revision()
	if err != nil {
		return "", err
	}

	lines := make([]string, 0, len(entries)-offset)
	for _, entry := range entries[offset:] {
		item := entry.Item
		name := boundedLoreCatalogName(item.Name, 512)
		lines = append(lines, fmt.Sprintf("- [%s/%s] %s\n", item.Type, item.Importance, name))
	}

	shown := 0
	shownLineBytes := 0
	for shown < len(lines) {
		candidateShown := shown + 1
		candidateBytes := len([]byte(renderLoreNameCatalogHeader(revision, len(entries), offset, candidateShown))) + shownLineBytes + len([]byte(lines[shown]))
		if candidateBytes > maxBytes {
			break
		}
		shownLineBytes += len([]byte(lines[shown]))
		shown++
	}
	result := renderLoreNameCatalog(revision, len(entries), offset, shown, lines[:shown])
	if len([]byte(result)) <= maxBytes {
		return strings.TrimSpace(result), nil
	}

	// Very small caller-provided budgets still receive useful source metadata.
	minimal := fmt.Sprintf("# 资料名称目录\nsource: lore/items.json\nrevision: %s\ntotal: %d\nshown: 0\nomitted: %d", revision, len(entries), len(entries)-offset)
	if len([]byte(minimal)) <= maxBytes {
		return minimal, nil
	}
	compact := fmt.Sprintf("# 资料名称目录\n共 %d 条，省略 %d 条；使用 list_lore_items。", len(entries), len(entries)-offset)
	if len([]byte(compact)) <= maxBytes {
		return compact, nil
	}
	return "", fmt.Errorf("资料名称目录上限过小，至少需要 %d bytes", len([]byte(minimal)))
}

// QueryLoreItems returns the same deterministic page used by the index tool.
// It lets callers render complete bodies without requiring a second lookup.
func (s *LoreStore) QueryLoreItems(options LoreIndexOptions) ([]LoreItem, error) {
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	entries, _, _ := filterLoreIndexEntries(items, options)
	result := make([]LoreItem, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Item)
	}
	return result, nil
}

func renderLoreNameCatalog(revision string, total, offset, shown int, lines []string) string {
	header := renderLoreNameCatalogHeader(revision, total, offset, shown)
	var sb strings.Builder
	sb.Grow(len(header) + len(lines)*32)
	sb.WriteString(header)
	for _, line := range lines {
		sb.WriteString(line)
	}
	return sb.String()
}

func renderLoreNameCatalogHeader(revision string, total, offset, shown int) string {
	omitted := total - offset - shown
	if omitted < 0 {
		omitted = 0
	}
	nextOffset := "null"
	if omitted > 0 {
		nextOffset = fmt.Sprintf("%d", offset+shown)
	}
	var sb strings.Builder
	sb.WriteString("# 资料名称目录\n")
	sb.WriteString("source: lore/items.json\n")
	fmt.Fprintf(&sb, "revision: %s\n", revision)
	fmt.Fprintf(&sb, "total: %d\n", total)
	fmt.Fprintf(&sb, "shown: %d\n", shown)
	fmt.Fprintf(&sb, "omitted: %d\n", omitted)
	fmt.Fprintf(&sb, "next_offset: %s\n\n", nextOffset)
	fmt.Fprintf(&sb, "共 %d 条启用资料。以下仅用于发现候选；可直接按唯一名称读取正文，或带筛选条件检索。\n\n", total)
	return sb.String()
}

func boundedLoreCatalogName(value string, maxBytes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return "（未命名）"
	}
	if maxBytes <= 0 || len([]byte(value)) <= maxBytes {
		return value
	}
	const suffix = "…"
	limit := maxBytes - len([]byte(suffix))
	if limit <= 0 {
		return suffix
	}
	cut := 0
	for index := range value {
		if index > limit {
			break
		}
		cut = index
	}
	if cut == 0 && utf8.RuneCountInString(value) > 0 {
		_, size := utf8.DecodeRuneInString(value)
		if size <= limit {
			cut = size
		}
	}
	return strings.TrimSpace(value[:cut]) + suffix
}
