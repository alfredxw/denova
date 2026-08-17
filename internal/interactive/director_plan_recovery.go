package interactive

import (
	"errors"
	"fmt"

	"denova/internal/interactive/director"
)

var errDirectorRuntimeInterrupted = errors.New("Director task was interrupted before the workspace runtime started; the existing plan was preserved")

// RecoverInterruptedDirectorRuns reconciles persisted Director state before a
// workspace runtime starts. At that boundary no in-memory task can own a
// running status, so pending work from the previous process is failed while
// existing plan documents and their last usable start-ready state are kept.
func (s *Store) RecoverInterruptedDirectorRuns() (int, error) {
	if s == nil {
		return 0, nil
	}
	index, err := s.Index()
	if err != nil {
		return 0, fmt.Errorf("read interactive story index for Director recovery: %w", err)
	}
	recovered := 0
	failures := make([]error, 0)
	for _, story := range index.Stories {
		branches, branchErr := s.Branches(story.ID)
		if branchErr != nil {
			failures = append(failures, fmt.Errorf("list Director recovery branches story_id=%s: %w", story.ID, branchErr))
			continue
		}
		for _, branch := range branches {
			status, statusErr := s.DirectorPlanStatus(story.ID, branch.ID)
			if statusErr != nil {
				failures = append(failures, fmt.Errorf("read Director recovery status story_id=%s branch_id=%s: %w", story.ID, branch.ID, statusErr))
				continue
			}
			sourceTurnID := status.SourceTurnID
			interrupted := status.Status == director.PlanStatusRunning
			if status.Status == director.PlanStatusWaitingOpening {
				snapshot, snapshotErr := s.Snapshot(story.ID, branch.ID)
				if snapshotErr != nil {
					failures = append(failures, fmt.Errorf("read Director recovery snapshot story_id=%s branch_id=%s: %w", story.ID, branch.ID, snapshotErr))
					continue
				}
				interrupted = snapshot.TurnCount > 0
				if sourceTurnID == "" && snapshot.CurrentTurn != nil {
					sourceTurnID = snapshot.CurrentTurn.ID
				}
			}
			if !interrupted {
				continue
			}
			if markErr := s.MarkDirectorPlanRunFailed(story.ID, branch.ID, sourceTurnID, errDirectorRuntimeInterrupted); markErr != nil {
				failures = append(failures, fmt.Errorf("recover interrupted Director run story_id=%s branch_id=%s: %w", story.ID, branch.ID, markErr))
				continue
			}
			recovered++
		}
	}
	return recovered, errors.Join(failures...)
}
