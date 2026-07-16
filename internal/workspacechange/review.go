package workspacechange

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type inversePlan struct {
	target       *ChangeSet
	before       []byte
	after        []byte
	beforeExists bool
	afterExists  bool
	selectedIDs  []string
	allRejected  bool
}

type pathInversePlan struct {
	path         string
	before       []byte
	after        []byte
	beforeExists bool
	afterExists  bool
	decisions    []inversePlan
}

func (s *Service) Review(ctx context.Context, req ReviewRequest) (ChangeGroup, error) {
	result, err := s.ReviewWithResult(ctx, req)
	return result.Group, err
}

// ReviewWithResult applies a review decision and returns an operation-scoped
// mutation receipt. AffectedPaths contains only paths changed by this call,
// never every path that has appeared in the group.
func (s *Service) ReviewWithResult(ctx context.Context, req ReviewRequest) (ReviewResult, error) {
	if s == nil {
		return ReviewResult{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return ReviewResult{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return ReviewResult{}, err
	}
	req.GroupID = strings.TrimSpace(req.GroupID)
	group := s.groups[req.GroupID]
	if group == nil {
		return ReviewResult{}, newError(ErrorCodeNotFound, "change group not found", map[string]any{"group_id": req.GroupID})
	}
	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	if decision != ReviewDecisionAccept && decision != ReviewDecisionReject {
		return ReviewResult{}, newError(ErrorCodeConflict, "review decision must be accept or reject", map[string]any{"decision": req.Decision})
	}
	targets, err := s.reviewTargets(group, req.ChangeSetID)
	if err != nil {
		return ReviewResult{}, err
	}
	wanted := makeStringSet(req.EditIDs)
	if err := validateExplicitReviewSelection(targets, wanted, req.GroupID); err != nil {
		return ReviewResult{}, err
	}
	targets, err = s.reviewableTargets(group, targets)
	if err != nil {
		return ReviewResult{}, err
	}
	if len(targets) == 0 {
		return s.reviewResultLocked(req.GroupID, nil)
	}
	if decision == ReviewDecisionAccept {
		projection := operationProjection{}
		for _, target := range targets {
			statuses := map[string]string{}
			for _, edit := range target.Edits {
				if len(wanted) > 0 && !wanted[edit.ID] {
					continue
				}
				if firstNonEmpty(edit.ReviewStatus, ReviewStatusPending) != ReviewStatusPending {
					continue
				}
				statuses[edit.ID] = ReviewStatusAccepted
			}
			if len(statuses) > 0 {
				projection.ReviewUpdates = append(projection.ReviewUpdates, operationReviewUpdate{
					ChangeSetID: target.ID,
					Statuses:    statuses,
				})
			}
		}
		if len(projection.ReviewUpdates) == 0 {
			return s.reviewResultLocked(req.GroupID, nil)
		}
		if err := s.commitGroupOperationLocked(ctx, operationKindReview, req.GroupID, nil, projection); err != nil {
			return ReviewResult{}, err
		}
		return s.reviewResultLocked(req.GroupID, nil)
	}

	plans, err := s.planReviewRejections(targets, wanted, req.BaseRevision)
	if err != nil {
		return ReviewResult{}, err
	}
	matched := map[string]bool{}
	selectedCount := 0
	for _, plan := range plans {
		for _, decision := range plan.decisions {
			selectedCount += len(decision.selectedIDs)
			for _, id := range decision.selectedIDs {
				matched[id] = true
			}
		}
	}
	if selectedCount == 0 && len(wanted) == 0 {
		return s.reviewResultLocked(req.GroupID, nil)
	}
	if selectedCount == 0 || (len(wanted) > 0 && len(matched) != len(wanted)) {
		return ReviewResult{}, newError(ErrorCodeNotFound, "one or more review edits were not found", map[string]any{"group_id": req.GroupID})
	}
	changePlans := make([]groupChangePlan, 0, len(plans))
	affectedPaths := make([]string, 0, len(plans))
	projection := operationProjection{}
	for _, plan := range plans {
		if plan.beforeExists == plan.afterExists && bytes.Equal(plan.before, plan.after) {
			continue
		}
		metadata := ChangeMetadata{Origin: OriginReview, ChangeGroupID: req.GroupID, ReviewThreadID: group.ReviewThreadID, AutoAccept: true}
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
		inverse := newChangeSet(plan.path, plan.before, plan.after, plan.beforeExists, plan.afterExists, []AppliedEdit{edit}, metadata)
		if len(plan.decisions) == 1 {
			inverse.RevertsID = plan.decisions[0].target.ID
		}
		changePlans = append(changePlans, groupChangePlan{
			ChangeSet: inverse,
			Metadata:  metadata,
			Before:    append([]byte(nil), plan.before...),
			After:     append([]byte(nil), plan.after...),
		})
		affectedPaths = append(affectedPaths, plan.path)
	}
	for _, plan := range plans {
		for _, decision := range plan.decisions {
			statuses := map[string]string{}
			for _, id := range decision.selectedIDs {
				statuses[id] = ReviewStatusRejected
			}
			projection.ReviewUpdates = append(projection.ReviewUpdates, operationReviewUpdate{
				ChangeSetID: decision.target.ID,
				Statuses:    statuses,
			})
			if decision.allRejected {
				projection.ChangeStates = append(projection.ChangeStates, operationChangeState{
					ChangeSetID: decision.target.ID,
					ApplyState:  ApplyStateReverted,
				})
			}
		}
	}
	if err := s.commitGroupOperationLocked(ctx, operationKindReview, req.GroupID, changePlans, projection); err != nil {
		return ReviewResult{}, err
	}
	return s.reviewResultLocked(req.GroupID, affectedPaths)
}

func (s *Service) reviewResultLocked(groupID string, affectedPaths []string) (ReviewResult, error) {
	group, err := s.groupDetailLocked(groupID, true)
	if err != nil {
		return ReviewResult{}, err
	}
	return ReviewResult{
		Group:         group,
		AffectedPaths: append([]string(nil), affectedPaths...),
	}, nil
}

func (s *Service) reviewTargets(group *ChangeGroup, changeSetID string) ([]*ChangeSet, error) {
	changeSetID = strings.TrimSpace(changeSetID)
	var targets []*ChangeSet
	for _, listed := range group.ChangeSets {
		change := s.changeSets[listed.ID]
		if change == nil || change.Origin == OriginReview || change.Origin == OriginUndo || change.Origin == OriginRedo {
			continue
		}
		if changeSetID != "" && change.ID != changeSetID {
			continue
		}
		targets = append(targets, change)
	}
	if len(targets) == 0 {
		return nil, newError(ErrorCodeNotFound, "change set not found", map[string]any{"group_id": group.ID, "change_set_id": changeSetID})
	}
	return targets, nil
}

func (s *Service) reviewableTargets(group *ChangeGroup, targets []*ChangeSet) ([]*ChangeSet, error) {
	if s.undone[group.ID] {
		return nil, newError(ErrorCodeConflict, "an undone change group cannot receive review decisions", map[string]any{"group_id": group.ID})
	}
	reviewable := make([]*ChangeSet, 0, len(targets))
	for _, target := range targets {
		if target.ApplyState == ApplyStateApplied {
			reviewable = append(reviewable, target)
			continue
		}
		pending := 0
		for _, edit := range target.Edits {
			if firstNonEmpty(edit.ReviewStatus, ReviewStatusPending) == ReviewStatusPending {
				pending++
			}
		}
		if pending > 0 {
			return nil, newError(ErrorCodeConflict, "non-applied changes with pending edits cannot receive review decisions", map[string]any{
				"group_id": group.ID, "change_set_id": target.ID, "apply_state": target.ApplyState, "pending_edit_count": pending,
			})
		}
		// A fully decided reverted target is terminal. Group-level All operations
		// skip it so other applied targets can finish review.
	}
	return reviewable, nil
}

func validateExplicitReviewSelection(targets []*ChangeSet, wanted map[string]bool, groupID string) error {
	if len(wanted) == 0 {
		return nil
	}
	matched := map[string]bool{}
	for _, target := range targets {
		for _, edit := range target.Edits {
			if !wanted[edit.ID] {
				continue
			}
			matched[edit.ID] = true
			status := firstNonEmpty(edit.ReviewStatus, ReviewStatusPending)
			if status != ReviewStatusPending {
				return newError(ErrorCodeConflict, "review edit already has a terminal decision", map[string]any{
					"group_id": groupID, "change_set_id": target.ID, "edit_id": edit.ID, "review_status": status,
				})
			}
		}
	}
	if len(matched) != len(wanted) {
		return newError(ErrorCodeNotFound, "one or more review edits were not found", map[string]any{"group_id": groupID})
	}
	return nil
}

func (s *Service) planReviewRejections(targets []*ChangeSet, wanted map[string]bool, expectedRevision string) ([]pathInversePlan, error) {
	expectedRevision = strings.TrimSpace(expectedRevision)
	byPath := map[string][]*ChangeSet{}
	for _, target := range targets {
		if len(wanted) > 0 {
			relevant := false
			for _, edit := range target.Edits {
				if wanted[edit.ID] {
					relevant = true
					break
				}
			}
			if !relevant {
				continue
			}
		}
		byPath[target.Path] = append(byPath[target.Path], target)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if expectedRevision != "" && len(paths) > 1 {
		return nil, newError(ErrorCodeConflict, "one base revision cannot validate a multi-path review", map[string]any{"path_count": len(paths)})
	}
	plans := make([]pathInversePlan, 0, len(paths))
	for _, path := range paths {
		pathTargets := byPath[path]
		sort.SliceStable(pathTargets, func(i, j int) bool {
			return pathTargets[i].Sequence > pathTargets[j].Sequence
		})
		current, currentExists, err := s.readVisibleState(path)
		if err != nil {
			return nil, err
		}
		if expectedRevision != "" {
			if err := requireRevision(path, expectedRevision, stateRevision(current, currentExists)); err != nil {
				return nil, err
			}
		}
		plan := pathInversePlan{
			path:         path,
			before:       append([]byte(nil), current...),
			beforeExists: currentExists,
		}
		for _, target := range pathTargets {
			decision, selected, err := s.planReviewRejection(*target, wanted, current, currentExists)
			if err != nil {
				return nil, err
			}
			if selected == 0 {
				continue
			}
			plan.decisions = append(plan.decisions, decision)
			current = decision.after
			currentExists = decision.afterExists
		}
		if len(plan.decisions) == 0 {
			continue
		}
		plan.after = append([]byte(nil), current...)
		plan.afterExists = currentExists
		plans = append(plans, plan)
	}
	return plans, nil
}

func (s *Service) planReviewRejection(target ChangeSet, wanted map[string]bool, current []byte, currentExists bool) (inversePlan, int, error) {
	if err := s.hydrateChange(&target, true); err != nil {
		return inversePlan{}, 0, err
	}
	selected := make([]AppliedEdit, 0, len(target.Edits))
	for _, edit := range target.Edits {
		if len(wanted) > 0 && !wanted[edit.ID] {
			continue
		}
		if firstNonEmpty(edit.ReviewStatus, ReviewStatusPending) != ReviewStatusPending {
			continue
		}
		selected = append(selected, edit)
	}
	if len(selected) == 0 {
		return inversePlan{}, 0, nil
	}
	actualRevision := stateRevision(current, currentExists)
	exactTargetState := currentExists == target.AfterExists && actualRevision == target.Revision
	if target.BeforeExists != target.AfterExists && !exactTargetState {
		return inversePlan{}, 0, newError(ErrorCodeRevisionConflict, "an existence-changing edit cannot be relocated after the file changed", map[string]any{"path": target.Path, "actual_revision": actualRevision, "expected_revision": target.Revision})
	}
	var spans []reviewInverseSpan
	if exactTargetState {
		for index, edit := range selected {
			for _, hunk := range edit.Hunks {
				if !validSlice(current, hunk.AfterStart, hunk.AfterEnd) || string(current[hunk.AfterStart:hunk.AfterEnd]) != edit.NewString {
					return inversePlan{}, 0, newError(ErrorCodeConflict, "stored review hunk no longer matches the file", map[string]any{"path": target.Path, "edit_id": edit.ID})
				}
				spans = append(spans, reviewInverseSpan{editIndex: index, start: hunk.AfterStart, end: hunk.AfterEnd, replacement: edit.OldString})
			}
		}
	} else {
		segments, ok := stableReviewSegments(target.AfterContent, string(current))
		if !ok {
			return inversePlan{}, 0, newError(ErrorCodeRevisionConflict, "changed file cannot safely map the rejected edit", map[string]any{
				"path": target.Path, "actual_revision": actualRevision, "expected_revision": target.Revision,
			})
		}
		uniqueSegments := map[int]bool{}
		for index, edit := range selected {
			for _, hunk := range edit.Hunks {
				segmentIndex, start, end, mapped := mapStableReviewHunk(target.AfterContent, string(current), edit, hunk, segments, uniqueSegments)
				if !mapped {
					return inversePlan{}, 0, newError(ErrorCodeRevisionConflict, "changed file cannot safely relocate rejected text", map[string]any{
						"path": target.Path, "edit_id": edit.ID, "actual_revision": actualRevision, "expected_revision": target.Revision,
					})
				}
				uniqueSegments[segmentIndex] = true
				spans = append(spans, reviewInverseSpan{editIndex: index, start: start, end: end, replacement: edit.OldString})
			}
		}
	}
	after, _, err := applyReviewInverse(target.Path, string(current), selected, spans)
	if err != nil {
		return inversePlan{}, 0, err
	}
	selectedIDs := make([]string, 0, len(selected))
	for _, edit := range selected {
		selectedIDs = append(selectedIDs, edit.ID)
	}
	nonRejected := 0
	for _, edit := range target.Edits {
		if edit.ReviewStatus != ReviewStatusRejected && !containsString(selectedIDs, edit.ID) {
			nonRejected++
		}
	}
	allRejected := nonRejected == 0
	afterExists := currentExists
	if allRejected && target.BeforeExists != target.AfterExists {
		afterExists = target.BeforeExists
	}
	return inversePlan{
		target:       s.changeSets[target.ID],
		before:       append([]byte(nil), current...),
		after:        []byte(after),
		beforeExists: currentExists,
		afterExists:  afterExists,
		selectedIDs:  selectedIDs,
		allRejected:  allRejected,
	}, len(selected), nil
}

type stableReviewSegment struct {
	targetStart  int
	targetEnd    int
	currentStart int
	currentEnd   int
	text         string
}

// stableReviewSegments builds a byte-offset map only from exact equality
// segments between the recorded post-change snapshot and the current file.
// An edit that was deleted and merely has the same text elsewhere therefore
// cannot be relocated by a global literal search.
func stableReviewSegments(target, current string) ([]stableReviewSegment, bool) {
	if !utf8.ValidString(target) || !utf8.ValidString(current) {
		return nil, false
	}
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 0
	diffs := dmp.DiffMain(target, current, false)
	targetOffset := 0
	currentOffset := 0
	segments := make([]stableReviewSegment, 0, len(diffs))
	for _, diff := range diffs {
		size := len(diff.Text)
		switch diff.Type {
		case diffmatchpatch.DiffDelete:
			targetOffset += size
		case diffmatchpatch.DiffInsert:
			currentOffset += size
		case diffmatchpatch.DiffEqual:
			segments = append(segments, stableReviewSegment{
				targetStart:  targetOffset,
				targetEnd:    targetOffset + size,
				currentStart: currentOffset,
				currentEnd:   currentOffset + size,
				text:         diff.Text,
			})
			targetOffset += size
			currentOffset += size
		default:
			return nil, false
		}
	}
	return segments, targetOffset == len(target) && currentOffset == len(current)
}

func mapStableReviewHunk(
	target, current string,
	edit AppliedEdit,
	hunk Hunk,
	segments []stableReviewSegment,
	uniqueSegments map[int]bool,
) (segmentIndex, start, end int, ok bool) {
	if hunk.AfterStart < 0 || hunk.AfterEnd <= hunk.AfterStart || hunk.AfterEnd > len(target) ||
		target[hunk.AfterStart:hunk.AfterEnd] != edit.NewString {
		return 0, 0, 0, false
	}
	for index, segment := range segments {
		if hunk.AfterStart < segment.targetStart || hunk.AfterEnd > segment.targetEnd {
			continue
		}
		unique, checked := uniqueSegments[index]
		if !checked {
			unique = uniqueOccurrenceAt(target, segment.text, segment.targetStart) &&
				uniqueOccurrenceAt(current, segment.text, segment.currentStart)
		}
		if !unique {
			return 0, 0, 0, false
		}
		start = segment.currentStart + hunk.AfterStart - segment.targetStart
		end = start + hunk.AfterEnd - hunk.AfterStart
		if !validSlice([]byte(current), start, end) || current[start:end] != edit.NewString {
			return 0, 0, 0, false
		}
		return index, start, end, true
	}
	return 0, 0, 0, false
}

func uniqueOccurrenceAt(content, needle string, expected int) bool {
	if needle == "" || expected < 0 || expected+len(needle) > len(content) || content[expected:expected+len(needle)] != needle {
		return false
	}
	first := strings.Index(content, needle)
	if first != expected {
		return false
	}
	return strings.Index(content[first+1:], needle) < 0
}

type reviewInverseSpan struct {
	editIndex   int
	start       int
	end         int
	replacement string
}

func applyReviewInverse(path, current string, selected []AppliedEdit, spans []reviewInverseSpan) (string, []AppliedEdit, error) {
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end < spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	for index := 1; index < len(spans); index++ {
		if spans[index].start < spans[index-1].end {
			return "", nil, newError(ErrorCodeConflict, "rejected edit ranges overlap", map[string]any{"path": path})
		}
	}
	inverse := make([]AppliedEdit, len(selected))
	for index, edit := range selected {
		inverse[index] = AppliedEdit{ID: newID("edit"), OldString: edit.NewString, NewString: edit.OldString, ReplaceAll: edit.ReplaceAll, ReviewStatus: ReviewStatusAccepted}
	}
	delta := 0
	for _, span := range spans {
		afterStart := span.start + delta
		afterEnd := afterStart + len(span.replacement)
		inverse[span.editIndex].Hunks = append(inverse[span.editIndex].Hunks, Hunk{ID: newID("hunk"), BeforeStart: span.start, BeforeEnd: span.end, AfterStart: afterStart, AfterEnd: afterEnd})
		delta += len(span.replacement) - (span.end - span.start)
	}
	result := current
	for index := len(spans) - 1; index >= 0; index-- {
		span := spans[index]
		result = result[:span.start] + span.replacement + result[span.end:]
	}
	return result, inverse, nil
}

func makeStringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = true
		}
	}
	return result
}
