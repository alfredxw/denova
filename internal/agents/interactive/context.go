package interactive

import (
	"context"
	agentrun "denova/internal/agents/run"

	story "denova/internal/interactive"
)

// InteractiveStoryToolContext is the application-facing input used to build
// story and director tool surfaces. Runtime-only collaborators stay here in
// the agents package; concrete tools receive a narrower projection so the
// tools package does not depend on agent orchestration internals.
type InteractiveStoryToolContext struct {
	Store *story.Store
	// agentrun.CommandID is the App-derived durable identity for this root Director run.
	// Callers must reuse it only for the same immutable plan token and task.
	CommandID       string
	StoryID         string
	BranchID        string
	TurnID          string
	MaintenanceTask string
	// StableContext is a bounded, source-labelled model prefix kept separate
	// from the changing task instruction so providers can reuse prompt caches.
	StableContextTitle     string
	StableContext          string
	StableContextMaxBytes  int
	OnLoreItemsRead        func([]string)
	SubmitStateSchemaBatch func(
		context.Context,
		story.ActorStateSchemaBatch,
	) (story.ActorStateSchemaBatchResult, error)
	SubmitDirectorPlanUpdate func(
		context.Context,
		story.DirectorPlanUpdateSubmission,
	) (story.DirectorPlanUpdateReceipt, error)
	// DomainCommitParticipant owns the canonical Director result. The App
	// supplies it so the durable harness can authorize and acknowledge the
	// final plan write before settling the model cycle successfully.
	DomainCommitParticipant agentrun.DomainCommitParticipant
	// DisplayConversation receives display-only progress for background helper
	// agents. It must not receive final assistant text as model-visible context.
	DisplayConversation any
	PrepareTurn         func(context.Context, story.TurnCheckRequest) (story.RuleResolution, error)
	SubmitTurnResult    func(context.Context, story.TurnSubmissionInput) (story.TurnSubmissionReceipt, error)
	TurnResultReady     func() bool
}
