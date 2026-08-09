package session

import (
	"context"
	"time"

	"denova/internal/agents/goal"
)

// Goal returns the latest canonical conversation goal. Cleared goals are
// represented as absent to callers while their revision tombstone stays in the
// journal for monotonic compare-and-swap updates.
func (s *Session) Goal(ctx context.Context) (goal.State, bool, error) {
	if s == nil {
		return goal.State{}, false, nil
	}
	if err := s.RefreshCanonical(ctx); err != nil {
		return goal.State{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.goal.Visible() {
		return goal.State{}, false, nil
	}
	return s.goal, true, nil
}

func (s *Session) SetGoal(ctx context.Context, objective string, expectedRevision uint64) (goal.State, error) {
	return s.changeGoal(ctx, "set conversation goal", func(current goal.State, now time.Time) (goal.State, error) {
		return goal.Set(current, objective, expectedRevision, now)
	})
}

func (s *Session) PauseGoal(ctx context.Context, expectedRevision uint64) (goal.State, error) {
	return s.changeGoal(ctx, "pause conversation goal", func(current goal.State, now time.Time) (goal.State, error) {
		return goal.Pause(current, expectedRevision, now)
	})
}

func (s *Session) ResumeGoal(ctx context.Context, expectedRevision uint64) (goal.State, error) {
	return s.changeGoal(ctx, "resume conversation goal", func(current goal.State, now time.Time) (goal.State, error) {
		return goal.Resume(current, expectedRevision, now)
	})
}

func (s *Session) ClearGoal(ctx context.Context, expectedRevision uint64) (goal.State, error) {
	return s.changeGoal(ctx, "clear conversation goal", func(current goal.State, now time.Time) (goal.State, error) {
		return goal.Clear(current, expectedRevision, now)
	})
}

func (s *Session) FinishGoal(ctx context.Context, expectedID string, expectedRevision uint64, outcome goal.Status, report string) (goal.State, error) {
	return s.changeGoal(ctx, "finish conversation goal", func(current goal.State, now time.Time) (goal.State, error) {
		return goal.Finish(current, expectedID, expectedRevision, outcome, report, now)
	})
}

func (s *Session) changeGoal(ctx context.Context, operation string, transition func(goal.State, time.Time) (goal.State, error)) (goal.State, error) {
	var next goal.State
	err := s.withCanonicalMutation(ctx, operation, func() error {
		candidate, err := transition(s.goal, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := s.appendJournalRecordLocked(goalChangedRecord{Type: historyTypeGoalChanged, Goal: candidate}); err != nil {
			return err
		}
		s.goal = candidate
		advanceUpdatedAt(s, candidate.UpdatedAt)
		next = candidate
		return nil
	})
	return next, err
}
