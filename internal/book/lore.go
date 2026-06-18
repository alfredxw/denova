package book

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const loreItemsVersion = 1

const (
	LoreLoadModeResident = "resident"
	LoreLoadModeAuto     = "auto"
	LoreLoadModeManual   = "manual"

	LoreResidentItemWarningChars  = 8000
	LoreResidentTotalWarningChars = 40000
)

// LoreItem 是用户可编辑的作品资料条目。固定字段只负责索引和展示，正文继续使用 Markdown。
type LoreItem struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Name             string   `json:"name"`
	Importance       string   `json:"importance"`
	Tags             []string `json:"tags"`
	BriefDescription string   `json:"brief_description"`
	Keywords         []string `json:"keywords"`
	LoadMode         string   `json:"load_mode"`
	Content          string   `json:"content"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type LoreItemInput struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Name             string   `json:"name"`
	Importance       string   `json:"importance"`
	Tags             []string `json:"tags"`
	BriefDescription string   `json:"brief_description"`
	Keywords         []string `json:"keywords"`
	LoadMode         string   `json:"load_mode"`
	Content          string   `json:"content"`
}

type LoreCollection struct {
	Version int        `json:"version"`
	Items   []LoreItem `json:"items"`
}

type LoreOperation struct {
	Op   string        `json:"op"`
	ID   string        `json:"id,omitempty"`
	Item LoreItemInput `json:"item,omitempty"`
}

type LoreApplyResult struct {
	Message    string     `json:"message"`
	Items      []LoreItem `json:"items"`
	Created    []LoreItem `json:"created"`
	Updated    []LoreItem `json:"updated"`
	DeletedIDs []string   `json:"deleted_ids"`
}

type LoreStore struct {
	workspace string
}

func NewLoreStore(workspace string) *LoreStore {
	return &LoreStore{workspace: workspace}
}

func (s *LoreStore) List() ([]LoreItem, error) {
	collection, err := s.loadOrCreate()
	if err != nil {
		return nil, err
	}
	items := append([]LoreItem(nil), collection.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		if loreImportanceRank(items[i].Importance) != loreImportanceRank(items[j].Importance) {
			return loreImportanceRank(items[i].Importance) < loreImportanceRank(items[j].Importance)
		}
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *LoreStore) Create(input LoreItemInput) (LoreItem, error) {
	collection, err := s.loadOrCreate()
	if err != nil {
		return LoreItem{}, err
	}
	now := time.Now().Format(time.RFC3339)
	item := normalizeLoreItem(LoreItem{
		ID:               input.ID,
		Type:             input.Type,
		Name:             input.Name,
		Importance:       input.Importance,
		Tags:             input.Tags,
		BriefDescription: input.BriefDescription,
		Keywords:         input.Keywords,
		LoadMode:         input.LoadMode,
		Content:          input.Content,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if item.ID == "" {
		item.ID = newUniqueLoreID(collection.Items, item.Name, item.Type)
	}
	if item.Name == "" {
		return LoreItem{}, errors.New("资料名称不能为空")
	}
	if s.hasItem(collection.Items, item.ID) {
		return LoreItem{}, fmt.Errorf("资料 ID 已存在: %s", item.ID)
	}
	collection.Items = append(collection.Items, item)
	if err := s.save(collection); err != nil {
		return LoreItem{}, err
	}
	return item, nil
}

func (s *LoreStore) Update(id string, input LoreItemInput) (LoreItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LoreItem{}, errors.New("资料 ID 不能为空")
	}
	collection, err := s.loadOrCreate()
	if err != nil {
		return LoreItem{}, err
	}
	for i := range collection.Items {
		if collection.Items[i].ID != id {
			continue
		}
		updated := normalizeLoreItem(LoreItem{
			ID:               id,
			Type:             input.Type,
			Name:             input.Name,
			Importance:       input.Importance,
			Tags:             input.Tags,
			BriefDescription: input.BriefDescription,
			Keywords:         input.Keywords,
			LoadMode:         input.LoadMode,
			Content:          input.Content,
			CreatedAt:        collection.Items[i].CreatedAt,
			UpdatedAt:        time.Now().Format(time.RFC3339),
		})
		if updated.Name == "" {
			return LoreItem{}, errors.New("资料名称不能为空")
		}
		collection.Items[i] = updated
		if err := s.save(collection); err != nil {
			return LoreItem{}, err
		}
		return updated, nil
	}
	return LoreItem{}, fmt.Errorf("资料不存在: %s", id)
}

func (s *LoreStore) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("资料 ID 不能为空")
	}
	collection, err := s.loadOrCreate()
	if err != nil {
		return err
	}
	next := make([]LoreItem, 0, len(collection.Items))
	found := false
	for _, item := range collection.Items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return fmt.Errorf("资料不存在: %s", id)
	}
	collection.Items = next
	return s.save(collection)
}

func (s *LoreStore) ApplyOperations(message string, ops []LoreOperation) (LoreApplyResult, error) {
	if len(ops) == 0 {
		return LoreApplyResult{}, errors.New("没有可执行的资料库操作")
	}
	collection, err := s.loadOrCreate()
	if err != nil {
		return LoreApplyResult{}, err
	}

	next := append([]LoreItem(nil), collection.Items...)
	result := LoreApplyResult{Message: strings.TrimSpace(message)}
	for _, op := range ops {
		switch strings.TrimSpace(op.Op) {
		case "create":
			item := normalizeLoreItem(LoreItem{
				ID:               op.Item.ID,
				Type:             op.Item.Type,
				Name:             op.Item.Name,
				Importance:       op.Item.Importance,
				Tags:             op.Item.Tags,
				BriefDescription: op.Item.BriefDescription,
				Keywords:         op.Item.Keywords,
				LoadMode:         op.Item.LoadMode,
				Content:          op.Item.Content,
				CreatedAt:        time.Now().Format(time.RFC3339),
				UpdatedAt:        time.Now().Format(time.RFC3339),
			})
			if item.ID == "" {
				item.ID = newUniqueLoreID(next, item.Name, item.Type)
			}
			if item.Name == "" {
				return LoreApplyResult{}, errors.New("创建资料时名称不能为空")
			}
			if loreItemIndex(next, item.ID) >= 0 {
				return LoreApplyResult{}, fmt.Errorf("资料 ID 已存在: %s", item.ID)
			}
			next = append(next, item)
			result.Created = append(result.Created, item)
		case "update":
			id := normalizeLoreID(firstNonEmptyLoreValue(op.ID, op.Item.ID))
			if id == "" {
				return LoreApplyResult{}, errors.New("更新资料时 ID 不能为空")
			}
			idx := loreItemIndex(next, id)
			if idx < 0 {
				return LoreApplyResult{}, fmt.Errorf("资料不存在: %s", id)
			}
			updated := normalizeLoreItem(LoreItem{
				ID:               id,
				Type:             firstNonEmptyLoreValue(op.Item.Type, next[idx].Type),
				Name:             firstNonEmptyLoreValue(op.Item.Name, next[idx].Name),
				Importance:       firstNonEmptyLoreValue(op.Item.Importance, next[idx].Importance),
				Tags:             op.Item.Tags,
				BriefDescription: firstNonEmptyLoreValue(op.Item.BriefDescription, next[idx].BriefDescription),
				Keywords:         op.Item.Keywords,
				LoadMode:         firstNonEmptyLoreValue(op.Item.LoadMode, next[idx].LoadMode),
				Content:          firstNonEmptyLoreValue(op.Item.Content, next[idx].Content),
				CreatedAt:        next[idx].CreatedAt,
				UpdatedAt:        time.Now().Format(time.RFC3339),
			})
			if op.Item.Tags == nil {
				updated.Tags = append([]string(nil), next[idx].Tags...)
			}
			if op.Item.Keywords == nil {
				updated.Keywords = append([]string(nil), next[idx].Keywords...)
			}
			if updated.Name == "" {
				return LoreApplyResult{}, fmt.Errorf("资料名称不能为空: %s", id)
			}
			next[idx] = updated
			result.Updated = append(result.Updated, updated)
		case "delete":
			id := normalizeLoreID(firstNonEmptyLoreValue(op.ID, op.Item.ID))
			if id == "" {
				return LoreApplyResult{}, errors.New("删除资料时 ID 不能为空")
			}
			idx := loreItemIndex(next, id)
			if idx < 0 {
				return LoreApplyResult{}, fmt.Errorf("资料不存在: %s", id)
			}
			next = append(next[:idx], next[idx+1:]...)
			result.DeletedIDs = append(result.DeletedIDs, id)
		default:
			return LoreApplyResult{}, fmt.Errorf("不支持的资料库操作: %s", op.Op)
		}
	}
	collection.Items = next
	if err := s.save(collection); err != nil {
		return LoreApplyResult{}, err
	}
	result.Items, err = s.List()
	if err != nil {
		return LoreApplyResult{}, err
	}
	return result, nil
}

func (s *LoreStore) Read(id string) (LoreItem, error) {
	id = normalizeLoreID(id)
	if id == "" {
		return LoreItem{}, errors.New("资料 ID 不能为空")
	}
	items, err := s.List()
	if err != nil {
		return LoreItem{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return LoreItem{}, fmt.Errorf("资料不存在: %s", id)
}

func (s *LoreStore) ReadMany(ids []string) ([]LoreItem, error) {
	if len(ids) == 0 {
		return nil, errors.New("资料 ID 列表不能为空")
	}
	wanted := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = normalizeLoreID(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		wanted = append(wanted, id)
	}
	if len(wanted) == 0 {
		return nil, errors.New("资料 ID 列表不能为空")
	}
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]LoreItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := make([]LoreItem, 0, len(wanted))
	for _, id := range wanted {
		item, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("资料不存在: %s", id)
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *LoreStore) Search(query, itemType string, limit int) ([]LoreItem, error) {
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	itemType = normalizeOptionalLoreType(itemType)
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	result := make([]LoreItem, 0, limit)
	for _, item := range items {
		if itemType != "" && item.Type != itemType {
			continue
		}
		if query != "" && !loreItemMatchesQuery(item, query) {
			continue
		}
		result = append(result, item)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *LoreStore) ResidentContextMarkdown() (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	totalChars := 0
	for _, item := range items {
		if item.LoadMode != LoreLoadModeResident {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		chars := utf8.RuneCountInString(content)
		totalChars += chars
		if chars > LoreResidentItemWarningChars {
			log.Printf("[lore-context] resident item too long id=%s name=%s chars=%d threshold=%d", item.ID, item.Name, chars, LoreResidentItemWarningChars)
		}
		sb.WriteString(formatLoreItemMarkdown(item, true))
		sb.WriteString("\n\n")
	}
	if totalChars > LoreResidentTotalWarningChars {
		log.Printf("[lore-context] resident context too long chars=%d threshold=%d", totalChars, LoreResidentTotalWarningChars)
	}
	return strings.TrimSpace(sb.String()), nil
}

func (s *LoreStore) IndexMarkdown() (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, item := range items {
		if item.LoadMode == LoreLoadModeResident {
			continue
		}
		fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  重要度: %s\n  加载策略: %s\n", item.ID, item.Name, loreTypeLabel(item.Type), loreImportanceLabel(item.Importance), loreLoadModeLabel(item.LoadMode))
		if len(item.Tags) > 0 {
			fmt.Fprintf(&sb, "  标签: %s\n", strings.Join(item.Tags, "、"))
		}
		if item.BriefDescription != "" {
			fmt.Fprintf(&sb, "  简介: %s\n", item.BriefDescription)
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func (s *LoreStore) ProgressiveContextMarkdown() (string, error) {
	resident, err := s.ResidentContextMarkdown()
	if err != nil {
		return "", err
	}
	index, err := s.IndexMarkdown()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if resident != "" {
		sb.WriteString("## 常驻资料库\n\n")
		sb.WriteString(resident)
		sb.WriteString("\n\n")
	}
	if index != "" {
		sb.WriteString("## 资料库索引\n\n")
		sb.WriteString(index)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func (s *LoreStore) ContextMarkdown() (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		sb.WriteString(formatLoreItemMarkdown(item, true))
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func (s *LoreStore) Ensure() error {
	_, err := s.loadOrCreate()
	return err
}

func (s *LoreStore) loadOrCreate() (LoreCollection, error) {
	path := s.itemsPath()
	data, err := os.ReadFile(path)
	if err == nil {
		var collection LoreCollection
		if err := json.Unmarshal(data, &collection); err != nil {
			return LoreCollection{}, fmt.Errorf("解析 lore items 失败: %w", err)
		}
		collection.Version = loreItemsVersion
		collection.Items = normalizeLoreItems(collection.Items)
		return collection, nil
	}
	if !os.IsNotExist(err) {
		return LoreCollection{}, err
	}
	collection := LoreCollection{Version: loreItemsVersion}
	if err := s.save(collection); err != nil {
		return LoreCollection{}, err
	}
	return collection, nil
}

func (s *LoreStore) save(collection LoreCollection) error {
	collection.Version = loreItemsVersion
	collection.Items = normalizeLoreItems(collection.Items)
	if err := os.MkdirAll(filepath.Dir(s.itemsPath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.itemsPath(), append(data, '\n'), 0o644)
}

func (s *LoreStore) itemsPath() string {
	return filepath.Join(s.workspace, ".nova", "lore", "items.json")
}

func (s *LoreStore) hasItem(items []LoreItem, id string) bool {
	return loreItemIndex(items, id) >= 0
}

func normalizeLoreItems(items []LoreItem) []LoreItem {
	normalized := make([]LoreItem, 0, len(items))
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

func normalizeLoreItem(item LoreItem) LoreItem {
	item.ID = normalizeLoreID(item.ID)
	item.Type = normalizeLoreType(item.Type)
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
	return item
}

func defaultLoreBriefDescription(item LoreItem) string {
	item.Type = normalizeLoreType(item.Type)
	name := strings.TrimSpace(item.Name)
	typeLabel := loreTypeLabel(item.Type)
	subject := typeLabel
	if name != "" {
		subject = fmt.Sprintf("%s %s", typeLabel, name)
	}

	if summary := lorePlainTextSummary(item.Content, 72); summary != "" {
		return truncateRunes(subject+"。"+summary+"。上下文出现"+loreBriefTriggerSubject(typeLabel, name)+"相关内容时，一定要参考本项详情。", 240)
	}

	signals := normalizeLoreStringList(append(append([]string{}, item.Tags...), item.Keywords...))
	if len(signals) > 0 {
		return truncateRunes(subject+"。触发词："+strings.Join(signals, "、")+"。上下文出现"+loreBriefTriggerSubject(typeLabel, name)+"相关内容时，一定要参考本项详情。", 240)
	}
	if name != "" {
		return subject + "。请补充 3-5 句身份、别名、关键事实、适用场景和触发词。上下文出现" + loreBriefTriggerSubject(typeLabel, name) + "相关内容时，一定要参考本项详情。"
	}
	return "资料库条目。请补充 3-5 句类型、名称、关键事实、适用场景和触发词。上下文出现相关内容时，一定要参考本项详情。"
}

func loreBriefTriggerSubject(typeLabel, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return typeLabel
	}
	return name + "、" + typeLabel
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

func (item LoreItem) EffectiveKeywords() []string {
	return normalizeLoreKeywords(append(append([]string{item.Name}, item.Tags...), item.Keywords...))
}

func normalizeLoreType(t string) string {
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
	return normalizeLoreType(t)
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
	case LoreLoadModeResident, LoreLoadModeAuto, LoreLoadModeManual:
		return strings.TrimSpace(v)
	}
	if normalizeLoreImportance(importance) == "major" {
		return LoreLoadModeResident
	}
	return LoreLoadModeAuto
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
		base = normalizeLoreType(itemType)
	}
	return fmt.Sprintf("%s_%s", base, randomLoreIDSuffix())
}

func newUniqueLoreID(items []LoreItem, name, itemType string) string {
	return uniqueLoreIDFromBase(items, newLoreID(name, itemType))
}

func uniqueLoreIDFromBase(items []LoreItem, base string) string {
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
		case r == '-' || r == '_' || unicode.IsSpace(r):
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
	return strings.Trim(sb.String(), "_")
}

func randomLoreIDSuffix() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		n := time.Now().UTC().UnixNano()
		for i := range b {
			b[i] = byte(n >> (i * 8))
		}
	}
	var sb strings.Builder
	for _, v := range b {
		sb.WriteByte(alphabet[int(v)%len(alphabet)])
	}
	return sb.String()
}

func loreItemIndex(items []LoreItem, id string) int {
	id = normalizeLoreID(id)
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
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

func loreTypeLabel(t string) string {
	switch normalizeLoreType(t) {
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
	case LoreLoadModeResident:
		return "常驻 system prompt"
	case LoreLoadModeManual:
		return "手动引用"
	default:
		return "按简介自动加载"
	}
}

func formatLoreItemMarkdown(item LoreItem, includeContent bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s（%s / %s / %s）\n\n", item.Name, loreTypeLabel(item.Type), loreImportanceLabel(item.Importance), loreLoadModeLabel(item.LoadMode))
	if item.ID != "" {
		fmt.Fprintf(&sb, "ID：%s\n", item.ID)
	}
	if len(item.Tags) > 0 {
		sb.WriteString("标签：")
		sb.WriteString(strings.Join(item.Tags, "、"))
		sb.WriteString("\n")
	}
	if item.BriefDescription != "" {
		sb.WriteString("简介：")
		sb.WriteString(item.BriefDescription)
		sb.WriteString("\n")
	}
	if includeContent {
		content := strings.TrimSpace(item.Content)
		if content != "" {
			sb.WriteString("\n")
			sb.WriteString(content)
		}
	}
	return strings.TrimSpace(sb.String())
}

func loreItemMatchesQuery(item LoreItem, query string) bool {
	parts := []string{item.ID, item.Name, item.Type, loreTypeLabel(item.Type), item.Importance, loreImportanceLabel(item.Importance), item.LoadMode, loreLoadModeLabel(item.LoadMode), item.BriefDescription}
	parts = append(parts, item.Tags...)
	for _, part := range parts {
		if strings.Contains(strings.ToLower(part), query) {
			return true
		}
	}
	return false
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
