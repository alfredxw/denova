package workspacechange

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var workspaceServices = struct {
	sync.Mutex
	items map[string]*Service
}{items: map[string]*Service{}}

// Service owns one workspace's mutation lock and durable change projection.
type Service struct {
	workspace  string
	store      *eventStore
	durability *durabilityOps

	mu                 sync.RWMutex
	groups             map[string]*ChangeGroup
	changeSets         map[string]*ChangeSet
	comments           map[string]*Comment
	prepared           map[string]preparedChange
	operations         map[string]preparedOperation
	operationTerminals map[string]string
	undone             map[string]bool
	redoInvalid        map[string]bool
	pendingParentSync  map[string]string
	pendingSaves       map[string]pendingSaveIntent
	nextSequence       uint64

	// groupOperationHook is a package-private crash/cancellation injection point
	// used by recovery tests. Production services leave it nil.
	groupOperationHook func(stage, path string) error
}

type preparedChange struct {
	ChangeSet ChangeSet
	Metadata  ChangeMetadata
}

type pendingSaveIntent struct {
	Path            string
	ParentRel       string
	BaseRevision    string
	Revision        string
	Durable         bool
	RedoInvalidated bool
}

// ForWorkspace returns the process-wide service shared by all callers for an
// absolute workspace. This is the production constructor: Agent, HTTP, review,
// and history operations must use the same instance.
func ForWorkspace(workspace string) (*Service, error) {
	abs, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	workspaceServices.Lock()
	defer workspaceServices.Unlock()
	if existing := workspaceServices.items[abs]; existing != nil {
		return existing, nil
	}
	service, err := newService(abs)
	if err != nil {
		return nil, err
	}
	workspaceServices.items[abs] = service
	return service, nil
}

// NewService creates an independent service, primarily for isolated tests.
func NewService(workspace string) (*Service, error) {
	abs, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	return newService(abs)
}

func normalizeWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", newError(ErrorCodeConflict, "workspace path is empty", nil)
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", newError(ErrorCodeConflict, "workspace path is not a directory", map[string]any{"workspace": abs})
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func newService(workspace string) (*Service, error) {
	return newServiceWithDurability(workspace, defaultDurabilityOps())
}

func newServiceWithDurability(workspace string, durability *durabilityOps) (*Service, error) {
	if durability == nil {
		durability = defaultDurabilityOps()
	}
	store, err := newEventStore(workspace, durability)
	if err != nil {
		return nil, err
	}
	s := &Service{
		workspace:          workspace,
		store:              store,
		durability:         durability,
		groups:             map[string]*ChangeGroup{},
		changeSets:         map[string]*ChangeSet{},
		comments:           map[string]*Comment{},
		prepared:           map[string]preparedChange{},
		operations:         map[string]preparedOperation{},
		operationTerminals: map[string]string{},
		undone:             map[string]bool{},
		redoInvalid:        map[string]bool{},
		pendingParentSync:  map[string]string{},
		pendingSaves:       map[string]pendingSaveIntent{},
	}
	if err := s.load(); err != nil {
		store.close()
		return nil, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		store.close()
		return nil, err
	}
	return s, nil
}

func (s *Service) Workspace() string {
	if s == nil {
		return ""
	}
	return s.workspace
}

// ReadFile reads one visible workspace file and hashes exactly the returned bytes.
func (s *Service) ReadFile(path string) (content string, revision string, err error) {
	if s == nil {
		return "", "", newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rel, err := s.visibleRelPath(path)
	if err != nil {
		return "", "", err
	}
	data, err := s.readVisibleFile(rel)
	if err != nil {
		return "", "", err
	}
	return string(data), Revision(data), nil
}

func (s *Service) visibleRelPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", newError(ErrorCodeConflict, "file path is empty", nil)
	}
	var rel string
	if filepath.IsAbs(input) {
		candidate, err := filepath.Rel(s.workspace, filepath.Clean(input))
		if err != nil {
			return "", err
		}
		rel = candidate
	} else {
		rel = filepath.Clean(filepath.FromSlash(input))
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", newError(ErrorCodeConflict, "file path is outside the workspace", map[string]any{"path": input})
	}
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if component == "" || strings.HasPrefix(component, ".") {
			return "", newError(ErrorCodeConflict, "hidden workspace paths cannot be changed", map[string]any{"path": input})
		}
	}
	return filepath.ToSlash(rel), nil
}

