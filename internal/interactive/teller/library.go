package teller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"denova/internal/revisionfile"
	"denova/internal/revisionjson"
	"denova/internal/style"
)

const (
	tellerVersion = 9
)

type Library struct {
	novaDir string
}

var ErrRevisionConflict = errors.New("叙事风格已被其他操作更新，请重新加载后再保存")

type Definition struct {
	Version           int           `json:"version"`
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	StyleRefs         []string      `json:"style_refs,omitempty"`
	StyleRules        []StyleRule   `json:"style_rules,omitempty"`
	ContextPolicy     ContextPolicy `json:"context_policy"`
	Slots             []PromptSlot  `json:"slots"`
	Path              string        `json:"path,omitempty"`
	Custom            bool          `json:"custom"`
	BuiltinOverridden bool          `json:"builtin_overridden,omitempty"`
	Invalid           bool          `json:"invalid,omitempty"`
	Error             string        `json:"error,omitempty"`
	CreatedAt         string        `json:"created_at,omitempty"`
	UpdatedAt         string        `json:"updated_at,omitempty"`
	Revision          string        `json:"revision,omitempty"`
}

type ContextPolicy struct {
	Creator      string `json:"creator"`
	Lore         string `json:"lore"`
	RuntimeState string `json:"runtime_state"`
}

type PromptSlot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

// StyleRule 表示叙事风格自己的「场景 → 共享文风参考」映射。
type StyleRule struct {
	Scene         string   `json:"scene"`
	StyleRefs     []string `json:"style_refs,omitempty"`
	StyleContents []string `json:"style_contents,omitempty"`
}

func NewLibrary(novaDir string) *Library {
	return &Library{novaDir: novaDir}
}

