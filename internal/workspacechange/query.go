package workspacechange

import (
	"context"
	"sort"
	"strings"
)

func (s *Service) ListGroups(ctx context.Context, filter ChangeFilter) ([]ChangeGroupSummary, error) {
	if s == nil {
		return nil, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.contextError(ctx); err != nil {
		return nil, err
	}
	wantedStatus := strings.TrimSpace(filter.Status)
	wantedPath := strings.TrimSpace(filter.Path)
	wantedRunID := strings.TrimSpace(filter.RunID)
	wantedSessionID := strings.TrimSpace(filter.SessionID)
	wantedThreadID := strings.TrimSpace(filter.ReviewThreadID)
	if wantedPath != "" {
		var err error
		wantedPath, err = s.visibleRelPath(wantedPath)
		if err != nil {
			return nil, err
		}
	}
	result := make([]ChangeGroupSummary, 0, len(s.groups))
	for _, group := range s.groups {
		if wantedRunID != "" && group.RunID != wantedRunID {
			continue
		}
		if wantedSessionID != "" && group.SessionID != wantedSessionID {
			continue
		}
		if wantedThreadID != "" && reviewThreadID(group) != wantedThreadID {
			continue
		}
		if wantedStatus == ReviewStatusPending {
			if group.PendingEditCount == 0 {
				continue
			}
		} else if wantedStatus != "" && group.ReviewStatus != wantedStatus && group.ApplyState != wantedStatus {
			continue
		}
		paths := uniqueGroupPaths(group.ChangeSets)
		if wantedPath != "" && !containsString(paths, wantedPath) {
			continue
		}
		result = append(result, s.groupSummaryLocked(group, paths))
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Service) groupSummaryLocked(group *ChangeGroup, paths []string) ChangeGroupSummary {
	canUndo, canRedo := s.liveHistoryCapabilities(group)
	return ChangeGroupSummary{
		ID:                     group.ID,
		Origin:                 group.Origin,
		ReviewThreadID:         reviewThreadID(group),
		RunID:                  group.RunID,
		SessionID:              group.SessionID,
		CreatedAt:              group.CreatedAt,
		ReviewStatus:           group.ReviewStatus,
		ApplyState:             group.ApplyState,
		CanUndo:                canUndo,
		CanRedo:                canRedo,
		PendingEditCount:       group.PendingEditCount,
		UnresolvedCommentCount: group.UnresolvedCommentCount,
		ChangeSetCount:         len(group.ChangeSets),
		Paths:                  append([]string(nil), paths...),
	}
}

func reviewThreadID(group *ChangeGroup) string {
	if group == nil {
		return ""
	}
	return firstNonEmpty(group.ReviewThreadID, group.ID)
}

func (s *Service) GetGroup(ctx context.Context, id string) (ChangeGroup, error) {
	if s == nil {
		return ChangeGroup{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.contextError(ctx); err != nil {
		return ChangeGroup{}, err
	}
	id = strings.TrimSpace(id)
	group := s.groups[id]
	if group == nil {
		return ChangeGroup{}, newError(ErrorCodeNotFound, "change group not found", map[string]any{"group_id": id})
	}
	return s.groupDetailLocked(id, true)
}

func (s *Service) groupDetailLocked(id string, includeContent bool) (ChangeGroup, error) {
	group := s.groups[id]
	if group == nil {
		return ChangeGroup{}, newError(ErrorCodeNotFound, "change group not found", map[string]any{"group_id": id})
	}
	result := cloneGroup(*group)
	result.CanUndo, result.CanRedo = s.liveHistoryCapabilities(group)
	if !includeContent {
		return result, nil
	}
	for index := range result.ChangeSets {
		if err := s.hydrateChange(&result.ChangeSets[index], true); err != nil {
			return ChangeGroup{}, err
		}
	}
	return result, nil
}

func uniqueGroupPaths(changes []ChangeSet) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Path == "" || seen[change.Path] {
			continue
		}
		seen[change.Path] = true
		result = append(result, change.Path)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
