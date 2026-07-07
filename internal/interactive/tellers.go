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

	"denova/internal/styleref"
)

const (
	tellerVersion                  = 6
	MaxStyleRefsPerRule            = 12
	MaxStyleContentChars           = 8000
	MaxEventCardDescriptionChars   = 8000
	maxTellerEventCardsPerPackage  = 24
	maxEventCardSummaryChars       = 240
	maxEventCardContextSummaryByte = 900
)

type TellerLibrary struct {
	novaDir string
}

var ErrTellerRevisionConflict = errors.New("叙事风格已被其他操作更新，请重新加载后再保存")

type Teller struct {
	Version           int                        `json:"version"`
	ID                string                     `json:"id"`
	Name              string                     `json:"name"`
	Description       string                     `json:"description"`
	RandomEventRate   float64                    `json:"random_event_rate"`
	StyleRefs         []string                   `json:"style_refs,omitempty"`
	StyleRules        []StyleRule                `json:"style_rules,omitempty"`
	Orchestration     *TellerOrchestrationConfig `json:"orchestration,omitempty"`
	Tags              []string                   `json:"tags"`
	ContextPolicy     TellerContextPolicy        `json:"context_policy"`
	Slots             []TellerPromptSlot         `json:"slots"`
	Path              string                     `json:"path,omitempty"`
	Custom            bool                       `json:"custom"`
	BuiltinOverridden bool                       `json:"builtin_overridden,omitempty"`
	Invalid           bool                       `json:"invalid,omitempty"`
	Error             string                     `json:"error,omitempty"`
	CreatedAt         string                     `json:"created_at,omitempty"`
	UpdatedAt         string                     `json:"updated_at,omitempty"`
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

// TellerOrchestrationConfig stores reusable game-mode narrative orchestration
// defaults. Each story copies these into branch-scoped Director Plan templates
// and Director Agent planning inputs at creation time.
type TellerOrchestrationConfig struct {
	Enabled          bool                 `json:"enabled"`
	MainlineStrength string               `json:"mainline_strength,omitempty"`
	FailurePolicy    string               `json:"failure_policy,omitempty"`
	PacingCurve      string               `json:"pacing_curve,omitempty"`
	EventPackages    []TellerEventPackage `json:"event_packages,omitempty"`
	CustomEvents     []DirectorEvent      `json:"custom_events,omitempty"`
	RuleTemplates    []RuleCheck          `json:"rule_templates,omitempty"`
	Opening          TellerOpeningConfig  `json:"opening,omitempty"`
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
	Weight              float64  `json:"weight,omitempty"`
	CooldownTurns       int      `json:"cooldown_turns,omitempty"`
	Intensity           string   `json:"intensity,omitempty"`
}

type TellerOpeningConfig struct {
	Enabled         bool               `json:"enabled"`
	TraitPools      []OpeningTraitPool `json:"trait_pools,omitempty"`
	InitialStateOps []StateOp          `json:"initial_state_ops,omitempty"`
}

type OpeningTraitPool struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	DrawCount int            `json:"draw_count,omitempty"`
	Traits    []OpeningTrait `json:"traits,omitempty"`
}

