package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const tellerVersion = 3

type TellerLibrary struct {
	novaDir string
}

type Teller struct {
	Version          int                 `json:"version"`
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Description      string              `json:"description"`
	RandomEventRate  float64             `json:"random_event_rate"`
	ReplyTargetChars *int                `json:"reply_target_chars,omitempty"`
	StyleRules       []StyleRule         `json:"style_rules,omitempty"`
	Tags             []string            `json:"tags"`
	ContextPolicy    TellerContextPolicy `json:"context_policy"`
	Slots            []TellerPromptSlot  `json:"slots"`
	Path             string              `json:"path,omitempty"`
	Custom           bool                `json:"custom"`
	Invalid          bool                `json:"invalid,omitempty"`
	Error            string              `json:"error,omitempty"`
	CreatedAt        string              `json:"created_at,omitempty"`
	UpdatedAt        string              `json:"updated_at,omitempty"`
}

type TellerContextPolicy struct {
	Creator      string `json:"creator"`
	Lore         string `json:"lore"`
	RuntimeState string `json:"runtime_state"`
	RecentTurns  int    `json:"recent_turns"`
}

type TellerPromptSlot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

// StyleRule 表示导演自己的「场景 → 风格参考」映射。
type StyleRule struct {
	Scene  string   `json:"scene"`
	Styles []string `json:"styles"`
}

func NewTellerLibrary(novaDir string) *TellerLibrary {
	return &TellerLibrary{novaDir: novaDir}
}

func (l *TellerLibrary) List() ([]Teller, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	tellers := make([]Teller, 0, len(files))
	for _, file := range files {
		teller, err := parseTellerFile(file)
		if err != nil {
			tellers = append(tellers, Teller{
				ID:      strings.TrimSuffix(filepath.Base(file), ".json"),
				Path:    file,
				Invalid: true,
				Error:   err.Error(),
				Custom:  !isBuiltinTellerFile(file),
			})
			continue
		}
		teller.Path = file
		teller.Custom = !isBuiltinID(teller.ID)
		tellers = append(tellers, teller)
	}
	sort.Slice(tellers, func(i, j int) bool {
		if tellers[i].Custom != tellers[j].Custom {
			return !tellers[i].Custom
		}
		return tellers[i].ID < tellers[j].ID
	})
	return tellers, nil
}

func (l *TellerLibrary) Get(id string) (Teller, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Teller{}, err
	}
	if err := validateTellerID(id); err != nil {
		return Teller{}, err
	}
	teller, err := parseTellerFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return Teller{}, err
	}
	teller.Custom = !isBuiltinID(teller.ID)
	return teller, nil
}

func (l *TellerLibrary) Create(teller Teller) (Teller, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Teller{}, err
	}
	teller = normalizeTeller(teller)
	if teller.ID == "" {
		teller.ID = newTellerID()
	}
	if err := validateTeller(teller); err != nil {
		return Teller{}, err
	}
	path := filepath.Join(l.dir(), teller.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return Teller{}, fmt.Errorf("导演 ID 已存在: %s", teller.ID)
	} else if !os.IsNotExist(err) {
		return Teller{}, err
	}
	now := time.Now().Format(time.RFC3339)
	teller.CreatedAt = now
	teller.UpdatedAt = now
	if err := writeTellerFile(path, teller); err != nil {
		return Teller{}, err
	}
	teller.Path = path
	teller.Custom = !isBuiltinID(teller.ID)
	return teller, nil
}