func (s *Service) readVisibleFile(rel string) ([]byte, error) {
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := root.ReadFile(filepath.FromSlash(rel))
	if errors.Is(err, os.ErrNotExist) {
		return nil, newError(ErrorCodeNotFound, "workspace file not found", map[string]any{"path": rel})
	}
	return data, err
}

func (s *Service) appendAndApply(event ledgerEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if err := s.store.append(event); err != nil {
		return err
	}
	return s.applyEvent(event)
}

func (s *Service) load() error {
	events, err := s.store.readAll()
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := s.applyEvent(event); err != nil {
			return fmt.Errorf("replay workspace change ledger event %q: %w", event.Type, err)
		}
	}
	return nil
}

func (s *Service) applyEvent(event ledgerEvent) error {
	switch event.Type {
	case eventChangePrepared:
		if event.ChangeSet == nil || event.Metadata == nil {
			return errors.New("prepared event is missing change data")
		}
		// Projections retain only immutable blob references and structural edit
		// metadata. Keeping the caller's hydrated strings here would make memory
		// usage grow with every manuscript revision even though the ledger itself
		// is sanitized before append.
		change := ledgerChangeSet(*event.ChangeSet)
		s.assignChangeSequence(&change)
		s.prepared[change.ID] = preparedChange{ChangeSet: change, Metadata: *event.Metadata}
	case eventChangeApplied, eventChangeRecoveredApplied:
		prepared, ok := s.prepared[event.ChangeSetID]
		if !ok {
			if existing := s.changeSets[event.ChangeSetID]; existing != nil && existing.ApplyState == ApplyStateApplied {
				return nil
			}
			return fmt.Errorf("applied change %s has no prepared event", event.ChangeSetID)
		}
		change := cloneChangeSet(prepared.ChangeSet)
		change.ApplyState = ApplyStateApplied
		s.addProjectedChange(change, prepared.Metadata)
		delete(s.prepared, event.ChangeSetID)
	case eventChangeConflicted:
		prepared, ok := s.prepared[event.ChangeSetID]
		if !ok {
			if existing := s.changeSets[event.ChangeSetID]; existing != nil && existing.ApplyState == ApplyStateConflicted {
				return nil
			}
			return fmt.Errorf("conflicted change %s has no prepared event", event.ChangeSetID)
		}
		change := cloneChangeSet(prepared.ChangeSet)
		change.ApplyState = ApplyStateConflicted
		s.addProjectedChange(change, prepared.Metadata)
		delete(s.prepared, event.ChangeSetID)
	case eventChangeAborted:
		if existing := s.changeSets[event.ChangeSetID]; existing != nil {
			return fmt.Errorf("aborted change %s already has terminal state %s", event.ChangeSetID, existing.ApplyState)
		}
		delete(s.prepared, event.ChangeSetID)
	case eventReviewUpdated:
		change := s.changeSets[event.ChangeSetID]
		if change == nil {
			return fmt.Errorf("reviewed change %s not found", event.ChangeSetID)
		}
		for i := range change.Edits {
			if status, ok := event.EditStatuses[change.Edits[i].ID]; ok {
				change.Edits[i].ReviewStatus = status
			}
		}
		change.ReviewStatus = aggregateEditReviewStatus(change.Edits)
		s.refreshGroup(change.GroupID)
	case eventChangeState:
		change := s.changeSets[event.ChangeSetID]
		if change == nil {
			return fmt.Errorf("state target change %s not found", event.ChangeSetID)
		}
		change.ApplyState = event.ApplyState
		s.refreshGroup(change.GroupID)
	case eventCommentUpserted:
		if event.Comment == nil {
			return errors.New("comment event is missing comment")
		}
		comment := *event.Comment
		s.comments[comment.ID] = &comment
		s.refreshGroup(comment.GroupID)
	case eventCommentsUpserted:
		if len(event.Comments) == 0 {
			return errors.New("comments event is missing comments")
		}
		for i := range event.Comments {
			comment := event.Comments[i]
			s.comments[comment.ID] = &comment
			s.refreshGroup(comment.GroupID)
		}
	case eventHistoryState:
		s.applyHistoryState(event.GroupID, event.HistoryState)
	case eventOperationPrepared:
		if event.Operation == nil {
			return errors.New("prepared operation event is missing operation data")
		}
		operation := cloneDurableOperation(*event.Operation)
		if terminal := s.operationTerminals[operation.ID]; terminal != "" {
			return fmt.Errorf("prepared operation %s follows terminal state %s", operation.ID, terminal)
		}
		if _, exists := s.operations[operation.ID]; exists {
			return fmt.Errorf("group operation %s is already prepared", operation.ID)
		}
		for index := range operation.Changes {
			change := ledgerChangeSet(operation.Changes[index].ChangeSet)
			operation.Changes[index].ChangeSet = change
			changeRef := &operation.Changes[index].ChangeSet
			s.assignChangeSequence(changeRef)
		}
		if err := s.validateOperationProjection(operation); err != nil {
			return err
		}
		s.operations[operation.ID] = preparedOperation{
			Operation:    operation,
			AppliedPaths: map[string]bool{},
		}
	case eventOperationPathApplied:
		prepared, ok := s.operations[event.OperationID]
		if !ok {
			if terminal := s.operationTerminals[event.OperationID]; terminal != "" {
				return fmt.Errorf("operation progress %s follows terminal state %s", event.OperationID, terminal)
			}
			return fmt.Errorf("operation progress %s has no prepared event", event.OperationID)
		}
		knownPath := false
		for _, planned := range prepared.Operation.Changes {
			if planned.ChangeSet.Path == event.OperationPath {
				knownPath = true
				break
			}
		}
		if !knownPath {
			return fmt.Errorf("operation progress %s references unknown path %q", event.OperationID, event.OperationPath)
		}
		prepared.AppliedPaths[event.OperationPath] = true
		s.operations[event.OperationID] = prepared
	case eventOperationCommitted:
		if terminal := s.operationTerminals[event.OperationID]; terminal != "" {
			if terminal == eventOperationCommitted {
				return nil
			}
			return fmt.Errorf("committed operation %s conflicts with terminal state %s", event.OperationID, terminal)
		}
		prepared, ok := s.operations[event.OperationID]
		if !ok {
			return fmt.Errorf("committed operation %s has no prepared event", event.OperationID)
		}
		if err := s.applyOperationProjection(prepared.Operation); err != nil {
			return err
		}
		delete(s.operations, event.OperationID)
		s.operationTerminals[event.OperationID] = eventOperationCommitted
	case eventOperationConflicted:
		if terminal := s.operationTerminals[event.OperationID]; terminal != "" {
			if terminal == eventOperationConflicted {
				return nil
			}
			return fmt.Errorf("conflicted operation %s conflicts with terminal state %s", event.OperationID, terminal)
		}
		prepared, ok := s.operations[event.OperationID]
		if !ok {
			return fmt.Errorf("conflicted operation %s has no prepared event", event.OperationID)
		}
		if err := s.applyOperationConflict(prepared.Operation); err != nil {
			return err
		}
		delete(s.operations, event.OperationID)
		s.operationTerminals[event.OperationID] = eventOperationConflicted
	default:
		return fmt.Errorf("unknown workspace change ledger event type %q", event.Type)
	}
	return nil
}

