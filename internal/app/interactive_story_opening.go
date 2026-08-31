package app

import (
	"fmt"
	"strings"

	"denova/internal/interactive"
)

// InteractiveStoryOpeningInstruction validates that the selected branch has
// not begun and derives its model-only opening instruction from durable meta.
func (a *App) InteractiveStoryOpeningInstruction(storyID, branchID string) (string, error) {
	return a.interactiveService().InteractiveStoryOpeningInstruction(storyID, branchID)
}

func (s *InteractiveAppService) InteractiveStoryOpeningInstruction(storyID, branchID string) (string, error) {
	store := s.store()
	if store == nil {
		return "", ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return "", err
	}
	if storyContext.Snapshot.TurnCount > 0 {
		return "", fmt.Errorf("故事已经开始 / Story has already started")
	}
	return interactive.StoryOpeningInstruction(storyContext.Meta)
}
