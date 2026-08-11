package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const StateOpSourceStateRevision = "state_revision"

var ErrStateRevisionConflict = errors.New("state revision conflict")

func (s *Store) CreateStateRevision(storyID string, req CreateStateRevisionRequest) (StateRevisionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateRevisionEvent{}, err
	}
	branchID, branch, err := resolveBranch(meta, strings.TrimSpace(req.BranchID))
	if err != nil {
		return StateRevisionEvent{}, err
	}
	if err := validateStateRevisionHead(branch.Head, req.ExpectedHead); err != nil {
		return StateRevisionEvent{}, err
	}
	if len(req.Ops)+len(req.ActorOps) > maxInteractiveListItems {
		return StateRevisionEvent{}, fmt.Errorf("state revision has too many operations: %d > %d", len(req.Ops)+len(req.ActorOps), maxInteractiveListItems)
	}
	ops := normalizeStateOps(req.Ops)
	actorOps := normalizeActorStateOps(req.ActorOps)
	if len(ops) == 0 && len(actorOps) == 0 {
		return StateRevisionEvent{}, fmt.Errorf("state revision cannot be empty")
	}
	baseTurnID := nearestTurnAncestor(branch.Head, eventsByID(lines))
	if baseTurnID == "" || strings.TrimSpace(req.BaseTurnID) == "" {
		return StateRevisionEvent{}, fmt.Errorf("state revision requires a base turn")
	}
	if strings.TrimSpace(req.BaseTurnID) != baseTurnID {
		return StateRevisionEvent{}, fmt.Errorf("%w: base turn changed: expected=%s current=%s", ErrStateRevisionConflict, strings.TrimSpace(req.BaseTurnID), baseTurnID)
	}
	source := trimBytes(strings.TrimSpace(req.Source), 128)
	if source == "" {
		return StateRevisionEvent{}, fmt.Errorf("state revision source is required")
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))
	state := stateFromPath(path)
	inverseOps, inverseActorOps := buildStateRevisionInverse(state, ops, actorOps)
	event := newStateRevisionEvent(branch.Head, branchID, baseTurnID, source, StateRevisionActionApply, "", ops, actorOps, inverseOps, inverseActorOps)
	return s.appendStateRevisionLocked(storyID, meta, lines, branch, event)
}

func (s *Store) ApplyStateRevisionAction(storyID string, req StateRevisionActionRequest) (StateRevisionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.Action != StateRevisionActionUndo && req.Action != StateRevisionActionRestore {
		return StateRevisionEvent{}, fmt.Errorf("unsupported state revision action: %s", req.Action)
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateRevisionEvent{}, err
	}
	branchID, branch, err := resolveBranch(meta, strings.TrimSpace(req.BranchID))
	if err != nil {
		return StateRevisionEvent{}, err
	}
	if err := validateStateRevisionHead(branch.Head, req.ExpectedHead); err != nil {
		return StateRevisionEvent{}, err
	}
	events := eventsByID(lines)
	path, pathSet := eventPath(branch.Head, events)
	revisionID := strings.TrimSpace(req.RevisionID)
	targetRecord, ok := events[revisionID]
	if !ok || !pathSet[revisionID] || targetRecord.Envelope.Type != StoryEventTypeStateRevision {
		return StateRevisionEvent{}, fmt.Errorf("state revision does not exist on this branch: %s", revisionID)
	}
	var target StateRevisionEvent
	if err := mapToStruct(targetRecord.Raw, &target); err != nil {
		return StateRevisionEvent{}, err
	}
	if target.Action != StateRevisionActionApply {
		return StateRevisionEvent{}, fmt.Errorf("only an applied state revision can be undone or restored: %s", revisionID)
	}
	var ops []StateOp
	var actorOps []ActorStateOp
	if req.Action == StateRevisionActionUndo {
		ops = append([]StateOp(nil), target.InverseOps...)
		actorOps = append([]ActorStateOp(nil), target.InverseActorOps...)
	} else {
		ops = append([]StateOp(nil), target.Ops...)
		actorOps = append([]ActorStateOp(nil), target.ActorOps...)
	}
	if len(ops) == 0 && len(actorOps) == 0 {
		return StateRevisionEvent{}, fmt.Errorf("state revision action has no durable operations: %s", revisionID)
	}
	baseTurnID := nearestTurnAncestor(branch.Head, events)
	if baseTurnID == "" {
		return StateRevisionEvent{}, fmt.Errorf("state revision action requires a base turn")
	}
	source := trimBytes(strings.TrimSpace(req.Source), 128)
	if source == "" {
		return StateRevisionEvent{}, fmt.Errorf("state revision source is required")
	}
	state := stateFromPath(path)
	inverseOps, inverseActorOps := buildStateRevisionInverse(state, ops, actorOps)
	event := newStateRevisionEvent(branch.Head, branchID, baseTurnID, source, req.Action, revisionID, ops, actorOps, inverseOps, inverseActorOps)
	return s.appendStateRevisionLocked(storyID, meta, lines, branch, event)
}

func (s *Store) appendStateRevisionLocked(storyID string, meta StoryMeta, lines []StoryEventRecord, branch BranchMeta, event StateRevisionEvent) (StateRevisionEvent, error) {
	stampStateRevisionProvenance(event.Ops, event.ActorOps, event.ID, event.BaseTurnID)
	branch.Head = event.ID
	meta.Branches[event.BranchID] = branch
	meta.UpdatedAt = event.Ts
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return StateRevisionEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, event.Ts, 1); err != nil {
		return StateRevisionEvent{}, err
	}
	return event, nil
}

