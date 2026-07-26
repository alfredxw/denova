package tools

import (
	"context"

	"denova/internal/interactive"
)

// InteractiveContext is the concrete tools' story-scoped dependency set.
// Agent lifecycle, conversation, and durable runtime collaborators are
// intentionally excluded; the agents package owns those concerns.
type InteractiveContext struct {
	Store                     *interactive.Store
	StoryID                   string
	BranchID                  string
	TurnID                    string
	MaintenanceTask           string
	OnLoreItemsRead           func([]string)
	SubmitStateSchemaBatch    func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error)
	SubmitDirectorPlanUpdate  func(context.Context, interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error)
	RequestDirectorCompletion func(context.Context) bool
	RequestTurnCompletion     func(context.Context) bool
	PrepareTurn               func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error)
	SubmitTurnResult          func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)
}
