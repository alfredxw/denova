package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/localfs"
	workspacelayout "denova/internal/workspace"
)

type inboxFile struct {
	Items []TriggerInboxItem `json:"items"`
}

func (s *Store) UpdateTriggerState(id string, triggerID string, state TriggerState) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	triggerID = strings.TrimSpace(triggerID)
	if triggerID == "" {
		return Task{}, fmt.Errorf("trigger id is required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, err
		}
		updated, err := withTaskStoreWriteLease(context.Background(), path, func() (Task, error) {
			tasks, readErr := location.store.readScope(location.scope)
			if readErr != nil {
				return Task{}, readErr
			}
			for i := range tasks {
				if !taskMatchesID(tasks[i], id) {
					continue
				}
				if tasks[i].TriggerState == nil {
					tasks[i].TriggerState = map[string]TriggerState{}
				}
				tasks[i].TriggerState[triggerID] = state
				tasks[i].UpdatedAt = time.Now().UTC()
				normalized, normalizeErr := location.store.normalizeTaskTarget(tasks[i])
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

func (s *Store) ListInbox() ([]TriggerInboxItem, error) {
	items := []TriggerInboxItem{}
	for _, location := range s.inboxLocations() {
		path, err := location.store.inboxPathForScope(location.scope)
		if err != nil {
			return nil, err
		}
		unlock := storePathLocks.Lock(path)
		scopeItems, err := location.store.readInboxScope(location.scope)
		unlock()
		if err != nil {
			return nil, err
		}
		for _, item := range scopeItems {
			if s.visibleInboxItem(item) {
				items = append(items, item)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (s *Store) CreateInboxItem(item TriggerInboxItem) (TriggerInboxItem, error) {
	if strings.TrimSpace(item.Workspace) == "" && strings.TrimSpace(s.workspace) != "" {
		item.Workspace = s.workspace
	}
	normalized, err := NormalizeInboxItem(item)
	if err != nil {
		return TriggerInboxItem{}, err
	}
	destination := s
	if normalized.Scope == ScopeWorkspace && strings.TrimSpace(normalized.Workspace) != "" {
		destination = s.storeForWorkspace(normalized.Workspace)
	}
	normalized = destination.bindProjectInboxWorkspace(normalized)
	path, err := destination.inboxPathForScope(normalized.Scope)
	if err != nil {
		return TriggerInboxItem{}, err
	}
	unlock := storePathLocks.Lock(path)
	defer unlock()
	release, err := localfs.AcquireLease(context.Background(), path+".lock")
	if err != nil {
		return TriggerInboxItem{}, err
	}
	defer func() { _ = release() }()
	items, err := destination.readInboxScope(normalized.Scope)
	if err != nil {
		return TriggerInboxItem{}, err
	}
	items = append([]TriggerInboxItem{normalized}, items...)
	items = boundInboxProjection(items)
	if err := destination.writeInboxScope(normalized.Scope, items); err != nil {
		return TriggerInboxItem{}, err
	}
	return normalized, nil
}

func (s *Store) GetInboxItem(id string) (TriggerInboxItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TriggerInboxItem{}, fmt.Errorf("inbox item id is required")
	}
	for _, location := range s.inboxLocations() {
		path, err := location.store.inboxPathForScope(location.scope)
		if err != nil {
			return TriggerInboxItem{}, err
		}
		unlock := storePathLocks.Lock(path)
		items, err := location.store.readInboxScope(location.scope)
		if err != nil {
			unlock()
			return TriggerInboxItem{}, err
		}
		for _, item := range items {
			if item.ID == id && s.visibleInboxItem(item) {
				unlock()
				return item, nil
			}
		}
		unlock()
	}
	return TriggerInboxItem{}, fmt.Errorf("automation inbox item %s not found", id)
}

func (s *Store) FindOpenInboxItem(taskID, triggerID, fingerprint string) (TriggerInboxItem, bool, error) {
	taskID = strings.TrimSpace(taskID)
	triggerID = strings.TrimSpace(triggerID)
	fingerprint = strings.TrimSpace(fingerprint)
	if taskID == "" || triggerID == "" || fingerprint == "" {
		return TriggerInboxItem{}, false, nil
	}
	items, err := s.ListInbox()
	if err != nil {
		return TriggerInboxItem{}, false, err
	}
	for _, item := range items {
		if item.TaskID != taskID || item.TriggerID != triggerID || item.Fingerprint != fingerprint {
			continue
		}
		if item.Status == InboxStatusPending || item.Status == InboxStatusAutoRun {
			return item, true, nil
		}
	}
	return TriggerInboxItem{}, false, nil
}

func (s *Store) FindInboxItemByEvidence(taskID, triggerID string, evidence []TriggerEvidence) (TriggerInboxItem, bool, error) {
	taskID = strings.TrimSpace(taskID)
	triggerID = strings.TrimSpace(triggerID)
	evidenceKey := triggerEvidenceRefsKey(evidence)
	if taskID == "" || triggerID == "" || evidenceKey == "" {
		return TriggerInboxItem{}, false, nil
	}
	items, err := s.ListInbox()
	if err != nil {
		return TriggerInboxItem{}, false, err
	}
	for _, item := range items {
		if item.TaskID != taskID || item.TriggerID != triggerID {
			continue
		}
		if triggerEvidenceRefsKey(item.Evidence) == evidenceKey {
			return item, true, nil
		}
	}
	return TriggerInboxItem{}, false, nil
}

func (s *Store) MarkInboxItemRead(id string) (TriggerInboxItem, error) {
	return s.updateInboxItem(id, func(item TriggerInboxItem, now time.Time) TriggerInboxItem {
		if item.ReadAt == nil {
			item.ReadAt = &now
		}
		return item
	})
}

func triggerEvidenceRefsKey(evidence []TriggerEvidence) string {
	refs := make([]string, 0, len(evidence))
	for _, item := range evidence {
		ref := strings.TrimSpace(item.Ref)
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return ""
	}
	return strings.Join(refs, "\x00")
}

func (s *Store) DismissInboxItem(id string) (TriggerInboxItem, error) {
	return s.mutateInboxItem(context.Background(), id, func(item TriggerInboxItem, now time.Time) (TriggerInboxItem, bool, error) {
		switch item.Status {
		case InboxStatusDismissed:
			return item, false, nil
		case InboxStatusPending:
			// A non-empty RunID is the durable confirmation claim. Claim and
			// dismiss share this file lease, so exactly one semantic action can
			// win before Agent admission.
			if strings.TrimSpace(item.RunID) != "" {
				return TriggerInboxItem{}, false, fmt.Errorf("%w: inbox_id=%s confirmation is already claimed", ErrTriggerActionConflict, item.ID)
			}
			item.Status = InboxStatusDismissed
			item.HandledAt = &now
			if item.ReadAt == nil {
				item.ReadAt = &now
			}
			return item, true, nil
		default:
			return TriggerInboxItem{}, false, fmt.Errorf("automation inbox item %s cannot be dismissed from status %s", item.ID, item.Status)
		}
	})
}

func (s *Store) ConfirmInboxItem(id, runID string) (TriggerInboxItem, error) {
	if _, _, err := s.ClaimInboxRun(context.Background(), id, runID); err != nil {
		return TriggerInboxItem{}, err
	}
	return s.CompleteInboxRun(context.Background(), id, runID)
}

func (s *Store) MarkInboxItemRunStartFailed(id, summary string) (TriggerInboxItem, error) {
	return s.updateInboxItem(id, func(item TriggerInboxItem, now time.Time) TriggerInboxItem {
		item.Status = InboxStatusPending
		item.ActionError = strings.TrimSpace(summary)
		item.UpdatedAt = now
		return item
	})
}

func (s *Store) AttachInboxRun(id, runID string) (TriggerInboxItem, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return TriggerInboxItem{}, fmt.Errorf("inbox run id is required")
	}
	return s.mutateInboxItem(context.Background(), id, func(item TriggerInboxItem, _ time.Time) (TriggerInboxItem, bool, error) {
		if item.RunID != "" && item.RunID != runID {
			return TriggerInboxItem{}, false, fmt.Errorf("%w: inbox_id=%s attached run differs", ErrTriggerActionConflict, item.ID)
		}
		if item.Status == InboxStatusAutoRun && item.RunID == runID {
			return item, false, nil
		}
		item.Status = InboxStatusAutoRun
		item.RunID = runID
		item.ActionError = ""
		return item, true, nil
	})
}

func (s *Store) readInboxScope(scope string) ([]TriggerInboxItem, error) {
	path, err := s.inboxPathForScope(scope)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []TriggerInboxItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	var file inboxFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("read automation inbox %s failed: %w", path, err)
	}
	out := make([]TriggerInboxItem, 0, len(file.Items))
	for _, item := range file.Items {
		item = s.bindProjectInboxWorkspace(item)
		normalized, err := NormalizeInboxItem(item)
		if err != nil {
			return nil, fmt.Errorf("invalid automation inbox item %s: %w", item.ID, err)
		}
		out = append(out, normalized)
	}
	return out, nil
}

func (s *Store) bindProjectInboxWorkspace(item TriggerInboxItem) TriggerInboxItem {
	if item.Scope == ScopeWorkspace && strings.TrimSpace(s.workspaceStateRoot) != "" {
		// Project state owns workspace-scoped inbox records. Binding the stable ID
		// here is also the read-time migration for released records that only
		// persisted a workspace path before Project identities were introduced.
		item.ProjectID = s.projectID
		item.Workspace = s.workspace
	}
	return item
}

func (s *Store) writeInboxScope(scope string, items []TriggerInboxItem) error {
	path, err := s.inboxPathForScope(scope)
	if err != nil {
		return err
	}
	persisted := make([]TriggerInboxItem, len(items))
	for index := range items {
		persisted[index] = portableInboxItem(items[index])
	}
	data, err := json.MarshalIndent(inboxFile{Items: persisted}, "", "  ")
	if err != nil {
		return err
	}
	return durableWriteJSON(path, append(data, '\n'), 0o644)
}

// boundInboxProjection treats MaxInboxItems as a soft display/history bound,
// never as an obligation bound. Every pending or auto-run item remains durable
// even when the actionable set alone exceeds the soft limit; only settled
// confirmation/dismissal history is eligible for eviction.
func boundInboxProjection(items []TriggerInboxItem) []TriggerInboxItem {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	actionable := make([]TriggerInboxItem, 0, len(items))
	settled := make([]TriggerInboxItem, 0, len(items))
	for _, item := range items {
		switch item.Status {
		case InboxStatusPending, InboxStatusAutoRun:
			actionable = append(actionable, item)
		default:
			settled = append(settled, item)
		}
	}
	settledLimit := MaxInboxItems - len(actionable)
	if settledLimit < 0 {
		settledLimit = 0
	}
	if len(settled) > settledLimit {
		settled = settled[:settledLimit]
	}
	result := append(actionable, settled...)
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (s *Store) updateInboxItem(id string, update func(TriggerInboxItem, time.Time) TriggerInboxItem) (TriggerInboxItem, error) {
	return s.mutateInboxItem(context.Background(), id, func(item TriggerInboxItem, now time.Time) (TriggerInboxItem, bool, error) {
		return update(item, now), true, nil
	})
}

func (s *Store) mutateInboxItem(ctx context.Context, id string, mutate func(TriggerInboxItem, time.Time) (TriggerInboxItem, bool, error)) (TriggerInboxItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TriggerInboxItem{}, fmt.Errorf("inbox item id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, location := range s.inboxLocations() {
		path, err := location.store.inboxPathForScope(location.scope)
		if err != nil {
			return TriggerInboxItem{}, err
		}
		unlock := storePathLocks.Lock(path)
		release, leaseErr := localfs.AcquireLease(ctx, path+".lock")
		if leaseErr != nil {
			unlock()
			return TriggerInboxItem{}, leaseErr
		}
		items, err := location.store.readInboxScope(location.scope)
		if err != nil {
			_ = release()
			unlock()
			return TriggerInboxItem{}, err
		}
		for i := range items {
			if items[i].ID != id || !s.visibleInboxItem(items[i]) {
				continue
			}
			now := time.Now().UTC()
			next, changed, mutateErr := mutate(items[i], now)
			if mutateErr != nil {
				_ = release()
				unlock()
				return TriggerInboxItem{}, mutateErr
			}
			if !changed {
				releaseErr := release()
				unlock()
				if releaseErr != nil {
					return TriggerInboxItem{}, releaseErr
				}
				return items[i], nil
			}
			next.UpdatedAt = now
			normalized, err := NormalizeInboxItem(next)
			if err != nil {
				_ = release()
				unlock()
				return TriggerInboxItem{}, err
			}
			items[i] = normalized
			if err := location.store.writeInboxScope(location.scope, items); err != nil {
				_ = release()
				unlock()
				return TriggerInboxItem{}, err
			}
			releaseErr := release()
			unlock()
			if releaseErr != nil {
				return TriggerInboxItem{}, releaseErr
			}
			return normalized, nil
		}
		releaseErr := release()
		unlock()
		if releaseErr != nil {
			return TriggerInboxItem{}, releaseErr
		}
	}
	return TriggerInboxItem{}, fmt.Errorf("automation inbox item %s not found", id)
}

func (s *Store) visibleInboxItem(item TriggerInboxItem) bool {
	if len(s.knownWorkspaces) > 0 {
		return true
	}
	if item.Scope != ScopeUser || strings.TrimSpace(s.workspace) == "" {
		return true
	}
	return canonicalStoreRoot(item.Workspace) == canonicalStoreRoot(s.workspace)
}

func (s *Store) inboxLocations() []taskStoreLocation {
	locations := []taskStoreLocation{{store: NewStore(s.userDir, ""), scope: ScopeUser}}
	seen := map[string]bool{}
	appendWorkspace := func(workspace string) {
		canonical := canonicalStoreRoot(workspace)
		if canonical == "" || seen[canonical] {
			return
		}
		seen[canonical] = true
		locations = append(locations, taskStoreLocation{store: s.storeForWorkspace(canonical), scope: ScopeWorkspace})
	}
	appendWorkspace(s.workspace)
	for _, workspace := range s.knownWorkspaces {
		appendWorkspace(workspace)
	}
	return locations
}

func (s *Store) inboxPathForScope(scope string) (string, error) {
	switch scope {
	case ScopeUser:
		if strings.TrimSpace(s.userDir) == "" {
			return "", fmt.Errorf("user nova dir is required")
		}
		return filepath.Join(s.userDir, "automations", "inbox.json"), nil
	case ScopeWorkspace:
		if strings.TrimSpace(s.workspace) == "" {
			return "", fmt.Errorf("workspace is required")
		}
		if strings.TrimSpace(s.workspaceStateRoot) != "" {
			return filepath.Join(s.workspaceStateRoot, "automations", "inbox.json"), nil
		}
		return workspacelayout.Path(s.workspace, "automations", "inbox.json"), nil
	default:
		return "", fmt.Errorf("unknown automation scope %q", scope)
	}
}
