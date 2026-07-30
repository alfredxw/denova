package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/fsdurability"
)

const (
	durableRunFileVersion   = 1
	durableRunHistoryDir    = "runs"
	durableRunObligationDir = "run-obligations"
)

var ErrRunNotFound = errors.New("automation run not found")

// DurableRun is one recovery-grade run record joined with its owning task.
// Task.RecentRuns is only a bounded UI projection; this ledger is the source
// of truth for accepted runtime work and pending completion effects.
type DurableRun struct {
	Task Task
	Run  RunRecord
}

type durableRunFile struct {
	Version       int       `json:"version"`
	Revision      uint64    `json:"revision,omitempty"`
	TaskCatalogID string    `json:"task_catalog_id"`
	Run           RunRecord `json:"run"`
}

func authoritativeDurableRun(
	obligation durableRunFile,
	obligationFound bool,
	history durableRunFile,
	historyFound bool,
) (durableRunFile, bool) {
	switch {
	case !obligationFound:
		return history, historyFound
	case !historyFound:
		return obligation, true
	case obligation.Revision > history.Revision:
		return obligation, true
	case history.Revision > obligation.Revision:
		return history, true
	case obligation.Revision > 0:
		// Equal revisions are the two copies of one append. Prefer full history;
		// it remains authoritative after obligation cleanup is interrupted.
		return history, true
	}
	// Legacy revision-zero records need a one-way ordering heuristic. A higher
	// Runtime receipt is a successor obligation; for the same operation, a
	// terminal full-history record was written after the pending hot copy.
	if obligation.Run.RuntimeReceiptCursor > history.Run.RuntimeReceiptCursor {
		return obligation, true
	}
	if history.Run.RuntimeReceiptCursor > obligation.Run.RuntimeReceiptCursor ||
		(isTerminalRunStatus(history.Run.Status) && !isTerminalRunStatus(obligation.Run.Status)) {
		return history, true
	}
	return history, true
}

func maxDurableRunRevision(
	obligation durableRunFile,
	obligationFound bool,
	history durableRunFile,
	historyFound bool,
) uint64 {
	var revision uint64
	if obligationFound && obligation.Revision > revision {
		revision = obligation.Revision
	}
	if historyFound && history.Revision > revision {
		revision = history.Revision
	}
	return revision
}

// GetRunByID resolves a single run across the user and workspace scopes this
// store can see. It reads the independent run ledger before falling back to
// legacy RecentRuns records created before the ledger existed.
func (s *Store) GetRunByID(runID string) (Task, RunRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Task{}, RunRecord{}, fmt.Errorf("run_id is required")
	}
	for _, location := range s.taskLocations() {
		taskPath, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, RunRecord{}, err
		}
		unlock := storePathLocks.Lock(taskPath)
		tasks, err := location.store.readScope(location.scope)
		if err != nil {
			unlock()
			return Task{}, RunRecord{}, err
		}
		obligation, obligationFound, err := location.store.readDurableRunObligation(location.scope, runID)
		if err != nil {
			unlock()
			return Task{}, RunRecord{}, err
		}
		history, historyFound, err := location.store.readDurableRun(location.scope, runID)
		if err != nil {
			unlock()
			return Task{}, RunRecord{}, err
		}
		entry, found := authoritativeDurableRun(obligation, obligationFound, history, historyFound)
		if found {
			task, ok := taskForDurableRun(tasks, entry)
			unlock()
			if !ok {
				return Task{}, RunRecord{}, fmt.Errorf("durable automation run %s refers to missing task %s", runID, entry.TaskCatalogID)
			}
			return task, entry.Run, nil
		}
		for _, task := range tasks {
			for _, run := range task.RecentRuns {
				if strings.TrimSpace(run.ID) == runID {
					unlock()
					return task, run, nil
				}
			}
		}
		unlock()
	}
	return Task{}, RunRecord{}, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
}

// ListDurableRuns returns complete durable history. Startup recovery should use
// ListDurableObligations so settled history never enters the hot path.
func (s *Store) ListDurableRuns() ([]DurableRun, error) {
	return s.listDurableRuns(false)
}

