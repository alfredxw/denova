package interactive

import (
	"fmt"
	"strings"
)

const StateOpSourceTurnResult = "turn_result"

// TurnContract records the Game Agent's bounded intent for one playable turn.
// It is persisted with the final turn and never rendered as story prose.
type TurnContract struct {
	PlayerIntent             string              `json:"player_intent" jsonschema_description:"玩家本轮实际意图。"`
	SceneGoal                string              `json:"scene_goal" jsonschema_description:"本轮需要推进或解决的场景目标。"`
	Beats                    []string            `json:"beats,omitempty" jsonschema_description:"本轮正文需要完成的 1 到 3 个叙事节拍。"`
	NPCIntents               []string            `json:"npc_intents,omitempty" jsonschema_description:"本轮重要角色各自主动追求的目标。"`
	Reveals                  []string            `json:"reveals,omitempty" jsonschema_description:"本轮确定向玩家揭示的信息。"`
	Costs                    []string            `json:"costs,omitempty" jsonschema_description:"本轮已经成立或可能成立的代价。"`
	ContinuityConstraints    []string            `json:"continuity_constraints,omitempty" jsonschema_description:"正文不可违背的既有事实和连续性约束。"`
	ChoiceAxes               []string            `json:"choice_axes,omitempty" jsonschema_description:"正文结尾留下的不同下一步行动维度。"`
	ExpectedStateChanges     []string            `json:"expected_state_changes,omitempty" jsonschema_description:"本轮预计会改变的状态语义说明；实际状态以 actor_state_patches 和 RuleResolution 为准。"`
	SceneTransitionCandidate TurnSceneTransition `json:"scene_transition_candidate,omitempty" jsonschema_description:"Game Agent 对场景边界的候选判断。"`
	PlanAlignmentCandidate   TurnPlanAlignment   `json:"plan_alignment_candidate,omitempty" jsonschema_description:"Game Agent 对当前计划是否仍然成立的候选判断。"`
}

type TurnSceneTransition struct {
	Kind   string `json:"kind,omitempty" jsonschema:"enum=none,enum=exit,enum=enter,enum=replace" jsonschema_description:"场景变化类型；没有场景边界时使用 none。"`
	From   string `json:"from,omitempty" jsonschema_description:"离开的场景或阶段。"`
	To     string `json:"to,omitempty" jsonschema_description:"进入的新场景或阶段候选。"`
	Reason string `json:"reason,omitempty" jsonschema_description:"判断为场景边界的正文依据。"`
}

type TurnPlanAlignment struct {
	Level               string   `json:"level,omitempty" jsonschema:"enum=aligned,enum=minor_deviation,enum=major_deviation" jsonschema_description:"本轮结果与当前计划的对齐程度。"`
	InvalidatedPlanRefs []string `json:"invalidated_plan_refs,omitempty" jsonschema_description:"可能被本轮事实推翻的 beat 或计划引用。"`
	Reason              string   `json:"reason,omitempty" jsonschema_description:"偏航候选判断依据。"`
}

type StoryFactCandidate struct {
	Kind       string   `json:"kind" jsonschema_description:"事实类型，例如 plot、relationship、clue、secret、world、commitment。"`
	Subject    string   `json:"subject" jsonschema_description:"事实主要涉及的人物、地点、势力、物品或事件。"`
	Fact       string   `json:"fact" jsonschema_description:"本轮正文已经明确成立、后续需要承接的事实。"`
	Visibility string   `json:"visibility,omitempty" jsonschema:"enum=public,enum=player_known,enum=private" jsonschema_description:"事实可见性；未来计划不得作为事实候选。"`
	Importance string   `json:"importance,omitempty" jsonschema:"enum=low,enum=medium,enum=high,enum=critical" jsonschema_description:"长期记忆重要程度。"`
	People     []string `json:"people,omitempty" jsonschema_description:"相关人物。"`
	Places     []string `json:"places,omitempty" jsonschema_description:"相关地点。"`
	Tags       []string `json:"tags,omitempty" jsonschema_description:"检索标签。"`
}

