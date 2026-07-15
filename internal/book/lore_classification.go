package book

import (
	"regexp"
	"strings"
)

const (
	LoreClassificationModeHeuristic = "heuristic"
	LoreClassificationModeSemantic  = "semantic"

	LoreClassificationConfidenceHigh   = "high"
	LoreClassificationConfidenceMedium = "medium"
	LoreClassificationConfidenceLow    = "low"
)

// LoreClassificationInput is the bounded semantic-classification payload.
// Content callers provide here must already be clipped to the task budget.
type LoreClassificationInput struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Tags             []string `json:"tags,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	BriefDescription string   `json:"brief_description,omitempty"`
	Content          string   `json:"content,omitempty"`
	CurrentType      string   `json:"current_type,omitempty"`
}

type LoreClassificationSuggestion struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason,omitempty"`
}

type LoreSemanticClassifier func([]LoreClassificationInput) ([]LoreClassificationSuggestion, error)

type loreTypeSignal struct {
	Type     string
	Prefixes []string
	Pattern  *regexp.Regexp
}

var loreTypeSignals = []loreTypeSignal{
	{
		Type: "character",
		Prefixes: []string{
			"角色", "人物", "npc", "character", "characterprofile", "人物详情", "详细人物", "人物详细", "角色详情", "详细角色", "角色档案", "人物档案", "角色设定", "人物设定", "人设",
		},
		Pattern: regexp.MustCompile(`(?im)^\s*(?:#{1,4}\s*)?(?:角色|人物|npc|人物详情|详细人物|人物详细|角色详情|角色档案|人物档案|人设)(?:设定|资料|信息|详情|档案)?\s*[:：]`),
	},
	{
		Type:     "location",
		Prefixes: []string{"地点", "场景", "区域", "地理", "地图", "城镇", "城市", "建筑", "副本", "location", "place", "region", "map", "city"},
		Pattern:  regexp.MustCompile(`(?im)^\s*(?:#{1,4}\s*)?(?:地点|场景|区域|地理|地图|城镇|城市|建筑|副本)(?:设定|资料|信息|详情|档案)?\s*[:：]`),
	},
	{
		Type:     "faction",
		Prefixes: []string{"势力", "组织", "门派", "阵营", "宗门", "家族", "公会", "公司", "政权", "faction", "organization", "organisation", "guild", "clan"},
		Pattern:  regexp.MustCompile(`(?im)^\s*(?:#{1,4}\s*)?(?:势力|组织|门派|阵营|宗门|家族|公会|公司|政权)(?:设定|资料|信息|详情|档案)?\s*[:：]`),
	},
	{
		Type:     "rule",
		Prefixes: []string{"规则", "机制", "法则", "系统", "能力体系", "力量体系", "魔法体系", "修炼体系", "rule", "rules", "mechanic", "mechanics", "system"},
		Pattern:  regexp.MustCompile(`(?im)^\s*(?:#{1,4}\s*)?(?:规则|机制|法则|系统|能力体系|力量体系|魔法体系|修炼体系)(?:设定|资料|信息|详情)?\s*[:：]`),
	},
	{
		Type:     "item",
		Prefixes: []string{"物品", "道具", "装备", "武器", "宝物", "法宝", "药剂", "材料", "item", "equipment", "weapon", "artifact"},
		Pattern:  regexp.MustCompile(`(?im)^\s*(?:#{1,4}\s*)?(?:物品|道具|装备|武器|宝物|法宝|药剂|材料)(?:设定|资料|信息|详情|档案)?\s*[:：]`),
	},
	{
		Type:     "world",
		Prefixes: []string{"世界", "世界观", "背景", "历史", "纪年", "种族", "文化", "社会", "时代", "world", "worldbuilding", "history", "culture"},
		Pattern:  regexp.MustCompile(`(?im)^\s*(?:#{1,4}\s*)?(?:世界|世界观|背景|历史|纪年|种族|文化|社会|时代)(?:设定|资料|信息|详情)?\s*[:：]`),
	},
}

// ClassifyLoreItemHeuristic performs a deterministic name-first pass. It is
// intentionally conservative: unknown entries remain other for later review.
func ClassifyLoreItemHeuristic(input LoreClassificationInput) LoreClassificationSuggestion {
	name := normalizeLoreClassificationName(input.Name)
	for _, signal := range loreTypeSignals {
		for _, prefix := range signal.Prefixes {
			if loreNameHasTypeSignal(name, prefix) {
				return LoreClassificationSuggestion{ID: input.ID, Type: signal.Type, Confidence: LoreClassificationConfidenceHigh, Reason: "名称包含明确类型信号：" + prefix}
			}
		}
	}
	probe := strings.Join([]string{
		input.Name,
		strings.Join(input.Tags, " "),
		strings.Join(input.Keywords, " "),
		input.BriefDescription,
		firstCardRunes(input.Content, 600),
	}, "\n")
	for _, signal := range loreTypeSignals {
		if signal.Pattern.MatchString(probe) {
			return LoreClassificationSuggestion{ID: input.ID, Type: signal.Type, Confidence: LoreClassificationConfidenceMedium, Reason: "简介或正文标题包含类型信号"}
		}
	}
	return LoreClassificationSuggestion{ID: input.ID, Type: "other", Confidence: LoreClassificationConfidenceLow, Reason: "未发现稳定类型信号"}
}

func normalizeLoreClassificationName(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.TrimLeft(value, "# ")))
	value = strings.Trim(value, "【】[]（）()<>《》 ")
	return strings.Join(strings.Fields(value), "")
}

func loreNameHasTypeSignal(name, signal string) bool {
	signal = strings.ToLower(strings.TrimSpace(signal))
	if name == "" || signal == "" {
		return false
	}
	if strings.HasPrefix(name, signal) || strings.HasSuffix(name, signal) {
		return true
	}
	for _, separator := range []string{":", "：", "-", "—", "_", "/", "·"} {
		if strings.Contains(name, separator+signal) || strings.Contains(name, signal+separator) {
			return true
		}
	}
	return false
}

func normalizeLoreClassificationMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), LoreClassificationModeSemantic) {
		return LoreClassificationModeSemantic
	}
	return LoreClassificationModeHeuristic
}

func validLoreClassificationType(value string) bool {
	switch strings.TrimSpace(value) {
	case "character", "world", "location", "faction", "rule", "item", "other":
		return true
	default:
		return false
	}
}
