package agent

import (
	"context"
	"time"
)

const goalCapability = "agent.goal"

type GoalStatus string

const (
	GoalActive    GoalStatus = "active"
	GoalPaused    GoalStatus = "paused"
	GoalCompleted GoalStatus = "completed"
	GoalBlocked   GoalStatus = "blocked"
	GoalCleared   GoalStatus = "cleared"
)

type GoalState struct {
	ID                   string     `json:"id"`
	Objective            string     `json:"objective,omitempty"`
	Status               GoalStatus `json:"status"`
	Revision             uint64     `json:"revision"`
	Report               string     `json:"report,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ActiveSince          *time.Time `json:"active_since,omitempty"`
	ActiveDurationMillis int64      `json:"active_duration_millis,omitempty"`
	LastMutationID       string     `json:"last_mutation_id,omitempty"`
}

func (state GoalState) Visible() bool { return state.Status != "" && state.Status != GoalCleared }
func (state GoalState) Active() bool  { return state.Status == GoalActive }

type GoalMutationKind string

const (
	GoalSet      GoalMutationKind = "set"
	GoalPause    GoalMutationKind = "pause"
	GoalResume   GoalMutationKind = "resume"
	GoalComplete GoalMutationKind = "complete"
	GoalBlock    GoalMutationKind = "block"
	GoalClear    GoalMutationKind = "clear"
)

type GoalMutation struct {
	Kind             GoalMutationKind `json:"kind"`
	Objective        string           `json:"objective,omitempty"`
	ExpectedID       string           `json:"expected_id,omitempty"`
	ExpectedRevision uint64           `json:"expected_revision,omitempty"`
	Report           string           `json:"report,omitempty"`
	MutationID       string           `json:"mutation_id,omitempty"`
}

type GoalApplyRequest struct {
	Session  SessionView
	Run      RunView
	Current  GoalState
	Present  bool
	Mutation GoalMutation
}

type GoalPrepareRequest struct {
	Session SessionView
	Run     RunView
	State   GoalState
	Present bool
}

type GoalPreparation struct {
	Context      []ContextFragment
	Tools        []ToolDefinition
	StandardTool bool
}

type GoalAfterRunRequest struct {
	Session SessionView
	Run     RunView
	State   GoalState
	Present bool
	Result  Result
}

type GoalContinuation struct {
	Continue bool
	Prompt   string
}

// GoalManager owns goal transitions and model-facing preparation. Agent owns
// durable CAS, events, and execution ordering.
type GoalManager interface {
	Identity() CapabilityIdentity
	Apply(context.Context, GoalApplyRequest) (GoalState, error)
	Prepare(context.Context, GoalPrepareRequest) (GoalPreparation, error)
	AfterRun(context.Context, GoalAfterRunRequest) (GoalContinuation, error)
}