type TurnSceneResult struct {
	Status        string `json:"status,omitempty" jsonschema:"enum=continued,enum=completed,enum=transitioned,enum=terminal" jsonschema_description:"本轮结束后的场景状态。"`
	SceneID       string `json:"scene_id,omitempty" jsonschema_description:"当前场景稳定标识或名称。"`
	NextSceneID   string `json:"next_scene_id,omitempty" jsonschema_description:"发生场景切换时的下一场景标识或名称。"`
	Summary       string `json:"summary,omitempty" jsonschema_description:"本轮场景结果的简短事实摘要。"`
	NextSceneGoal string `json:"next_scene_goal,omitempty" jsonschema_description:"下一场景已经明确时的即时目标。"`
}

type TurnPlanSignals struct {
	SceneTransition TurnSceneTransition `json:"scene_transition,omitempty"`
	DeviationLevel  string              `json:"deviation_level,omitempty" jsonschema:"enum=none,enum=minor,enum=major" jsonschema_description:"最终结果对当前计划的影响等级。"`
	InvalidatedRefs []string            `json:"invalidated_refs,omitempty" jsonschema_description:"被最终事实推翻的计划引用。"`
	Reason          string              `json:"reason,omitempty" jsonschema_description:"最终计划信号判断依据。"`
}

// TurnResult is produced by the Game Agent alongside narrative. The backend
// validates it and atomically materializes Actor State changes with the turn.
type TurnResult struct {
	Contract          TurnContract         `json:"contract"`
	ActorStatePatches []ActorStatePatch    `json:"actor_state_patches,omitempty" jsonschema_description:"非规则叙事已经明确建立的 Actor 状态更新；数值检定结果不要在这里重复写。"`
	FactCandidates    []StoryFactCandidate `json:"fact_candidates,omitempty" jsonschema_description:"已经发生、值得由 Memory Recorder 整理为长期记忆的事实候选。"`
	SceneResult       TurnSceneResult      `json:"scene_result,omitempty"`
	PlanSignals       TurnPlanSignals      `json:"plan_signals,omitempty"`
	Choices           []string             `json:"choices,omitempty" jsonschema_description:"与正文结尾一致、可直接作为下一轮输入的 2 到 4 个行动建议。"`
}

func NormalizeTurnResult(result TurnResult) TurnResult {
	result.Contract = normalizeTurnContract(result.Contract)
	if len(result.ActorStatePatches) > maxTurnBriefListItems {
		result.ActorStatePatches = result.ActorStatePatches[:maxTurnBriefListItems]
	}
	for i := range result.ActorStatePatches {
		result.ActorStatePatches[i].ActorID = normalizeActorStateID(result.ActorStatePatches[i].ActorID)
		result.ActorStatePatches[i].TemplateID = normalizeActorStateID(result.ActorStatePatches[i].TemplateID)
		result.ActorStatePatches[i].ActorName = trimBytes(result.ActorStatePatches[i].ActorName, 128)
		result.ActorStatePatches[i].Role = trimBytes(result.ActorStatePatches[i].Role, 128)
		result.ActorStatePatches[i].Description = trimBytes(result.ActorStatePatches[i].Description, maxTurnBriefTextBytes)
		result.ActorStatePatches[i].Reason = trimBytes(result.ActorStatePatches[i].Reason, maxTurnBriefTextBytes)
		result.ActorStatePatches[i].SourceTurnID = ""
	}
	result.FactCandidates = normalizeStoryFactCandidates(result.FactCandidates)
	result.SceneResult.Status = normalizeEnum(result.SceneResult.Status, "continued", "completed", "transitioned", "terminal")
	result.SceneResult.SceneID = trimBytes(result.SceneResult.SceneID, 256)
	result.SceneResult.NextSceneID = trimBytes(result.SceneResult.NextSceneID, 256)
	result.SceneResult.Summary = trimBytes(result.SceneResult.Summary, maxTurnBriefTextBytes)
	result.SceneResult.NextSceneGoal = trimBytes(result.SceneResult.NextSceneGoal, maxTurnBriefTextBytes)
	result.PlanSignals.SceneTransition = normalizeTurnSceneTransition(result.PlanSignals.SceneTransition)
	result.PlanSignals.DeviationLevel = normalizeEnum(result.PlanSignals.DeviationLevel, "none", "minor", "major")
	result.PlanSignals.InvalidatedRefs = normalizeStringListLimit(result.PlanSignals.InvalidatedRefs, maxTurnBriefListItems)
	result.PlanSignals.Reason = trimBytes(result.PlanSignals.Reason, maxTurnBriefTextBytes)
	result.Choices = normalizeChoiceListLimit(result.Choices, 4)
	return result
}

