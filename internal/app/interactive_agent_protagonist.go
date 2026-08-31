package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	booklore "denova/internal/book/lore"
	"denova/internal/interactive"
)

// selectStoryProtagonist is the opening-run mutation boundary. It accepts any
// enabled character Lore entry and freezes it without changing the stable
// runtime Actor ID.
func (c *interactiveAgentCycle) selectStoryProtagonist(ctx context.Context, loreItemID string) (interactive.StoryProtagonist, error) {
	if c == nil || c.store == nil {
		return interactive.StoryProtagonist{}, fmt.Errorf("the interactive story runtime is unavailable")
	}
	loreItemID = strings.TrimSpace(loreItemID)
	storyContext, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-agent] load story before protagonist selection failed story_id=%s branch_id=%s err=%v", c.storyID, c.branchID, err))
		return interactive.StoryProtagonist{}, fmt.Errorf("the story context is unavailable; retry the same protagonist selection")
	}
	if storyContext.Snapshot.TurnCount > 0 {
		return interactive.StoryProtagonist{}, fmt.Errorf("the protagonist cannot be selected after the first turn")
	}
	if storyContext.Meta.Protagonist.Mode != interactive.StoryProtagonistModeDefault {
		if storyContext.Meta.Protagonist.Mode == interactive.StoryProtagonistModeLore && storyContext.Meta.Protagonist.SourceLoreItemID == loreItemID {
			if branch, ok := storyContext.Meta.Branches[storyContext.Snapshot.BranchID]; ok && c.conversation != nil {
				c.conversation.WithBaseParentID(branch.Head)
			}
			return storyContext.Meta.Protagonist, nil
		}
		return interactive.StoryProtagonist{}, fmt.Errorf("the story protagonist is already resolved")
	}
	if err := ctx.Err(); err != nil {
		return interactive.StoryProtagonist{}, err
	}
	item, err := booklore.NewStore(c.workspace).ReadAny(loreItemID)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-agent] read protagonist Lore item failed story_id=%s lore_item_id=%s err=%v", c.storyID, loreItemID, err))
		return interactive.StoryProtagonist{}, fmt.Errorf("the selected Lore character is unavailable; choose an exact ID from the provided catalog")
	}
	selected, err := storyProtagonistSnapshotFromLoreItem(item)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-agent] validate protagonist Lore item failed story_id=%s lore_item_id=%s err=%v", c.storyID, loreItemID, err))
		return interactive.StoryProtagonist{}, fmt.Errorf("the selected Lore item must be an enabled character with a non-empty name")
	}
	updated, err := c.store.UpdateStory(c.storyID, interactive.UpdateStoryRequest{Protagonist: &selected})
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-agent] freeze protagonist snapshot failed story_id=%s lore_item_id=%s err=%v", c.storyID, loreItemID, err))
		return interactive.StoryProtagonist{}, fmt.Errorf("the protagonist snapshot could not be frozen; retry the same lore_item_id")
	}
	refreshed, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-agent] refresh story after protagonist selection failed story_id=%s branch_id=%s err=%v", c.storyID, c.branchID, err))
		return interactive.StoryProtagonist{}, fmt.Errorf("the protagonist was selected but the story context could not refresh; retry the same lore_item_id")
	}
	if branch, ok := refreshed.Meta.Branches[refreshed.Snapshot.BranchID]; ok && c.conversation != nil {
		c.conversation.WithBaseParentID(branch.Head)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent] selected opening protagonist story_id=%s branch_id=%s lore_item_id=%s", c.storyID, c.branchID, selected.SourceLoreItemID))
	return updated.Protagonist, nil
}
