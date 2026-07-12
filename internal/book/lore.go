package book

import (
	"crypto/sha256"
	"encoding/hex"
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

	"denova/internal/workspacepath"
)

const loreItemsVersion = 1

const (
	LoreLoadModeResident = "resident"
	LoreLoadModeAuto     = "auto"
	LoreLoadModeManual   = "manual"

	LoreResidentItemWarningChars  = 8000
	LoreResidentTotalWarningChars = 40000

	LoreIndexDefaultMaxBytes   = 64 * 1024
	LoreIndexDefaultQueryLimit = 20
	LoreIndexMaxQueryLimit     = 50
)

// LoreItem 是用户可编辑的作品资料条目。固定字段只负责索引和展示，正文继续使用 Markdown。
type LoreItem struct {
	ID               string          `json:"id"`
	Enabled          bool            `json:"enabled"`
	Type             string          `json:"type"`
	Name             string          `json:"name"`
	Importance       string          `json:"importance"`
	Tags             []string        `json:"tags"`
	BriefDescription string          `json:"brief_description"`
	Keywords         []string        `json:"keywords"`
	LoadMode         string          `json:"load_mode"`
	Content          string          `json:"content"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	Image            *LoreItemImage  `json:"image,omitempty"`
	Provenance       *LoreProvenance `json:"provenance,omitempty"`
}

type LoreItemInput struct {
	ID               string          `json:"id"`
	Enabled          *bool           `json:"enabled,omitempty"`
	Type             string          `json:"type"`
	Name             string          `json:"name"`
	Importance       string          `json:"importance"`
	Tags             []string        `json:"tags"`
	BriefDescription string          `json:"brief_description"`
	Keywords         []string        `json:"keywords"`
	LoadMode         string          `json:"load_mode"`
	Content          string          `json:"content"`
	Image            *LoreItemImage  `json:"image,omitempty"`
	Provenance       *LoreProvenance `json:"provenance,omitempty"`
	BaseRevision     string          `json:"base_revision,omitempty"`
}

// LoreProvenance records an item's external origin without exposing it in
// model-visible lore markdown. It is intentionally generic so future importers
// can use the same storage boundary.
type LoreProvenance struct {
	Kind           string `json:"kind"`
	SourceName     string `json:"source_name"`
	SourceRecordID string `json:"source_record_id"`
	SourceHash     string `json:"source_hash"`
}

// LoreItemImage is the current visual asset attached to a lore item.
type LoreItemImage struct {
	Schema        string `json:"schema"`
	ImagePath     string `json:"image_path"`
	MetaPath      string `json:"meta_path"`
	AltText       string `json:"alt_text,omitempty"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	ProfileID     string `json:"profile_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`

	RevisedPrompt string `json:"revised_prompt,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
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

type loreNameAllocator struct {
	used map[string]bool
}

// LoreIndexOptions controls model-visible lore index rendering.
// Empty options render a compact all-item index; query/type options render a
// narrower searchable view without including full lore content.
type LoreIndexOptions struct {
	Query           string
	Type            string
	Limit           int
	Offset          int
	Paginate        bool
	MaxBytes        int
	ExcludeResident bool
	OmitTitle       bool
}

var ErrLoreRevisionConflict = errors.New("资料已被其他操作更新，请重新加载后再保存")

func (item *LoreItem) UnmarshalJSON(data []byte) error {
	type loreItemAlias LoreItem
	raw := struct {
		Enabled *bool `json:"enabled"`
		*loreItemAlias
	}{
		loreItemAlias: (*loreItemAlias)(item),
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	item.Enabled = true
	if raw.Enabled != nil {
		item.Enabled = *raw.Enabled
	}
	return nil
}

func NewLoreStore(workspace string) *LoreStore {
	return &LoreStore{workspace: workspace}
}

func (s *LoreStore) List() ([]LoreItem, error) {
	return s.list(false)
}

func (s *LoreStore) ListAll() ([]LoreItem, error) {
	return s.list(true)
}

// Revision identifies the current enabled lore catalog for incremental
// Director review. It changes when a name, summary, body or enabled state
// changes, without exposing the full collection to metadata consumers.
func (s *LoreStore) Revision() (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12]), nil
}

func (s *LoreStore) list(includeDisabled bool) ([]LoreItem, error) {
	collection, err := s.loadOrCreate()
	if err != nil {
		return nil, err
	}
	items := make([]LoreItem, 0, len(collection.Items))
	for _, item := range collection.Items {
		if !includeDisabled && !item.Enabled {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Enabled != items[j].Enabled {
			return items[i].Enabled
		}
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := normalizeLoreItem(LoreItem{
		ID:               input.ID,
		Enabled:          loreInputEnabled(input.Enabled, true),
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
		Image:            input.Image,
		Provenance:       input.Provenance,
	})
	if item.ID == "" {
		item.ID = newUniqueLoreID(collection.Items, item.Name, item.Type)
	}
	if item.Name == "" {
		return LoreItem{}, errors.New("资料名称不能为空")
	}
	if err := validateLoreReferenceName(item.Name); err != nil {
		return LoreItem{}, err
	}
	if loreItemNameIndex(collection.Items, item.Name, "") >= 0 {
		return LoreItem{}, fmt.Errorf("资料名称已存在: %s", item.Name)
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
		if input.BaseRevision != "" && collection.Items[i].UpdatedAt != input.BaseRevision {
			return LoreItem{}, ErrLoreRevisionConflict
		}
		previous := collection.Items[i]
		updated := normalizeLoreItem(LoreItem{
			ID:               id,
			Enabled:          loreInputEnabled(input.Enabled, collection.Items[i].Enabled),
			Type:             input.Type,
			Name:             input.Name,
			Importance:       input.Importance,
			Tags:             input.Tags,
			BriefDescription: input.BriefDescription,
			Keywords:         input.Keywords,
			LoadMode:         input.LoadMode,
			Content:          input.Content,
			CreatedAt:        collection.Items[i].CreatedAt,
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
			Image:            firstLoreImage(input.Image, collection.Items[i].Image),
			Provenance:       collection.Items[i].Provenance,
		})
		if updated.Name == "" {
			return LoreItem{}, errors.New("资料名称不能为空")
		}
		if err := validateLoreReferenceName(updated.Name); err != nil {
			return LoreItem{}, err
		}
		if loreItemNameIndex(collection.Items, updated.Name, id) >= 0 {
			return LoreItem{}, fmt.Errorf("资料名称已存在: %s", updated.Name)
		}
		if !updated.Enabled && previous.Enabled {
			paths, err := loreReferencePaths(s.workspace, previous.Name)
			if err != nil {
				return LoreItem{}, err
			}
			if len(paths) > 0 {
				return LoreItem{}, fmt.Errorf("资料 %s 正被 %d 个互动分支引用，请先从 lore-context.md 移除后再禁用", previous.Name, len(paths))
			}
		}
		rewrites, err := prepareLoreReferenceRewrites(s.workspace, previous.Name, updated.Name)
		if err != nil {
			return LoreItem{}, err
		}
		collection.Items[i] = updated
		if err := s.save(collection); err != nil {
			return LoreItem{}, err
		}
		if err := applyLoreReferenceRewrites(rewrites); err != nil {
			collection.Items[i] = previous
			if rollbackErr := s.save(collection); rollbackErr != nil {
				log.Printf("[lore-reference] rollback lore item failed id=%s err=%v", id, rollbackErr)
			}
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
	for _, item := range collection.Items {
		if item.ID != id {
			continue
		}
		paths, refErr := loreReferencePaths(s.workspace, item.Name)
		if refErr != nil {
			return refErr
		}
		if len(paths) > 0 {
			return fmt.Errorf("资料 %s 正被 %d 个互动分支引用，请先从 lore-context.md 移除后再删除", item.Name, len(paths))
		}
		break
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
			now := time.Now().UTC().Format(time.RFC3339Nano)
			item := normalizeLoreItem(LoreItem{
				ID:               op.Item.ID,
				Enabled:          loreInputEnabled(op.Item.Enabled, true),
				Type:             op.Item.Type,
				Name:             op.Item.Name,
				Importance:       op.Item.Importance,
				Tags:             op.Item.Tags,
				BriefDescription: op.Item.BriefDescription,
				Keywords:         op.Item.Keywords,
				LoadMode:         op.Item.LoadMode,
				Content:          op.Item.Content,
				CreatedAt:        now,
				UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
				Image:            op.Item.Image,
				Provenance:       op.Item.Provenance,
			})
			if item.Name == "" {
				return LoreApplyResult{}, errors.New("创建资料时名称不能为空")
			}
			if loreItemNameIndex(next, item.Name, "") >= 0 {
				return LoreApplyResult{}, fmt.Errorf("资料名称已存在: %s", item.Name)
			}
			if item.ID == "" {
				item.ID = newUniqueLoreID(next, item.Name, item.Type)
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
				Enabled:          loreInputEnabled(op.Item.Enabled, next[idx].Enabled),
				Type:             firstNonEmptyLoreValue(op.Item.Type, next[idx].Type),
				Name:             firstNonEmptyLoreValue(op.Item.Name, next[idx].Name),
				Importance:       firstNonEmptyLoreValue(op.Item.Importance, next[idx].Importance),
				Tags:             op.Item.Tags,
				BriefDescription: firstNonEmptyLoreValue(op.Item.BriefDescription, next[idx].BriefDescription),
				Keywords:         op.Item.Keywords,
				LoadMode:         firstNonEmptyLoreValue(op.Item.LoadMode, next[idx].LoadMode),
				Content:          firstNonEmptyLoreValue(op.Item.Content, next[idx].Content),
				CreatedAt:        next[idx].CreatedAt,
				UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
				Image:            firstLoreImage(op.Item.Image, next[idx].Image),
				Provenance:       next[idx].Provenance,
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
			if loreItemNameIndex(next, updated.Name, id) >= 0 {
				return LoreApplyResult{}, fmt.Errorf("资料名称已存在: %s", updated.Name)
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

func (s *LoreStore) ReadAny(id string) (LoreItem, error) {
	id = normalizeLoreID(id)
	if id == "" {
		return LoreItem{}, errors.New("资料 ID 不能为空")
	}
	items, err := s.ListAll()
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

func (s *LoreStore) SetImage(id string, image *LoreItemImage) (LoreItem, error) {
	id = normalizeLoreID(id)
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
		collection.Items[i].Image = normalizeLoreItemImage(image)
		collection.Items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		collection.Items[i] = normalizeLoreItem(collection.Items[i])
		if err := s.save(collection); err != nil {
			return LoreItem{}, err
		}
		return collection.Items[i], nil
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

// ReadManyNames resolves user-facing unique lore names in request order.
func (s *LoreStore) ReadManyNames(names []string) ([]LoreItem, error) {
	if len(names) == 0 {
		return nil, errors.New("资料名称列表不能为空")
	}
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	byName := make(map[string]LoreItem, len(items))
	for _, item := range items {
		byName[loreNameKey(item.Name)] = item
	}
	result := make([]LoreItem, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		key := loreNameKey(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		item, ok := byName[key]
		if !ok {
			return nil, fmt.Errorf("资料不存在: %s", strings.TrimSpace(name))
		}
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil, errors.New("资料名称列表不能为空")
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

func (s *LoreStore) LoreIndexMarkdown(options LoreIndexOptions) (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	entries, total := filterLoreIndexEntries(items, options)
	if len(entries) == 0 && options.OmitTitle {
		return "", nil
	}
	return renderLoreIndexMarkdown(entries, total, options), nil
}

func (s *LoreStore) ResidentContextMarkdown() (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
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

// ResidentContentBytes returns the exact UTF-8 size of enabled resident lore
// bodies. Import budget checks use the same body set as model context assembly.
func (s *LoreStore) ResidentContentBytes() (int, error) {
	items, err := s.List()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, item := range items {
		if item.LoadMode == LoreLoadModeResident {
			total += len([]byte(strings.TrimSpace(item.Content)))
		}
	}
	return total, nil
}

func (s *LoreStore) IndexMarkdown() (string, error) {
	return s.LoreIndexMarkdown(LoreIndexOptions{ExcludeResident: true, OmitTitle: true})
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

// StoryMemoryContextMarkdown returns bounded lore context for interactive story-memory consolidation.
// It prefers full entries within maxBytes and keeps an index for entries that cannot be expanded.
func (s *LoreStore) StoryMemoryContextMarkdown(maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}
	items, err := s.List()
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", nil
	}

	fullBudget := maxBytes * 3 / 4
	if fullBudget < maxBytes-8*1024 {
		fullBudget = maxBytes - 8*1024
	}
	if fullBudget < maxBytes/2 {
		fullBudget = maxBytes / 2
	}

	includedFull := map[string]bool{}
	var full strings.Builder
	for _, item := range items {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		block := formatLoreItemMarkdown(item, true) + "\n\n"
		if full.Len()+len([]byte(block)) > fullBudget {
			continue
		}
		full.WriteString(block)
		includedFull[item.ID] = true
	}

	var sb strings.Builder
	if strings.TrimSpace(full.String()) != "" {
		appendLoreContextPart(&sb, "## 资料库完整条目（供故事记忆校准）\n\n", maxBytes)
		appendLoreContextPart(&sb, strings.TrimSpace(full.String())+"\n\n", maxBytes)
	}

	remaining := make([]LoreItem, 0)
	for _, item := range items {
		if !includedFull[item.ID] {
			remaining = append(remaining, item)
		}
	}
	if len(remaining) > 0 {
		if sb.Len() > 0 {
			appendLoreContextPart(&sb, "\n", maxBytes)
		}
		appendLoreContextPart(&sb, "## 资料库索引（未完整注入，整理记忆时不得改写这些既有设定）\n\n", maxBytes)
		omitted := 0
		for _, item := range remaining {
			if !appendLoreContextPart(&sb, formatLoreItemIndexMarkdown(item), maxBytes) {
				omitted++
			}
		}
		if omitted > 0 {
			appendLoreContextPart(&sb, fmt.Sprintf("\n（还有 %d 条资料因上下文上限未展开。）\n", omitted), maxBytes)
		}
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
	return workspacepath.Path(s.workspace, "lore", "items.json")
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
	item.Image = normalizeLoreItemImage(item.Image)
	item.Provenance = normalizeLoreProvenance(item.Provenance)
	return item
}

func normalizeLoreProvenance(value *LoreProvenance) *LoreProvenance {
	if value == nil {
		return nil
	}
	normalized := &LoreProvenance{
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

func firstLoreImage(value, fallback *LoreItemImage) *LoreItemImage {
	if value != nil {
		return value
	}
	return fallback
}

func normalizeLoreItemImage(image *LoreItemImage) *LoreItemImage {
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
	return base
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

func loreItemIndex(items []LoreItem, id string) int {
	id = normalizeLoreID(id)
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func loreItemNameIndex(items []LoreItem, name, exceptID string) int {
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

func newLoreNameAllocator(items []LoreItem) *loreNameAllocator {
	allocator := &loreNameAllocator{used: make(map[string]bool, len(items))}
	for _, item := range items {
		if key := loreNameKey(item.Name); key != "" {
			allocator.used[key] = true
		}
	}
	return allocator
}

func (a *loreNameAllocator) Claim(name string) string {
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

type loreIndexEntry struct {
	Item         LoreItem
	MatchSources []string
}

func filterLoreIndexEntries(items []LoreItem, options LoreIndexOptions) ([]loreIndexEntry, int) {
	query := strings.TrimSpace(options.Query)
	itemType := normalizeOptionalLoreType(options.Type)
	shouldLimit := options.Paginate || query != "" || itemType != ""
	limit := normalizeLoreIndexLimit(options.Limit)
	matched := make([]loreIndexEntry, 0, len(items))
	total := 0
	for _, item := range items {
		if options.ExcludeResident && item.LoadMode == LoreLoadModeResident {
			continue
		}
		if itemType != "" && item.Type != itemType {
			continue
		}
		matchSources := loreItemMatchSources(item, query)
		if query != "" && len(matchSources) == 0 {
			continue
		}
		total++
		matched = append(matched, loreIndexEntry{Item: item, MatchSources: matchSources})
	}
	if !shouldLimit {
		return matched, len(matched)
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matched) {
		return nil, total
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], total
}

func normalizeLoreIndexLimit(limit int) int {
	if limit <= 0 {
		return LoreIndexDefaultQueryLimit
	}
	if limit > LoreIndexMaxQueryLimit {
		return LoreIndexMaxQueryLimit
	}
	return limit
}

func renderLoreIndexMarkdown(entries []loreIndexEntry, total int, options LoreIndexOptions) string {
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = LoreIndexDefaultMaxBytes
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
		{briefRunes: 72, hint: "（索引已压缩：简介已截断；需要具体设定时请用 query 缩小范围，再调用 read_lore_items 读取正文。）"},
		{nameOnly: true, hint: "（索引过大，已降级为仅 ID 和名称；需要具体设定时请用 query 细查，再调用 read_lore_items 读取正文。）"},
	}
	for _, candidate := range candidates {
		out := renderLoreIndexCandidate(entries, total, options, candidate.briefRunes, candidate.nameOnly, candidate.hint)
		if len([]byte(out)) <= maxBytes {
			return strings.TrimSpace(out)
		}
	}
	return renderBoundedLoreNameIndex(entries, total, options, maxBytes)
}

func renderLoreIndexCandidate(entries []loreIndexEntry, total int, options LoreIndexOptions, briefRunes int, nameOnly bool, hint string) string {
	var sb strings.Builder
	writeLoreIndexHeader(&sb, total, len(entries), options)
	if hint != "" {
		sb.WriteString(hint)
		sb.WriteString("\n\n")
	}
	for _, entry := range entries {
		sb.WriteString(formatCompactLoreIndexEntry(entry, briefRunes, nameOnly))
	}
	return strings.TrimSpace(sb.String())
}

func writeLoreIndexHeader(sb *strings.Builder, total, returned int, options LoreIndexOptions) {
	if !options.OmitTitle {
		sb.WriteString("# 资料库索引\n\n")
	}
	query := strings.TrimSpace(options.Query)
	itemType := normalizeOptionalLoreType(options.Type)
	if total == 0 {
		if query != "" || itemType != "" {
			sb.WriteString("未找到匹配资料库条目。\n")
			return
		}
		sb.WriteString("资料库暂无条目。\n")
		return
	}
	if options.Paginate || query != "" || itemType != "" {
		offset := options.Offset
		if offset < 0 {
			offset = 0
		}
		fmt.Fprintf(sb, "匹配 %d 条资料，本页返回 %d 条（offset=%d）。", total, returned, offset)
		if total > offset+returned {
			fmt.Fprintf(sb, " 下一页使用 offset=%d；每页最大 %d。", offset+returned, LoreIndexMaxQueryLimit)
		}
		sb.WriteString("\n\n")
		return
	}
	scope := "启用资料"
	if options.ExcludeResident {
		scope = "非驻留资料"
	}
	fmt.Fprintf(sb, "共 %d 条%s。默认索引只含 ID、名称和简介；需要正文时调用 read_lore_items。\n\n", total, scope)
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
	if len(entry.MatchSources) > 0 {
		fmt.Fprintf(&sb, "  匹配: %s\n", strings.Join(entry.MatchSources, "、"))
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

func renderBoundedLoreNameIndex(entries []loreIndexEntry, total int, options LoreIndexOptions, maxBytes int) string {
	var sb strings.Builder
	writeLoreIndexHeader(&sb, total, len(entries), options)
	hint := fmt.Sprintf("（索引预算不足，以下仅展示能放入预算的 ID 和名称；未显示条目请用 list_lore_items 带 query 细查，limit 最大 %d。）\n\n", LoreIndexMaxQueryLimit)
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
		notice := fmt.Sprintf("\n（还有 %d 条资料因索引预算未显示；请使用 query 细查。）\n", omitted)
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

func formatLoreItemIndexMarkdown(item LoreItem) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  重要度: %s\n  加载策略: %s\n", item.ID, item.Name, loreTypeLabel(item.Type), loreImportanceLabel(item.Importance), loreLoadModeLabel(item.LoadMode))
	if len(item.Tags) > 0 {
		fmt.Fprintf(&sb, "  标签: %s\n", strings.Join(item.Tags, "、"))
	}
	if item.BriefDescription != "" {
		fmt.Fprintf(&sb, "  简介: %s\n", item.BriefDescription)
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

func loreItemMatchesQuery(item LoreItem, query string) bool {
	return len(loreItemMatchSources(item, query)) > 0
}

func loreItemMatchSources(item LoreItem, query string) []string {
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
	addSource("类型", item.Type, loreTypeLabel(item.Type))
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
