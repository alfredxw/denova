package lore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

func NewStore(workspace string) *Store {
	return &Store{workspace: workspace, mutationMu: sharedLoreMutationLock(workspace)}
}

func sharedLoreMutationLock(workspace string) *sync.Mutex {
	path := ItemsPath(workspace)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	lock, _ := loreMutationLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (s *Store) List() ([]Item, error) {
	return s.list(false)
}

func (s *Store) ListAll() ([]Item, error) {
	return s.list(true)
}

// Get resolves one lore item by stable ID, including disabled items. Review
// comments and editor tabs bind to identity, so visibility must not make an
// existing item appear deleted.
func (s *Store) Get(id string) (Item, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Item{}, fmt.Errorf("资料 ID 不能为空: %w", os.ErrNotExist)
	}
	collection, err := s.loadOrCreate()
	if err != nil {
		return Item{}, err
	}
	for _, item := range collection.Items {
		if item.ID == id {
			return item, nil
		}
	}
	return Item{}, fmt.Errorf("资料不存在: %s: %w", id, os.ErrNotExist)
}

// Revision identifies the current enabled lore catalog for incremental
// Director review. It changes when a name, summary, body or enabled state
// changes, without exposing the full collection to metadata consumers.
func (s *Store) Revision() (string, error) {
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

func (s *Store) list(includeDisabled bool) ([]Item, error) {
	collection, err := s.loadOrCreate()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(collection.Items))
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

func (s *Store) Create(input ItemInput) (Item, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return Item{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := normalizeLoreItem(Item{
		ID:               input.ID,
		Enabled:          loreInputEnabled(input.Enabled, true),
		Type:             input.Type,
		TypeSource:       firstNonEmptyLoreValue(input.TypeSource, TypeSourceManual),
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
		return Item{}, errors.New("资料名称不能为空")
	}
	if err := validateLoreReferenceName(item.Name); err != nil {
		return Item{}, err
	}
	if loreItemNameIndex(collection.Items, item.Name, "") >= 0 {
		return Item{}, fmt.Errorf("资料名称已存在: %s", item.Name)
	}
	if s.hasItem(collection.Items, item.ID) {
		return Item{}, fmt.Errorf("资料 ID 已存在: %s", item.ID)
	}
	collection.Items = append(collection.Items, item)
	if err := s.save(collection); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Store) Update(id string, input ItemInput) (Item, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Item{}, errors.New("资料 ID 不能为空")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return Item{}, err
	}
	for i := range collection.Items {
		if collection.Items[i].ID != id {
			continue
		}
		if input.BaseRevision != "" && collection.Items[i].UpdatedAt != input.BaseRevision {
			return Item{}, ErrRevisionConflict
		}
		previous := collection.Items[i]
		typeSource := previous.TypeSource
		if NormalizeType(input.Type) != previous.Type {
			typeSource = TypeSourceManual
		}
		updated := normalizeLoreItem(Item{
			ID:               id,
			Enabled:          loreInputEnabled(input.Enabled, collection.Items[i].Enabled),
			Type:             input.Type,
			TypeSource:       typeSource,
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
			return Item{}, errors.New("资料名称不能为空")
		}
		if err := validateLoreReferenceName(updated.Name); err != nil {
			return Item{}, err
		}
		if loreItemNameIndex(collection.Items, updated.Name, id) >= 0 {
			return Item{}, fmt.Errorf("资料名称已存在: %s", updated.Name)
		}
		if !updated.Enabled && previous.Enabled {
			paths, err := loreReferencePaths(s.workspace, previous.Name)
			if err != nil {
				return Item{}, err
			}
			if len(paths) > 0 {
				return Item{}, fmt.Errorf("资料 %s 正被 %d 个互动分支引用，请先从 lore-context.md 移除后再禁用", previous.Name, len(paths))
			}
		}
		rewrites, err := prepareLoreReferenceRewrites(s.workspace, previous.Name, updated.Name)
		if err != nil {
			return Item{}, err
		}
		collection.Items[i] = updated
		if err := s.save(collection); err != nil {
			return Item{}, err
		}
		if err := applyLoreReferenceRewrites(rewrites); err != nil {
			collection.Items[i] = previous
			if rollbackErr := s.save(collection); rollbackErr != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[lore-reference] rollback lore item failed id=%s err=%v", id, rollbackErr))
			}
			return Item{}, err
		}
		return updated, nil
	}
	return Item{}, fmt.Errorf("资料不存在: %s", id)
}

func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("资料 ID 不能为空")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

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
	next := make([]Item, 0, len(collection.Items))
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

func (s *Store) ApplyOperations(message string, ops []Operation) (ApplyResult, error) {
	if len(ops) == 0 {
		return ApplyResult{}, errors.New("没有可执行的资料库操作")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return ApplyResult{}, err
	}

	next := append([]Item(nil), collection.Items...)
	result := ApplyResult{Message: strings.TrimSpace(message)}
	for _, op := range ops {
		switch strings.TrimSpace(op.Op) {
		case "create":
			now := time.Now().UTC().Format(time.RFC3339Nano)
			item := normalizeLoreItem(Item{
				ID:               op.Item.ID,
				Enabled:          loreInputEnabled(op.Item.Enabled, true),
				Type:             op.Item.Type,
				TypeSource:       firstNonEmptyLoreValue(op.Item.TypeSource, TypeSourceManual),
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
				return ApplyResult{}, errors.New("创建资料时名称不能为空")
			}
			if loreItemNameIndex(next, item.Name, "") >= 0 {
				return ApplyResult{}, fmt.Errorf("资料名称已存在: %s", item.Name)
			}
			if item.ID == "" {
				item.ID = newUniqueLoreID(next, item.Name, item.Type)
			}
			if loreItemIndex(next, item.ID) >= 0 {
				return ApplyResult{}, fmt.Errorf("资料 ID 已存在: %s", item.ID)
			}
			next = append(next, item)
			result.Created = append(result.Created, item)
		case "update":
			id := normalizeLoreID(firstNonEmptyLoreValue(op.ID, op.Item.ID))
			if id == "" {
				return ApplyResult{}, errors.New("更新资料时 ID 不能为空")
			}
			idx := loreItemIndex(next, id)
			if idx < 0 {
				return ApplyResult{}, fmt.Errorf("资料不存在: %s", id)
			}
			typeName := firstNonEmptyLoreValue(op.Item.Type, next[idx].Type)
			typeSource := next[idx].TypeSource
			if NormalizeType(typeName) != next[idx].Type {
				typeSource = TypeSourceManual
			}
			updated := normalizeLoreItem(Item{
				ID:               id,
				Enabled:          loreInputEnabled(op.Item.Enabled, next[idx].Enabled),
				Type:             typeName,
				TypeSource:       typeSource,
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
				return ApplyResult{}, fmt.Errorf("资料名称不能为空: %s", id)
			}
			if loreItemNameIndex(next, updated.Name, id) >= 0 {
				return ApplyResult{}, fmt.Errorf("资料名称已存在: %s", updated.Name)
			}
			next[idx] = updated
			result.Updated = append(result.Updated, updated)
		case "delete":
			id := normalizeLoreID(firstNonEmptyLoreValue(op.ID, op.Item.ID))
			if id == "" {
				return ApplyResult{}, errors.New("删除资料时 ID 不能为空")
			}
			idx := loreItemIndex(next, id)
			if idx < 0 {
				return ApplyResult{}, fmt.Errorf("资料不存在: %s", id)
			}
			next = append(next[:idx], next[idx+1:]...)
			result.DeletedIDs = append(result.DeletedIDs, id)
		default:
			return ApplyResult{}, fmt.Errorf("不支持的资料库操作: %s", op.Op)
		}
	}
	collection.Items = next
	if err := s.save(collection); err != nil {
		return ApplyResult{}, err
	}
	result.Items, err = s.List()
	if err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}
