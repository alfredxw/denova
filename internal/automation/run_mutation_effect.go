package automation

import (
	"context"
	"fmt"
	"strings"
)

// MergeRunMutationEffect transfers one exact Runtime host-effect obligation
// into the Automation run outbox. A late effect may explicitly reopen only the
// matching operation's completed post-effects; ordinary AppendRun callers keep
// the stronger monotonic rule and cannot regress a completed receipt.
func (s *Store) MergeRunMutationEffect(
	ctx context.Context,
	runID string,
	operationID string,
	effectID string,
	paths []string,
) (DurableRun, bool, error) {
	runID = strings.TrimSpace(runID)
	operationID = strings.TrimSpace(operationID)
	effectID = strings.TrimSpace(effectID)
	if runID == "" || operationID == "" || effectID == "" {
		return DurableRun{}, false, fmt.Errorf("run, operation, and host effect identities are required")
	}
	task, run, err := s.GetRunByID(runID)
	if err != nil {
		return DurableRun{}, false, err
	}
	if strings.TrimSpace(run.RuntimeOperationID) != operationID {
		return DurableRun{}, false, fmt.Errorf(
			"%w: run_id=%s host effect operation changed from %s to %s",
			ErrRunIdentityConflict,
			runID,
			operationID,
			run.RuntimeOperationID,
		)
	}
	for _, existingID := range run.CompletionMutationEffectIDs {
		if strings.TrimSpace(existingID) == effectID {
			return DurableRun{Task: task, Run: run}, false, nil
		}
	}
	normalizedPaths := mergeRunMutationPaths(nil, paths)
	hasNewPath := !runMutationPathsSubset(normalizedPaths, run.CompletionMutationPaths)
	run.CompletionMutationPaths = mergeRunMutationPaths(run.CompletionMutationPaths, normalizedPaths)
	run.CompletionMutationEffectIDs = mergeRunMutationPaths(run.CompletionMutationEffectIDs, []string{effectID})
	if hasNewPath && isTerminalRunStatus(run.Status) {
		run.CompletionEffectsOperationID = operationID
		run.CompletionEffectsPending = true
		run.CompletionEffectsCompleted = false
	}
	updatedTask, err := s.appendRun(ctx, automationTaskPersistenceID(task), run, true)
	if err != nil {
		return DurableRun{}, false, err
	}
	_, persisted, err := s.GetRunByID(runID)
	if err != nil {
		return DurableRun{}, false, err
	}
	return DurableRun{Task: updatedTask, Run: persisted}, true, nil
}

func automationTaskPersistenceID(task Task) string {
	if id := strings.TrimSpace(task.CatalogID); id != "" {
		return id
	}
	return strings.TrimSpace(task.ID)
}