func (l *TellerLibrary) Update(id string, teller Teller) (Teller, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Teller{}, err
	}
	if err := validateTellerID(id); err != nil {
		return Teller{}, err
	}
	current, err := l.Get(id)
	if err != nil {
		return Teller{}, err
	}
	teller.ID = id
	teller.CreatedAt = current.CreatedAt
	teller.UpdatedAt = time.Now().Format(time.RFC3339)
	teller = normalizeTeller(teller)
	if err := validateTeller(teller); err != nil {
		return Teller{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeTellerFile(path, teller); err != nil {
		return Teller{}, err
	}
	teller.Path = path
	teller.Custom = !isBuiltinID(teller.ID)
	return teller, nil
}

func (l *TellerLibrary) Delete(id string) error {
	if err := validateTellerID(id); err != nil {
		return err
	}
	if isBuiltinID(id) {
		return errors.New("内置导演不能删除")
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *TellerLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-tellers")
}

func (l *TellerLibrary) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	for id, teller := range builtinTellers {
		path := filepath.Join(l.dir(), id+".json")
		version, versionErr := readTellerFileVersion(path)
		current, parseErr := parseTellerFile(path)
		if versionErr == nil && parseErr == nil && current.Version == tellerVersion && version == tellerVersion {
			continue
		}
		if err := writeTellerFile(path, teller); err != nil {
			return err
		}
	}
	return nil
}

func readTellerFileVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, err
	}
	return payload.Version, nil
}

func parseTellerFile(path string) (Teller, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Teller{}, err
	}
	var teller Teller
	if err := json.Unmarshal(data, &teller); err != nil {
		return Teller{}, fmt.Errorf("解析导演 JSON 失败: %w", err)
	}
	teller = normalizeTeller(teller)
	if err := validateTeller(teller); err != nil {
		return Teller{}, err
	}
	teller.Path = path
	return teller, nil
}

func writeTellerFile(path string, teller Teller) error {
	teller = normalizeTeller(teller)
	data, err := json.MarshalIndent(teller, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (t Teller) PromptForTargets(targets ...string) string {
	allowed := map[string]bool{}
	for _, target := range targets {
		allowed[target] = true
	}
	var sb strings.Builder
	for _, slot := range t.Slots {
		if !slot.Enabled || !allowed[slot.Target] || strings.TrimSpace(slot.Content) == "" {
			continue
		}
		fmt.Fprintf(&sb, "## %s（%s）\n\n%s\n\n", slot.Name, slot.Target, strings.TrimSpace(slot.Content))
	}
	return strings.TrimSpace(sb.String())
}

func normalizeTeller(teller Teller) Teller {
	teller.Version = tellerVersion
	teller.ID = strings.TrimSpace(teller.ID)
	teller.Name = strings.TrimSpace(teller.Name)
	teller.Description = strings.TrimSpace(teller.Description)
	if teller.ReplyTargetChars != nil && *teller.ReplyTargetChars <= 0 {
		teller.ReplyTargetChars = nil
	}
	teller.StyleRules = normalizeStyleRules(teller.StyleRules)
	teller.Tags = normalizeTellerTags(teller.Tags)
	teller.ContextPolicy = normalizeContextPolicy(teller.ContextPolicy)
	teller.Slots = normalizePromptSlots(teller.Slots)
	return teller
}

func normalizeStyleRules(rules []StyleRule) []StyleRule {
	result := make([]StyleRule, 0, len(rules))
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" {
			continue
		}
		styles := make([]string, 0, len(rule.Styles))
		seen := map[string]bool{}
		for _, style := range rule.Styles {
			style = strings.TrimSpace(style)
			if style == "" || seen[style] {
				continue
			}
			seen[style] = true
			styles = append(styles, style)
		}
		if len(styles) == 0 {
			continue
		}
		result = append(result, StyleRule{Scene: scene, Styles: styles})
	}
	return result
}

func normalizeContextPolicy(policy TellerContextPolicy) TellerContextPolicy {
	if strings.TrimSpace(policy.Creator) == "" {
		policy.Creator = "always"
	}
	if strings.TrimSpace(policy.Lore) == "" {
		policy.Lore = "relevant"
	}
	if strings.TrimSpace(policy.RuntimeState) == "" {
		policy.RuntimeState = "always"
	}
	if policy.RecentTurns <= 0 {
		policy.RecentTurns = 8
	}
	return policy
}

func normalizePromptSlots(slots []TellerPromptSlot) []TellerPromptSlot {
	result := make([]TellerPromptSlot, 0, len(slots))
	seen := map[string]bool{}
	for _, slot := range slots {
		slot.ID = normalizeSlotID(slot.ID)
		if slot.ID == "" {
			slot.ID = fmt.Sprintf("slot-%d", len(result)+1)
		}
		if seen[slot.ID] {
			continue
		}
		seen[slot.ID] = true
		slot.Name = strings.TrimSpace(slot.Name)
		if slot.Name == "" {
			slot.Name = slot.ID
		}
		slot.Target = normalizeSlotTarget(slot.Target)
		slot.Content = strings.TrimSpace(slot.Content)
		result = append(result, slot)
	}
	return result
}

