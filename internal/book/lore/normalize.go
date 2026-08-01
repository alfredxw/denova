package lore

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func normalizeLoreItems(items []Item) []Item {
	normalized := make([]Item, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item = normalizeLoreItem(item)
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeLoreItem(item Item) Item {
	item.ID = normalizeLoreID(item.ID)
	item.Type = NormalizeType(item.Type)
	item.TypeSource = normalizeLoreTypeSource(item.TypeSource)
	item.Name = strings.TrimSpace(item.Name)
	item.Importance = normalizeLoreImportance(item.Importance)
	item.LoadMode = normalizeLoreLoadMode(item.LoadMode, item.Importance)
	item.Content = strings.TrimSpace(item.Content)
	item.Tags = normalizeLoreTags(item.Tags)
	item.Keywords = normalizeLoreKeywords(item.Keywords)
	item.BriefDescription = strings.TrimSpace(item.BriefDescription)
	if item.BriefDescription == "" {
		item.BriefDescription = defaultLoreBriefDescription(item)
	}
	item.Image = normalizeLoreItemImage(item.Image)
	item.Provenance = normalizeLoreProvenance(item.Provenance)
	return item
}

func normalizeLoreTypeSource(value string) string {
	switch strings.TrimSpace(value) {
	case TypeSourceHeuristic, TypeSourceSemantic, TypeSourceManual, TypeSourceLegacy:
		return strings.TrimSpace(value)
	default:
		return TypeSourceLegacy
	}
}

func normalizeLoreProvenance(value *Provenance) *Provenance {
	if value == nil {
		return nil
	}
	normalized := &Provenance{
		Kind:           strings.TrimSpace(value.Kind),
		SourceName:     strings.TrimSpace(value.SourceName),
		SourceRecordID: strings.TrimSpace(value.SourceRecordID),
		SourceHash:     strings.TrimSpace(value.SourceHash),
	}
	if normalized.Kind == "" && normalized.SourceName == "" && normalized.SourceRecordID == "" && normalized.SourceHash == "" {
		return nil
	}
	return normalized
}

func firstLoreImage(value, fallback *Image) *Image {
	if value != nil {
		return value
	}
	return fallback
}

func normalizeLoreItemImage(image *Image) *Image {
	if image == nil {
		return nil
	}
	normalized := *image
	normalized.Schema = strings.TrimSpace(normalized.Schema)
	normalized.ImagePath = filepath.ToSlash(strings.TrimSpace(normalized.ImagePath))
	normalized.MetaPath = filepath.ToSlash(strings.TrimSpace(normalized.MetaPath))
	normalized.AltText = strings.TrimSpace(normalized.AltText)
	normalized.ImagePresetID = strings.TrimSpace(normalized.ImagePresetID)
	normalized.ProfileID = strings.TrimSpace(normalized.ProfileID)
	normalized.Provider = strings.TrimSpace(normalized.Provider)
	normalized.Model = strings.TrimSpace(normalized.Model)
	normalized.Size = strings.TrimSpace(normalized.Size)
	normalized.Quality = strings.TrimSpace(normalized.Quality)
	normalized.OutputFormat = strings.TrimSpace(normalized.OutputFormat)
	normalized.CreatedAt = strings.TrimSpace(normalized.CreatedAt)
	normalized.RevisedPrompt = strings.TrimSpace(normalized.RevisedPrompt)
	normalized.MIMEType = strings.TrimSpace(normalized.MIMEType)
	if normalized.ImagePath == "" {
		return nil
	}
	return &normalized
}

func loreInputEnabled(enabled *bool, fallback bool) bool {
	if enabled == nil {
		return fallback
	}
	return *enabled
}

func defaultLoreBriefDescription(item Item) string {
	item.Type = NormalizeType(item.Type)
	name := strings.TrimSpace(item.Name)
	typeLabel := TypeLabel(item.Type)
	subject := typeLabel
	if name != "" {
		subject = fmt.Sprintf("%s %s", typeLabel, name)
	}

	if summary := lorePlainTextSummary(item.Content, 72); summary != "" {
		return truncateRunes(subject+"。"+summary, 240)
	}

	signals := normalizeLoreStringList(append(append([]string{}, item.Tags...), item.Keywords...))
	if len(signals) > 0 {
		return truncateRunes(subject+"。触发词："+strings.Join(signals, "、"), 240)
	}
	if name != "" {
		return subject + "。请补充 3-5 句身份、别名、关键事实、适用场景和触发词。"
	}
	return "资料库条目。请补充 3-5 句类型、名称、关键事实、适用场景和触发词。"
}

func lorePlainTextSummary(content string, limit int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if limit <= 0 {
		limit = 72
	}

	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		line = normalizeLoreSummaryLine(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if utf8.RuneCountInString(strings.Join(lines, " / ")) >= limit {
			break
		}
	}
	return truncateRunes(strings.Join(lines, " / "), limit)
}

func normalizeLoreSummaryLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "#>*-+ 	")
	line = strings.TrimSpace(line)
	if line == "" || strings.Trim(line, "-|: ") == "" {
		return ""
	}
	for _, marker := range []string{"**", "__", "`"} {
		line = strings.ReplaceAll(line, marker, "")
	}
	return strings.Join(strings.Fields(line), " ")
}

