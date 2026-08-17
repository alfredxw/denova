package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"fmt"
	"strings"
	"time"
)

func (s *Store) AppendStateDelta(storyID string, req AppendStateDeltaRequest) (StateDeltaEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	defer releaseStory()
	if len(req.Ops) == 0 && len(req.ActorOps) == 0 {
		return StateDeltaEvent{}, fmt.Errorf("状态变化不能为空")
	}

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return StateDeltaEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		parentID = latestLogicalTurnID(meta, lines, branchID)
	}
	if parentID == "" {
		return StateDeltaEvent{}, fmt.Errorf("状态变化缺少所属回合")
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, parentID); err != nil {
		return StateDeltaEvent{}, err
	}
	ops := normalizeStateOps(req.Ops)
	actorOps := normalizeActorStateOps(req.ActorOps)
	if len(ops) == 0 && len(actorOps) == 0 {
		return StateDeltaEvent{}, fmt.Errorf("状态变化不能为空")
	}
	for _, op := range ops {
		if err := validateStateOp(op); err != nil {
			return StateDeltaEvent{}, err
		}
	}
	for _, op := range actorOps {
		if err := validateActorStateOp(op); err != nil {
			return StateDeltaEvent{}, err
		}
	}
	projected, err := projectStoryEventOverlays(lines)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	record, ok := eventsByID(projected)[parentID]
	if !ok || record.Envelope.Type != StoryEventTypeTurn {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不存在: %s", parentID)
	}
	var turn TurnEvent
	if err := mapToStruct(record.Raw, &turn); err != nil {
		return StateDeltaEvent{}, err
	}
	nextOps := append([]interactivestate.Op(nil), ops...)
	nextActorOps := append([]ActorStateOp(nil), actorOps...)
	if turn.StateDelta != nil {
		nextOps = append(append([]interactivestate.Op(nil), turn.StateDelta.Ops...), nextOps...)
		nextActorOps = append(append([]ActorStateOp(nil), turn.StateDelta.ActorOps...), nextActorOps...)
	}
	delta := newStateDeltaWithActorOps(nextOps, nextActorOps)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	revision := TurnStateRevisedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnStateRevised, ID: newID("tsr"),
		ParentID: parentID, BranchID: branchID, Ts: now, TurnID: parentID,
		StateDelta: &delta, StateStatus: "ready", Reason: "state_settled",
	}
	event := newStateDeltaEventWithActorOps(revision.ID, parentID, branchID, now, ops, actorOps)
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, revision); err != nil {
		return StateDeltaEvent{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return StateDeltaEvent{}, err
	}
	return event, nil
}

func (s *Store) MarkStateFailed(storyID string, req MarkStateFailedRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		parentID = latestLogicalTurnID(meta, lines, branchID)
	}
	if parentID == "" {
		return fmt.Errorf("状态失败标记缺少所属回合")
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, parentID); err != nil {
		return err
	}
	errText := strings.TrimSpace(req.Error)
	if errText == "" {
		errText = "状态生成失败"
	}
	projected, err := projectStoryEventOverlays(lines)
	if err != nil {
		return err
	}
	record, ok := eventsByID(projected)[parentID]
	if !ok || record.Envelope.Type != StoryEventTypeTurn {
		return fmt.Errorf("状态失败标记所属回合不存在: %s", parentID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	revision := TurnStateRevisedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnStateRevised, ID: newID("tsr"),
		ParentID: parentID, BranchID: branchID, Ts: now, TurnID: parentID,
		StateStatus: "failed", StateError: errText, Reason: "state_failed",
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, revision); err != nil {
		return err
	}
	return s.syncStorySummaryLocked(storyID)
}

func (s *Store) RerollRuleResolution(storyID, resolutionID string, req RuleResolutionRerollRequest) (RuleResolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return RuleResolution{}, err
	}
	defer releaseStory()

	resolutionID = strings.TrimSpace(resolutionID)
	if resolutionID == "" {
		return RuleResolution{}, fmt.Errorf("规则结算 ID 不能为空")
	}
	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return RuleResolution{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return RuleResolution{}, err
	}
	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		turnID = latestLogicalTurnID(meta, lines, branchID)
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, turnID); err != nil {
		return RuleResolution{}, err
	}
	projected, err := projectStoryEventOverlays(lines)
	if err != nil {
		return RuleResolution{}, err
	}
	path, _ := eventPath(branch.Head, eventsByID(projected))
	var target TurnEvent
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			continue
		}
		if turn.ID != turnID {
			continue
		}
		if turn.RuleResolution != nil && turn.RuleResolution.ID == resolutionID {
			target = turn
			break
		}
	}
	if target.ID == "" {
		return RuleResolution{}, fmt.Errorf("当前分支路径中未找到规则结算: %s", resolutionID)
	}
	request := NormalizeTurnCheckRequest(target.RuleResolution.Request)
	branchProjection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return RuleResolution{}, err
	}
	state := cloneStoryState(branchProjection.StateBeforeLatest)
	director := s.storyDirectorForMeta(meta)
	actorState := actorStateSystemFromSnapshot(meta.ActorStateSchema, director.ActorState)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	next, err := ResolveTurnRulesWithDirector(storyID, branchID, state, director, request)
	if err != nil {
		return RuleResolution{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next.CreatedAt = now
	next.ID = newID("rr")
	ruleOps, ruleActorOps := applyRuleStateConsumptionV2(state, actorState, target.ID, &next, director.Strategy.RuleStateConsumptionMode)
	terminalOutcome := terminalOutcomeFromRuleResolution(next, target.ID, target.Narrative)
	existingOps := []interactivestate.Op{}
	existingActorOps := []ActorStateOp{}
	if target.StateDelta != nil {
		existingOps = append(existingOps, target.StateDelta.Ops...)
		existingActorOps = append(existingActorOps, target.StateDelta.ActorOps...)
	}
	nextOps := append(removeRuleResolutionStateOps(existingOps, target.RuleResolution.ID), ruleOps...)
	nextActorOps := append(removeRuleResolutionActorOps(existingActorOps, target.RuleResolution.ID), ruleActorOps...)
	for _, op := range nextOps {
		if err := validateStateOp(op); err != nil {
			return RuleResolution{}, err
		}
	}
	for _, op := range nextActorOps {
		if err := validateActorStateOp(op); err != nil {
			return RuleResolution{}, err
		}
	}
	revision := TurnStateRevisedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnStateRevised, ID: newID("tsr"),
		ParentID: target.ID, BranchID: branchID, Ts: now, TurnID: target.ID,
		RuleResolution: &next, Reason: "rule_rerolled",
	}
	if len(nextOps) > 0 || len(nextActorOps) > 0 {
		delta := newStateDeltaWithActorOps(nextOps, nextActorOps)
		revision.StateDelta = &delta
		revision.StateStatus = "ready"
	} else {
		revision.ClearStateDelta = true
		revision.StateStatus = "pending"
	}
	if terminalOutcome != nil {
		revision.TerminalOutcome = terminalOutcome
	} else {
		revision.ClearTerminalOutcome = true
	}
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, revision); err != nil {
		return RuleResolution{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return RuleResolution{}, err
	}
	return next, nil
}