// ListDurableObligations scans only the small write-ahead obligation ledger.
// Legacy RecentRuns entries are inspected only as a bounded migration fallback.
func (s *Store) ListDurableObligations() ([]DurableRun, error) {
	return s.listDurableRuns(true)
}

func (s *Store) listDurableRuns(obligationsOnly bool) ([]DurableRun, error) {
	result := make([]DurableRun, 0)
	seen := make(map[string]struct{})
	for _, location := range s.taskLocations() {
		taskPath, err := location.store.pathForScope(location.scope)
		if err != nil {
			return nil, err
		}
		unlock := storePathLocks.Lock(taskPath)
		tasks, err := location.store.readScope(location.scope)
		if err != nil {
			unlock()
			return nil, err
		}
		entries, err := location.store.readDurableRuns(location.scope)
		if obligationsOnly {
			entries, err = location.store.readDurableRunObligations(location.scope)
		}
		if err != nil {
			unlock()
			return nil, err
		}
		for _, entry := range entries {
			if obligationsOnly {
				history, found, historyErr := location.store.readDurableRun(location.scope, entry.Run.ID)
				if historyErr != nil {
					unlock()
					return nil, historyErr
				}
				entry, _ = authoritativeDurableRun(entry, true, history, found)
				if !RunHasDurableObligation(entry.Run) {
					continue
				}
			}
			task, ok := taskForDurableRun(tasks, entry)
			if !ok {
				unlock()
				return nil, fmt.Errorf("durable automation run %s refers to missing task %s", entry.Run.ID, entry.TaskCatalogID)
			}
			key := task.CatalogID + "\x00" + entry.Run.ID
			seen[key] = struct{}{}
			result = append(result, DurableRun{Task: task, Run: entry.Run})
		}
		// One-way migration remains readable before the next mutation has had a
		// chance to backfill the ledger.
		for _, task := range tasks {
			for _, run := range task.RecentRuns {
				candidate := run
				if obligationsOnly {
					// The full ledger is committed before a settled obligation is
					// removed, while the task projection is written last. Resolve a
					// legacy/projection row through that authoritative record so a
					// crash cannot resurrect stale pending UI state.
					history, found, historyErr := location.store.readDurableRun(location.scope, run.ID)
					if historyErr != nil {
						unlock()
						return nil, historyErr
					}
					if found {
						if !durableRunMatchesTask(history, task) {
							unlock()
							return nil, fmt.Errorf("%w: run_id=%s belongs to task %s", ErrRunIdentityConflict, run.ID, history.TaskCatalogID)
						}
						candidate = history.Run
					}
					if !RunHasDurableObligation(candidate) {
						continue
					}
				}
				key := task.CatalogID + "\x00" + candidate.ID
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				result = append(result, DurableRun{Task: task, Run: candidate})
			}
		}
		unlock()
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Run.StartedAt.Equal(result[j].Run.StartedAt) {
			return result[i].Run.ID < result[j].Run.ID
		}
		return result[i].Run.StartedAt.Before(result[j].Run.StartedAt)
	})
	return result, nil
}

func (s *Store) AppendRun(id string, run RunRecord) (Task, error) {
	return s.appendRun(context.Background(), id, run, false)
}

