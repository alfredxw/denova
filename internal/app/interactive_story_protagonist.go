package app

import (
	"context"
	"fmt"
	"strings"

	booklore "denova/internal/book/lore"
	"denova/internal/interactive"
)

// resolveStoryProtagonist turns a Lore selection into a story-owned snapshot.
// The story never depends on the Lore item after this boundary.
func (s *InteractiveAppService) resolveStoryProtagonist(ctx context.Context, selection interactive.StoryProtagonist) (interactive.StoryProtagonist, error) {
	mode := strings.ToLower(strings.TrimSpace(selection.Mode))
	if mode != interactive.StoryProtagonistModeDefault && mode != interactive.StoryProtagonistModeLore {
		return selection, nil
	}
	if s == nil || s.app == nil {
		return interactive.StoryProtagonist{}, ErrNoWorkspace
	}
	s.app.mu.RLock()
	workspace := strings.TrimSpace(s.app.workspace)
	s.app.mu.RUnlock()
	if workspace == "" {
		return interactive.StoryProtagonist{}, ErrNoWorkspace
	}
	if err := ctx.Err(); err != nil {
		return interactive.StoryProtagonist{}, err
	}
	loreStore := booklore.NewStore(workspace)
	if mode == interactive.StoryProtagonistModeDefault {
		items, err := loreStore.List()
		if err != nil {
			return interactive.StoryProtagonist{}, fmt.Errorf("读取资料库角色失败 / Failed to read Lore characters: %w", err)
		}
		item, ok := defaultTaggedLoreProtagonist(items)
		if !ok {
			return interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeDefault}, nil
		}
		return storyProtagonistSnapshotFromLoreItem(item)
	}

	sourceID := strings.TrimSpace(selection.SourceLoreItemID)
	if sourceID == "" {
		return interactive.StoryProtagonist{}, fmt.Errorf("请选择资料库角色 / Select a Lore character")
	}
	item, err := loreStore.ReadAny(sourceID)
	if err != nil {
		return interactive.StoryProtagonist{}, fmt.Errorf("读取资料库角色失败 / Failed to read Lore character: %w", err)
	}
	return storyProtagonistSnapshotFromLoreItem(item)
}

// defaultTaggedLoreProtagonist returns only the default recommendation. The
// complete enabled character list remains eligible for explicit selection.
func defaultTaggedLoreProtagonist(items []booklore.Item) (booklore.Item, bool) {
	for _, item := range items {
		if !item.Enabled || strings.TrimSpace(item.Type) != "character" {
			continue
		}
		for _, tag := range item.Tags {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag == "主角" || tag == "protagonist" {
				return item, true
			}
		}
	}
	return booklore.Item{}, false
}

func storyProtagonistSnapshotFromLoreItem(item booklore.Item) (interactive.StoryProtagonist, error) {
	if !item.Enabled || strings.TrimSpace(item.Type) != "character" {
		return interactive.StoryProtagonist{}, fmt.Errorf("所选资料必须是已启用的角色 / Selected Lore must be an enabled character")
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return interactive.StoryProtagonist{}, fmt.Errorf("资料库角色名称不能为空 / Lore character name is required")
	}
	profile := strings.TrimSpace(item.Content)
	if profile == "" {
		profile = strings.TrimSpace(item.BriefDescription)
	}
	return interactive.StoryProtagonist{
		Mode:                interactive.StoryProtagonistModeLore,
		Name:                name,
		Profile:             profile,
		SourceLoreItemID:    strings.TrimSpace(item.ID),
		SourceLoreUpdatedAt: strings.TrimSpace(item.UpdatedAt),
	}, nil
}
