// Package goal provides the standard revisioned Goal capability.
package goal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

const MaxObjectiveBytes = 64 * 1024

var (
	ErrNotFound         = errors.New("Goal does not exist")
	ErrRevisionConflict = errors.New("Goal revision conflict")
	ErrNotActive        = errors.New("Goal is not active")
)

type Clock func() time.Time

type Option func(*standardManager)

func WithClock(clock Clock) Option {
	return func(manager *standardManager) {
		if clock != nil {
			manager.clock = clock
		}
	}
}

// Standard returns the built-in objective state machine and model tool.
func Standard(options ...Option) agent.GoalManager {
	manager := &standardManager{clock: func() time.Time { return time.Now().UTC() }}
	for _, option := range options {
		if option != nil {
			option(manager)
		}
	}
	return manager
}

type standardManager struct{ clock Clock }

func (*standardManager) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "goal.standard", Version: 1}
}

func (manager *standardManager) Apply(_ context.Context, request agent.GoalApplyRequest) (agent.GoalState, error) {
	current := request.Current
	mutation := request.Mutation
	if len(mutation.Data) != 0 {
		return agent.GoalState{}, errors.New("standard Goal does not accept custom mutation data")
	}
	if mutation.MutationID != "" && current.LastMutationID == mutation.MutationID {
		return current, nil
	}
	now := manager.clock().UTC()
	var next agent.GoalState
	var err error
	switch mutation.Kind {
	case agent.GoalSet:
		next, err = set(current, request.Present, mutation, now)
	case agent.GoalPause:
		next, err = transition(current, mutation, now, agent.GoalActive, agent.GoalPaused)
	case agent.GoalResume:
		if current.Status != agent.GoalPaused && current.Status != agent.GoalBlocked {
			err = fmt.Errorf("Goal cannot resume from status %q", current.Status)
			break
		}
		next, err = transition(current, mutation, now, current.Status, agent.GoalActive)
	case agent.GoalComplete:
		next, err = finish(current, mutation, now, agent.GoalCompleted)
	case agent.GoalBlock:
		next, err = finish(current, mutation, now, agent.GoalBlocked)
	case agent.GoalClear:
		next, err = transition(current, mutation, now, current.Status, agent.GoalCleared)
		if err == nil {
			next.Objective, next.Report = "", ""
		}
	default:
		err = fmt.Errorf("unsupported Goal mutation %q", mutation.Kind)
	}
	if err != nil {
		return agent.GoalState{}, err
	}
	next.LastMutationID = mutation.MutationID
	return next, nil
}

func (*standardManager) Prepare(_ context.Context, request agent.GoalPrepareRequest) (agent.GoalPreparation, error) {
	if !request.Present || !request.State.Active() {
		return agent.GoalPreparation{}, nil
	}
	content := fmt.Sprintf(
		"<active_goal id=\"%s\" revision=\"%d\">\n<objective>%s</objective>\n</active_goal>\n\n"+
			"Goal terminal protocol:\n"+
			"- Complete this exact revision only when the entire objective is achieved. An intermediate milestone is never completion.\n"+
			"- Block it only when meaningful progress genuinely requires user input or an external state change.\n"+
			"- Otherwise keep working and do not call the goal tool.",
		html.EscapeString(request.State.ID), request.State.Revision, html.EscapeString(request.State.Objective),
	)
	return agent.GoalPreparation{StandardTool: true, Context: []agent.ContextFragment{{
		Source: "goal.standard", Purpose: "active objective", Resource: "session-goal",
		Revision: fmt.Sprintf("%d", request.State.Revision), Stability: agent.ContextTurn, Placement: agent.ContextFinalUserPrefix,
		Content: content, HardLimit: 128 << 10,
	}}}, nil
}

