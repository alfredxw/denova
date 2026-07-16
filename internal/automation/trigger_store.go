package automation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/workspacepath"
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
	for _, scope := range s.availableScopes() {
		path, err := s.pathForScope(scope)
		if err != nil {
			return Task{}, err
		}
		unlock := storePathLocks.lock(path)
		tasks, err := s.readScope(scope)
		if err != nil {
			unlock()
			return Task{}, err
		}
		for i := range tasks {
			if tasks[i].ID != id {
				continue
			}
			if tasks[i].TriggerState == nil {
				tasks[i].TriggerState = map[string]TriggerState{}
			}
			tasks[i].TriggerState[triggerID] = state
			tasks[i].UpdatedAt = time.Now().UTC()
			normalized, err := NormalizeTask(tasks[i])
			if err != nil {
				unlock()
				return Task{}, err
			}
			tasks[i] = normalized
			if err := s.writeScope(scope, tasks); err != nil {
				unlock()
				return Task{}, err
			}
			unlock()
			return normalized, nil
		}
		unlock()
	}
	return Task{}, fmt.Errorf("automation task %s not found", id)
}

func (s *Store) ListInbox() ([]TriggerInboxItem, error) {
	items := []TriggerInboxItem{}
	for _, scope := range s.availableScopes() {
		path, err := s.inboxPathForScope(scope)
		if err != nil {
			return nil, err
		}
		unlock := storePathLocks.lock(path)
		scopeItems, err := s.readInboxScope(scope)
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
	path, err := s.inboxPathForScope(normalized.Scope)
	if err != nil {
		return TriggerInboxItem{}, err
	}
	unlock := storePathLocks.lock(path)
	defer unlock()
	items, err := s.readInboxScope(normalized.Scope)
	if err != nil {
		return TriggerInboxItem{}, err
	}
	items = append([]TriggerInboxItem{normalized}, items...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > MaxInboxItems {
		items = items[:MaxInboxItems]
	}
	if err := s.writeInboxScope(normalized.Scope, items); err != nil {
		return TriggerInboxItem{}, err
	}
	return normalized, nil
}

func (s *Store) GetInboxItem(id string) (TriggerInboxItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TriggerInboxItem{}, fmt.Errorf("inbox item id is required")
	}
	for _, scope := range s.availableScopes() {
		path, err := s.inboxPathForScope(scope)
		if err != nil {
			return TriggerInboxItem{}, err
		}
		unlock := storePathLocks.lock(path)
		items, err := s.readInboxScope(scope)
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
	return s.updateInboxItem(id, func(item TriggerInboxItem, now time.Time) TriggerInboxItem {
		item.Status = InboxStatusDismissed
		item.HandledAt = &now
		if item.ReadAt == nil {
			item.ReadAt = &now
		}
		return item
	})
}

func (s *Store) ConfirmInboxItem(id, runID string) (TriggerInboxItem, error) {
	return s.updateInboxItem(id, func(item TriggerInboxItem, now time.Time) TriggerInboxItem {
		item.Status = InboxStatusConfirmed
		item.RunID = strings.TrimSpace(runID)
		item.HandledAt = &now
		if item.ReadAt == nil {
			item.ReadAt = &now
		}
		return item
	})
}

func (s *Store) MarkInboxItemRunStartFailed(id, summary string) (TriggerInboxItem, error) {
	return s.updateInboxItem(id, func(item TriggerInboxItem, now time.Time) TriggerInboxItem {
		item.Status = InboxStatusPending
		item.ActionPolicy = ActionPolicyConfirm
		item.NotifyPolicy = NotifyPolicyInbox
		item.Summary = strings.TrimSpace(summary)
		item.UpdatedAt = now
		return item
	})
}

func (s *Store) AttachInboxRun(id, runID string) (TriggerInboxItem, error) {
	return s.updateInboxItem(id, func(item TriggerInboxItem, now time.Time) TriggerInboxItem {
		item.Status = InboxStatusAutoRun
		item.RunID = strings.TrimSpace(runID)
		item.UpdatedAt = now
		return item
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
		normalized, err := NormalizeInboxItem(item)
		if err != nil {
			return nil, fmt.Errorf("invalid automation inbox item %s: %w", item.ID, err)
		}
		out = append(out, normalized)
	}
	return out, nil
}

func (s *Store) writeInboxScope(scope string, items []TriggerInboxItem) error {
	path, err := s.inboxPathForScope(scope)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(inboxFile{Items: items}, "", "  ")
	if err != nil {
		return err
	}
	return durableWriteJSON(path, append(data, '\n'), 0o644)
}

func (s *Store) updateInboxItem(id string, update func(TriggerInboxItem, time.Time) TriggerInboxItem) (TriggerInboxItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TriggerInboxItem{}, fmt.Errorf("inbox item id is required")
	}
	for _, scope := range s.availableScopes() {
		path, err := s.inboxPathForScope(scope)
		if err != nil {
			return TriggerInboxItem{}, err
		}
		unlock := storePathLocks.lock(path)
		items, err := s.readInboxScope(scope)
		if err != nil {
			unlock()
			return TriggerInboxItem{}, err
		}
		for i := range items {
			if items[i].ID != id || !s.visibleInboxItem(items[i]) {
				continue
			}
			now := time.Now().UTC()
			next := update(items[i], now)
			next.UpdatedAt = now
			normalized, err := NormalizeInboxItem(next)
			if err != nil {
				unlock()
				return TriggerInboxItem{}, err
			}
			items[i] = normalized
			if err := s.writeInboxScope(scope, items); err != nil {
				unlock()
				return TriggerInboxItem{}, err
			}
			unlock()
			return normalized, nil
		}
		unlock()
	}
	return TriggerInboxItem{}, fmt.Errorf("automation inbox item %s not found", id)
}

func (s *Store) visibleInboxItem(item TriggerInboxItem) bool {
	if item.Scope != ScopeUser || strings.TrimSpace(s.workspace) == "" {
		return true
	}
	return canonicalStoreRoot(item.Workspace) == canonicalStoreRoot(s.workspace)
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
		return workspacepath.Path(s.workspace, "automations", "inbox.json"), nil
	default:
		return "", fmt.Errorf("unknown automation scope %q", scope)
	}
}