func validateTeller(teller Teller) error {
	if err := validateTellerID(teller.ID); err != nil {
		return err
	}
	if teller.Name == "" {
		return errors.New("导演名称不能为空")
	}
	if len(teller.Slots) == 0 {
		return errors.New("导演至少需要一个 prompt slot")
	}
	for _, slot := range teller.Slots {
		if !isAllowedSlotTarget(slot.Target) {
			return fmt.Errorf("导演规则 %q 使用了无效注入位置 %q，仅支持 system、turn_context、state_memory", slot.Name, slot.Target)
		}
	}
	return nil
}

func validateTellerID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("导演 ID 不能为空")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("导演 ID 包含非法字符: %s", id)
	}
	return nil
}

func normalizeTellerTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		result = append(result, tag)
	}
	return result
}

func normalizeSlotID(id string) string {
	id = strings.TrimSpace(id)
	var sb strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func normalizeSlotTarget(target string) string {
	return strings.TrimSpace(target)
}

func isAllowedSlotTarget(target string) bool {
	switch target {
	case "system", "turn_context", "state_memory":
		return true
	default:
		return false
	}
}

func newTellerID() string {
	return fmt.Sprintf("teller-%d", time.Now().UTC().UnixNano())
}

func isBuiltinTellerFile(path string) bool {
	return isBuiltinID(strings.TrimSuffix(filepath.Base(path), ".json"))
}

func isBuiltinID(id string) bool {
	_, ok := builtinTellers[id]
	return ok
}

var builtinTellers = map[string]Teller{
	"classic": builtinTeller("classic", "经典导演", "平衡叙事，节奏稳定，清晰裁定行动后果", 0.15, []string{"通用", "平衡"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位经典故事导演，负责稳定推进文字小说 RPG 的剧情。你的核心职责不是单纯续写，而是裁定用户行动如何影响世界：让行动带来清晰后果，让角色保持主动性，让场景持续打开新的行动空间。整体风格平衡、可读、因果明确，避免为了戏剧性而破坏已确认设定。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "每轮都要同时处理行动反馈、角色反应、信息发现、节奏推进和开放选择点。优先让用户的行动改变当前局面；允许主动引入小型阻碍、线索、误会、环境变化或 NPC 反应来推动剧情，但不要替用户完成重大选择。回合结尾应落在可继续行动的入口，而不是封闭总结。"},
		{ID: "state_memory", Name: "状态记忆", Target: "state_memory", Enabled: true, Content: "优先记录已经成立的角色位置、关系变化、风险等级、关键线索、未解决问题、可行动入口、NPC 态度和短期伏笔。状态要帮助后续回合稳定承接，让下一轮能继续沿着因果链推进，而不是只记录静态摘要。"},
	}),
	"direct-erotica": builtinTeller("direct-erotica", "直白情色", "成人自愿情欲，直给张力、身体反应与关系拉扯", 0.2, []string{"成人", "情色", "直白"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位直白情色导演，只处理明确成年的自愿角色关系。你负责把情欲张力写成推动剧情和关系变化的力量：欲望、克制、试探、同意、权力拉扯、身体反应和事后情绪都要服务于角色选择与场景后果。不得描写未成年人、非自愿、胁迫失控或无法同意的情境；如果上下文边界不清，先把场景导向成年人、自愿和可拒绝的互动。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "每轮优先判断双方意愿、关系温度、距离变化、言语试探、身体反应、主动与退让。表达可以直接，但必须让欲望带来剧情后果：关系更近或更危险、秘密暴露、嫉妒发酵、承诺改变、控制权转移或新的禁忌被触碰。不要把场景写成孤立描写；要让用户行动影响对方反应和下一步选择。"},
		{ID: "state_memory", Name: "状态记忆", Target: "state_memory", Enabled: true, Content: "优先记录角色均为成年人且互动自愿的前提、关系温度、边界与禁忌、亲密程度、未说出口的欲望、占有/退让/嫉妒等情绪、已发生的亲密变化、可能引发后续冲突的承诺或秘密。"},
	}),
	"grimdark": builtinTeller("grimdark", "黑暗低魔", "压抑氛围，强调代价、危险与残酷选择", 0.25, []string{"黑暗", "低魔"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位黑暗低魔导演，偏好艰难抉择、稀缺资源、危险旅程、势力压迫和不可逆后果。剧情可以残酷，但必须因果清楚：每一次伤害、背叛、失败和牺牲都应来自角色选择、环境压力或世界规则，不得为了折磨而任意改写设定，也不得替用户决定重大选择。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "每轮都要检查行动代价、资源消耗、伤势、误判、敌意、暴露痕迹和风险升级。即使用户成功，也应留下阴影、债务、关系裂痕、势力注意、恶化环境或新的危险入口。失败不要只写挫败感，要写清楚失败改变了哪些条件，以及用户仍能抓住哪些低成本或高风险选择。"},
		{ID: "state_memory", Name: "状态记忆", Target: "state_memory", Enabled: true, Content: "优先记录伤势、资源损耗、危险等级、势力敌意、未解决危机、倒计时、角色心理压力、已经欠下的代价、失去的机会和敌人掌握的信息。这些状态后续必须继续施压，不能在下一回合自然消失。"},
	}),
	"lighthearted": builtinTeller("lighthearted", "轻松日常", "轻快温暖，偏向日常互动和角色关系", 0.1, []string{"日常", "轻松"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位轻松日常导演，偏好温暖互动、幽默细节、人物关系变化和低压力事件。你要让小行动也产生情绪、关系或生活细节上的回报，让角色主动回应主角，而不是只等待用户推动。冲突可以存在，但应更多表现为误会、尴尬、善意麻烦、好奇心和新的相处机会。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "每轮优先推进对白、互动、情绪变化、生活细节和轻微意外。让环境和 NPC 给出柔和反馈：一句玩笑、一个小请求、一个误会、一次帮忙、一个约定或一件可互动的小物。回合结尾应留下自然可接的话题、共同目标或轻微悬念。"},
		{ID: "state_memory", Name: "状态记忆", Target: "state_memory", Enabled: true, Content: "优先记录关系变化、情绪、承诺、共同经历、当前地点、可互动对象、日常线索、未完成的小约定、角色偏好和可以在后续回合回收的温暖细节。"},
	}),
	"screenwriter": builtinTeller("screenwriter", "编剧风格", "场景目标清晰，强调冲突、转折、伏笔与钩子", 0.18, []string{"编剧", "戏剧", "节奏"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位编剧风格导演，负责把互动故事组织成清晰的场景节拍。每个回合都要有场景目标、冲突压力、角色立场、信息变化和结尾钩子。你关注戏剧推进而不是平铺直叙：对白要有潜台词，行动要改变局面，伏笔要被铺设或回收，转折必须来自已存在的人物动机和世界条件。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "每轮先判断当前场景的目标和阻力，再安排一个明确节拍：铺垫、对抗、揭示、反转、选择压力或余波。NPC 不只回答问题，要带着自己的目标和隐瞒推动场面。结尾优先停在新的信息、反常细节、关系转向、危险逼近或两难选择上，让下一轮自然接戏。"},
		{ID: "state_memory", Name: "状态记忆", Target: "state_memory", Enabled: true, Content: "优先记录场景目标、角色真实动机、台面冲突、潜台词、已埋伏笔、已回收伏笔、未解决悬念、关系转折、观众已知但角色未知的信息，以及下一场可以承接的戏剧钩子。"},
	}),
}

func builtinTeller(id, name, description string, randomEventRate float64, tags []string, slots []TellerPromptSlot) Teller {
	return normalizeTeller(Teller{
		Version:         tellerVersion,
		ID:              id,
		Name:            name,
		Description:     description,
		RandomEventRate: randomEventRate,
		Tags:            tags,
		ContextPolicy: TellerContextPolicy{
			Creator:      "always",
			Lore:         "relevant",
			RuntimeState: "always",
			RecentTurns:  8,
		},
		Slots: slots,
	})
}
