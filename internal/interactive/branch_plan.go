package interactive

import (
	"fmt"
	"strings"
)

const (
	StoryPlanningModeEnabled  = "enabled"
	StoryPlanningModeDisabled = "disabled"

	TurnPlanUpdateModeReplaceDocument = "replace_document"
	TurnPlanUpdateModeReplaceSections = "replace_sections"

	maxBranchPlanBytes = 64 * 1024
	// StoryContextMaxBytes is the common ceiling for one bounded game-context
	// fragment. Individual sources may define a smaller limit.
	StoryContextMaxBytes = 256 * 1024
)

// BranchPlan is the Game Agent's current future-facing intent for one branch.
// Persistence always stores one complete Markdown document. The model-facing
// turn protocol may compose that document from independently retryable H2
// section replacements before the Turn is committed.
type BranchPlan struct {
	Markdown      string `json:"markdown"`
	UpdatedTurnID string `json:"updated_turn_id"`
	UpdatedAt     string `json:"updated_at"`
}

// TurnPlanUpdateInput is the model-facing branch-plan mutation. A document
// replacement initializes or substantially restructures a plan; section
// replacements are routine edits whose headings are bound to the current plan.
type TurnPlanUpdateInput struct {
	Mode     string                  `json:"mode" jsonschema_description:"Use replace_document to initialize or substantially restructure the plan. Use replace_sections for routine edits to existing unique H2 sections."`
	Markdown string                  `json:"markdown,omitempty" jsonschema_description:"Complete non-empty branch-plan Markdown. Required only for replace_document."`
	Sections []TurnPlanSectionUpdate `json:"sections,omitempty" jsonschema_description:"Existing H2 section bodies to replace independently. Required only for replace_sections."`
}

// TurnPlanSectionUpdate replaces only the body beneath one existing unique H2
// heading. The heading itself remains backend-owned so routine edits cannot
// silently rename, add, remove, or reorder planning modules.
type TurnPlanSectionUpdate struct {
	Heading     string `json:"heading" jsonschema_description:"Exact visible text of one existing unique H2 heading, without the leading ##."`
	Markdown    string `json:"markdown" jsonschema_description:"Complete non-empty replacement body for this section. H1 and H2 headings are not allowed; H3 and deeper headings are allowed."`
	sourceIndex *int
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
		return fmt.Errorf("branch plan Markdown must be non-empty")
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
