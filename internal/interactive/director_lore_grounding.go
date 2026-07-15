package interactive

import (
	"fmt"
	"strings"

	"denova/internal/book"
)

// validateDirectorLoreGrounding prevents a Director run from casting new
// on-demand lore entries based only on a name or index summary. Reviewed IDs
// come from successful model-visible lore body tool results and cannot be
// supplied by the model submission itself.
func (s *Store) validateDirectorLoreGrounding(previousContent, nextContent string, reviewedIDs []string) error {
	previous := ParseDirectorLoreContextReferences(previousContent)
	next := ParseDirectorLoreContextReferences(nextContent)
	previousNames := loreReferenceNameSet(previous.All())
	previousReviewedCast := loreReferenceNameSet(append(append([]string{}, previous.Active...), previous.Candidates...))
	reviewed := map[string]bool{}
	for _, id := range reviewedIDs {
		if id = strings.TrimSpace(id); id != "" {
			reviewed[id] = true
		}
	}
	items, err := book.NewLoreStore(s.root).List()
	if err != nil {
		return fmt.Errorf("读取资料库以校验选角来源失败: %w", err)
	}
	byName := make(map[string]book.LoreItem, len(items))
	for _, item := range items {
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	for _, name := range next.All() {
		key := strings.ToLower(strings.TrimSpace(name))
		if previousNames[key] {
			continue
		}
		if _, ok := byName[key]; !ok {
			return fmt.Errorf("新增资料引用不存在或未启用: %s；请从名称目录选择有效资料", name)
		}
	}
	for _, name := range append(append([]string{}, next.Active...), next.Candidates...) {
		key := strings.ToLower(strings.TrimSpace(name))
		if previousReviewedCast[key] {
			continue
		}
		item, ok := byName[key]
		if !ok {
			continue
		}
		if item.LoadMode != book.LoreLoadModeResident && !reviewed[item.ID] {
			return fmt.Errorf("新增当前/候场资料 %s 尚未读取完整正文；请先用 read_lore_items 或 detail=full 审阅后再引用", name)
		}
	}
	return nil
}

func loreReferenceNameSet(names []string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		if key := strings.ToLower(strings.TrimSpace(name)); key != "" {
			result[key] = true
		}
	}
	return result
}
