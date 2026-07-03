package interactive

import (
	"fmt"
	"strings"
	"time"
)

const (
	directorPatchSourceManual  = "manual"
	directorPatchSourceRebuild = "rebuild"
	directorPatchSourceForce   = "force_event"
	directorPatchSourceDisable = "disable_event"
	directorPatchSourceTurn    = "turn_auto"
)

// UpdateDirectorStateRequest patches the story-level director plan. Slice and
// map fields replace existing values when present so clients can intentionally
// clear queues without ambiguous nil semantics.
type UpdateDirectorStateRequest struct {
	BranchID            string             `json:"branch_id,omitempty"`
	Enabled             *bool              `json:"enabled,omitempty"`
	SpoilerMode         *string            `json:"spoiler_mode,omitempty"`
	MainArc             *string            `json:"main_arc,omitempty"`
	StagePlan           *string            `json:"stage_plan,omitempty"`
	BeatQueue           *[]DirectorBeat    `json:"beat_queue,omitempty"`
	EventQueue          *[]DirectorEvent   `json:"event_queue,omitempty"`
	Foreshadowing       *[]DirectorThread  `json:"foreshadowing,omitempty"`
	PotentialCharacters *[]DirectorThread  `json:"potential_characters,omitempty"`
	BranchPatches       *map[string]string `json:"branch_patches,omitempty"`
	ForcedEvents        *[]string          `json:"forced_events,omitempty"`
	DisabledEvents      *[]string          `json:"disabled_events,omitempty"`
	LastDirectorRun     *DirectorRunStatus `json:"last_director_run,omitempty"`
	Source              string             `json:"source,omitempty"`
	Summary             string             `json:"summary,omitempty"`
}

type RebuildDirectorStateRequest struct {
	BranchID     string           `json:"branch_id,omitempty"`
	Source       string           `json:"source,omitempty"`
	EventCatalog *[]DirectorEvent `json:"-"`
}

type DirectorEventActionRequest struct {
	BranchID string         `json:"branch_id,omitempty"`
	Event    *DirectorEvent `json:"event,omitempty"`
	Reason   string         `json:"reason,omitempty"`
	Source   string         `json:"source,omitempty"`
}

// DirectorPatchEvent is an append-only audit row for director state changes.
// It does not advance BranchMeta.Head, because director planning is background
// state rather than a visible story node.
type DirectorPatchEvent struct {
	V             int           `json:"v"`
	Type          string        `json:"type"`
	ID            string        `json:"id"`
	ParentID      any           `json:"parent_id,omitempty"`
	BranchID      string        `json:"branch_id"`
	Ts            string        `json:"ts"`
	Source        string        `json:"source,omitempty"`
	Summary       string        `json:"summary,omitempty"`
	DirectorState DirectorState `json:"director_state"`
}

func (s *Store) DirectorState(storyID, branchID string) (DirectorState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return DirectorState{}, err
	}
	if _, _, err := resolveBranch(meta, branchID); err != nil {
		return DirectorState{}, err
	}
	return NormalizeDirectorState(meta.DirectorState), nil
}

func (s *Store) UpdateDirectorState(storyID string, req UpdateDirectorStateRequest) (DirectorState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, branchID, branch, err := s.directorStoryForUpdateLocked(storyID, req.BranchID)
	if err != nil {
		return DirectorState{}, err
	}
	next := applyDirectorStatePatch(NormalizeDirectorState(meta.DirectorState), req)
	return s.persistDirectorStateLocked(storyID, meta, lines, branchID, branch, next, firstNonEmpty(req.Source, directorPatchSourceManual), req.Summary)
}

func (s *Store) RebuildDirectorState(storyID string, req RebuildDirectorStateRequest) (DirectorState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, branchID, branch, err := s.directorStoryForUpdateLocked(storyID, req.BranchID)
	if err != nil {
		return DirectorState{}, err
	}
	currentTurn := latestTurnForBranchHead(lines, branch.Head)
	next := buildRebuiltDirectorState(meta, branchID, NormalizeDirectorState(meta.DirectorState), currentTurn, req.EventCatalog)
	summary := "已基于当前分支重建导演计划。"
	if currentTurn != nil && strings.TrimSpace(currentTurn.User) != "" {
		summary = "已围绕最近行动重建导演计划：" + trimBytes(currentTurn.User, 160)
	}
	return s.persistDirectorStateLocked(storyID, meta, lines, branchID, branch, next, firstNonEmpty(req.Source, directorPatchSourceRebuild), summary)
}

