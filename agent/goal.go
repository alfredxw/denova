package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	// Data is manager-owned durable state. Agent treats it as opaque JSON while
	// retaining the stable ID, status, revision, and mutation fence required by
	// the shared lifecycle. Custom Goal Managers may use their own schema here;
	// the built-in standard Manager leaves it empty.
	Data json.RawMessage `json:"data,omitempty"`
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
	// Data is an opaque command payload interpreted only by the selected Goal
	// Manager. Custom mutation kinds therefore do not need fields added to the
	// Agent package.
	Data json.RawMessage `json:"data,omitempty"`
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
	Context []ContextFragment
	Tools   []ToolDefinition
}

type GoalAfterRunRequest struct {
	Session SessionView
	Run     RunView
	// Input is the exact accepted input for the completed cycle. Dynamic hosts
	// use it to derive valid product metadata for an autonomous continuation
	// without copying the previous command's opaque HostData.
	Input   Input
	State   GoalState
	Present bool
	Result  Result
	// ModelRequest is the exact final provider request that produced Final. A
	// Goal Manager may append a bounded evaluation suffix to this immutable
	// snapshot without rebuilding model, provider, tool, option, or cache state.
	ModelRequest *ModelRequestSnapshot
	// Final is the canonical assistant result committed for this cycle. It is
	// not part of ModelRequest because that snapshot was captured immediately
	// before the provider generated it.
	Final *Message
}

type GoalVerdict string

const (
	GoalVerdictContinue GoalVerdict = "continue"
	GoalVerdictComplete GoalVerdict = "complete"
	GoalVerdictBlocked  GoalVerdict = "blocked"
)

// GoalAfterRunDecision is a manager's bounded judgment about one completed
// cycle. Agent owns durable state transitions and revision fences; Managers
// only evaluate evidence and, for Continue, prepare the next model input.
type GoalAfterRunDecision struct {
	Verdict GoalVerdict
	// Reason is the evaluator's concise rationale. Agent does not inject it into
	// the conversation; terminal verdicts may persist it as the Goal report.
	Reason string
	// Input is a new autonomous turn, not a mutation of the completed caller
	// command. Agent owns its durable command identity and ignores any supplied
	// IdempotencyKey.
	Input        Input
	Usage        *TokenUsage
	FinishReason string
}

// GoalManager owns goal transitions, its opaque Data schema, and model-facing
// preparation. Agent owns the small durable lifecycle envelope, CAS, events,
// and execution ordering. GoalStatus and GoalMutationKind are open strings for
// custom Managers; the constants above are the built-in standard protocol.
type GoalManager interface {
	Identity() CapabilityIdentity
	Apply(context.Context, GoalApplyRequest) (GoalState, error)
	Prepare(context.Context, GoalPrepareRequest) (GoalPreparation, error)
	AfterRun(context.Context, GoalAfterRunRequest) (GoalAfterRunDecision, error)
}

func applyGoalMutation(ctx context.Context, manager GoalManager, request GoalApplyRequest) (GoalState, error) {
	if manager == nil {
		return GoalState{}, ErrCapabilityUnsupported
	}
	mutationID := strings.TrimSpace(request.Mutation.MutationID)
	if mutationID == "" {
		return GoalState{}, errors.New("Goal mutation requires a stable mutation identity")
	}
	if len(request.Mutation.Data) != 0 && !json.Valid(request.Mutation.Data) {
		return GoalState{}, errors.New("Goal mutation Data must be valid JSON")
	}
	if request.Present && request.Current.LastMutationID == mutationID {
		return request.Current, nil
	}
	next, err := manager.Apply(ctx, request)
	if err != nil {
		return GoalState{}, err
	}
	wantRevision := uint64(1)
	if request.Present {
		wantRevision = request.Current.Revision + 1
	}
	if strings.TrimSpace(next.ID) == "" || strings.TrimSpace(string(next.Status)) == "" || next.Revision != wantRevision {
		return GoalState{}, fmt.Errorf(
			"Goal Manager returned an invalid lifecycle envelope: id=%q status=%q revision=%d want_revision=%d",
			next.ID, next.Status, next.Revision, wantRevision,
		)
	}
	if len(next.Data) != 0 && !json.Valid(next.Data) {
		return GoalState{}, errors.New("Goal Manager returned invalid JSON Data")
	}
	next.LastMutationID = mutationID
	next.Data = append(json.RawMessage(nil), next.Data...)
	return next, nil
}
