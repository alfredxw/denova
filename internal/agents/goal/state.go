// Package goal owns the durable, conversation-scoped objective state machine.
// It deliberately knows nothing about HTTP, UI, or a concrete Session store.
package goal

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	// MaxObjectiveBytes is intentionally high because the complete user-owned
	// objective is injected without truncation on every active Goal turn.
	MaxObjectiveBytes = 64 * 1024

	StatusActive    Status = "active"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusBlocked   Status = "blocked"
	StatusCleared   Status = "cleared"
)

var (
	ErrNotFound         = errors.New("conversation goal does not exist")
	ErrRevisionConflict = errors.New("conversation goal revision conflict")
	ErrNotActive        = errors.New("conversation goal is not active")
)

// State is the complete durable state for one conversation goal. ActiveSince
// plus ActiveDurationMillis lets clients render elapsed active time without
// journal writes from a timer.
type State struct {
	ID                   string     `json:"id"`
	Objective            string     `json:"objective,omitempty"`
	Status               Status     `json:"status"`
	Revision             uint64     `json:"revision"`
	Report               string     `json:"report,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ActiveSince          *time.Time `json:"active_since,omitempty"`
	ActiveDurationMillis int64      `json:"active_duration_millis,omitempty"`
}

func (state State) Visible() bool { return state.Status != "" && state.Status != StatusCleared }

func (state State) IsActive() bool { return state.Status == StatusActive }

// ElapsedMillis returns accumulated active time without mutating state.
func (state State) ElapsedMillis(now time.Time) int64 {
	elapsed := state.ActiveDurationMillis
	if state.Status == StatusActive && state.ActiveSince != nil {
		elapsed += max(0, now.UTC().Sub(state.ActiveSince.UTC()).Milliseconds())
	}
	return elapsed
}

func New(objective string, now time.Time) (State, error) {
	objective, err := validateObjective(objective)
	if err != nil {
		return State{}, err
	}
	now = now.UTC()
	return State{
		ID: uuid.NewString(), Objective: objective, Status: StatusActive, Revision: 1,
		CreatedAt: now, UpdatedAt: now, ActiveSince: &now,
	}, nil
}

// Set creates a goal or edits the existing goal in place. Editing always
// reactivates it and clears the previous terminal report.
func Set(current State, objective string, expectedRevision uint64, now time.Time) (State, error) {
	if !current.Visible() {
		if expectedRevision != 0 {
			return State{}, fmt.Errorf("%w: have=0 want=%d", ErrRevisionConflict, expectedRevision)
		}
		next, err := New(objective, now)
		if err != nil {
			return State{}, err
		}
		// A cleared goal leaves a revision tombstone. Keep the Session-wide
		// revision monotonic while assigning the replacement a fresh identity.
		if current.Revision > 0 {
			next.Revision = current.Revision + 1
		}
		return next, nil
	}
	if expectedRevision == 0 || current.Revision != expectedRevision {
		return State{}, fmt.Errorf("%w: have=%d want=%d", ErrRevisionConflict, current.Revision, expectedRevision)
	}
	objective, err := validateObjective(objective)
	if err != nil {
		return State{}, err
	}
	now = now.UTC()
	next := stopActiveClock(current, now)
	next.Objective = objective
	next.Status = StatusActive
	next.Report = ""
	next.Revision++
	next.UpdatedAt = now
	next.ActiveSince = &now
	return next, nil
}

func validateObjective(objective string) (string, error) {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return "", errors.New("goal objective is required")
	}
	if len(objective) > MaxObjectiveBytes {
		return "", fmt.Errorf("goal objective exceeds the %d-byte limit", MaxObjectiveBytes)
	}
	return objective, nil
}

func Pause(current State, expectedRevision uint64, now time.Time) (State, error) {
	if err := validateTransition(current, expectedRevision); err != nil {
		return State{}, err
	}
	if current.Status != StatusActive {
		return State{}, ErrNotActive
	}
	now = now.UTC()
	next := stopActiveClock(current, now)
	next.Status = StatusPaused
	next.Revision++
	next.UpdatedAt = now
	return next, nil
}

func Resume(current State, expectedRevision uint64, now time.Time) (State, error) {
	if err := validateTransition(current, expectedRevision); err != nil {
		return State{}, err
	}
	if current.Status != StatusPaused && current.Status != StatusBlocked {
		return State{}, fmt.Errorf("goal cannot resume from status %q", current.Status)
	}
	now = now.UTC()
	next := current
	next.Status = StatusActive
	next.Report = ""
	next.Revision++
	next.UpdatedAt = now
	next.ActiveSince = &now
	return next, nil
}

func Finish(current State, expectedID string, expectedRevision uint64, outcome Status, report string, now time.Time) (State, error) {
	if current.ID != strings.TrimSpace(expectedID) {
		return State{}, fmt.Errorf("%w: goal identity changed", ErrRevisionConflict)
	}
	if err := validateTransition(current, expectedRevision); err != nil {
		return State{}, err
	}
	if current.Status != StatusActive {
		return State{}, ErrNotActive
	}
	if outcome != StatusCompleted && outcome != StatusBlocked {
		return State{}, fmt.Errorf("unsupported goal outcome %q", outcome)
	}
	now = now.UTC()
	next := stopActiveClock(current, now)
	next.Status = outcome
	next.Report = strings.TrimSpace(report)
	next.Revision++
	next.UpdatedAt = now
	return next, nil
}

func Clear(current State, expectedRevision uint64, now time.Time) (State, error) {
	if err := validateTransition(current, expectedRevision); err != nil {
		return State{}, err
	}
	now = now.UTC()
	next := stopActiveClock(current, now)
	next.Objective = ""
	next.Status = StatusCleared
	next.Report = ""
	next.Revision++
	next.UpdatedAt = now
	return next, nil
}

func validateTransition(current State, expectedRevision uint64) error {
	if !current.Visible() {
		return ErrNotFound
	}
	if expectedRevision == 0 || current.Revision != expectedRevision {
		return fmt.Errorf("%w: have=%d want=%d", ErrRevisionConflict, current.Revision, expectedRevision)
	}
	return nil
}

func stopActiveClock(current State, now time.Time) State {
	next := current
	if current.Status == StatusActive && current.ActiveSince != nil {
		next.ActiveDurationMillis += max(0, now.UTC().Sub(current.ActiveSince.UTC()).Milliseconds())
	}
	next.ActiveSince = nil
	return next
}
