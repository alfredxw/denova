package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"denova/internal/narrativestyle"
	"denova/internal/styleref"
)

const (
	tellerVersion                  = 9
	MaxEventCardDescriptionChars   = 8000
	maxEventCardSummaryChars       = 240
	maxEventCardContextSummaryByte = 900
)

type TellerLibrary struct {
	novaDir string
}

var ErrTellerRevisionConflict = errors.New("叙事风格已被其他操作更新，请重新加载后再保存")

type Teller struct {
	Version           int                 `json:"version"`
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Description       string              `json:"description"`
	Modes             []string            `json:"modes,omitempty"`
	StyleRefs         []string            `json:"style_refs,omitempty"`
	StyleRules        []StyleRule         `json:"style_rules,omitempty"`
	ContextPolicy     TellerContextPolicy `json:"context_policy"`
	Slots             []TellerPromptSlot  `json:"slots"`
	Path              string              `json:"path,omitempty"`
	Custom            bool                `json:"custom"`
	BuiltinOverridden bool                `json:"builtin_overridden,omitempty"`
	Invalid           bool                `json:"invalid,omitempty"`
	Error             string              `json:"error,omitempty"`
	CreatedAt         string              `json:"created_at,omitempty"`
	UpdatedAt         string              `json:"updated_at,omitempty"`
}

type TellerContextPolicy struct {
	Creator      string `json:"creator"`
	Lore         string `json:"lore"`
	RuntimeState string `json:"runtime_state"`
}

type TellerPromptSlot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

type TellerEventPackage struct {
	ID      string            `json:"id,omitempty"`
	Name    string            `json:"name,omitempty"`
	Enabled bool              `json:"enabled"`
	Events  []TellerEventCard `json:"events,omitempty"`
}

