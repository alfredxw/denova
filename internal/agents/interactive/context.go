package interactive

import (
	"context"

	story "denova/internal/interactive"
)

// InteractiveStoryToolContext is the application-facing input used to build
// the Game Agent tool surface. Runtime-only collaborators stay here in
// the agents package; concrete tools receive a narrower projection so the
// tools package does not depend on agent orchestration internals.
type InteractiveStoryToolContext struct {
	Store                  *story.Store
	StoryID                string
	BranchID               string
	SubmitStateSchemaBatch func(
		context.Context,
		story.ActorStateSchemaBatch,
	) (story.ActorStateSchemaBatchResult, error)
	PrepareTurn      func(context.Context, story.TurnCheckRequest) (story.RuleResolution, error)
	SubmitTurnResult func(context.Context, story.TurnSubmissionInput) (story.TurnSubmissionReceipt, error)
	TurnResultReady  func() bool
}
