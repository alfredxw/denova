package lore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type TypeChange struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type TypeApplyResult struct {
	Revision string `json:"revision"`
	Items    []Item `json:"items"`
	Updated  []Item `json:"updated"`
}

// AllRevision includes disabled entries and derived type metadata, making it
// suitable for preview/confirm organization flows.
func (s *Store) AllRevision() (string, error) {
	items, err := s.ListAll()
	if err != nil {
		return "", err
	}
	return loreAllRevision(items), nil
}

// ApplyTypeChanges atomically updates only type metadata after verifying the
// preview revision. User confirmation is recorded as manual provenance.
func (s *Store) ApplyTypeChanges(expectedRevision string, changes []TypeChange) (TypeApplyResult, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return TypeApplyResult{}, err
	}
	currentRevision := loreAllRevision(collection.Items)
	if strings.TrimSpace(expectedRevision) == "" || strings.TrimSpace(expectedRevision) != currentRevision {
		return TypeApplyResult{}, ErrRevisionConflict
	}
	if len(changes) == 0 {
		return TypeApplyResult{}, fmt.Errorf("没有选中需要应用的分类")
	}
	byID := map[string]int{}
	for index, item := range collection.Items {
		byID[item.ID] = index
	}
	seen := map[string]bool{}
	updated := make([]Item, 0, len(changes))
	for _, change := range changes {
		id := normalizeLoreID(change.ID)
		if id == "" || seen[id] {
			return TypeApplyResult{}, fmt.Errorf("分类变更包含空或重复 ID: %s", change.ID)
		}
		seen[id] = true
		if !ValidClassificationType(change.Type) {
			return TypeApplyResult{}, fmt.Errorf("资料 %s 的分类无效: %s", id, change.Type)
		}
		index, ok := byID[id]
		if !ok {
			return TypeApplyResult{}, fmt.Errorf("资料不存在: %s", id)
		}
		item := collection.Items[index]
		item.Type = strings.TrimSpace(change.Type)
		item.TypeSource = TypeSourceManual
		item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		item = normalizeLoreItem(item)
		collection.Items[index] = item
		updated = append(updated, item)
	}
	if err := s.save(collection); err != nil {
		return TypeApplyResult{}, err
	}
	items, err := s.ListAll()
	if err != nil {
		return TypeApplyResult{}, err
	}
	return TypeApplyResult{Revision: loreAllRevision(items), Items: items, Updated: updated}, nil
}

func loreAllRevision(items []Item) string {
	normalized := normalizeLoreItems(append([]Item(nil), items...))
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].ID < normalized[j].ID })
	data, _ := json.Marshal(normalized)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12])
}