func normalizeLoreID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var sb strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (item Item) EffectiveKeywords() []string {
	return normalizeLoreKeywords(append(append([]string{item.Name}, item.Tags...), item.Keywords...))
}

// NormalizeType returns one canonical lore type, falling back to other.
func NormalizeType(t string) string {
	switch strings.TrimSpace(t) {
	case "character", "world", "location", "faction", "rule", "item", "other":
		return strings.TrimSpace(t)
	default:
		return "other"
	}
}

func normalizeOptionalLoreType(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	return NormalizeType(t)
}

func normalizeLoreImportance(v string) string {
	switch strings.TrimSpace(v) {
	case "major", "important", "minor":
		return strings.TrimSpace(v)
	default:
		return "important"
	}
}

func normalizeLoreLoadMode(v, importance string) string {
	switch strings.TrimSpace(v) {
	case LoadModeResident, LoadModeAuto, LoadModeManual:
		return strings.TrimSpace(v)
	}
	if normalizeLoreImportance(importance) == "major" {
		return LoadModeResident
	}
	return LoadModeAuto
}

func normalizeLoreTags(tags []string) []string {
	return normalizeLoreStringList(tags)
}

func normalizeLoreKeywords(keywords []string) []string {
	return normalizeLoreStringList(keywords)
}

func normalizeLoreStringList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func newLoreID(name, itemType string) string {
	base := loreIDBaseFromName(name)
	if base == "" {
		base = NormalizeType(itemType)
	}
	return base
}

func newUniqueLoreID(items []Item, name, itemType string) string {
	return uniqueLoreIDFromBase(items, newLoreID(name, itemType))
}

func uniqueLoreIDFromBase(items []Item, base string) string {
	base = normalizeLoreID(base)
	if base == "" {
		base = newLoreID("", "other")
	}
	if loreItemIndex(items, base) < 0 {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if loreItemIndex(items, candidate) < 0 {
			return candidate
		}
	}
}

func loreIDBaseFromName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var sb strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
		case r == '-' || r == '_':
			if sb.Len() > 0 && !lastUnderscore {
				sb.WriteRune(r)
				lastUnderscore = true
			}
		case unicode.IsSpace(r):
			if sb.Len() > 0 && !lastUnderscore {
				sb.WriteRune('_')
				lastUnderscore = true
			}
		default:
			if sb.Len() > 0 && !lastUnderscore {
				sb.WriteRune('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(sb.String(), "_-")
}

func loreItemIndex(items []Item, id string) int {
	id = normalizeLoreID(id)
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func loreItemNameIndex(items []Item, name, exceptID string) int {
	key := loreNameKey(name)
	if key == "" {
		return -1
	}
	exceptID = normalizeLoreID(exceptID)
	for i, item := range items {
		if exceptID != "" && item.ID == exceptID {
			continue
		}
		if loreNameKey(item.Name) == key {
			return i
		}
	}
	return -1
}

func loreNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// NewNameAllocator seeds a name allocator from existing items.
func NewNameAllocator(items []Item) *NameAllocator {
	allocator := &NameAllocator{used: make(map[string]bool, len(items))}
	for _, item := range items {
		if key := loreNameKey(item.Name); key != "" {
			allocator.used[key] = true
		}
	}
	return allocator
}

func (a *NameAllocator) Claim(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if a == nil {
		return name
	}
	if key := loreNameKey(name); key != "" && !a.used[key] {
		a.used[key] = true
		return name
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", name, suffix)
		key := loreNameKey(candidate)
		if key == "" || a.used[key] {
			continue
		}
		a.used[key] = true
		return candidate
	}
}

func firstNonEmptyLoreValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func loreImportanceRank(v string) int {
	switch normalizeLoreImportance(v) {
	case "major":
		return 0
	case "important":
		return 1
	default:
		return 2
	}
}

// TypeLabel returns the Chinese display label for a canonical lore type.
func TypeLabel(t string) string {
	switch NormalizeType(t) {
	case "character":
		return "角色"
	case "world":
		return "世界观"
	case "location":
		return "地点"
	case "faction":
		return "势力"
	case "rule":
		return "规则"
	case "item":
		return "物品"
	default:
		return "其他"
	}
}

func loreLoadModeLabel(v string) string {
	switch normalizeLoreLoadMode(v, "") {
	case LoadModeResident:
		return "常驻 system prompt"
	case LoadModeManual:
		return "手动引用"
	default:
		return "按简介自动加载"
	}
}
