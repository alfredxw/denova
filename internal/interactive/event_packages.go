package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"fmt"
	"strings"
)

const (
	MaxEventCardDescriptionChars   = 8000
	maxEventCardSummaryChars       = 240
	maxEventCardContextSummaryByte = 900
)

// EventPackage is one creator-editable collection of director event cards.
type EventPackage struct {
	ID      string      `json:"id,omitempty"`
	Name    string      `json:"name,omitempty"`
	Enabled bool        `json:"enabled"`
	Events  []EventCard `json:"events,omitempty"`
}

// EventCard is a reusable director event whose full behavior is expressed as
// Markdown and projected into DirectorEvent.Template at runtime.
type EventCard struct {
	ID                  string   `json:"id,omitempty"`
	TypeName            string   `json:"type_name,omitempty"`
	DescriptionMarkdown string   `json:"description_markdown,omitempty"`
	Enabled             bool     `json:"enabled"`
	Category            string   `json:"category,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	Intensity           string   `json:"intensity,omitempty"`
}

func DefaultRuleCheckTemplates() []RuleCheck {
	return normalizeRuleChecks([]RuleCheck{
		{
			ID:                  "balanced-dice-check",
			Label:               "均衡骰子检定",
			Dice:                "1d20",
			FailurePolicy:       "fail_forward",
			DifficultyGuidance:  "默认 normal。角色有明确能力、合适工具、合理计划或环境优势时降一档；时间压力、敌对环境、信息不足、受伤或连续失败后升一档。",
			StateEffectGuidance: "失败优先落到可承接的状态变化：资源消耗、警戒度、关系损伤、位置暴露、时间压力或后续劣势；避免因一次失败直接卡死剧情。",
			Trigger:             "玩家行动存在风险、不确定性和有意义的失败后果时使用；没有风险、结果显然、或玩家方案已直接解决问题时不要检定。",
			MustCheckExamples:   []string{"在守卫逼近时强行撬锁。", "试图说服立场摇摆的关键 NPC。", "冒险穿越正在崩塌的桥。"},
			SkipCheckExamples:   []string{"观察没有风险的空房间。", "和友善同伴闲聊。", "使用正确钥匙打开普通门。"},
			SuccessHint:         "成功时让行动达成核心目标，并给出清楚收益、线索、位置或关系推进。",
			FailureHint:         "失败时保留剧情推进空间，但写清楚代价、阻碍、资源消耗、关系变化或新的危险选择。",
		},
	})
}

func defaultEventCards() []EventCard {
	templates := DefaultDirectorEventTemplates()
	cards := make([]EventCard, 0, len(templates))
	for _, event := range templates {
		cards = append(cards, EventCard{
			ID: event.ID, TypeName: event.Name,
			DescriptionMarkdown: defaultEventCardMarkdown(event), Enabled: true,
			Category: event.Category, Intensity: event.Intensity,
		})
	}
	return cards
}

func defaultEventCardMarkdown(event DirectorEvent) string {
	details := defaultEventCardDetails(event)
	return strings.TrimSpace(fmt.Sprintf(`## 触发场景

%s

## 背景融合方式

%s

## 大致事件逻辑（起承转合）

%s

## 事件回收 / 后果

%s

## 奖励 / 代价

%s

## 避免生硬的约束

%s`, details.Trigger, details.Fusion, details.Logic, details.Payoff, details.RewardCost, details.Guardrail))
}

func normalizeOrchestrationOption(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeEventPackagesNoDefault(packages []EventPackage) []EventPackage {
	if packages == nil {
		return []EventPackage{}
	}
	result := make([]EventPackage, 0, len(packages))
	seen := map[string]bool{}
	for _, eventPackage := range packages {
		eventPackage.ID = normalizeInteractiveID(eventPackage.ID)
		if eventPackage.ID == "" {
			eventPackage.ID = fmt.Sprintf("event-package-%d", len(result)+1)
		}
		if seen[eventPackage.ID] {
			continue
		}
		seen[eventPackage.ID] = true
		eventPackage.Name = strings.TrimSpace(eventPackage.Name)
		if eventPackage.Name == "" {
			eventPackage.Name = eventPackage.ID
		}
		eventPackage.Events = normalizeEventCards(eventPackage.Events, eventPackage.ID)
		result = append(result, eventPackage)
	}
	return result
}

func normalizeEventCards(events []EventCard, packageID string) []EventCard {
	result := make([]EventCard, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		event.ID = normalizeInteractiveID(event.ID)
		if event.ID == "" {
			event.ID = fmt.Sprintf("%s-event-%d", packageID, len(result)+1)
		}
		if seen[event.ID] {
			continue
		}
		event.TypeName = strings.TrimSpace(event.TypeName)
		event.DescriptionMarkdown = strings.TrimSpace(event.DescriptionMarkdown)
		if event.TypeName == "" && event.DescriptionMarkdown == "" {
			continue
		}
		if event.TypeName == "" {
			event.TypeName = event.ID
		}
		seen[event.ID] = true
		event.Category = strings.TrimSpace(event.Category)
		if event.Category == "" {
			event.Category = event.TypeName
		}
		event.Tags = normalizeStringList(event.Tags)
		event.Intensity = strings.TrimSpace(event.Intensity)
		if event.Intensity == "" {
			event.Intensity = "medium"
		}
		result = append(result, event)
	}
	return result
}

func normalizeInteractiveID(id string) string {
	id = strings.TrimSpace(id)
	var normalized strings.Builder
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}

func normalizeRuleChecks(checks []RuleCheck) []RuleCheck {
	result := make([]RuleCheck, 0, len(checks))
	for index, check := range checks {
		if ruleCheckBlank(check) {
			continue
		}
		check = normalizeRuleCheck(check, index)
		if check.ID == "" && check.Label == "" {
			continue
		}
		result = append(result, check)
	}
	return result
}

func ruleCheckBlank(check RuleCheck) bool {
	return strings.TrimSpace(check.ID) == "" && strings.TrimSpace(check.Label) == "" &&
		strings.TrimSpace(check.Dice) == "" && strings.TrimSpace(check.FailurePolicy) == "" &&
		strings.TrimSpace(check.DifficultyGuidance) == "" && strings.TrimSpace(check.StateEffectGuidance) == "" &&
		strings.TrimSpace(check.Trigger) == "" && len(check.MustCheckExamples) == 0 &&
		len(check.SkipCheckExamples) == 0 && len(check.StateBindings) == 0 &&
		strings.TrimSpace(check.SuccessHint) == "" && strings.TrimSpace(check.FailureHint) == "" && check.Modifier == 0
}

func normalizeStateOps(ops []interactivestate.Op) []interactivestate.Op {
	return normalizeStateOpsUnbounded(ops)
}

// normalizeStateOpsUnbounded keeps the explicit name used by public mutation
// paths while sharing the same lossless item-count semantics.
func normalizeStateOpsUnbounded(ops []interactivestate.Op) []interactivestate.Op {
	result := make([]interactivestate.Op, 0, len(ops))
	for _, op := range ops {
		op.Op = strings.TrimSpace(op.Op)
		op.Path = canonicalStatePath(op.Path)
		op.Reason = trimBytes(op.Reason, maxInteractiveTextBytes)
		op.SourceTurnID = trimBytes(op.SourceTurnID, 128)
		op.SourceKind = trimBytes(op.SourceKind, 128)
		op.SourceID = trimBytes(op.SourceID, 128)
		if op.Op == "" || op.Path == "" {
			continue
		}
		result = append(result, op)
	}
	return result
}

func directorEventFromCard(card EventCard) DirectorEvent {
	summary := eventCardSummaryFromMarkdown(card.DescriptionMarkdown, card.TypeName)
	if summary == "" {
		summary = card.TypeName
	}
	return DirectorEvent{
		ID: card.ID, Name: card.TypeName, Category: firstNonEmpty(card.Category, "自定义事件"),
		Status: "available", Enabled: true, Summary: summary, PublicSummary: summary,
		Template:          card.DescriptionMarkdown,
		NormalizedTrigger: firstNonEmpty(card.Category, card.TypeName, card.ID),
		Intensity:         card.Intensity, CompatibleGenres: card.Tags, UserConfigured: true,
		DirectorInstructionNote: fmt.Sprintf("来源事件卡：%s。template 字段包含触发场景、背景融合、起承转合、回收/后果、奖励/代价和避免生硬约束。", card.TypeName),
	}
}

func eventCardSummaryFromMarkdown(markdown, fallback string) string {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return truncateRunes(strings.TrimSpace(fallback), maxEventCardSummaryChars)
	}
	parts := make([]string, 0, 4)
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
		cleaned := cleanMarkdownSummaryLine(line)
		if cleaned == "" {
			continue
		}
		parts = append(parts, cleaned)
		if len(strings.Join(parts, " ")) >= maxEventCardSummaryChars {
			break
		}
	}
	return truncateRunes(strings.TrimSpace(strings.Join(parts, " ")), maxEventCardSummaryChars)
}

func cleanMarkdownSummaryLine(line string) string {
	line = strings.TrimLeft(line, "#>-*+ 0123456789.")
	line = strings.NewReplacer("**", "", "__", "", "`", "", "[", "", "]", "").Replace(line)
	return strings.TrimSpace(line)
}

func truncateRunes(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}