// TellerEventCard is a reusable, creator-editable narrative event package card.
// Director Agent planning prompts receive it as a DirectorEvent, with Markdown
// stored in DirectorEvent.Template.
type TellerEventCard struct {
	ID                  string   `json:"id,omitempty"`
	TypeName            string   `json:"type_name,omitempty"`
	DescriptionMarkdown string   `json:"description_markdown,omitempty"`
	Enabled             bool     `json:"enabled"`
	Category            string   `json:"category,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	Intensity           string   `json:"intensity,omitempty"`
}

// StyleRule 表示叙事风格自己的「场景 → 共享文风参考」映射。
type StyleRule struct {
	Scene         string   `json:"scene"`
	StyleRefs     []string `json:"style_refs,omitempty"`
	StyleContents []string `json:"style_contents,omitempty"`
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
		teller = applyTellerOwnership(teller)
		tellers = append(tellers, teller)
	}
	sort.Slice(tellers, func(i, j int) bool {
		if (tellers[i].ID == "rhythm") != (tellers[j].ID == "rhythm") {
			return tellers[i].ID == "rhythm"
		}
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
	teller = applyTellerOwnership(teller)
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
	teller.BuiltinOverridden = false
	if err := validateTeller(teller); err != nil {
		return Teller{}, err
	}
	path := filepath.Join(l.dir(), teller.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return Teller{}, fmt.Errorf("导演 ID 已存在: %s", teller.ID)
	} else if !os.IsNotExist(err) {
		return Teller{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	teller.CreatedAt = now
	teller.UpdatedAt = now
	if err := writeTellerFile(path, teller); err != nil {
		return Teller{}, err
	}
	teller.Path = path
	teller = applyTellerOwnership(teller)
	return teller, nil
}

func (l *TellerLibrary) Update(id string, teller Teller, baseRevision ...string) (Teller, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Teller{}, err
	}
	if err := validateTellerID(id); err != nil {
		return Teller{}, err
	}
	isBuiltin := isBuiltinID(id)
	current, err := l.Get(id)
	if err != nil {
		return Teller{}, err
	}
	if firstTellerRevision(baseRevision) != "" && current.UpdatedAt != firstTellerRevision(baseRevision) {
		return Teller{}, ErrTellerRevisionConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	teller.ID = id
	teller.CreatedAt = current.CreatedAt
	if teller.CreatedAt == "" {
		teller.CreatedAt = now
	}
	teller.UpdatedAt = now
	teller.BuiltinOverridden = isBuiltin
	teller = normalizeTeller(teller)
	if err := validateTeller(teller); err != nil {
		return Teller{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeTellerFile(path, teller); err != nil {
		return Teller{}, err
	}
	teller.Path = path
	teller = applyTellerOwnership(teller)
	return teller, nil
}

func firstTellerRevision(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (l *TellerLibrary) Delete(id string) error {
	if err := validateTellerID(id); err != nil {
		return err
	}
	if isBuiltinID(id) {
		return l.restoreBuiltin(id)
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *TellerLibrary) restoreBuiltin(id string) error {
	teller, ok := builtinTellers[id]
	if !ok {
		return fmt.Errorf("内置叙事风格不存在: %s", id)
	}
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	return writeTellerFile(filepath.Join(l.dir(), id+".json"), teller)
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
		if parseErr == nil && current.BuiltinOverridden {
			continue
		}
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

func applyTellerOwnership(teller Teller) Teller {
	if !isBuiltinID(teller.ID) {
		teller.Custom = true
		teller.BuiltinOverridden = false
		return teller
	}
	teller.Custom = false
	teller.BuiltinOverridden = teller.BuiltinOverridden || tellerDiffersFromBuiltin(teller)
	return teller
}

func tellerDiffersFromBuiltin(teller Teller) bool {
	builtin, ok := builtinTellers[teller.ID]
	if !ok {
		return false
	}
	return !reflect.DeepEqual(tellerComparable(teller), tellerComparable(builtin))
}

func tellerComparable(teller Teller) Teller {
	teller = normalizeTeller(teller)
	teller.Path = ""
	teller.Custom = false
	teller.BuiltinOverridden = false
	teller.Invalid = false
	teller.Error = ""
	teller.CreatedAt = ""
	teller.UpdatedAt = ""
	return teller
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

// SupportsMode applies the legacy-compatible mode contract to one style.
func (t Teller) SupportsMode(mode string) bool {
	return narrativestyle.Supports(t.Modes, mode)
}

func normalizeTeller(teller Teller) Teller {
	teller.Version = tellerVersion
	teller.ID = strings.TrimSpace(teller.ID)
	teller.Name = strings.TrimSpace(teller.Name)
	teller.Description = strings.TrimSpace(teller.Description)
	teller.Modes = narrativestyle.NormalizeModes(teller.Modes)
	teller.StyleRefs = normalizeStyleRefs(teller.StyleRefs)
	teller.StyleRules = normalizeStyleRules(teller.StyleRules)
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
		refs := normalizeStyleRefs(rule.StyleRefs)
		contents := make([]string, 0, len(rule.StyleContents))
		seen := map[string]bool{}
		for _, content := range rule.StyleContents {
			content = strings.TrimSpace(content)
			if content == "" || seen[content] {
				continue
			}
			seen[content] = true
			contents = append(contents, content)
		}
		if len(refs) == 0 && len(contents) == 0 {
			continue
		}
		result = append(result, StyleRule{Scene: scene, StyleRefs: refs, StyleContents: contents})
	}
	return result
}

func normalizeStyleRefs(input []string) []string {
	refs := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, ref := range input {
		ref = styleref.NormalizeStoragePath(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	return refs
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

func defaultTellerEventCards() []TellerEventCard {
	templates := DefaultDirectorEventTemplates()
	cards := make([]TellerEventCard, 0, len(templates))
	for _, event := range templates {
		cards = append(cards, TellerEventCard{
			ID:                  event.ID,
			TypeName:            event.Name,
			DescriptionMarkdown: defaultTellerEventCardMarkdown(event),
			Enabled:             true,
			Category:            event.Category,
			Intensity:           event.Intensity,
		})
	}
	return cards
}

func defaultTellerEventCardMarkdown(event DirectorEvent) string {
	details := defaultTellerEventCardDetails(event)
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

func normalizeTellerEventPackagesNoDefault(packages []TellerEventPackage) []TellerEventPackage {
	if packages == nil {
		return []TellerEventPackage{}
	}
	result := make([]TellerEventPackage, 0, len(packages))
	seen := map[string]bool{}
	for _, pkg := range packages {
		pkg.ID = normalizeSlotID(pkg.ID)
		if pkg.ID == "" {
			pkg.ID = fmt.Sprintf("event-package-%d", len(result)+1)
		}
		if seen[pkg.ID] {
			continue
		}
		seen[pkg.ID] = true
		pkg.Name = strings.TrimSpace(pkg.Name)
		if pkg.Name == "" {
			pkg.Name = pkg.ID
		}
		pkg.Events = normalizeTellerEventCards(pkg.Events, pkg.ID)
		result = append(result, pkg)
	}
	return result
}

func normalizeTellerEventCards(events []TellerEventCard, packageID string) []TellerEventCard {
	result := make([]TellerEventCard, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		event.ID = normalizeSlotID(event.ID)
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
		if seen[event.ID] {
			continue
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

func normalizeRuleChecks(checks []RuleCheck) []RuleCheck {
	result := make([]RuleCheck, 0, len(checks))
	for i, check := range checks {
		if ruleCheckBlank(check) {
			continue
		}
		check = normalizeRuleCheck(check, i)
		if check.ID == "" && check.Label == "" {
			continue
		}
		result = append(result, check)
	}
	return result
}

func ruleCheckBlank(check RuleCheck) bool {
	return strings.TrimSpace(check.ID) == "" &&
		strings.TrimSpace(check.Label) == "" &&
		strings.TrimSpace(check.Dice) == "" &&
		strings.TrimSpace(check.FailurePolicy) == "" &&
		strings.TrimSpace(check.DifficultyGuidance) == "" &&
		strings.TrimSpace(check.StateEffectGuidance) == "" &&
		strings.TrimSpace(check.Trigger) == "" &&
		len(check.MustCheckExamples) == 0 &&
		len(check.SkipCheckExamples) == 0 &&
		len(check.StateBindings) == 0 &&
		strings.TrimSpace(check.SuccessHint) == "" &&
		strings.TrimSpace(check.FailureHint) == "" &&
		check.Modifier == 0
}

func normalizeStateOps(ops []StateOp) []StateOp {
	return normalizeStateOpsUnbounded(ops)
}

// normalizeStateOpsUnbounded keeps the explicit name used by public mutation
// paths while sharing the same lossless item-count semantics.
func normalizeStateOpsUnbounded(ops []StateOp) []StateOp {
	result := make([]StateOp, 0, len(ops))
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

func directorEventFromTellerEventCard(card TellerEventCard) DirectorEvent {
	summary := eventCardSummaryFromMarkdown(card.DescriptionMarkdown, card.TypeName)
	if summary == "" {
		summary = card.TypeName
	}
	return DirectorEvent{
		ID:                      card.ID,
		Name:                    card.TypeName,
		Category:                firstNonEmpty(card.Category, "自定义事件"),
		Status:                  "available",
		Enabled:                 true,
		Summary:                 summary,
		PublicSummary:           summary,
		Template:                card.DescriptionMarkdown,
		NormalizedTrigger:       firstNonEmpty(card.Category, card.TypeName, card.ID),
		Intensity:               card.Intensity,
		CompatibleGenres:        card.Tags,
		UserConfigured:          true,
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

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
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
			return fmt.Errorf("导演规则 %q 使用了无效注入位置 %q，仅支持 system、turn_context", slot.Name, slot.Target)
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
	case "system", "turn_context":
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