func (s *Store) ForceDirectorEvent(storyID, eventID string, req DirectorEventActionRequest) (DirectorState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return DirectorState{}, fmt.Errorf("导演事件 ID 不能为空")
	}
	meta, lines, branchID, branch, err := s.directorStoryForUpdateLocked(storyID, req.BranchID)
	if err != nil {
		return DirectorState{}, err
	}
	next := NormalizeDirectorState(meta.DirectorState)
	event := directorEventForAction(eventID, req.Event)
	event.Status = "forced"
	event.Enabled = true
	next.ForcedEvents = appendUniqueString(next.ForcedEvents, event.ID)
	next.DisabledEvents = removeString(next.DisabledEvents, event.ID)
	next.EventQueue = upsertDirectorEvent(next.EventQueue, event)
	next.LastDirectorRun = &DirectorRunStatus{
		Status:    "ready",
		Summary:   firstNonEmpty(strings.TrimSpace(req.Reason), "已强制安排事件："+event.Name),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return s.persistDirectorStateLocked(storyID, meta, lines, branchID, branch, next, firstNonEmpty(req.Source, directorPatchSourceForce), next.LastDirectorRun.Summary)
}

func (s *Store) DisableDirectorEvent(storyID, eventID string, req DirectorEventActionRequest) (DirectorState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return DirectorState{}, fmt.Errorf("导演事件 ID 不能为空")
	}
	meta, lines, branchID, branch, err := s.directorStoryForUpdateLocked(storyID, req.BranchID)
	if err != nil {
		return DirectorState{}, err
	}
	next := NormalizeDirectorState(meta.DirectorState)
	next.DisabledEvents = appendUniqueString(next.DisabledEvents, eventID)
	next.ForcedEvents = removeString(next.ForcedEvents, eventID)
	next.EventQueue = disableDirectorEvent(next.EventQueue, eventID)
	next.LastDirectorRun = &DirectorRunStatus{
		Status:    "ready",
		Summary:   firstNonEmpty(strings.TrimSpace(req.Reason), "已禁用事件："+eventID),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return s.persistDirectorStateLocked(storyID, meta, lines, branchID, branch, next, firstNonEmpty(req.Source, directorPatchSourceDisable), next.LastDirectorRun.Summary)
}

func (s *Store) directorStoryForUpdateLocked(storyID, branchID string) (StoryMeta, []StoryEventRecord, string, BranchMeta, error) {
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryMeta{}, nil, "", BranchMeta{}, err
	}
	resolvedBranchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return StoryMeta{}, nil, "", BranchMeta{}, err
	}
	return meta, lines, resolvedBranchID, branch, nil
}

