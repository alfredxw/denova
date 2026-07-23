package interactive

import (
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

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return StateDeltaEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		parentID = branch.Head
	}
	if parentID == "" {
		return StateDeltaEvent{}, fmt.Errorf("状态变化缺少所属回合")
	}
	if parentID != branch.Head {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不是当前分支头: turn=%s head=%s", parentID, branch.Head)
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := newStateDeltaEventWithActorOps(parentID, parentID, branchID, now, ops, actorOps)
	updated := false
	for i := range lines {
		raw := lines[i].Raw
		if lines[i].Envelope.ID != parentID || lines[i].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(raw, &turn); err != nil {
			return StateDeltaEvent{}, err
		}
		nextOps := append([]StateOp(nil), ops...)
		nextActorOps := append([]ActorStateOp(nil), actorOps...)
		if turn.StateDelta != nil && len(turn.StateDelta.Ops) > 0 {
			nextOps = append(append([]StateOp(nil), turn.StateDelta.Ops...), nextOps...)
		}
		if turn.StateDelta != nil && len(turn.StateDelta.ActorOps) > 0 {
			nextActorOps = append(append([]ActorStateOp(nil), turn.StateDelta.ActorOps...), nextActorOps...)
		}
		raw["state_delta"] = newStateDeltaWithActorOps(nextOps, nextActorOps)
		raw["state_status"] = "ready"
		delete(raw, "state_error")
		updated = true
		break
	}
	if !updated {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不存在: %s", parentID)
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateDeltaEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 0); err != nil {
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

	meta, lines, err := s.readStoryLocked(storyID)
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
		return fmt.Errorf("状态失败标记缺少所属回合")
	}
	errText := strings.TrimSpace(req.Error)
	if errText == "" {
		errText = "状态生成失败"
	}
	updated := false
	for _, record := range lines {
		raw := record.Raw
		if record.Envelope.ID != parentID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		raw["state_status"] = "failed"
		raw["state_error"] = errText
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("状态失败标记所属回合不存在: %s", parentID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
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
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return RuleResolution{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return RuleResolution{}, err
	}
	path, pathSet := eventPath(branch.Head, eventsByID(lines))
	var target TurnEvent
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			continue
		}
		if strings.TrimSpace(req.TurnID) != "" && turn.ID != strings.TrimSpace(req.TurnID) {
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
	state := stateBeforeTurn(path, target.ID)
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
	updated := false
	for i := range lines {
		if lines[i].Envelope.ID != target.ID || !pathSet[target.ID] {
			continue
		}
		lines[i].Raw["rule_resolution"] = next
		existingOps := []StateOp{}
		existingActorOps := []ActorStateOp{}
		if target.StateDelta != nil {
			existingOps = append(existingOps, target.StateDelta.Ops...)
			existingActorOps = append(existingActorOps, target.StateDelta.ActorOps...)
		}
		nextOps := append(removeRuleResolutionStateOps(existingOps, target.RuleResolution.ID), ruleOps...)
		nextActorOps := append(removeRuleResolutionActorOps(existingActorOps, target.RuleResolution.ID), ruleActorOps...)
		if len(nextOps) > 0 || len(nextActorOps) > 0 {
			for _, op := range nextOps {
				if err := validateStateOp(op); err != nil {
					return RuleResolution{}, err
				}
			}
			lines[i].Raw["state_delta"] = newStateDeltaWithActorOps(nextOps, nextActorOps)
			lines[i].Raw["state_status"] = "ready"
			delete(lines[i].Raw, "state_error")
		} else {
			delete(lines[i].Raw, "state_delta")
			lines[i].Raw["state_status"] = "pending"
			delete(lines[i].Raw, "state_error")
		}
		if terminalOutcome != nil {
			lines[i].Raw["terminal_outcome"] = terminalOutcome
		} else {
			delete(lines[i].Raw, "terminal_outcome")
		}
		updated = true
		break
	}
	if !updated {
		return RuleResolution{}, fmt.Errorf("规则结算所属回合不存在: %s", target.ID)
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return RuleResolution{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 0); err != nil {
		return RuleResolution{}, err
	}
	return next, nil
}