func newStateRevisionEvent(parentID, branchID, baseTurnID, source string, action StateRevisionAction, sourceRevisionID string, ops []StateOp, actorOps []ActorStateOp, inverseOps []StateOp, inverseActorOps []ActorStateOp) StateRevisionEvent {
	return StateRevisionEvent{
		V:                schemaVersion,
		Type:             StoryEventTypeStateRevision,
		ID:               newID("sr"),
		ParentID:         parentID,
		BranchID:         branchID,
		Ts:               time.Now().UTC().Format(time.RFC3339Nano),
		SchemaVersion:    stateOpSchemaVersion,
		BaseTurnID:       baseTurnID,
		Source:           source,
		Action:           action,
		SourceRevisionID: sourceRevisionID,
		Ops:              ops,
		ActorOps:         actorOps,
		InverseOps:       inverseOps,
		InverseActorOps:  inverseActorOps,
	}
}

func validateStateRevision(revision StateRevisionEvent, forWrite bool) error {
	if revision.Type != StoryEventTypeStateRevision {
		return fmt.Errorf("invalid state revision type: %q", revision.Type)
	}
	if strings.TrimSpace(revision.BaseTurnID) == "" || strings.TrimSpace(revision.Source) == "" {
		return fmt.Errorf("state revision is missing source or base turn")
	}
	switch revision.Action {
	case StateRevisionActionApply, StateRevisionActionUndo, StateRevisionActionRestore:
	default:
		return fmt.Errorf("invalid state revision action: %q", revision.Action)
	}
	if revision.Action != StateRevisionActionApply && strings.TrimSpace(revision.SourceRevisionID) == "" {
		return fmt.Errorf("state revision action is missing source_revision_id")
	}
	if len(revision.Ops)+len(revision.ActorOps) == 0 {
		return fmt.Errorf("state revision cannot be empty")
	}
	validate := validateStateDelta
	if forWrite {
		validate = validateStateDeltaForWrite
	}
	if err := validate(StateDelta{SchemaVersion: revision.SchemaVersion, Ops: revision.Ops, ActorOps: revision.ActorOps}); err != nil {
		return err
	}
	return validate(StateDelta{SchemaVersion: revision.SchemaVersion, Ops: revision.InverseOps, ActorOps: revision.InverseActorOps})
}

func validateStateRevisionHead(current, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return fmt.Errorf("expected_head_id is required")
	}
	if current != expected {
		return fmt.Errorf("%w: branch head changed: expected=%s current=%s", ErrStateRevisionConflict, expected, current)
	}
	return nil
}

func buildStateRevisionInverse(state map[string]any, ops []StateOp, actorOps []ActorStateOp) ([]StateOp, []ActorStateOp) {
	inverseOps := make([]StateOp, 0, len(ops))
	seenPaths := map[string]bool{}
	for _, op := range ops {
		path := canonicalStatePath(op.Path)
		if seenPaths[path] {
			continue
		}
		seenPaths[path] = true
		value, exists := statePathValue(state, path)
		if exists {
			inverseOps = append(inverseOps, StateOp{Op: "set", Path: path, Value: cloneStateRevisionValue(value), Reason: "restore pre-revision state"})
		} else {
			inverseOps = append(inverseOps, StateOp{Op: "unset", Path: path, Reason: "restore pre-revision state"})
		}
	}
	inverseActorOps := make([]ActorStateOp, 0, len(actorOps))
	seenActorFields := map[string]bool{}
	for _, op := range actorOps {
		actorID := normalizeStatePanelActorID(op.ActorID)
		fieldID := normalizeActorStateFieldName(op.FieldID)
		key := actorID + "\x00" + fieldID
		if seenActorFields[key] {
			continue
		}
		seenActorFields[key] = true
		value, exists := stateRevisionActorFieldValue(state, actorID, fieldID)
		if exists {
			inverseActorOps = append(inverseActorOps, ActorStateOp{Op: "set", ActorID: actorID, FieldID: fieldID, Value: cloneStateRevisionValue(value), Reason: "restore pre-revision state"})
		} else {
			inverseActorOps = append(inverseActorOps, ActorStateOp{Op: "unset", ActorID: actorID, FieldID: fieldID, Reason: "restore pre-revision state"})
		}
	}
	return inverseOps, inverseActorOps
}

func stampStateRevisionProvenance(ops []StateOp, actorOps []ActorStateOp, revisionID, baseTurnID string) {
	for index := range ops {
		ops[index].SourceKind = StateOpSourceStateRevision
		ops[index].SourceID = revisionID
		ops[index].SourceTurnID = baseTurnID
	}
	for index := range actorOps {
		actorOps[index].SourceKind = StateOpSourceStateRevision
		actorOps[index].SourceID = revisionID
		actorOps[index].SourceTurnID = baseTurnID
	}
}

func statePathValue(state map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	current := state
	for index, part := range parts {
		value, ok := current[part]
		if !ok {
			return nil, false
		}
		if index == len(parts)-1 {
			return value, true
		}
		current, ok = value.(map[string]any)
		if !ok {
			return nil, false
		}
	}
	return nil, false
}

func stateRevisionActorFieldValue(state map[string]any, actorID, fieldID string) (any, bool) {
	actors, _ := state[actorStateRoot].(map[string]any)
	actor, _ := actors[actorID].(map[string]any)
	fields, _ := actor["state"].(map[string]any)
	value, ok := fields[fieldID]
	return value, ok
}

func cloneStateRevisionValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return value
	}
	return cloned
}