func (s *Store) appendRun(ctx context.Context, id string, run RunRecord, allowCompletionReopen bool) (Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(id) == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(run.ID) == "" {
		return Task{}, fmt.Errorf("run id is required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, err
		}
		updated, err := withTaskStoreWriteLease(ctx, path, func() (Task, error) {
			tasks, readErr := location.store.readScope(location.scope)
			if readErr != nil {
				return Task{}, readErr
			}
			for i := range tasks {
				if !taskMatchesID(tasks[i], id) {
					continue
				}
				task := tasks[i]
				if err := location.store.backfillDurableRuns(location.scope, task); err != nil {
					return Task{}, err
				}
				obligation, obligationFound, err := location.store.readDurableRunObligation(location.scope, run.ID)
				if err != nil {
					return Task{}, err
				}
				history, historyFound, err := location.store.readDurableRun(location.scope, run.ID)
				if err != nil {
					return Task{}, err
				}
				entry, found := authoritativeDurableRun(obligation, obligationFound, history, historyFound)
				if found {
					if !durableRunMatchesTask(entry, task) {
						return Task{}, fmt.Errorf("%w: run_id=%s belongs to task %s", ErrRunIdentityConflict, run.ID, entry.TaskCatalogID)
					}
					run = preserveMonotonicRunReceipt(entry.Run, run, allowCompletionReopen)
					if transitionErr := validateRunAppendTransition(entry.Run, run, allowCompletionReopen); transitionErr != nil {
						return Task{}, transitionErr
					}
				}
				revision := maxDurableRunRevision(obligation, obligationFound, history, historyFound) + 1
				persisted := durableRunFile{Version: durableRunFileVersion, Revision: revision, TaskCatalogID: task.CatalogID, Run: run}
				// New obligations cross their dedicated write-ahead boundary before
				// history. Settled transitions cross history before removing that
				// obligation. Either crash order leaves startup with conservative work.
				if RunHasDurableObligation(run) {
					if err := location.store.writeDurableRunObligation(location.scope, persisted); err != nil {
						return Task{}, err
					}
				}
				if err := location.store.writeDurableRun(location.scope, persisted); err != nil {
					return Task{}, err
				}
				if !RunHasDurableObligation(run) {
					if err := location.store.removeDurableRunObligation(location.scope, run.ID); err != nil {
						return Task{}, err
					}
				}
				task.LastRun = &run
				nextRuns := []RunRecord{run}
				for _, existing := range task.RecentRuns {
					if existing.ID == run.ID {
						continue
					}
					nextRuns = append(nextRuns, existing)
				}
				task.RecentRuns = nextRuns
				if len(task.RecentRuns) > MaxRecentRuns {
					task.RecentRuns = task.RecentRuns[:MaxRecentRuns]
				}
				task.UpdatedAt = time.Now().UTC()
				normalized, normalizeErr := location.store.normalizeTaskTarget(task)
				if normalizeErr != nil {
					return Task{}, normalizeErr
				}
				tasks[i] = normalized
				if writeErr := location.store.writeScope(location.scope, tasks); writeErr != nil {
					return Task{}, writeErr
				}
				return normalized, nil
			}
			return Task{}, nil
		})
		if err != nil {
			return Task{}, err
		}
		if updated.ID != "" {
			return updated, nil
		}
	}
	return Task{}, fmt.Errorf("automation task %s not found", id)
}

func (s *Store) durableRunPath(scope, runID string) (string, error) {
	return s.durableRunPathIn(scope, durableRunHistoryDir, runID)
}

func (s *Store) durableRunObligationPath(scope, runID string) (string, error) {
	return s.durableRunPathIn(scope, durableRunObligationDir, runID)
}

func (s *Store) durableRunPathIn(scope, directory, runID string) (string, error) {
	taskPath, err := s.pathForScope(scope)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(runID)))
	return filepath.Join(filepath.Dir(taskPath), directory, hex.EncodeToString(digest[:])+".json"), nil
}

func (s *Store) readDurableRun(scope, runID string) (durableRunFile, bool, error) {
	path, err := s.durableRunPath(scope, runID)
	return readDurableRunFile(path, runID, err)
}

func (s *Store) readDurableRunObligation(scope, runID string) (durableRunFile, bool, error) {
	path, err := s.durableRunObligationPath(scope, runID)
	return readDurableRunFile(path, runID, err)
}

func readDurableRunFile(path, runID string, pathErr error) (durableRunFile, bool, error) {
	if pathErr != nil {
		return durableRunFile{}, false, pathErr
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return durableRunFile{}, false, nil
	}
	if err != nil {
		return durableRunFile{}, false, fmt.Errorf("read automation run ledger %s: %w", path, err)
	}
	entry, err := decodeDurableRun(path, data)
	if err != nil {
		return durableRunFile{}, false, err
	}
	if entry.Run.ID != strings.TrimSpace(runID) {
		return durableRunFile{}, false, fmt.Errorf("automation run ledger hash collision: wanted %s got %s", runID, entry.Run.ID)
	}
	return entry, true, nil
}