func ValidateTurnResult(result TurnResult) error {
	if strings.TrimSpace(result.Contract.PlayerIntent) == "" && strings.TrimSpace(result.Contract.SceneGoal) == "" {
		return fmt.Errorf("TurnResult 缺少 player_intent 或 scene_goal")
	}
	for _, fact := range result.FactCandidates {
		if strings.TrimSpace(fact.Fact) == "" {
			return fmt.Errorf("TurnResult 事实候选内容不能为空")
		}
	}
	if len(result.Choices) == 1 || (result.SceneResult.Status != "terminal" && len(result.Choices) < 2) {
		return fmt.Errorf("TurnResult choices 必须提供 2 到 4 个与正文结尾一致的行动建议；终局回合可以为空")
	}
	return nil
}

func normalizeTurnResultPointer(result *TurnResult) *TurnResult {
	if result == nil {
		return nil
	}
	normalized := NormalizeTurnResult(*result)
	if err := ValidateTurnResult(normalized); err != nil {
		return nil
	}
	return &normalized
}

func normalizeTurnContract(contract TurnContract) TurnContract {
	contract.PlayerIntent = trimBytes(contract.PlayerIntent, maxTurnBriefTextBytes)
	contract.SceneGoal = trimBytes(contract.SceneGoal, maxTurnBriefTextBytes)
	contract.Beats = normalizeStringListLimit(contract.Beats, 3)
	contract.NPCIntents = normalizeStringListLimit(contract.NPCIntents, maxTurnBriefListItems)
	contract.Reveals = normalizeStringListLimit(contract.Reveals, maxTurnBriefListItems)
	contract.Costs = normalizeStringListLimit(contract.Costs, maxTurnBriefListItems)
	contract.ContinuityConstraints = normalizeStringListLimit(contract.ContinuityConstraints, maxTurnBriefListItems)
	contract.ChoiceAxes = normalizeStringListLimit(contract.ChoiceAxes, maxTurnBriefListItems)
	contract.ExpectedStateChanges = normalizeStringListLimit(contract.ExpectedStateChanges, maxTurnBriefListItems)
	contract.SceneTransitionCandidate = normalizeTurnSceneTransition(contract.SceneTransitionCandidate)
	contract.PlanAlignmentCandidate.Level = normalizeEnum(contract.PlanAlignmentCandidate.Level, "aligned", "minor_deviation", "major_deviation")
	contract.PlanAlignmentCandidate.InvalidatedPlanRefs = normalizeStringListLimit(contract.PlanAlignmentCandidate.InvalidatedPlanRefs, maxTurnBriefListItems)
	contract.PlanAlignmentCandidate.Reason = trimBytes(contract.PlanAlignmentCandidate.Reason, maxTurnBriefTextBytes)
	return contract
}

func normalizeTurnSceneTransition(value TurnSceneTransition) TurnSceneTransition {
	value.Kind = normalizeEnum(value.Kind, "none", "exit", "enter", "replace")
	value.From = trimBytes(value.From, 256)
	value.To = trimBytes(value.To, 256)
	value.Reason = trimBytes(value.Reason, maxTurnBriefTextBytes)
	return value
}

func normalizeStoryFactCandidates(values []StoryFactCandidate) []StoryFactCandidate {
	if len(values) > maxTurnBriefListItems {
		values = values[:maxTurnBriefListItems]
	}
	out := make([]StoryFactCandidate, 0, len(values))
	for _, value := range values {
		value.Kind = trimBytes(value.Kind, 128)
		value.Subject = trimBytes(value.Subject, 256)
		value.Fact = trimBytes(value.Fact, maxTurnBriefTextBytes)
		if strings.TrimSpace(value.Fact) == "" {
			continue
		}
		value.Visibility = normalizeEnum(value.Visibility, "public", "player_known", "private")
		value.Importance = normalizeEnum(value.Importance, "low", "medium", "high", "critical")
		value.People = normalizeStringListLimit(value.People, maxTurnBriefListItems)
		value.Places = normalizeStringListLimit(value.Places, maxTurnBriefListItems)
		value.Tags = normalizeStringListLimit(value.Tags, maxTurnBriefListItems)
		out = append(out, value)
	}
	return out
}

func normalizeEnum(value string, allowed ...string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	if len(allowed) > 0 {
		return allowed[0]
	}
	return ""
}