type OpeningTrait struct {
	ID      string    `json:"id,omitempty"`
	Name    string    `json:"name,omitempty"`
	Summary string    `json:"summary,omitempty"`
	Weight  float64   `json:"weight,omitempty"`
	Ops     []StateOp `json:"ops,omitempty"`
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

func normalizeTeller(teller Teller) Teller {
	teller.Version = tellerVersion
	teller.ID = strings.TrimSpace(teller.ID)
	teller.Name = strings.TrimSpace(teller.Name)
	teller.Description = strings.TrimSpace(teller.Description)
	teller.StyleRefs = normalizeStyleRefs(teller.StyleRefs, MaxStyleRefsPerRule)
	teller.StyleRules = normalizeStyleRules(teller.StyleRules)
	teller.Orchestration = normalizeTellerOrchestrationPointer(teller.Orchestration)
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
		refs := normalizeStyleRefs(rule.StyleRefs, MaxStyleRefsPerRule)
		contents := make([]string, 0, len(rule.StyleContents))
		seen := map[string]bool{}
		for _, content := range rule.StyleContents {
			content = truncateRunes(strings.TrimSpace(content), MaxStyleContentChars)
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

func normalizeStyleRefs(input []string, max int) []string {
	if max <= 0 {
		return nil
	}
	refs := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, ref := range input {
		ref = styleref.NormalizeStoragePath(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
		if len(refs) >= max {
			break
		}
	}
	return refs
}

func DefaultTellerOrchestrationConfig() TellerOrchestrationConfig {
	return TellerOrchestrationConfig{
		Enabled:          true,
		MainlineStrength: "soft_guidance",
		FailurePolicy:    "reversible",
		PacingCurve:      "progressive",
		EventPackages: []TellerEventPackage{{
			ID:      "webnovel_core",
			Name:    "爽文核心事件包",
			Enabled: true,
			Events:  defaultTellerEventCards(),
		}},
		RuleTemplates: DefaultRuleCheckTemplates(),
		Opening:       TellerOpeningConfig{Enabled: true},
	}
}

func DefaultRuleCheckTemplates() []RuleCheck {
	return normalizeRuleChecks([]RuleCheck{
		{
			ID:                "high-risk-action",
			Label:             "高风险行动",
			Category:          "generic_action",
			DefaultDifficulty: "normal",
			DefaultRollMode:   "normal",
			FailurePolicy:     "fail_forward",
			Impact:            "none",
			Trigger:           "玩家采取存在明显风险、代价或不确定结果的行动，且直接叙述裁定会削弱互动张力时使用。",
			SuccessHint:       "成功时让行动达成核心目标，并给出清晰收益或信息推进。",
			FailureHint:       "失败时继续推进局势，但引入新的代价、压力或选择。",
		},
		{
			ID:                "combat-exchange",
			Label:             "战斗攻防",
			Category:          "combat",
			DefaultDifficulty: "normal",
			DefaultRollMode:   "normal",
			FailurePolicy:     "fail_forward",
			Impact:            "hp_damage",
			Trigger:           "攻击、防御、闪避、夺取战斗位置或承受敌方压制时使用。",
			SuccessHint:       "成功时扩大优势、造成有效压制或争取战斗节奏。",
			FailureHint:       "失败时让角色承受伤害、失去位置或暴露破绽，但保留下一步行动空间。",
		},
		{
			ID:                "stealth-lock",
			Label:             "潜行与开锁",
			Category:          "stealth",
			DefaultDifficulty: "normal",
			DefaultRollMode:   "normal",
			FailurePolicy:     "blocked",
			Impact:            "stamina_cost",
			Trigger:           "潜行、开锁、拆陷阱、绕过守卫或进行精细操作时使用。",
			SuccessHint:       "成功时悄无声息地越过阻碍，并保留行动主动权。",
			FailureHint:       "失败时让操作受阻、消耗时间或体力，并提高被发现的风险。",
		},
		{
			ID:                "investigation",
			Label:             "探索调查",
			Category:          "exploration",
			DefaultDifficulty: "normal",
			DefaultRollMode:   "normal",
			FailurePolicy:     "fail_forward",
			Impact:            "clue_progress",
			Trigger:           "搜索线索、辨认异常、分析痕迹、探索未知区域或判断信息真伪时使用。",
			SuccessHint:       "成功时给出可行动的关键线索，并连接到下一步选择。",
			FailureHint:       "失败时仍给出方向，但附带误导、遗漏、额外风险或时间压力。",
		},
		{
			ID:                "social-negotiation",
			Label:             "社交谈判",
			Category:          "social",
			DefaultDifficulty: "normal",
			DefaultRollMode:   "normal",
			FailurePolicy:     "success_at_cost",
			Impact:            "relationship_change",
			Trigger:           "说服、威胁、交易、套话、安抚、挑衅或争取 NPC 支持时使用。",
			SuccessHint:       "成功时让对方让步、透露信息或提供有限帮助。",
			FailureHint:       "失败时可以达成部分目标，但提高条件、暴露意图或损害关系。",
		},
		{
			ID:                "endurance-pressure",
			Label:             "体力/意志抗压",
			Category:          "endurance",
			DefaultDifficulty: "hard",
			DefaultRollMode:   "normal",
			FailurePolicy:     "success_at_cost",
			Impact:            "stamina_cost",
			Trigger:           "长时间奔逃、忍痛、抵抗诱惑/恐惧、强撑伤势或突破极限时使用。",
			SuccessHint:       "成功时角色稳住状态并争取关键窗口。",
			FailureHint:       "失败时允许勉强撑过，但留下疲惫、伤势、心理压力或后续劣势。",
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
			Weight:              event.Weight,
			CooldownTurns:       event.CooldownTurns,
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

func normalizeTellerOrchestrationPointer(config *TellerOrchestrationConfig) *TellerOrchestrationConfig {
	if config == nil {
		defaultConfig := DefaultTellerOrchestrationConfig()
		return &defaultConfig
	}
	normalized := *config
	normalized.MainlineStrength = normalizeOrchestrationOption(normalized.MainlineStrength, "soft_guidance")
	normalized.FailurePolicy = normalizeOrchestrationOption(normalized.FailurePolicy, "reversible")
	normalized.PacingCurve = normalizeOrchestrationOption(normalized.PacingCurve, "progressive")
	normalized.EventPackages = normalizeTellerEventPackages(normalized.EventPackages)
	normalized.CustomEvents = normalizeDirectorEvents(normalized.CustomEvents)
	normalized.RuleTemplates = normalizeRuleChecks(normalized.RuleTemplates)
	normalized.Opening = normalizeTellerOpeningConfig(normalized.Opening)
	return &normalized
}

func normalizeOrchestrationOption(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeTellerEventPackages(packages []TellerEventPackage) []TellerEventPackage {
	if packages == nil {
		return DefaultTellerOrchestrationConfig().EventPackages
	}
	return normalizeTellerEventPackagesNoDefault(packages)
}

func normalizeTellerEventPackagesNoDefault(packages []TellerEventPackage) []TellerEventPackage {
	if packages == nil {
		return []TellerEventPackage{}
	}
	if len(packages) > maxTurnBriefListItems {
		packages = packages[:maxTurnBriefListItems]
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
	if len(events) > maxTellerEventCardsPerPackage {
		events = events[:maxTellerEventCardsPerPackage]
	}
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
		event.DescriptionMarkdown = truncateRunes(strings.TrimSpace(event.DescriptionMarkdown), MaxEventCardDescriptionChars)
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
		event.Tags = normalizeStringListLimit(event.Tags, maxTurnBriefListItems)
		if event.Weight <= 0 {
			event.Weight = 1
		}
		if event.CooldownTurns < 0 {
			event.CooldownTurns = 0
		}
		event.Intensity = strings.TrimSpace(event.Intensity)
		if event.Intensity == "" {
			event.Intensity = "medium"
		}
		result = append(result, event)
	}
	return result
}

func normalizeRuleChecks(checks []RuleCheck) []RuleCheck {
	if len(checks) > maxRuleChecksPerTurn {
		checks = checks[:maxRuleChecksPerTurn]
	}
	result := make([]RuleCheck, 0, len(checks))
	for i, check := range checks {
		check = normalizeRuleCheck(check, i)
		if check.ID == "" && check.Label == "" {
			continue
		}
		result = append(result, check)
	}
	return result
}

func normalizeTellerOpeningConfig(config TellerOpeningConfig) TellerOpeningConfig {
	config.TraitPools = normalizeOpeningTraitPools(config.TraitPools)
	config.InitialStateOps = normalizeStateOps(config.InitialStateOps)
	return config
}

func normalizeOpeningTraitPools(pools []OpeningTraitPool) []OpeningTraitPool {
	if len(pools) > maxTurnBriefListItems {
		pools = pools[:maxTurnBriefListItems]
	}
	result := make([]OpeningTraitPool, 0, len(pools))
	seen := map[string]bool{}
	for _, pool := range pools {
		pool.ID = normalizeSlotID(pool.ID)
		if pool.ID == "" {
			pool.ID = fmt.Sprintf("trait-pool-%d", len(result)+1)
		}
		if seen[pool.ID] {
			continue
		}
		seen[pool.ID] = true
		pool.Name = strings.TrimSpace(pool.Name)
		if pool.Name == "" {
			pool.Name = pool.ID
		}
		if pool.DrawCount <= 0 {
			pool.DrawCount = 1
		}
		if pool.DrawCount > 10 {
			pool.DrawCount = 10
		}
		pool.Traits = normalizeOpeningTraits(pool.Traits)
		result = append(result, pool)
	}
	return result
}

func normalizeOpeningTraits(traits []OpeningTrait) []OpeningTrait {
	if len(traits) > 64 {
		traits = traits[:64]
	}
	result := make([]OpeningTrait, 0, len(traits))
	seen := map[string]bool{}
	for _, trait := range traits {
		trait.ID = normalizeSlotID(trait.ID)
		if trait.ID == "" {
			trait.ID = fmt.Sprintf("trait-%d", len(result)+1)
		}
		if seen[trait.ID] {
			continue
		}
		seen[trait.ID] = true
		trait.Name = strings.TrimSpace(trait.Name)
		if trait.Name == "" {
			trait.Name = trait.ID
		}
		trait.Summary = truncateRunes(strings.TrimSpace(trait.Summary), 1000)
		if trait.Weight <= 0 {
			trait.Weight = 1
		}
		trait.Ops = normalizeStateOps(trait.Ops)
		result = append(result, trait)
	}
	return result
}

func normalizeStateOps(ops []StateOp) []StateOp {
	if len(ops) > maxTurnBriefListItems {
		ops = ops[:maxTurnBriefListItems]
	}
	result := make([]StateOp, 0, len(ops))
	for _, op := range ops {
		op.Op = strings.TrimSpace(op.Op)
		op.Path = canonicalStatePath(op.Path)
		op.Reason = trimBytes(op.Reason, maxTurnBriefTextBytes)
		op.SourceTurnID = trimBytes(op.SourceTurnID, 128)
		if op.Op == "" || op.Path == "" {
			continue
		}
		result = append(result, op)
	}
	return result
}

// DirectorEventCatalogFromTeller returns built-in and teller-defined event cards
// in runtime DirectorEvent form for Director Agent planning prompts.
func DirectorEventCatalogFromTeller(teller Teller) []DirectorEvent {
	teller = normalizeTeller(teller)
	events := []DirectorEvent{}
	if teller.Orchestration == nil {
		return appendDefaultDirectorEventTemplates(events)
	}
	for _, pkg := range teller.Orchestration.EventPackages {
		for _, eventCard := range pkg.Events {
			if !eventCard.Enabled {
				continue
			}
			events = upsertDirectorEvent(events, directorEventFromTellerEventCard(eventCard))
		}
	}
	for _, event := range teller.Orchestration.CustomEvents {
		events = upsertDirectorEvent(events, event)
	}
	return appendDefaultDirectorEventTemplates(events)
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
		Weight:                  card.Weight,
		CooldownTurns:           card.CooldownTurns,
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
	"classic": builtinTeller("classic", "经典叙事", "平衡叙事，节奏稳定，清晰裁定行动后果", 0.15, []string{"通用", "平衡"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位经典故事导演，负责稳定推进文字小说 RPG 的剧情。你的核心职责不是单纯续写，而是裁定用户行动如何影响世界：让行动带来清晰后果，让角色保持主动性，让场景持续打开新的行动空间。整体风格平衡、可读、因果明确，避免为了戏剧性而破坏已确认设定。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "每轮都要同时处理行动反馈、角色反应、信息发现、节奏推进和开放选择点。优先让用户的行动改变当前局面；允许主动引入小型阻碍、线索、误会、环境变化或 NPC 反应来推动剧情，但不要替用户完成重大选择。回合结尾应落在可继续行动的入口，而不是封闭总结。"},
		{ID: "state_memory", Name: "记忆沉淀规则", Target: "state_memory", Enabled: true, Content: "优先记录已经成立的角色位置、关系变化、风险等级、关键线索、未解决问题、可行动入口、NPC 态度和短期伏笔。状态要帮助后续回合稳定承接，让下一轮能继续沿着因果链推进，而不是只记录静态摘要。"},
	}),
	"grimdark": builtinTeller("grimdark", "黑暗低魔", "压抑氛围，强调代价、危险与残酷选择", 0.25, []string{"黑暗", "低魔"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位黑暗低魔导演，偏好艰难抉择、稀缺资源、危险旅程、势力压迫和不可逆后果。剧情可以残酷，但必须因果清楚：每一次伤害、背叛、失败和牺牲都应来自角色选择、环境压力或世界规则，不得为了折磨而任意改写设定，也不得替用户决定重大选择。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "每轮都要检查行动代价、资源消耗、伤势、误判、敌意、暴露痕迹和风险升级。即使用户成功，也应留下阴影、债务、关系裂痕、势力注意、恶化环境或新的危险入口。失败不要只写挫败感，要写清楚失败改变了哪些条件，以及用户仍能抓住哪些低成本或高风险选择。"},
		{ID: "state_memory", Name: "记忆沉淀规则", Target: "state_memory", Enabled: true, Content: "优先记录伤势、资源损耗、危险等级、势力敌意、未解决危机、倒计时、角色心理压力、已经欠下的代价、失去的机会和敌人掌握的信息。这些状态后续必须继续施压，不能在下一回合自然消失。"},
	}),
	"screenwriter": builtinTeller("screenwriter", "编剧风格", "以场景目标、冲突升级和转折节拍推动剧情", 0.18, []string{"编剧", "节拍"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一位编剧式叙事风格助手，负责把互动小说回合组织成清晰的场景节拍。你关注场景目标、人物欲望、冲突升级、信息揭示和转折收束；每轮都要让角色行动产生戏剧后果，但不得替用户完成关键选择。叙事应有镜头感、动作线和对白推进，避免只写内心总结。"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "处理本轮时，先判断当前场景的目标和阻力，再安排一个可见的行动反馈、一个关系或信息变化，以及一个推动下一拍的开放入口。成功要带来新压力，失败要留下可继续尝试的路径；如果场景已经达到高潮，应及时给出转折、代价或短暂收束，而不是无限拖延同一冲突。"},
		{ID: "state_memory", Name: "记忆沉淀规则", Target: "state_memory", Enabled: true, Content: "优先记录场景目标、当前冲突层级、已揭示信息、角色欲望变化、未兑现伏笔、下一场景入口和需要回收的转折。状态要服务后续节拍安排，帮助下一轮判断是继续升级、反转、缓和还是切换场景。"},
	}),
	"direct-erotica": builtinTeller("direct-erotica", "直白情色", "以事件驱动故事，自然导向情色场景，文风直白粗俗", 0.2, []string{"成人", "情色", "直白"}, []TellerPromptSlot{
		{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "你是一个专为情色小说服务的讲述者，风格直白粗俗，毫不隐晦。你的任务是设计各种事件和情境，将故事自然导向情色发展，而非每轮强制插入性爱场面。你要让主角的行动影响剧情，让性成为情节的自然结果。使用最直接、最露骨的词汇描写性器官、性行为和性快感，避免委婉语或文艺修饰。保持剧情连贯，让情色内容服务于故事推进。必须使用下流露骨的词汇刻画性器官和其他身体部位，包括但不限于“奶子,乳房,奶头,乳头,小穴,阴道,尻穴,逼,逼穴,阴蒂,阴唇,屁眼,肛门,菊花,肉棒,鸡巴,精液”等。禁止性器官使用：花心 / 肉刃 / 花径 / 花穴 等美化词汇"},
		{ID: "turn_context", Name: "本轮上下文", Target: "turn_context", Enabled: true, Content: "根据当前剧情阶段，决定是否引入情色元素。在铺垫期，重点推进故事、制造悬念、塑造人物关系、安排诱惑或暧昧情境；当情节点积累到合适时，才展开直白粗俗的性爱描写。不要强行让每轮都发生性行为，而是让事件驱动角色进入情色场景。描写时仍采用直白语言，详细刻画身体反应、性行为过程，但需确保情节合理、节奏有起伏。"},
		{ID: "state_memory", Name: "记忆沉淀规则", Target: "state_memory", Enabled: true, Content: "优先记录当前故事事件的发展阶段、酝酿中的情色伏笔、角色间的性紧张度、已建立的性关系、以及后续可能导向情色的情节入口。同时记录性兴奋程度、性行为历史、身体敏感点等，以备情色场景展开时使用。"},
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
		},
		Slots: slots,
	})
}