func (s *Store) readDurableRuns(scope string) ([]durableRunFile, error) {
	return s.readDurableRunsIn(scope, durableRunHistoryDir)
}

func (s *Store) readDurableRunObligations(scope string) ([]durableRunFile, error) {
	return s.readDurableRunsIn(scope, durableRunObligationDir)
}

func (s *Store) readDurableRunsIn(scope, directory string) ([]durableRunFile, error) {
	taskPath, err := s.pathForScope(scope)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(filepath.Dir(taskPath), directory)
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list automation run ledger %s: %w", dir, err)
	}
	result := make([]durableRunFile, 0, len(files))
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read automation run ledger %s: %w", path, err)
		}
		entry, err := decodeDurableRun(path, data)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, nil
}

func decodeDurableRun(path string, data []byte) (durableRunFile, error) {
	var entry durableRunFile
	if err := json.Unmarshal(data, &entry); err != nil {
		return durableRunFile{}, fmt.Errorf("decode automation run ledger %s: %w", path, err)
	}
	if entry.Version != durableRunFileVersion || strings.TrimSpace(entry.TaskCatalogID) == "" || strings.TrimSpace(entry.Run.ID) == "" {
		return durableRunFile{}, fmt.Errorf("invalid automation run ledger %s", path)
	}
	return entry, nil
}

func (s *Store) writeDurableRun(scope string, entry durableRunFile) error {
	path, err := s.durableRunPath(scope, entry.Run.ID)
	return writeDurableRunFile(path, entry, err)
}

func (s *Store) writeDurableRunObligation(scope string, entry durableRunFile) error {
	path, err := s.durableRunObligationPath(scope, entry.Run.ID)
	return writeDurableRunFile(path, entry, err)
}

func writeDurableRunFile(path string, entry durableRunFile, pathErr error) error {
	if pathErr != nil {
		return pathErr
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode automation run %s: %w", entry.Run.ID, err)
	}
	return durableWriteJSON(path, append(data, '\n'), 0o644)
}

func (s *Store) removeDurableRunObligation(scope, runID string) error {
	path, err := s.durableRunObligationPath(scope, runID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove settled automation run obligation %s: %w", path, err)
	}
	if err := fsdurability.SyncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync automation run obligation directory: %w", err)
	}
	return nil
}

func (s *Store) backfillDurableRuns(scope string, task Task) error {
	for _, run := range task.RecentRuns {
		entry, found, err := s.readDurableRun(scope, run.ID)
		if err != nil {
			return err
		}
		if found {
			if !durableRunMatchesTask(entry, task) {
				return fmt.Errorf("%w: run_id=%s belongs to task %s", ErrRunIdentityConflict, run.ID, entry.TaskCatalogID)
			}
			continue
		}
		if err := s.writeDurableRun(scope, durableRunFile{
			Version: durableRunFileVersion, Revision: 1, TaskCatalogID: task.CatalogID, Run: run,
		}); err != nil {
			return err
		}
		if RunHasDurableObligation(run) {
			if err := s.writeDurableRunObligation(scope, durableRunFile{
				Version: durableRunFileVersion, Revision: 1, TaskCatalogID: task.CatalogID, Run: run,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func taskForDurableRun(tasks []Task, entry durableRunFile) (Task, bool) {
	for _, task := range tasks {
		if durableRunMatchesTask(entry, task) {
			return task, true
		}
	}
	return Task{}, false
}

// durableRunMatchesTask accepts the path-owned locator written before Project
// IDs existed. The caller has already selected one exact scope/state root, so
// the immutable local task ID safely disambiguates the legacy prefix. Every
// subsequent AppendRun persists the current Project catalog locator.
func durableRunMatchesTask(entry durableRunFile, task Task) bool {
	if entry.TaskCatalogID == task.CatalogID {
		return true
	}
	localID := strings.TrimSpace(task.ID)
	if localID == "" || (strings.TrimSpace(entry.Run.TaskID) != "" && strings.TrimSpace(entry.Run.TaskID) != localID) {
		return false
	}
	legacyLocator := strings.TrimSpace(entry.TaskCatalogID)
	return legacyLocator == localID || strings.HasSuffix(legacyLocator, ":"+localID)
}
