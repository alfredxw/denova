package workspacechange

import (
	"bytes"
	"context"
	"sort"
	"strings"
)

type historyPlan struct {
	path         string
	before       []byte
	after        []byte
	beforeExists bool
	afterExists  bool
	targetID     string
}

func (s *Service) Undo(ctx context.Context, req HistoryRequest) (ChangeGroup, error) {
	if s == nil {
		return ChangeGroup{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return ChangeGroup{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return ChangeGroup{}, err
	}
	groupID := strings.TrimSpace(req.GroupID)
	group := s.groups[groupID]
	if group == nil {
		return ChangeGroup{}, newError(ErrorCodeNotFound, "change group not found", map[string]any{"group_id": groupID})
	}
	if s.undone[groupID] {
		return ChangeGroup{}, newError(ErrorCodeConflict, "change group is already undone", map[string]any{"group_id": groupID})
	}
	plans, timeline, err := s.planHistory(group, true)
	if err != nil {
		return ChangeGroup{}, err
	}
	changePlans := make([]groupChangePlan, 0, len(plans))
	for _, plan := range plans {
		metadata := ChangeMetadata{Origin: OriginUndo, ChangeGroupID: groupID, ReviewThreadID: group.ReviewThreadID, AutoAccept: true}
		change := historyChangeSet(plan, metadata)
		change.RevertsID = plan.targetID
		changePlans = append(changePlans, groupChangePlan{
			ChangeSet: change,
			Metadata:  metadata,
			Before:    append([]byte(nil), plan.before...),
			After:     append([]byte(nil), plan.after...),
		})
	}
	projection := operationProjection{HistoryState: historyStateUndone}
	for _, change := range timeline {
		projection.ChangeStates = append(projection.ChangeStates, operationChangeState{
			ChangeSetID: change.ID,
			ApplyState:  ApplyStateReverted,
		})
	}
	if err := s.commitGroupOperationLocked(ctx, operationKindUndo, groupID, changePlans, projection); err != nil {
		return ChangeGroup{}, err
	}
	return s.groupDetailLocked(groupID, true)
}

func (s *Service) Redo(ctx context.Context, req HistoryRequest) (ChangeGroup, error) {
	if s == nil {
		return ChangeGroup{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return ChangeGroup{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return ChangeGroup{}, err
	}
	groupID := strings.TrimSpace(req.GroupID)
	group := s.groups[groupID]
	if group == nil {
		return ChangeGroup{}, newError(ErrorCodeNotFound, "change group not found", map[string]any{"group_id": groupID})
	}
	if !s.undone[groupID] || s.redoInvalid[groupID] {
		return ChangeGroup{}, newError(ErrorCodeNoRedo, "change group cannot be redone", map[string]any{"group_id": groupID})
	}
	plans, timeline, err := s.planHistory(group, false)
	if err != nil {
		return ChangeGroup{}, err
	}
	changePlans := make([]groupChangePlan, 0, len(plans))
	for _, plan := range plans {
		metadata := ChangeMetadata{Origin: OriginRedo, ChangeGroupID: groupID, ReviewThreadID: group.ReviewThreadID, AutoAccept: true}
		change := historyChangeSet(plan, metadata)
		change.ReplaysID = plan.targetID
		changePlans = append(changePlans, groupChangePlan{
			ChangeSet: change,
			Metadata:  metadata,
			Before:    append([]byte(nil), plan.before...),
			After:     append([]byte(nil), plan.after...),
		})
	}
	projection := operationProjection{HistoryState: historyStateRedone}
	for _, change := range timeline {
		projection.ChangeStates = append(projection.ChangeStates, operationChangeState{
			ChangeSetID: change.ID,
			ApplyState:  ApplyStateApplied,
		})
	}
	if err := s.commitGroupOperationLocked(ctx, operationKindRedo, groupID, changePlans, projection); err != nil {
		return ChangeGroup{}, err
	}
	return s.groupDetailLocked(groupID, true)
}

func (s *Service) planHistory(group *ChangeGroup, undo bool) ([]historyPlan, []*ChangeSet, error) {
	timeline := make([]*ChangeSet, 0, len(group.ChangeSets))
	for _, listed := range group.ChangeSets {
		change := s.changeSets[listed.ID]
		if change == nil || change.Origin == OriginUndo || change.Origin == OriginRedo {
			continue
		}
		if change.ApplyState == ApplyStateConflicted {
			return nil, nil, newError(ErrorCodeConflict, "change group has an unresolved workspace conflict", map[string]any{"group_id": group.ID, "change_set_id": change.ID})
		}
		timeline = append(timeline, change)
	}
	if len(timeline) == 0 {
		return nil, nil, newError(ErrorCodeConflict, "change group has no reversible changes", map[string]any{"group_id": group.ID})
	}
	sort.SliceStable(timeline, func(i, j int) bool {
		return timeline[i].Sequence < timeline[j].Sequence
	})
	byPath := map[string][]*ChangeSet{}
	for _, change := range timeline {
		byPath[change.Path] = append(byPath[change.Path], change)
	}
	if err := validateHistoryContinuity(group.ID, byPath); err != nil {
		return nil, nil, err
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	plans := make([]historyPlan, 0, len(paths))
	for _, path := range paths {
		changes := byPath[path]
		first := cloneChangeSet(*changes[0])
		last := cloneChangeSet(*changes[len(changes)-1])
		if err := s.hydrateChange(&first, true); err != nil {
			return nil, nil, err
		}
		if err := s.hydrateChange(&last, true); err != nil {
			return nil, nil, err
		}
		if first.BeforeExists == last.AfterExists && (!first.BeforeExists || bytes.Equal([]byte(first.BeforeContent), []byte(last.AfterContent))) {
			continue
		}
		plan := historyPlan{path: path, targetID: last.ID}
		if undo {
			plan.before = []byte(last.AfterContent)
			plan.after = []byte(first.BeforeContent)
			plan.beforeExists = last.AfterExists
			plan.afterExists = first.BeforeExists
		} else {
			plan.before = []byte(first.BeforeContent)
			plan.after = []byte(last.AfterContent)
			plan.beforeExists = first.BeforeExists
			plan.afterExists = last.AfterExists
		}
		actualRevision, actualExists := s.currentRevision(path)
		expectedRevision := "missing"
		if plan.beforeExists {
			expectedRevision = Revision(plan.before)
		}
		if actualExists != plan.beforeExists || actualRevision != expectedRevision {
			return nil, nil, newError(ErrorCodeRevisionConflict, "workspace file changed since the recorded history state", map[string]any{
				"path":              path,
				"expected_revision": expectedRevision,
				"actual_revision":   actualRevision,
				"group_id":          group.ID,
			})
		}
		plans = append(plans, plan)
	}
	if len(plans) == 0 {
		return nil, nil, newError(ErrorCodeConflict, "change group has no net change to undo", map[string]any{"group_id": group.ID})
	}
	return plans, timeline, nil
}

// validateHistoryContinuity prevents endpoint-based undo from erasing a write
// that occurred between two changes sharing a group/run. Selective undo across
// such a dependency gap requires inverse patches; the snapshot fallback must
// fail closed instead.
func validateHistoryContinuity(groupID string, byPath map[string][]*ChangeSet) error {
	for path, changes := range byPath {
		for index := 1; index < len(changes); index++ {
			previous := changes[index-1]
			next := changes[index]
			if previous.AfterExists == next.BeforeExists && previous.Revision == next.BaseRevision {
				continue
			}
			return newError(ErrorCodeConflict, "change group history is not contiguous; refusing to overwrite an intervening workspace change", map[string]any{
				"group_id":               groupID,
				"path":                   path,
				"previous_change_set_id": previous.ID,
				"previous_revision":      previous.Revision,
				"next_change_set_id":     next.ID,
				"next_base_revision":     next.BaseRevision,
			})
		}
	}
	return nil
}

func historyChangeSet(plan historyPlan, metadata ChangeMetadata) ChangeSet {
	edit := AppliedEdit{
		ID:           newID("edit"),
		OldString:    string(plan.before),
		NewString:    string(plan.after),
		ReviewStatus: ReviewStatusAccepted,
		Hunks: []Hunk{{
			ID:          newID("hunk"),
			BeforeStart: 0,
			BeforeEnd:   len(plan.before),
			AfterStart:  0,
			AfterEnd:    len(plan.after),
		}},
	}
	return newChangeSet(plan.path, plan.before, plan.after, plan.beforeExists, plan.afterExists, []AppliedEdit{edit}, metadata)
}
