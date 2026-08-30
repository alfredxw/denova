package interactive

import (
	"fmt"
	"strings"
)

const (
	StoryPlanningModeEnabled  = "enabled"
	StoryPlanningModeDisabled = "disabled"

	maxBranchPlanBytes = 64 * 1024
	// StoryContextMaxBytes is the common ceiling for one bounded game-context
	// fragment. Individual sources may define a smaller limit.
	StoryContextMaxBytes = 256 * 1024
)

// BranchPlan is the Game Agent's current future-facing intent for one branch.
// It is deliberately free-form: planning style and pacing belong to the
// selected game preset, Skills, and user instructions rather than this schema.
type BranchPlan struct {
	Markdown      string `json:"markdown"`
	UpdatedTurnID string `json:"updated_turn_id"`
	UpdatedAt     string `json:"updated_at"`
}

// BranchPlanUpdatedEvent persists a complete replacement beside the Turn that
// produced it. It is not part of the public Turn result or historical chat.
type BranchPlanUpdatedEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	TurnID   string `json:"turn_id"`
	Markdown string `json:"markdown"`
}

func normalizeStoryPlanningMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case StoryPlanningModeEnabled:
		return StoryPlanningModeEnabled
	case StoryPlanningModeDisabled:
		return StoryPlanningModeDisabled
	default:
		return StoryPlanningModeDisabled
	}
}

func validateStoryPlanningMode(mode string) error {
	switch mode {
	case StoryPlanningModeEnabled, StoryPlanningModeDisabled:
		return nil
	default:
		return fmt.Errorf("invalid story planning mode: %q", mode)
	}
}

func normalizeBranchPlanMarkdown(markdown string) string {
	return strings.TrimSpace(markdown)
}

func validateBranchPlanMarkdown(markdown string) error {
	markdown = normalizeBranchPlanMarkdown(markdown)
	if markdown == "" {
		return fmt.Errorf("plan_update must be a non-empty full replacement")
	}
	if len([]byte(markdown)) > maxBranchPlanBytes {
		return fmt.Errorf("plan_update exceeds %d bytes", maxBranchPlanBytes)
	}
	return nil
}

func cloneBranchPlan(plan *BranchPlan) *BranchPlan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	return &cloned
}
