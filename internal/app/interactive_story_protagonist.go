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
	if mode != interactive.StoryProtagonistModeLore {
		return selection, nil
	}
	sourceID := strings.TrimSpace(selection.SourceLoreItemID)
	if sourceID == "" {
		return interactive.StoryProtagonist{}, fmt.Errorf("请选择资料库角色 / Select a Lore character")
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
	item, err := booklore.NewStore(workspace).ReadAny(sourceID)
	if err != nil {
		return interactive.StoryProtagonist{}, fmt.Errorf("读取资料库角色失败 / Failed to read Lore character: %w", err)
	}
	return storyProtagonistSnapshotFromLoreItem(item)
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
