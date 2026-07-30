package automation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ListInScope returns only the user catalog or the Store's exact active
// workspace catalog. It never falls back to another known workspace.
func (s *Store) ListInScope(scope string) ([]Task, error) {
	location, err := s.exactScopeLocation(scope)
	if err != nil {
		return nil, err
	}
	path, err := location.store.pathForScope(location.scope)
	if err != nil {
		return nil, err
	}
	unlock := storePathLocks.Lock(path)
	tasks, err := location.store.readScope(location.scope)
	unlock()
	if err != nil {
		return nil, err
	}
	visible := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.ArchivedAt == nil {
			visible = append(visible, task)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool {
		return visible[i].UpdatedAt.After(visible[j].UpdatedAt)
	})
	return visible, nil
}

// GetInScope resolves an automation only within the exact requested scope.
func (s *Store) GetInScope(scope, id string) (Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	location, err := s.exactScopeLocation(scope)
	if err != nil {
		return Task{}, err
	}
	path, err := location.store.pathForScope(location.scope)
	if err != nil {
		return Task{}, err
	}
	unlock := storePathLocks.Lock(path)
	defer unlock()
	tasks, err := location.store.readScope(location.scope)
	if err != nil {
		return Task{}, err
	}
	for _, task := range tasks {
		if taskMatchesID(task, id) {
			return task, nil
		}
	}
	return Task{}, scopedTaskNotFound(id, location.scope)
}

// UpdateInScopeIfRevision atomically checks and updates one definition in the
// exact requested scope. The revision check and replacement share one
// cross-process store lease.
func (s *Store) UpdateInScopeIfRevision(scope, id string, patch Task, expectedRevision string) (Task, error) {
	id = strings.TrimSpace(id)
	expectedRevision = strings.TrimSpace(expectedRevision)
	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	if expectedRevision == "" {
		return Task{}, fmt.Errorf("expected automation revision is required")
	}
	location, err := s.exactScopeLocation(scope)
	if err != nil {
		return Task{}, err
	}
	path, err := location.store.pathForScope(location.scope)
	if err != nil {
		return Task{}, err
	}
	patch = definitionOnlyPatch(patch)
	return withTaskStoreWriteLease(context.Background(), path, func() (Task, error) {
		tasks, readErr := location.store.readScope(location.scope)
		if readErr != nil {
			return Task{}, readErr
		}
		for index := range tasks {
			if !taskMatchesID(tasks[index], id) {
				continue
			}
			if tasks[index].ArchivedAt != nil {
				return Task{}, fmt.Errorf("%w: task_id=%s", ErrTaskArchived, id)
			}
			if tasks[index].Revision != expectedRevision {
				return Task{}, &RevisionConflictError{TaskID: id, Expected: expectedRevision, Actual: tasks[index].Revision}
			}
			next := mergeTaskPatch(tasks[index], patch)
			next.Scope = tasks[index].Scope
			next.Target = tasks[index].Target
			next.UpdatedAt = time.Now().UTC()
			normalized, normalizeErr := location.store.normalizeTaskTarget(next)
			if normalizeErr != nil {
				return Task{}, normalizeErr
			}
			tasks[index] = normalized
			if writeErr := location.store.writeScope(location.scope, tasks); writeErr != nil {
				return Task{}, writeErr
			}
			return normalized, nil
		}
		return Task{}, scopedTaskNotFound(id, location.scope)
	})
}

// DeleteInScopeIfRevision atomically checks the latest definition revision and
// archives one task in the exact requested scope.
func (s *Store) DeleteInScopeIfRevision(scope, id, expectedRevision string) error {
	id = strings.TrimSpace(id)
	expectedRevision = strings.TrimSpace(expectedRevision)
	if id == "" {
		return fmt.Errorf("task id is required")
	}
	if expectedRevision == "" {
		return fmt.Errorf("expected automation revision is required")
	}
	location, err := s.exactScopeLocation(scope)
	if err != nil {
		return err
	}
	path, err := location.store.pathForScope(location.scope)
	if err != nil {
		return err
	}
	_, err = withTaskStoreWriteLease(context.Background(), path, func() (bool, error) {
		tasks, readErr := location.store.readScope(location.scope)
		if readErr != nil {
			return false, readErr
		}
		for index := range tasks {
			if !taskMatchesID(tasks[index], id) {
				continue
			}
			if tasks[index].Revision != expectedRevision {
				return false, &RevisionConflictError{TaskID: id, Expected: expectedRevision, Actual: tasks[index].Revision}
			}
			if tasks[index].ArchivedAt != nil {
				return true, nil
			}
			entries, listErr := location.store.readDurableRunObligations(location.scope)
			if listErr != nil {
				return false, listErr
			}
			for _, entry := range entries {
				if durableRunMatchesTask(entry, tasks[index]) && RunHasRuntimeObligation(entry.Run) {
					return false, fmt.Errorf("%w: task_id=%s run_id=%s", ErrTaskHasActiveRun, tasks[index].CatalogID, entry.Run.ID)
				}
			}
			for _, run := range tasks[index].RecentRuns {
				if RunHasRuntimeObligation(run) {
					return false, fmt.Errorf("%w: task_id=%s run_id=%s", ErrTaskHasActiveRun, tasks[index].CatalogID, run.ID)
				}
			}
			now := time.Now().UTC()
			tasks[index].Enabled = false
			tasks[index].ArchivedAt = &now
			tasks[index].UpdatedAt = now
			normalized, normalizeErr := location.store.normalizeTaskTarget(tasks[index])
			if normalizeErr != nil {
				return false, normalizeErr
			}
			tasks[index] = normalized
			return true, location.store.writeScope(location.scope, tasks)
		}
		return false, scopedTaskNotFound(id, location.scope)
	})
	return err
}

func (s *Store) exactScopeLocation(scope string) (taskStoreLocation, error) {
	if s == nil {
		return taskStoreLocation{}, fmt.Errorf("automation store is nil")
	}
	switch strings.TrimSpace(scope) {
	case ScopeUser:
		return taskStoreLocation{store: NewStore(s.userDir, ""), scope: ScopeUser}, nil
	case ScopeWorkspace:
		workspace := canonicalStoreRoot(s.workspace)
		if workspace == "" {
			return taskStoreLocation{}, fmt.Errorf("workspace is required for workspace-scoped automation")
		}
		return taskStoreLocation{store: s.storeForWorkspace(workspace), scope: ScopeWorkspace}, nil
	default:
		return taskStoreLocation{}, fmt.Errorf("automation scope must be %q or %q", ScopeUser, ScopeWorkspace)
	}
}

func scopedTaskNotFound(id, scope string) error {
	return fmt.Errorf("automation task %s not found in %s scope", id, scope)
}