func (s *Store) persistDirectorStateLocked(storyID string, meta StoryMeta, lines []StoryEventRecord, branchID string, branch BranchMeta, next DirectorState, source, summary string) (DirectorState, error) {
	next = NormalizeDirectorState(next)
	if branchID != "" && branchID != "main" {
		if next.BranchPatches == nil {
			next.BranchPatches = map[string]string{}
		}
		if strings.TrimSpace(next.BranchPatches[branchID]) == "" {
			next.BranchPatches[branchID] = "该分支继承祖先导演计划，后续导演变更以分支补丁方式记录。"
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.DirectorState = next
	meta.UpdatedAt = now
	event := DirectorPatchEvent{
		V:             schemaVersion,
		Type:          StoryEventTypeDirectorPatch,
		ID:            newID("dp"),
		ParentID:      parentIDForDirectorPatch(branch.Head),
		BranchID:      branchID,
		Ts:            now,
		Source:        strings.TrimSpace(source),
		Summary:       trimBytes(summary, maxTurnBriefTextBytes),
		DirectorState: next,
	}
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return DirectorState{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return DirectorState{}, err
	}
	return next, nil
}

func directorPatchAfterTurn(meta StoryMeta, branchID string, turn TurnEvent, ts string) (DirectorPatchEvent, DirectorState, bool) {
	state := NormalizeDirectorState(meta.DirectorState)
	if !state.Enabled {
		return DirectorPatchEvent{}, state, false
	}
	brief, hasBrief := directorBriefForTurn(turn)
	hasResolution := turn.RuleResolution != nil
	if !hasBrief && !hasResolution {
		return DirectorPatchEvent{}, state, false
	}
	next := updateDirectorStateAfterTurn(meta, branchID, state, turn, brief, hasBrief, ts)
	summary := "后台导演已根据本回合审计更新计划。"
	if hasBrief && strings.TrimSpace(brief.TurnGoal) != "" {
		summary = "后台导演已承接本回合目标：" + trimBytes(brief.TurnGoal, 180)
	}
	event := DirectorPatchEvent{
		V:             schemaVersion,
		Type:          StoryEventTypeDirectorPatch,
		ID:            newID("dp"),
		ParentID:      turn.ID,
		BranchID:      branchID,
		Ts:            ts,
		Source:        directorPatchSourceTurn,
		Summary:       summary,
		DirectorState: next,
	}
	return event, next, true
}

func directorBriefForTurn(turn TurnEvent) (TurnBrief, bool) {
	if turn.TurnBrief != nil {
		return NormalizeTurnBrief(*turn.TurnBrief), true
	}
	if turn.RuleResolution != nil {
		brief := NormalizeTurnBrief(turn.RuleResolution.AcceptedBrief)
		if strings.TrimSpace(brief.UserAction) != "" || strings.TrimSpace(brief.TurnGoal) != "" {
			return brief, true
		}
	}
	return TurnBrief{}, false
}

func updateDirectorStateAfterTurn(meta StoryMeta, branchID string, state DirectorState, turn TurnEvent, brief TurnBrief, hasBrief bool, ts string) DirectorState {
	if strings.TrimSpace(state.MainArc) == "" {
		state.MainArc = defaultDirectorMainArc(meta)
	}
	if turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal {
		state.StagePlan = "当前分支已进入终局结果：" + firstNonEmpty(turn.TerminalOutcome.Reason, turn.TerminalOutcome.Type)
	} else if hasBrief {
		state.StagePlan = firstNonEmpty(brief.StateExpectation, brief.TurnGoal, state.StagePlan)
	}
	if hasBrief {
		state.BeatQueue = directorBeatsAfterTurn(state.BeatQueue, turn, brief)
		state.EventQueue = directorEventsAfterTurn(state.EventQueue, state.DisabledEvents, turn, brief)
		state.Foreshadowing = directorForeshadowingAfterTurn(state.Foreshadowing, turn, brief)
	}
	if state.BranchPatches == nil {
		state.BranchPatches = map[string]string{}
	}
	if branchID != "main" && strings.TrimSpace(state.BranchPatches[branchID]) == "" {
		state.BranchPatches[branchID] = "本分支已根据回合审计独立更新导演计划。"
	}
	state.LastDirectorRun = &DirectorRunStatus{
		Status:    "ready",
		Summary:   directorRunSummaryForTurn(turn, brief, hasBrief),
		UpdatedAt: ts,
	}
	return NormalizeDirectorState(state)
}

func directorBeatsAfterTurn(existing []DirectorBeat, turn TurnEvent, brief TurnBrief) []DirectorBeat {
	nextBeat := DirectorBeat{
		ID:       "beat_after_" + turn.ID,
		Summary:  firstNonEmpty(brief.TurnGoal, "承接本回合行动后果"),
		Pressure: firstNonEmpty(brief.Pressure, "根据本回合选择安排下一层压力"),
		Payoff:   firstNonEmpty(brief.StateExpectation, ruleConstraintSummary(turn.RuleResolution), "给出可见后果、回报或代价"),
		Status:   "planned",
	}
	out := []DirectorBeat{nextBeat}
	for _, beat := range existing {
		if beat.ID == nextBeat.ID || beat.Status == "done" {
			continue
		}
		out = append(out, beat)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func directorEventsAfterTurn(existing []DirectorEvent, disabled []string, turn TurnEvent, brief TurnBrief) []DirectorEvent {
	events := make([]DirectorEvent, 0, len(existing)+len(brief.EventIntents))
	for _, event := range existing {
		if event.NextEligibleAfterTurns > 0 {
			event.NextEligibleAfterTurns--
		}
		events = upsertDirectorEvent(events, event)
	}
	for _, intent := range brief.EventIntents {
		event := directorEventForAction(intent, nil)
		if stringInList(event.ID, disabled) || stringInList(intent, disabled) {
			continue
		}
		event.Status = "planned"
		event.Enabled = true
		event.LastTriggeredTurnID = turn.ID
		event.NextEligibleAfterTurns = event.CooldownTurns
		event.DirectorInstructionNote = "Interactive Agent 本回合希望推进：" + intent
		events = upsertDirectorEvent(events, event)
	}
	return events
}

func directorForeshadowingAfterTurn(existing []DirectorThread, turn TurnEvent, brief TurnBrief) []DirectorThread {
	note := firstNonEmpty(brief.ContinuityNotes, brief.StateExpectation)
	if note == "" {
		return existing
	}
	thread := DirectorThread{
		ID:      "thread_after_" + turn.ID,
		Title:   "本回合连续性",
		Status:  "open",
		Summary: note,
	}
	out := []DirectorThread{thread}
	for _, existingThread := range existing {
		if existingThread.ID == thread.ID {
			continue
		}
		out = append(out, existingThread)
		if len(out) >= maxTurnBriefListItems {
			break
		}
	}
	return out
}

func directorRunSummaryForTurn(turn TurnEvent, brief TurnBrief, hasBrief bool) string {
	if turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal {
		return "已记录终局候选/终局结果：" + firstNonEmpty(turn.TerminalOutcome.Reason, turn.TerminalOutcome.Type)
	}
	if intents := eventIntentsString(brief); hasBrief && strings.TrimSpace(intents) != "" {
		return "已更新事件意图：" + intents
	}
	if hasBrief && strings.TrimSpace(brief.TurnGoal) != "" {
		return "已更新近期节拍：" + trimBytes(brief.TurnGoal, 180)
	}
	return "已根据本回合规则审计更新导演状态。"
}

func eventIntentsString(brief TurnBrief) string {
	return strings.Join(normalizeStringListLimit(brief.EventIntents, 4), "、")
}

func ruleConstraintSummary(resolution *RuleResolution) string {
	if resolution == nil {
		return ""
	}
	if resolution.TerminalCandidate != nil {
		return "遵守终局候选：" + firstNonEmpty(resolution.TerminalCandidate.Reason, resolution.TerminalCandidate.Type)
	}
	if len(resolution.RuleConstraints) > 0 {
		return strings.Join(normalizeStringListLimit(resolution.RuleConstraints, 3), "；")
	}
	return ""
}

func applyDirectorStatePatch(state DirectorState, req UpdateDirectorStateRequest) DirectorState {
	if req.Enabled != nil {
		state.Enabled = *req.Enabled
	}
	if req.SpoilerMode != nil {
		state.SpoilerMode = *req.SpoilerMode
	}
	if req.MainArc != nil {
		state.MainArc = *req.MainArc
	}
	if req.StagePlan != nil {
		state.StagePlan = *req.StagePlan
	}
	if req.BeatQueue != nil {
		state.BeatQueue = append([]DirectorBeat(nil), (*req.BeatQueue)...)
	}
	if req.EventQueue != nil {
		state.EventQueue = append([]DirectorEvent(nil), (*req.EventQueue)...)
	}
	if req.Foreshadowing != nil {
		state.Foreshadowing = append([]DirectorThread(nil), (*req.Foreshadowing)...)
	}
	if req.PotentialCharacters != nil {
		state.PotentialCharacters = append([]DirectorThread(nil), (*req.PotentialCharacters)...)
	}
	if req.BranchPatches != nil {
		state.BranchPatches = cloneStringMap(*req.BranchPatches)
	}
	if req.ForcedEvents != nil {
		state.ForcedEvents = append([]string(nil), (*req.ForcedEvents)...)
	}
	if req.DisabledEvents != nil {
		state.DisabledEvents = append([]string(nil), (*req.DisabledEvents)...)
	}
	if req.LastDirectorRun != nil {
		run := *req.LastDirectorRun
		state.LastDirectorRun = &run
	}
	return state
}

func buildRebuiltDirectorState(meta StoryMeta, branchID string, state DirectorState, currentTurn *TurnEvent, eventCatalog *[]DirectorEvent) DirectorState {
	state.Enabled = true
	if strings.TrimSpace(state.MainArc) == "" {
		state.MainArc = defaultDirectorMainArc(meta)
	}
	state.StagePlan = defaultDirectorStagePlan(meta, currentTurn)
	state.BeatQueue = rebuiltDirectorBeats(currentTurn)
	state.EventQueue = rebuiltDirectorEvents(state.EventQueue, state.ForcedEvents, state.DisabledEvents, eventCatalog)
	if len(state.Foreshadowing) == 0 {
		state.Foreshadowing = []DirectorThread{{
			ID:      "thread_core_hook",
			Title:   "核心伏笔",
			Status:  "open",
			Summary: firstNonEmpty(trimBytes(meta.Origin, 320), "围绕主角初始处境保留一个可回收的核心真相。"),
		}}
	}
	if state.BranchPatches == nil {
		state.BranchPatches = map[string]string{}
	}
	if branchID != "main" && strings.TrimSpace(state.BranchPatches[branchID]) == "" {
		state.BranchPatches[branchID] = "重建时确认该分支继续继承主线计划，并允许根据分支选择调整事件顺序。"
	}
	state.LastDirectorRun = &DirectorRunStatus{
		Status:    "ready",
		Summary:   "导演计划已重建，后续互动回合可读取有界摘要。",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return state
}

func defaultDirectorMainArc(meta StoryMeta) string {
	title := strings.TrimSpace(meta.Title)
	origin := strings.TrimSpace(meta.Origin)
	if title != "" && origin != "" {
		return title + "：从当前困局出发，围绕成长、选择代价与阶段性反转推进长期主线。"
	}
	if title != "" {
		return title + "：围绕主角目标、阻力、代价和阶段性反转推进长期主线。"
	}
	if origin != "" {
		return trimBytes(origin, 240) + "：延展为可分阶段推进的长期主线。"
	}
	return "围绕主角长期目标、阶段阻力、关键代价和伏笔回收推进主线。"
}

func defaultDirectorStagePlan(meta StoryMeta, currentTurn *TurnEvent) string {
	if currentTurn != nil {
		action := trimBytes(currentTurn.User, 240)
		if action != "" {
			return "承接用户最近行动“" + action + "”，安排后果、压力升级、可见选择与一个可回收线索。"
		}
	}
	if origin := strings.TrimSpace(meta.Origin); origin != "" {
		return "从开局设定推进第一阶段冲突：" + trimBytes(origin, 240)
	}
	return "建立当前篇章目标，安排一次压力升级和一次可见回报。"
}

func rebuiltDirectorBeats(currentTurn *TurnEvent) []DirectorBeat {
	action := "用户当前选择"
	if currentTurn != nil && strings.TrimSpace(currentTurn.User) != "" {
		action = trimBytes(currentTurn.User, 120)
	}
	return []DirectorBeat{
		{ID: "beat_followup", Summary: "承接“" + action + "”的直接后果", Pressure: "让选择产生可见风险或机会", Payoff: "给出下一步明确抓手", Status: "planned"},
		{ID: "beat_escalate", Summary: "让外部阻力主动逼近", Pressure: "通过对手、规则、资源或时间限制推高危机", Payoff: "制造一次爽点或反转机会", Status: "planned"},
		{ID: "beat_reveal", Summary: "回收一个线索并打开新问题", Pressure: "让真相不完全安全", Payoff: "提供成长、资源、关系或情报回报", Status: "planned"},
	}
}

func rebuiltDirectorEvents(existing []DirectorEvent, forced, disabled []string, eventCatalog *[]DirectorEvent) []DirectorEvent {
	events := make([]DirectorEvent, 0, maxTurnBriefListItems)
	for _, event := range existing {
		if stringInList(event.ID, disabled) {
			continue
		}
		events = upsertDirectorEvent(events, event)
	}
	for _, id := range forced {
		if stringInList(id, disabled) {
			continue
		}
		events = upsertDirectorEvent(events, directorEventForAction(id, nil))
	}
	templates := DefaultDirectorEventTemplates()
	if eventCatalog != nil {
		templates = *eventCatalog
	}
	for _, event := range templates {
		if len(events) >= 8 {
			break
		}
		if stringInList(event.ID, disabled) {
			continue
		}
		events = upsertDirectorEvent(events, event)
	}
	return events
}

func DefaultDirectorEventTemplates() []DirectorEvent {
	return []DirectorEvent{
		directorEventTemplate("face_slap", "打脸反转", "打脸", "让轻视主角的一方在公开场合被事实反证。"),
		directorEventTemplate("hidden_strength", "扮猪吃虎", "扮猪吃虎", "先建立低估，再用合理证据释放隐藏能力。"),
		directorEventTemplate("fortuitous_encounter", "奇遇", "奇遇", "在探索或困局中安排稀缺机缘，但附带代价或选择。"),
		directorEventTemplate("secret_realm", "秘境开启", "秘境", "引入高风险封闭场景，提供资源、秘密和竞争者。"),
		directorEventTemplate("heaven_sent", "天降变局", "天降", "通过突然到来的角色、资源或危机改变局势。"),
		directorEventTemplate("accident", "意外事故", "意外", "用非预期事件打断线性推进，并暴露隐藏矛盾。"),
		directorEventTemplate("world_event", "世界事件", "世界事件", "让远方大势影响当前选择，扩大故事格局。"),
		directorEventTemplate("conflict", "正面冲突", "冲突", "让目标与阻力直接碰撞，制造可结算的胜负与代价。"),
		directorEventTemplate("academy", "学院压力", "学院", "围绕师承、规训、资源分配和同辈竞争制造阶段目标。"),
		directorEventTemplate("contest", "比拼", "比拼", "安排明确规则、观众预期、胜负奖励和失败后果。"),
		directorEventTemplate("ranking", "排行变化", "排行", "用榜单、名次或评价体系外化成长与压力。"),
		directorEventTemplate("romance", "恋爱推进", "恋爱", "通过误解、协作、危险或选择推动情感关系变化。"),
		directorEventTemplate("rescue", "英雄救美", "英雄救美", "安排救援场景，但保持被救者能动性和后续关系代价。"),
		directorEventTemplate("misunderstanding", "误会与消解", "误会与消解", "制造合理误读，并设置可被行动澄清的证据。"),
		directorEventTemplate("comeback", "逆袭节点", "逆袭", "让长期压制在一次行动中获得阶段性回报。"),
		directorEventTemplate("revenge", "复仇推进", "复仇", "推进仇怨线索、代价、底线与阶段目标。"),
		directorEventTemplate("farming", "种田经营", "种田", "通过建设、产出、扩张和外部威胁推动长期积累。"),
		directorEventTemplate("resource_management", "资源经营", "资源经营", "围绕资源稀缺、投入产出和机会成本制造选择。"),
		directorEventTemplate("power_pressure", "势力压迫", "势力压迫", "让组织、家族、宗门或集团施压，推动反抗或交易。"),
		directorEventTemplate("public_trial", "公开审判", "公开审判/舆论反转", "在公开场合放大误会、证据和反转收益。"),
		directorEventTemplate("identity_reveal", "隐藏身份暴露", "隐藏身份暴露", "让身份秘密在收益和风险之间被迫显露。"),
		directorEventTemplate("foreshadowing_payoff", "伏笔回收", "伏笔回收", "回收之前埋下的线索，同时开启新的问题。"),
	}
}

func DirectorStateContextSummary(state DirectorState, branchID string, limitBytes int) string {
	if limitBytes <= 0 {
		limitBytes = 4096
	}
	if limitBytes > 16*1024 {
		limitBytes = 16 * 1024
	}
	state = NormalizeDirectorState(state)
	var sb strings.Builder
	writeDirectorSummaryLine(&sb, "启用", fmt.Sprintf("%t", state.Enabled))
	writeDirectorSummaryLine(&sb, "剧透层级", state.SpoilerMode)
	writeDirectorSummaryLine(&sb, "长期主线", state.MainArc)
	writeDirectorSummaryLine(&sb, "阶段计划", state.StagePlan)
	if branchID != "" && state.BranchPatches != nil {
		writeDirectorSummaryLine(&sb, "当前分支补丁", state.BranchPatches[branchID])
	}
	if len(state.ForcedEvents) > 0 {
		writeDirectorSummaryLine(&sb, "强制事件", strings.Join(normalizeStringListLimit(state.ForcedEvents, 8), "、"))
	}
	if len(state.DisabledEvents) > 0 {
		writeDirectorSummaryLine(&sb, "禁用事件", strings.Join(normalizeStringListLimit(state.DisabledEvents, 8), "、"))
	}
	for i, beat := range state.BeatQueue {
		if i >= 5 {
			break
		}
		writeDirectorSummaryLine(&sb, fmt.Sprintf("节拍 %d", i+1), directorBeatInteractiveLine(beat))
	}
	for i, event := range state.EventQueue {
		if i >= 8 {
			break
		}
		value := directorEventContextLine(event)
		writeDirectorSummaryLine(&sb, fmt.Sprintf("事件 %d", i+1), value)
	}
	for i, thread := range state.Foreshadowing {
		if i >= 6 {
			break
		}
		writeDirectorSummaryLine(&sb, fmt.Sprintf("伏笔 %d", i+1), firstNonEmpty(thread.Title, thread.Summary))
	}
	for i, character := range state.PotentialCharacters {
		if i >= 6 {
			break
		}
		writeDirectorSummaryLine(&sb, fmt.Sprintf("潜在角色 %d", i+1), firstNonEmpty(character.Title, character.Summary))
	}
	if state.LastDirectorRun != nil {
		writeDirectorSummaryLine(&sb, "最近后台更新", strings.Join([]string{state.LastDirectorRun.Status, state.LastDirectorRun.Summary, state.LastDirectorRun.Error}, " / "))
	}
	return strings.TrimSpace(trimBytes(sb.String(), limitBytes))
}

// DirectorStateInteractiveContextSummary returns the director plan visible to
// the prose agent. Raw event-system cards stay reserved for the background
// director so event packs act as planning material instead of a trigger list.
func DirectorStateInteractiveContextSummary(state DirectorState, branchID string, limitBytes int) string {
	if limitBytes <= 0 {
		limitBytes = 4096
	}
	if limitBytes > 16*1024 {
		limitBytes = 16 * 1024
	}
	state = NormalizeDirectorState(state)
	var sb strings.Builder
	writeDirectorSummaryLine(&sb, "启用", fmt.Sprintf("%t", state.Enabled))
	writeDirectorSummaryLine(&sb, "剧透层级", state.SpoilerMode)
	writeDirectorSummaryLine(&sb, "长期主线", state.MainArc)
	writeDirectorSummaryLine(&sb, "阶段计划", state.StagePlan)
	if branchID != "" && state.BranchPatches != nil {
		writeDirectorSummaryLine(&sb, "当前分支补丁", state.BranchPatches[branchID])
	}
	for i, beat := range state.BeatQueue {
		if i >= 5 {
			break
		}
		writeDirectorSummaryLine(&sb, fmt.Sprintf("节拍 %d", i+1), directorBeatInteractiveLine(beat))
	}
	writeForcedDirectorEvents(&sb, state)
	for i, thread := range state.Foreshadowing {
		if i >= 6 {
			break
		}
		writeDirectorSummaryLine(&sb, fmt.Sprintf("伏笔 %d", i+1), firstNonEmpty(thread.Title, thread.Summary))
	}
	for i, character := range state.PotentialCharacters {
		if i >= 6 {
			break
		}
		writeDirectorSummaryLine(&sb, fmt.Sprintf("潜在角色 %d", i+1), firstNonEmpty(character.Title, character.Summary))
	}
	return strings.TrimSpace(trimBytes(sb.String(), limitBytes))
}

func writeForcedDirectorEvents(sb *strings.Builder, state DirectorState) {
	if len(state.ForcedEvents) == 0 {
		return
	}
	for i, eventID := range normalizeStringListLimit(state.ForcedEvents, 8) {
		if i >= 8 {
			break
		}
		label := eventID
		if event, ok := forcedDirectorEventByID(state.EventQueue, eventID); ok {
			label = directorEventInteractiveLine(event)
		}
		writeDirectorSummaryLine(sb, fmt.Sprintf("强制事件 %d", i+1), label)
	}
}

func directorBeatInteractiveLine(beat DirectorBeat) string {
	return strings.TrimSpace(strings.Join(nonEmptyStrings([]string{
		beat.Summary,
		beat.Pressure,
		beat.Payoff,
	}), " / "))
}

func forcedDirectorEventByID(events []DirectorEvent, id string) (DirectorEvent, bool) {
	id = strings.TrimSpace(id)
	for _, event := range events {
		if strings.TrimSpace(event.ID) == id {
			return event, true
		}
	}
	return DirectorEvent{}, false
}

func directorEventInteractiveLine(event DirectorEvent) string {
	parts := []string{
		firstNonEmpty(event.Name, event.ID),
		event.Category,
		firstNonEmpty(event.PublicSummary, event.Summary),
	}
	if event.Reward != "" || event.Cost != "" || event.PayoffTarget != "" {
		parts = append(parts, strings.TrimSpace(strings.Join([]string{event.PayoffTarget, event.Reward, event.Cost}, " / ")))
	}
	return strings.TrimSpace(strings.Join(nonEmptyStrings(parts), " / "))
}

func directorEventContextLine(event DirectorEvent) string {
	parts := []string{
		firstNonEmpty(event.Name, event.ID),
		event.Category,
		event.Status,
		firstNonEmpty(event.PublicSummary, event.Summary),
	}
	if content := strings.TrimSpace(event.Template); content != "" && content != strings.TrimSpace(firstNonEmpty(event.PublicSummary, event.Summary)) {
		parts = append(parts, "事件卡: "+trimBytes(compactEventCardMarkdown(content), maxEventCardContextSummaryByte))
	}
	if event.Reward != "" || event.Cost != "" || event.PayoffTarget != "" {
		parts = append(parts, strings.TrimSpace(strings.Join([]string{event.PayoffTarget, event.Reward, event.Cost}, " / ")))
	}
	return strings.TrimSpace(strings.Join(parts, " / "))
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func compactEventCardMarkdown(markdown string) string {
	lines := []string{}
	inFence := false
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence || line == "" {
			continue
		}
		lines = append(lines, cleanMarkdownSummaryLine(line))
		if len(lines) >= 12 {
			break
		}
	}
	return strings.Join(lines, "；")
}

func writeDirectorSummaryLine(sb *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(sb, "- %s: %s\n", label, value)
}

func directorEventTemplate(id, name, category, summary string) DirectorEvent {
	return DirectorEvent{
		ID:                id,
		Name:              name,
		Category:          category,
		Status:            "available",
		Enabled:           true,
		Summary:           summary,
		PublicSummary:     summary,
		Template:          summary,
		NormalizedTrigger: category,
		Weight:            1,
		CooldownTurns:     2,
		Intensity:         "medium",
	}
}

func directorEventForAction(eventID string, override *DirectorEvent) DirectorEvent {
	if override != nil {
		event := *override
		if strings.TrimSpace(event.ID) == "" {
			event.ID = eventID
		}
		if strings.TrimSpace(event.Name) == "" {
			event.Name = event.ID
		}
		if event.Weight == 0 {
			event.Weight = 1
		}
		return event
	}
	for _, event := range DefaultDirectorEventTemplates() {
		if event.ID == eventID || event.Category == eventID || event.Name == eventID {
			return event
		}
	}
	return DirectorEvent{
		ID:                eventID,
		Name:              eventID,
		Category:          "custom",
		Status:            "available",
		Enabled:           true,
		Summary:           "用户强制安排的自定义事件。",
		PublicSummary:     "用户强制安排的自定义事件。",
		Template:          "根据当前剧情合理安排该自定义事件。",
		NormalizedTrigger: eventID,
		Weight:            1,
		Intensity:         "medium",
		UserConfigured:    true,
	}
}

func upsertDirectorEvent(events []DirectorEvent, next DirectorEvent) []DirectorEvent {
	normalized := normalizeDirectorEvents([]DirectorEvent{next})
	if len(normalized) == 0 {
		return events
	}
	next = normalized[0]
	for i := range events {
		if events[i].ID == next.ID {
			events[i] = next
			return events
		}
	}
	if len(events) >= maxTurnBriefListItems {
		return events
	}
	return append(events, next)
}

func disableDirectorEvent(events []DirectorEvent, eventID string) []DirectorEvent {
	out := make([]DirectorEvent, 0, len(events))
	for _, event := range events {
		if event.ID != eventID && event.Name != eventID && event.Category != eventID {
			out = append(out, event)
			continue
		}
		event.Enabled = false
		event.Status = "disabled"
		out = append(out, event)
	}
	return out
}

func latestTurnForBranchHead(lines []StoryEventRecord, head string) *TurnEvent {
	path, _ := eventPath(head, eventsByID(lines))
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(path[i].Raw, &turn); err != nil {
			continue
		}
		return &turn
	}
	return nil
}

func parentIDForDirectorPatch(head string) any {
	if strings.TrimSpace(head) == "" {
		return nil
	}
	return head
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || stringInList(value, values) {
		return values
	}
	if len(values) >= maxTurnBriefListItems {
		return values
	}
	return append(values, value)
}

func removeString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	out := values[:0]
	for _, existing := range values {
		if strings.TrimSpace(existing) == "" || existing == value {
			continue
		}
		out = append(out, existing)
	}
	return out
}

func stringInList(value string, values []string) bool {
	value = strings.TrimSpace(value)
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
