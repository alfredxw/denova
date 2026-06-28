package imagepreset

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

const (
	Version           = 2
	DefaultID         = "game-cg"
	MaxPromptChars    = 4000
	TargetAgentSystem = "agent_system"
	TargetToolRequest = "tool_request"
	defaultToolSlotID = "tool_request"
)

type Library struct {
	novaDir string
}

var ErrPresetRevisionConflict = errors.New("图像方案已被其他操作更新，请重新加载后再保存")

type Preset struct {
	Version     int      `json:"version"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt,omitempty"`
	Slots       []Slot   `json:"slots,omitempty"`
	Tags        []string `json:"tags"`
	Path        string   `json:"path,omitempty"`
	Custom      bool     `json:"custom"`
	Invalid     bool     `json:"invalid,omitempty"`
	Error       string   `json:"error,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

type Slot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

func NewLibrary(novaDir string) *Library {
	return &Library{novaDir: novaDir}
}

func (l *Library) List() ([]Preset, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	presets := make([]Preset, 0, len(files))
	for _, file := range files {
		preset, err := parsePresetFile(file)
		if err != nil {
			presets = append(presets, Preset{
				ID:      strings.TrimSuffix(filepath.Base(file), ".json"),
				Path:    file,
				Invalid: true,
				Error:   err.Error(),
				Custom:  !isBuiltinFile(file),
			})
			continue
		}
		preset.Path = file
		preset.Custom = !IsBuiltinID(preset.ID)
		presets = append(presets, preset)
	}
	sort.Slice(presets, func(i, j int) bool {
		if presets[i].Custom != presets[j].Custom {
			return !presets[i].Custom
		}
		return presets[i].ID < presets[j].ID
	})
	return presets, nil
}

func (l *Library) Get(id string) (Preset, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Preset{}, err
	}
	id = NormalizeID(id)
	if id == "" {
		id = DefaultID
	}
	if err := validateID(id); err != nil {
		return Preset{}, err
	}
	preset, err := parsePresetFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return Preset{}, err
	}
	preset.Custom = !IsBuiltinID(preset.ID)
	return preset, nil
}

func (l *Library) Create(preset Preset) (Preset, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Preset{}, err
	}
	preset = normalizePreset(preset)
	if preset.ID == "" {
		preset.ID = newPresetID()
	}
	if err := validatePreset(preset); err != nil {
		return Preset{}, err
	}
	path := filepath.Join(l.dir(), preset.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return Preset{}, fmt.Errorf("图像方案 ID 已存在: %s", preset.ID)
	} else if !os.IsNotExist(err) {
		return Preset{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	preset.CreatedAt = now
	preset.UpdatedAt = now
	if err := writePresetFile(path, preset); err != nil {
		return Preset{}, err
	}
	preset.Path = path
	preset.Custom = !IsBuiltinID(preset.ID)
	return preset, nil
}

func (l *Library) Update(id string, preset Preset, baseRevision ...string) (Preset, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Preset{}, err
	}
	id = NormalizeID(id)
	if err := validateID(id); err != nil {
		return Preset{}, err
	}
	current, err := l.Get(id)
	if err != nil {
		return Preset{}, err
	}
	if firstPresetRevision(baseRevision) != "" && current.UpdatedAt != firstPresetRevision(baseRevision) {
		return Preset{}, ErrPresetRevisionConflict
	}
	preset.ID = id
	preset.CreatedAt = current.CreatedAt
	preset.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	preset = normalizePreset(preset)
	if err := validatePreset(preset); err != nil {
		return Preset{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writePresetFile(path, preset); err != nil {
		return Preset{}, err
	}
	preset.Path = path
	preset.Custom = !IsBuiltinID(preset.ID)
	return preset, nil
}

func firstPresetRevision(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (l *Library) Delete(id string) error {
	id = NormalizeID(id)
	if err := validateID(id); err != nil {
		return err
	}
	if IsBuiltinID(id) {
		return errors.New("内置图像方案不能删除")
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *Library) dir() string {
	return filepath.Join(l.novaDir, "image-presets")
}

func (l *Library) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	for id, preset := range builtinPresets {
		path := filepath.Join(l.dir(), id+".json")
		version, versionErr := readPresetFileVersion(path)
		current, parseErr := parsePresetFile(path)
		if versionErr == nil && parseErr == nil && current.Version == Version && version == Version {
			continue
		}
		if err := writePresetFile(path, preset); err != nil {
			return err
		}
	}
	return nil
}

func readPresetFileVersion(path string) (int, error) {
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

func parsePresetFile(path string) (Preset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Preset{}, err
	}
	var preset Preset
	if err := json.Unmarshal(data, &preset); err != nil {
		return Preset{}, fmt.Errorf("解析图像方案 JSON 失败: %w", err)
	}
	preset = normalizePreset(preset)
	if err := validatePreset(preset); err != nil {
		return Preset{}, err
	}
	preset.Path = path
	return preset, nil
}

func writePresetFile(path string, preset Preset) error {
	preset = normalizePreset(preset)
	data, err := json.MarshalIndent(preset, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func normalizePreset(preset Preset) Preset {
	preset.Version = Version
	preset.ID = NormalizeID(preset.ID)
	preset.Name = strings.TrimSpace(preset.Name)
	preset.Description = strings.TrimSpace(preset.Description)
	legacyPrompt := truncateRunes(strings.TrimSpace(preset.Prompt), MaxPromptChars)
	preset.Slots = normalizeSlots(preset.Slots)
	if len(preset.Slots) == 0 && legacyPrompt != "" {
		preset.Slots = []Slot{defaultToolRequestSlot(legacyPrompt)}
	}
	preset.Prompt = preset.PromptForTargets(TargetToolRequest)
	preset.Tags = normalizeTags(preset.Tags)
	return preset
}

func validatePreset(preset Preset) error {
	if err := validateID(preset.ID); err != nil {
		return err
	}
	if preset.Name == "" {
		return errors.New("图像方案名称不能为空")
	}
	if len(preset.Slots) == 0 {
		return errors.New("图像方案至少需要一个注入规则")
	}
	hasEnabledContent := false
	for _, slot := range preset.Slots {
		if !isAllowedSlotTarget(slot.Target) {
			return fmt.Errorf("图像方案规则 %q 使用了无效注入位置 %q，仅支持 agent_system、tool_request", slot.Name, slot.Target)
		}
		if slot.Enabled && strings.TrimSpace(slot.Content) != "" {
			hasEnabledContent = true
		}
	}
	if !hasEnabledContent {
		return errors.New("图像方案至少需要一个启用且非空的注入规则")
	}
	return nil
}

func validateID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("图像方案 ID 不能为空")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("图像方案 ID 包含非法字符: %s", id)
	}
	return nil
}

func normalizeTags(tags []string) []string {
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

func newPresetID() string {
	return fmt.Sprintf("image-preset-%d", time.Now().UTC().UnixNano())
}

func isBuiltinFile(path string) bool {
	return IsBuiltinID(strings.TrimSuffix(filepath.Base(path), ".json"))
}

func IsBuiltinID(id string) bool {
	_, ok := builtinPresets[NormalizeID(id)]
	return ok
}

func NormalizeID(id string) string {
	id = strings.TrimSpace(id)
	var sb strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func DefaultPreset() Preset {
	return builtinPresets[DefaultID]
}

func (p Preset) PromptForTargets(targets ...string) string {
	allowed := map[string]bool{}
	for _, target := range targets {
		allowed[strings.TrimSpace(target)] = true
	}
	if len(p.Slots) == 0 && allowed[TargetToolRequest] && strings.TrimSpace(p.Prompt) != "" {
		p.Slots = []Slot{defaultToolRequestSlot(p.Prompt)}
	}
	var sb strings.Builder
	for _, slot := range p.Slots {
		if !slot.Enabled || !allowed[slot.Target] || strings.TrimSpace(slot.Content) == "" {
			continue
		}
		fmt.Fprintf(&sb, "## %s（%s）\n\n%s\n\n", slot.Name, slot.Target, strings.TrimSpace(slot.Content))
	}
	return strings.TrimSpace(sb.String())
}

var builtinPresets = map[string]Preset{
	DefaultID: builtinPreset(
		DefaultID, "游戏CG",
		"偏互动游戏事件图、角色立绘与关键场景 CG", []string{"游戏", "CG"}, "以高质量插画 CG 视觉呈现，大师级作品，高细节，虚幻5：主体清晰，镜头有戏剧张力，构图服务行动与场景信息；角色服装、道具、环境细节要贴合已发生剧情。光影偏电影化，色彩饱满但不过曝，画面可作为互动剧情关键回合插图。避免文字、水印、logo、UI 面板和未来剧情剧透。"),
	"realistic": builtinPreset(
		"realistic", "写实",
		"偏摄影感、真实材质和自然光影", []string{"写实", "摄影"}, "以写实摄影感呈现：强调真实材质、自然光影、可信空间关系和克制的戏剧化处理。人物表情、姿态和环境痕迹应符合当前情境，避免夸张二次元比例、过度滤镜、塑料质感和不必要的幻想装饰。避免文字、水印、logo 和未来剧情剧透。"),
	"2d-illustration": builtinPreset(
		"2d-illustration", "2D插画",
		"偏小说插图、手绘感与清晰氛围表达", []string{"2D", "插画"}, "以精致 2D 插画呈现：线条干净，色块和光影层次清晰，适合小说章节插图和角色场景氛围图。构图要突出主体与情绪，背景保留足够叙事信息但不喧宾夺主。避免过度写实摄影质感、3D 渲染感、文字、水印、logo 和未来剧情剧透。"),
}

func builtinPreset(id, name, description string, tags []string, prompt string) Preset {
	return normalizePreset(Preset{
		Version:     Version,
		ID:          id,
		Name:        name,
		Description: description,
		Slots:       []Slot{defaultToolRequestSlot(prompt)},
		Tags:        tags,
	})
}

func normalizeSlots(slots []Slot) []Slot {
	result := make([]Slot, 0, len(slots))
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
		slot.Content = truncateRunes(strings.TrimSpace(slot.Content), MaxPromptChars)
		result = append(result, slot)
	}
	return result
}

func defaultToolRequestSlot(prompt string) Slot {
	return Slot{
		ID:      defaultToolSlotID,
		Name:    "图像请求 Prompt",
		Target:  TargetToolRequest,
		Enabled: true,
		Content: truncateRunes(strings.TrimSpace(prompt), MaxPromptChars),
	}
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
	case TargetAgentSystem, TargetToolRequest:
		return true
	default:
		return false
	}
}
