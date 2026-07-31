package automation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/localfs"
)

// AcquireRunLease serializes admission of one deterministic run identity
// across processes. It is held only through runtime receipt persistence; the
// Agent operation itself continues independently in its durable runtime lane.
func (s *Store) AcquireRunLease(ctx context.Context, taskID, runID string) (func() error, error) {
	taskID = strings.TrimSpace(taskID)
	runID = strings.TrimSpace(runID)
	if taskID == "" || runID == "" {
		return nil, fmt.Errorf("task id and deterministic run id are required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return nil, err
		}
		unlock := storePathLocks.Lock(path)
		tasks, readErr := location.store.readScope(location.scope)
		found := false
		if readErr == nil {
			for _, task := range tasks {
				if taskMatchesID(task, taskID) {
					found = true
					break
				}
			}
		}
		unlock()
		if readErr != nil {
			return nil, readErr
		}
		if found {
			return localfs.AcquireLease(ctx, filepath.Join(filepath.Dir(path), ".run-leases", deterministicTriggerHash(runID)+".lock"))
		}
	}
	return nil, fmt.Errorf("automation task %s not found", taskID)
}

func acquireTaskStoreLease(ctx context.Context, path string) (func() error, error) {
	return localfs.AcquireLease(ctx, path+".lock")
}

func withTaskStoreWriteLease[T any](ctx context.Context, path string, operation func() (T, error)) (result T, err error) {
	unlock := storePathLocks.Lock(path)
	release, err := acquireTaskStoreLease(ctx, path)
	if err != nil {
		unlock()
		return result, err
	}
	defer func() {
		err = errors.Join(err, release())
		unlock()
	}()
	return operation()
}