func (*standardManager) AfterRun(_ context.Context, request agent.GoalAfterRunRequest) (agent.GoalContinuation, error) {
	if !request.Present || !request.State.Active() || request.Result.Status != agent.ResultCompleted {
		return agent.GoalContinuation{}, nil
	}
	return agent.GoalContinuation{
		Continue: true,
		Input:    agent.Input{Text: "Continue working autonomously on the active goal. Reassess the complete objective and current workspace state, make the next meaningful progress, and use the goal tool only when the objective is fully completed or genuinely blocked."},
	}, nil
}

func set(current agent.GoalState, present bool, mutation agent.GoalMutation, now time.Time) (agent.GoalState, error) {
	objective := strings.TrimSpace(mutation.Objective)
	if objective == "" || len(objective) > MaxObjectiveBytes {
		return agent.GoalState{}, fmt.Errorf("Goal objective must contain 1..%d bytes", MaxObjectiveBytes)
	}
	if !present || !current.Visible() {
		if mutation.ExpectedRevision != 0 {
			return agent.GoalState{}, revisionError(current.Revision, mutation.ExpectedRevision)
		}
		revision := uint64(1)
		if current.Revision != 0 {
			revision = current.Revision + 1
		}
		return agent.GoalState{
			ID: newGoalID(), Objective: objective, Status: agent.GoalActive, Revision: revision,
			CreatedAt: now, UpdatedAt: now, ActiveSince: &now,
		}, nil
	}
	if err := validateRevision(current, mutation); err != nil {
		return agent.GoalState{}, err
	}
	next := stopClock(current, now)
	next.Objective, next.Status, next.Report = objective, agent.GoalActive, ""
	next.Revision++
	next.UpdatedAt, next.ActiveSince = now, &now
	return next, nil
}

func transition(current agent.GoalState, mutation agent.GoalMutation, now time.Time, from, to agent.GoalStatus) (agent.GoalState, error) {
	if err := validateRevision(current, mutation); err != nil {
		return agent.GoalState{}, err
	}
	if current.Status != from {
		return agent.GoalState{}, fmt.Errorf("Goal cannot transition from %q to %q", current.Status, to)
	}
	next := stopClock(current, now)
	next.Status, next.UpdatedAt = to, now
	next.Revision++
	if to == agent.GoalActive {
		next.Report, next.ActiveSince = "", &now
	}
	return next, nil
}

func finish(current agent.GoalState, mutation agent.GoalMutation, now time.Time, status agent.GoalStatus) (agent.GoalState, error) {
	if current.Status != agent.GoalActive {
		return agent.GoalState{}, ErrNotActive
	}
	if err := validateRevision(current, mutation); err != nil {
		return agent.GoalState{}, err
	}
	next := stopClock(current, now)
	next.Status, next.Report, next.UpdatedAt = status, strings.TrimSpace(mutation.Report), now
	next.Revision++
	return next, nil
}

func validateRevision(current agent.GoalState, mutation agent.GoalMutation) error {
	if !current.Visible() {
		return ErrNotFound
	}
	if mutation.ExpectedID != "" && mutation.ExpectedID != current.ID {
		return fmt.Errorf("%w: Goal identity changed", ErrRevisionConflict)
	}
	if mutation.ExpectedRevision == 0 || mutation.ExpectedRevision != current.Revision {
		return revisionError(current.Revision, mutation.ExpectedRevision)
	}
	return nil
}

func revisionError(have, want uint64) error {
	return fmt.Errorf("%w: have=%d want=%d", ErrRevisionConflict, have, want)
}

func stopClock(current agent.GoalState, now time.Time) agent.GoalState {
	next := current
	if current.Status == agent.GoalActive && current.ActiveSince != nil {
		next.ActiveDurationMillis += max(0, now.Sub(current.ActiveSince.UTC()).Milliseconds())
	}
	next.ActiveSince = nil
	return next
}

func newGoalID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return "goal-" + hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("goal-fallback-%x", time.Now().UTC().UnixNano())
}

var _ agent.GoalManager = (*standardManager)(nil)