func (l *Library) List() ([]Definition, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	tellers := make([]Definition, 0, len(files))
	for _, file := range files {
		teller, err := parseTellerFile(file)
		if err != nil {
			tellers = append(tellers, Definition{
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

func (l *Library) Get(id string) (Definition, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Definition{}, err
	}
	if err := validateTellerID(id); err != nil {
		return Definition{}, err
	}
	teller, err := parseTellerFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return Definition{}, err
	}
	teller = applyTellerOwnership(teller)
	return teller, nil
}

func (l *Library) Create(teller Definition) (Definition, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Definition{}, err
	}
	teller = Normalize(teller)
	if teller.ID == "" {
		teller.ID = newTellerID()
	}
	teller.BuiltinOverridden = false
	if err := validateTeller(teller); err != nil {
		return Definition{}, err
	}
	path := filepath.Join(l.dir(), teller.ID+".json")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	teller.CreatedAt = now
	teller.UpdatedAt = now
	document, err := tellerFileStore(path).Create(context.Background(), teller)
	if errors.Is(err, revisionjson.ErrAlreadyExists) {
		return Definition{}, fmt.Errorf("导演 ID 已存在: %s", teller.ID)
	}
	if err != nil {
		return Definition{}, err
	}
	teller = document.Value
	teller.Path, teller.Revision = path, document.Revision
	teller = applyTellerOwnership(teller)
	return teller, nil
}

func (l *Library) Update(id string, teller Definition, baseRevision string) (Definition, error) {
	if err := l.ensureBuiltins(); err != nil {
		return Definition{}, err
	}
	if err := validateTellerID(id); err != nil {
		return Definition{}, err
	}
	teller.ID = id
	teller = Normalize(teller)
	if err := validateTeller(teller); err != nil {
		return Definition{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	document, err := tellerFileStore(path).Update(context.Background(), baseRevision, func(current Definition) (Definition, error) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		teller.CreatedAt = current.CreatedAt
		if teller.CreatedAt == "" {
			teller.CreatedAt = now
		}
		teller.UpdatedAt = now
		teller.BuiltinOverridden = isBuiltinID(id)
		return teller, validateTeller(teller)
	})
	if err != nil {
		if errors.Is(err, revisionfile.ErrRevisionConflict) || errors.Is(err, revisionjson.ErrRevisionRequired) {
			return Definition{}, fmt.Errorf("%w: %v", ErrRevisionConflict, err)
		}
		return Definition{}, err
	}
	teller = document.Value
	teller.Path, teller.Revision = path, document.Revision
	teller = applyTellerOwnership(teller)
	return teller, nil
}

func (l *Library) Delete(id string) error {
	if err := validateTellerID(id); err != nil {
		return err
	}
	if isBuiltinID(id) {
		return l.restoreBuiltin(id)
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *Library) restoreBuiltin(id string) error {
	teller, ok := builtinTellers[id]
	if !ok {
		return fmt.Errorf("内置叙事风格不存在: %s", id)
	}
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	return writeTellerFile(filepath.Join(l.dir(), id+".json"), teller)
}

func (l *Library) dir() string {
	return filepath.Join(l.novaDir, "story-tellers")
}

func (l *Library) ensureBuiltins() error {
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

func parseTellerFile(path string) (Definition, error) {
	document, err := tellerFileStore(path).Read(context.Background())
	if err != nil {
		return Definition{}, err
	}
	teller := document.Value
	teller.Path = path
	teller.Revision = document.Revision
	return teller, nil
}

func writeTellerFile(path string, teller Definition) error {
	_, err := tellerFileStore(path).Replace(context.Background(), teller)
	return err
}

func tellerFileStore(path string) revisionjson.Store[Definition] {
	return revisionjson.NewStore(path, revisionjson.Codec[Definition]{
		Decode: func(data []byte) (Definition, error) {
			var teller Definition
			if err := json.Unmarshal(data, &teller); err != nil {
				return Definition{}, fmt.Errorf("解析导演 JSON 失败: %w", err)
			}
			teller = Normalize(teller)
			return teller, validateTeller(teller)
		},
		Encode: func(teller Definition) ([]byte, error) {
			teller = Normalize(teller)
			if err := validateTeller(teller); err != nil {
				return nil, err
			}
			teller.Path, teller.Revision, teller.Error = "", "", ""
			teller.Invalid, teller.Custom = false, false
			data, err := json.MarshalIndent(teller, "", "  ")
			return append(data, '\n'), err
		},
	})
}

func applyTellerOwnership(teller Definition) Definition {
	if !isBuiltinID(teller.ID) {
		teller.Custom = true
		teller.BuiltinOverridden = false
		return teller
	}
	teller.Custom = false
	teller.BuiltinOverridden = teller.BuiltinOverridden || tellerDiffersFromBuiltin(teller)
	return teller
}

func tellerDiffersFromBuiltin(teller Definition) bool {
	builtin, ok := builtinTellers[teller.ID]
	if !ok {
		return false
	}
	return !reflect.DeepEqual(tellerComparable(teller), tellerComparable(builtin))
}

func tellerComparable(teller Definition) Definition {
	teller = Normalize(teller)
	teller.Path = ""
	teller.Custom = false
	teller.BuiltinOverridden = false
	teller.Invalid = false
	teller.Error = ""
	teller.CreatedAt = ""
	teller.UpdatedAt = ""
	teller.Revision = ""
	return teller
}

func (t Definition) PromptForTargets(targets ...string) string {
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

// Normalize canonicalizes one definition before persistence or comparison.
func Normalize(teller Definition) Definition {
	teller.Version = tellerVersion
	teller.ID = strings.TrimSpace(teller.ID)
	teller.Name = strings.TrimSpace(teller.Name)
	teller.Description = strings.TrimSpace(teller.Description)
	teller.StyleRefs = normalizeStyleRefs(teller.StyleRefs)
	teller.StyleRules = NormalizeStyleRules(teller.StyleRules)
	teller.ContextPolicy = normalizeContextPolicy(teller.ContextPolicy)
	teller.Slots = normalizePromptSlots(teller.Slots)
	return teller
}

// NormalizeStyleRules canonicalizes and de-duplicates scene style references.
func NormalizeStyleRules(rules []StyleRule) []StyleRule {
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
		ref = style.NormalizeStoragePath(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	return refs
}

func normalizeContextPolicy(policy ContextPolicy) ContextPolicy {
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

func normalizePromptSlots(slots []PromptSlot) []PromptSlot {
	result := make([]PromptSlot, 0, len(slots))
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

func validateTeller(teller Definition) error {
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