func (s *Service) applyOperationProjection(operation durableOperation) error {
	if err := s.validateOperationProjection(operation); err != nil {
		return err
	}
	for _, planned := range operation.Changes {
		change := cloneChangeSet(planned.ChangeSet)
		change.ApplyState = ApplyStateApplied
		s.addProjectedChange(change, planned.Metadata)
	}
	for _, update := range operation.Projection.ReviewUpdates {
		change := s.changeSets[update.ChangeSetID]
		for index := range change.Edits {
			if status, ok := update.Statuses[change.Edits[index].ID]; ok {
				change.Edits[index].ReviewStatus = status
			}
		}
		change.ReviewStatus = aggregateEditReviewStatus(change.Edits)
		s.refreshGroup(change.GroupID)
	}
	for _, update := range operation.Projection.ChangeStates {
		change := s.changeSets[update.ChangeSetID]
		change.ApplyState = update.ApplyState
		s.refreshGroup(change.GroupID)
	}
	if operation.Projection.HistoryState != "" {
		s.applyHistoryState(operation.GroupID, operation.Projection.HistoryState)
	}
	return nil
}

func (s *Service) applyOperationConflict(operation durableOperation) error {
	if err := s.validateOperationProjection(operation); err != nil {
		return err
	}
	for _, planned := range operation.Changes {
		change := cloneChangeSet(planned.ChangeSet)
		change.ApplyState = ApplyStateConflicted
		s.addProjectedChange(change, planned.Metadata)
	}
	targets := map[string]bool{}
	for _, update := range operation.Projection.ReviewUpdates {
		targets[update.ChangeSetID] = true
	}
	for _, update := range operation.Projection.ChangeStates {
		targets[update.ChangeSetID] = true
	}
	for id := range targets {
		change := s.changeSets[id]
		change.ApplyState = ApplyStateConflicted
		s.refreshGroup(change.GroupID)
	}
	return nil
}

