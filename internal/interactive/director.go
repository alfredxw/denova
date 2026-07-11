package interactive

import (
	"strings"
)

const DirectorPatchSourceInteractiveDirector = "interactive_director"

type DirectorAgentScheduleDecision struct {
	ShouldRun bool   `json:"should_run"`
	Reason    string `json:"reason,omitempty"`
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
		Intensity:         "medium",
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

func appendDefaultDirectorEventTemplates(events []DirectorEvent) []DirectorEvent {
	for _, event := range DefaultDirectorEventTemplates() {
		events = appendDirectorEventIfMissing(events, event)
	}
	return events
}

func appendDirectorEventIfMissing(events []DirectorEvent, next DirectorEvent) []DirectorEvent {
	normalized := normalizeDirectorEvents([]DirectorEvent{next})
	if len(normalized) == 0 {
		return events
	}
	next = normalized[0]
	for _, event := range events {
		if event.ID == next.ID {
			return events
		}
	}
	if len(events) >= maxTurnBriefListItems {
		return events
	}
	return append(events, next)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