func (s *Service) applyHistoryState(groupID, state string) {
	switch state {
	case historyStateUndone:
		s.undone[groupID] = true
		s.redoInvalid[groupID] = false
	case historyStateRedone:
		s.undone[groupID] = false
		s.redoInvalid[groupID] = false
	case historyStateRedoInvalidated:
		s.redoInvalid[groupID] = true
	}
	s.refreshGroup(groupID)
}

func (s *Service) addProjectedChange(change ChangeSet, metadata ChangeMetadata) {
	// Keep this boundary defensive: all callers, including live apply paths,
	// must project the same bounded representation used by ledger replay.
	copy := ledgerChangeSet(change)
	group := s.groups[copy.GroupID]
	if group == nil {
		threadID := firstNonEmpty(copy.ReviewThreadID, metadata.ReviewThreadID, copy.GroupID)
		group = &ChangeGroup{
			ID:             copy.GroupID,
			Origin:         firstNonEmpty(metadata.Origin, copy.Origin, OriginUser),
			ReviewThreadID: threadID,
			RunID:          firstNonEmpty(metadata.RunID, copy.RunID),
			SessionID:      firstNonEmpty(metadata.SessionID, copy.SessionID),
			CreatedAt:      copy.CreatedAt,
			ReviewStatus:   copy.ReviewStatus,
			ApplyState:     copy.ApplyState,
		}
		s.groups[group.ID] = group
	} else {
		group.ReviewThreadID = firstNonEmpty(group.ReviewThreadID, copy.ReviewThreadID, metadata.ReviewThreadID, group.ID)
	}
	// A group is one immutable run/history boundary, so every change in it must
	// inherit the group's thread even when an older Review/Undo caller omitted
	// the newer metadata field.
	copy.ReviewThreadID = firstNonEmpty(group.ReviewThreadID, group.ID)
	s.changeSets[copy.ID] = &copy
	group.ChangeSets = append(group.ChangeSets, copy)
	s.refreshGroup(group.ID)
}

func (s *Service) refreshGroup(groupID string) {
	group := s.groups[groupID]
	if group == nil {
		return
	}
	group.ReviewThreadID = firstNonEmpty(group.ReviewThreadID, group.ID)
	group.ChangeSets = group.ChangeSets[:0]
	for _, change := range s.changeSets {
		if change.GroupID == groupID {
			group.ChangeSets = append(group.ChangeSets, cloneChangeSet(*change))
		}
	}
	sort.SliceStable(group.ChangeSets, func(i, j int) bool {
		return group.ChangeSets[i].Sequence < group.ChangeSets[j].Sequence
	})
	group.Comments = group.Comments[:0]
	for _, comment := range s.comments {
		if comment.GroupID == groupID {
			group.Comments = append(group.Comments, *comment)
		}
	}
	sort.SliceStable(group.Comments, func(i, j int) bool { return group.Comments[i].CreatedAt.Before(group.Comments[j].CreatedAt) })
	group.ReviewStatus = aggregateChangeReviewStatus(group.ChangeSets)
	group.ApplyState = aggregateChangeApplyState(group.ChangeSets)
	group.CanUndo, group.CanRedo = s.liveHistoryCapabilities(group)
	group.PendingEditCount = countPendingReviewEdits(group.ChangeSets)
	group.UnresolvedCommentCount = countUnresolvedComments(group.Comments)
}

func countUnresolvedComments(comments []Comment) int {
	count := 0
	for _, comment := range comments {
		if !comment.Deleted && !comment.Resolved {
			count++
		}
	}
	return count
}

func (s *Service) assignChangeSequence(change *ChangeSet) {
	if change == nil {
		return
	}
	if change.Sequence == 0 {
		s.nextSequence++
		change.Sequence = s.nextSequence
		return
	}
	if change.Sequence > s.nextSequence {
		s.nextSequence = change.Sequence
	}
}

func (s *Service) invalidateRedoExcept(origin string) error {
	if origin == OriginRedo || origin == OriginUndo || origin == OriginReview {
		return nil
	}
	for groupID, isUndone := range s.undone {
		if !isUndone || s.redoInvalid[groupID] {
			continue
		}
		if err := s.appendAndApply(ledgerEvent{Type: eventHistoryState, GroupID: groupID, HistoryState: historyStateRedoInvalidated}); err != nil {
			return err
		}
	}
	return nil
}

func cloneChangeSet(change ChangeSet) ChangeSet {
	change.Edits = append([]AppliedEdit(nil), change.Edits...)
	for i := range change.Edits {
		change.Edits[i].Hunks = append([]Hunk(nil), change.Edits[i].Hunks...)
	}
	return change
}

func cloneGroup(group ChangeGroup) ChangeGroup {
	group.ChangeSets = append([]ChangeSet(nil), group.ChangeSets...)
	for i := range group.ChangeSets {
		group.ChangeSets[i] = cloneChangeSet(group.ChangeSets[i])
	}
	group.Comments = append([]Comment(nil), group.Comments...)
	return group
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func newID(prefix string) string {
	var random [8]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(random[:]))
}

func aggregateEditReviewStatus(edits []AppliedEdit) string {
	if len(edits) == 0 {
		return ReviewStatusPending
	}
	seen := map[string]bool{}
	for _, edit := range edits {
		status := edit.ReviewStatus
		if status == "" {
			status = ReviewStatusPending
		}
		seen[status] = true
	}
	if len(seen) == 1 {
		for status := range seen {
			return status
		}
	}
	return ReviewStatusMixed
}

func aggregateChangeReviewStatus(changes []ChangeSet) string {
	seen := map[string]bool{}
	for _, change := range changes {
		if change.Origin == OriginUndo || change.Origin == OriginRedo || change.Origin == OriginReview {
			continue
		}
		status := firstNonEmpty(change.ReviewStatus, ReviewStatusPending)
		seen[status] = true
	}
	if len(seen) == 0 {
		return ReviewStatusPending
	}
	if len(seen) == 1 {
		for status := range seen {
			return status
		}
	}
	return ReviewStatusMixed
}

func countPendingReviewEdits(changes []ChangeSet) int {
	count := 0
	for _, change := range changes {
		if change.Origin == OriginUndo || change.Origin == OriginRedo || change.Origin == OriginReview {
			continue
		}
		if change.ApplyState != ApplyStateApplied {
			continue
		}
		for _, edit := range change.Edits {
			if firstNonEmpty(edit.ReviewStatus, ReviewStatusPending) == ReviewStatusPending {
				count++
			}
		}
	}
	return count
}

func aggregateChangeApplyState(changes []ChangeSet) string {
	seenApplied := false
	seenReverted := false
	for _, change := range changes {
		if change.Origin == OriginUndo || change.Origin == OriginRedo || change.Origin == OriginReview {
			continue
		}
		switch change.ApplyState {
		case ApplyStateConflicted:
			return ApplyStateConflicted
		case ApplyStateApplied:
			seenApplied = true
		case ApplyStateReverted:
			seenReverted = true
		}
	}
	if seenApplied {
		return ApplyStateApplied
	}
	if seenReverted {
		return ApplyStateReverted
	}
	return ApplyStatePrepared
}

func (s *Service) contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
