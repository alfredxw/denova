package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"denova/internal/revisionfile"
	"denova/internal/revisionjson"
)

const (
	storyDirectorVersion   = 5
	DefaultStoryDirectorID = "default"

	MaxStoryDirectorStrategyPromptBytes = StoryContextMaxBytes
	DefaultRuleVisibilityMode           = RuleVisibilityModeAuditOnly
	RuleVisibilityModeAuditOnly         = "audit_only"
	RuleVisibilityModePublicRoll        = "public_roll"
)

var ErrStoryDirectorRevisionConflict = errors.New("游戏预设已被其他操作更新，请重新加载后再保存 / Game Preset changed elsewhere; reload before saving")

type StoryDirectorLibrary struct {
	novaDir string
}

type StoryDirector struct {
	Version           int                           `json:"version"`
	ID                string                        `json:"id"`
	Name              string                        `json:"name"`
	Description       string                        `json:"description"`
	ModuleRefs        StoryDirectorModuleRefs       `json:"module_refs,omitempty"`
	Strategy          StoryDirectorStrategy         `json:"strategy"`
	EventPackages     []EventPackage                `json:"event_packages,omitempty"`
	TRPGSystem        StoryDirectorTRPGSystem       `json:"trpg_system"`
	ActorState        StoryDirectorActorStateSystem `json:"actor_state,omitempty"`
	ResolvedSnapshot  StoryDirectorResolvedSnapshot `json:"resolved_snapshot,omitempty"`
	Path              string                        `json:"path,omitempty"`
	Custom            bool                          `json:"custom"`
	BuiltinOverridden bool                          `json:"builtin_overridden,omitempty"`
	Invalid           bool                          `json:"invalid,omitempty"`
	Error             string                        `json:"error,omitempty"`
	CreatedAt         string                        `json:"created_at,omitempty"`
	UpdatedAt         string                        `json:"updated_at,omitempty"`
	Revision          string                        `json:"revision,omitempty"`
}

type StoryDirectorStrategy struct {
	RuleStateConsumptionMode string `json:"rule_state_consumption_mode,omitempty"`
	RuleVisibilityMode       string `json:"rule_visibility_mode,omitempty"`
	PromptMarkdown           string `json:"prompt_markdown,omitempty"`
}

type StoryDirectorTRPGSystem struct {
	RuleTemplates []RuleCheck `json:"rule_templates,omitempty"`
}

func NewStoryDirectorLibrary(novaDir string) *StoryDirectorLibrary {
	return &StoryDirectorLibrary{novaDir: novaDir}
}

func (l *StoryDirectorLibrary) List() ([]StoryDirector, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	directors := make([]StoryDirector, 0, len(files))
	for _, file := range files {
		director, err := parseStoryDirectorFile(file)
		if err != nil {
			directors = append(directors, StoryDirector{
				ID:      strings.TrimSuffix(filepath.Base(file), ".json"),
				Path:    file,
				Invalid: true,
				Error:   err.Error(),
				Custom:  !isBuiltinStoryDirectorFile(file),
			})
			continue
		}
		director.Path = file
		director = applyStoryDirectorOwnership(director)
		director = ResolveStoryDirectorModules(l.novaDir, director)
		director = persistResolvedStoryDirectorSnapshot(file, director)
		directors = append(directors, director)
	}
	sort.Slice(directors, func(i, j int) bool {
		if directors[i].Custom != directors[j].Custom {
			return !directors[i].Custom
		}
		return directors[i].ID < directors[j].ID
	})
	return directors, nil
}

func (l *StoryDirectorLibrary) Get(id string) (StoryDirector, error) {
	if err := l.ensureBuiltins(); err != nil {
		return StoryDirector{}, err
	}
	id = NormalizeStoryDirectorID(id)
	if id == "" {
		id = DefaultStoryDirectorID
	}
	if err := validateStoryDirectorID(id); err != nil {
		return StoryDirector{}, err
	}
	director, err := parseStoryDirectorFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return StoryDirector{}, err
	}
	director = applyStoryDirectorOwnership(director)
	director = ResolveStoryDirectorModules(l.novaDir, director)
	director = persistResolvedStoryDirectorSnapshot(filepath.Join(l.dir(), id+".json"), director)
	return director, nil
}

func (l *StoryDirectorLibrary) Create(director StoryDirector) (StoryDirector, error) {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return StoryDirector{}, err
	}
	if err := validateStoryDirectorWriteBounds(director); err != nil {
		return StoryDirector{}, err
	}
	director = normalizeStoryDirector(director)
	if director.ID == "" {
		director.ID = newStoryDirectorID(director.Name)
	}
	director.BuiltinOverridden = false
	if err := validateStoryDirectorID(director.ID); err != nil {
		return StoryDirector{}, err
	}
	path := filepath.Join(l.dir(), director.ID+".json")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	director.CreatedAt = firstNonEmptyString(director.CreatedAt, now)
	director.UpdatedAt = now
	director = ResolveStoryDirectorModules(l.novaDir, director)
	document, err := storyDirectorFileStore(path).Create(context.Background(), director)
	if errors.Is(err, revisionjson.ErrAlreadyExists) {
		return StoryDirector{}, fmt.Errorf("游戏预设已存在 / Game Preset already exists: %s", director.ID)
	}
	if err != nil {
		return StoryDirector{}, err
	}
	director = document.Value
	director.Path, director.Revision = path, document.Revision
	return applyStoryDirectorOwnership(director), nil
}

func (l *StoryDirectorLibrary) Update(id string, director StoryDirector, baseRevision string) (StoryDirector, error) {
	if err := l.ensureBuiltins(); err != nil {
		return StoryDirector{}, err
	}
	id = NormalizeStoryDirectorID(id)
	if err := validateStoryDirectorID(id); err != nil {
		return StoryDirector{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := validateStoryDirectorWriteBounds(director); err != nil {
		return StoryDirector{}, err
	}
	director = normalizeStoryDirector(director)
	director.ID = id
	director = ResolveStoryDirectorModules(l.novaDir, director)
	document, err := storyDirectorFileStore(path).Update(context.Background(), baseRevision, func(current StoryDirector) (StoryDirector, error) {
		director.CreatedAt = firstNonEmptyString(current.CreatedAt, director.CreatedAt)
		director.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		director.BuiltinOverridden = IsBuiltinStoryDirectorID(id)
		return director, validatePersistedStoryDirector(director)
	})
	if err != nil {
		if errors.Is(err, revisionfile.ErrRevisionConflict) || errors.Is(err, revisionjson.ErrRevisionRequired) {
			return StoryDirector{}, fmt.Errorf("%w: %v", ErrStoryDirectorRevisionConflict, err)
		}
		return StoryDirector{}, err
	}
	director = document.Value
	director.Path, director.Revision = path, document.Revision
	return applyStoryDirectorOwnership(director), nil
}

func (l *StoryDirectorLibrary) Delete(id string) error {
	id = NormalizeStoryDirectorID(id)
	if err := validateStoryDirectorID(id); err != nil {
		return err
	}
	if IsBuiltinStoryDirectorID(id) {
		return writeStoryDirectorFile(filepath.Join(l.dir(), id+".json"), DefaultStoryDirector())
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *StoryDirectorLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-directors")
}

func (l *StoryDirectorLibrary) ensureBuiltins() error {
	if err := NewEventPackageLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := NewRuleSystemLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := NewActorStateLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(l.dir(), DefaultStoryDirectorID+".json")
	version, versionErr := readStoryDirectorFileVersion(path)
	current, parseErr := parseStoryDirectorFile(path)
	if parseErr == nil && current.BuiltinOverridden {
		return nil
	}
	if versionErr == nil && parseErr == nil && current.Version == storyDirectorVersion && version == storyDirectorVersion {
		return nil
	}
	return writeStoryDirectorFile(path, DefaultStoryDirector())
}

func readStoryDirectorFileVersion(path string) (int, error) {
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

func parseStoryDirectorFile(path string) (StoryDirector, error) {
	document, err := storyDirectorFileStore(path).Read(context.Background())
	if err != nil {
		return StoryDirector{}, err
	}
	director := document.Value
	director.Path = path
	director.Revision = document.Revision
	return applyStoryDirectorOwnership(director), nil
}

func decodeStoryDirectorJSON(data []byte) (StoryDirector, error) {
	var director StoryDirector
	if err := json.Unmarshal(data, &director); err != nil {
		return StoryDirector{}, err
	}
	return director, nil
}

func writeStoryDirectorFile(path string, director StoryDirector) error {
	_, err := storyDirectorFileStore(path).Replace(context.Background(), director)
	return err
}

func storyDirectorFileStore(path string) revisionjson.Store[StoryDirector] {
	return revisionjson.NewStore(path, revisionjson.Codec[StoryDirector]{
		Decode: func(data []byte) (StoryDirector, error) {
			director, err := decodeStoryDirectorJSON(data)
			if err != nil {
				return StoryDirector{}, fmt.Errorf("解析游戏预设 JSON 失败 / Failed to parse Game Preset JSON: %w", err)
			}
			director = normalizeStoryDirector(director)
			return director, validatePersistedStoryDirector(director)
		},
		Encode: func(director StoryDirector) ([]byte, error) {
			director = normalizeStoryDirector(director)
			if err := validatePersistedStoryDirector(director); err != nil {
				return nil, err
			}
			director.Path, director.Revision, director.Error = "", "", ""
			director.Invalid, director.Custom = false, false
			data, err := json.MarshalIndent(director, "", "  ")
			return append(data, '\n'), err
		},
	})
}

func validatePersistedStoryDirector(director StoryDirector) error {
	if err := validateStoryDirectorID(director.ID); err != nil {
		return err
	}
	return validateStoryDirectorWriteBounds(director)
}

func persistResolvedStoryDirectorSnapshot(path string, director StoryDirector) StoryDirector {
	if path == "" || IsBuiltinStoryDirectorID(director.ID) || director.Invalid {
		return director
	}
	document, err := storyDirectorFileStore(path).Update(context.Background(), director.Revision, func(StoryDirector) (StoryDirector, error) {
		return director, nil
	})
	if err != nil {
		slog.Warn("persist resolved game preset snapshot failed", "path", path, "error", err)
		if latest, readErr := parseStoryDirectorFile(path); readErr == nil {
			return ResolveStoryDirectorModules(filepath.Dir(filepath.Dir(path)), latest)
		}
		return director
	}
	director = document.Value
	director.Path, director.Revision = path, document.Revision
	return applyStoryDirectorOwnership(director)
}

func applyStoryDirectorOwnership(director StoryDirector) StoryDirector {
	if !IsBuiltinStoryDirectorID(director.ID) {
		director.Custom = true
		director.BuiltinOverridden = false
		return director
	}
	director.Custom = false
	director.BuiltinOverridden = director.BuiltinOverridden || storyDirectorDiffersFromBuiltin(director)
	return director
}

func storyDirectorDiffersFromBuiltin(director StoryDirector) bool {
	return !reflect.DeepEqual(storyDirectorComparable(director), storyDirectorComparable(DefaultStoryDirector()))
}

func storyDirectorComparable(director StoryDirector) StoryDirector {
	director = normalizeStoryDirector(director)
	if snapshot := FreezeActorStateSchema(director.ActorState, false); snapshot != nil {
		director.ActorState = snapshot.System
	}
	director.Path = ""
	director.Custom = false
	director.BuiltinOverridden = false
	director.Invalid = false
	director.Error = ""
	director.CreatedAt = ""
	director.UpdatedAt = ""
	director.Revision = ""
	director.ResolvedSnapshot = StoryDirectorResolvedSnapshot{}
	return director
}

func DefaultStoryDirector() StoryDirector {
	refs := DefaultStoryDirectorModuleRefs()
	defaultActorState := DefaultActorStateModule()
	return normalizeStoryDirector(StoryDirector{
		Version:     storyDirectorVersion,
		ID:          DefaultStoryDirectorID,
		Name:        "默认游戏预设",
		Description: "组合通用叙事风格、事件素材、规则、状态与图像方案，并允许创作者自由描述 Game Agent 的规划风格。",
		ModuleRefs:  refs,
		Strategy: StoryDirectorStrategy{
			RuleStateConsumptionMode: DefaultRuleStateConsumptionMode,
			RuleVisibilityMode:       DefaultRuleVisibilityMode,
		},
		EventPackages: []EventPackage{tellerEventPackageFromModule(DefaultEventPackageModule())},
		TRPGSystem:    DefaultRuleSystemModule().TRPGSystem,
		ActorState:    defaultActorState.ActorState,
	})
}

func normalizeStoryDirector(director StoryDirector) StoryDirector {
	director.Version = storyDirectorVersion
	director.ID = NormalizeStoryDirectorID(director.ID)
	director.Name = trimBytes(firstNonEmptyString(director.Name, director.ID, "游戏预设"), 256)
	director.Description = trimBytes(director.Description, 1024)
	director.ModuleRefs = NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
	if StoryDirectorModuleRefsEmpty(director.ModuleRefs) {
		director.ModuleRefs = DefaultStoryDirectorModuleRefs()
	}
	director.Strategy = normalizeStoryDirectorStrategy(director.Strategy)
	director.EventPackages = normalizeEventPackagesNoDefault(director.EventPackages)
	director.TRPGSystem.RuleTemplates = normalizeRuleChecks(director.TRPGSystem.RuleTemplates)
	if director.ModuleRefs.ActorStateDisabled {
		director.ActorState = normalizeActorStateSystem(StoryDirectorActorStateSystem{})
	} else {
		director.ActorState = normalizeActorStateSystem(director.ActorState)
	}
	director.TRPGSystem = resolveRuleStateFieldIDs(director.ActorState, director.TRPGSystem)
	director.ResolvedSnapshot = normalizeStoryDirectorResolvedSnapshot(director.ResolvedSnapshot)
	return director
}

func NormalizeStoryDirectorStrategy(strategy StoryDirectorStrategy) StoryDirectorStrategy {
	return normalizeStoryDirectorStrategy(strategy)
}

func normalizeStoryDirectorStrategy(strategy StoryDirectorStrategy) StoryDirectorStrategy {
	strategy.RuleStateConsumptionMode = normalizeRuleStateConsumptionMode(strategy.RuleStateConsumptionMode)
	strategy.RuleVisibilityMode = normalizeRuleVisibilityMode(strategy.RuleVisibilityMode)
	strategy.PromptMarkdown = trimBytes(strategy.PromptMarkdown, MaxStoryDirectorStrategyPromptBytes)
	return strategy
}

func validateStoryDirectorWriteBounds(director StoryDirector) error {
	if len([]byte(strings.TrimSpace(director.Name))) > 256 {
		return errors.New("name 超过 256 字节 / exceeds 256 bytes")
	}
	if len([]byte(strings.TrimSpace(director.Description))) > 1024 {
		return errors.New("description 超过 1024 字节 / exceeds 1024 bytes")
	}
	if len([]byte(strings.TrimSpace(director.Strategy.PromptMarkdown))) > MaxStoryDirectorStrategyPromptBytes {
		return fmt.Errorf("strategy.prompt_markdown 超过 %d 字节 / exceeds %d bytes", MaxStoryDirectorStrategyPromptBytes, MaxStoryDirectorStrategyPromptBytes)
	}
	return nil
}

func normalizeRuleVisibilityMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", RuleVisibilityModeAuditOnly:
		return RuleVisibilityModeAuditOnly
	case RuleVisibilityModePublicRoll:
		return RuleVisibilityModePublicRoll
	default:
		return RuleVisibilityModeAuditOnly
	}
}

func StoryDirectorStrategyPromptMarkdown(director StoryDirector) string {
	director = normalizeStoryDirector(director)
	return director.Strategy.PromptMarkdown
}

// StoryPlanningGuideMarkdown projects only creator-authored planning style and
// optional event material. Backend pacing fields and legacy Director runtime
// policy are intentionally excluded: the Game Agent interprets this guide in
// light of the current story and user instructions.
func StoryPlanningGuideMarkdown(director StoryDirector, limitBytes int) string {
	director = normalizeStoryDirector(director)
	var sb strings.Builder
	if prompt := strings.TrimSpace(director.Strategy.PromptMarkdown); prompt != "" {
		sb.WriteString("### Planning style\n\n")
		sb.WriteString(prompt)
	}
	for _, pkg := range director.EventPackages {
		if !pkg.Enabled {
			continue
		}
		for _, card := range pkg.Events {
			if !card.Enabled || strings.TrimSpace(card.DescriptionMarkdown) == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			fmt.Fprintf(&sb, "### Optional event material: %s / %s\n\n%s", strings.TrimSpace(pkg.Name), strings.TrimSpace(card.TypeName), strings.TrimSpace(card.DescriptionMarkdown))
		}
	}
	return trimBytes(strings.TrimSpace(sb.String()), limitBytes)
}

func StoryDirectorRuleSummary(director StoryDirector, limitBytes int) string {
	director = normalizeStoryDirector(director)
	payload := map[string]any{
		"source": map[string]string{
			"kind":           "game_preset_rule_summary",
			"game_preset_id": director.ID,
			"name":           director.Name,
		},
		"limits": map[string]int{"max_bytes": limitBytes},
		"rule_policy": map[string]string{
			"state_consumption_mode": director.Strategy.RuleStateConsumptionMode,
			"visibility_mode":        director.Strategy.RuleVisibilityMode,
		},
		"trpg_system": director.TRPGSystem,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return trimBytes(string(data), limitBytes)
}

func storyDirectorActorStateSchemaSummary(system StoryDirectorActorStateSystem) StoryDirectorActorStateSystem {
	system = normalizeActorStateSystem(system)
	for poolIndex := range system.TraitPools {
		system.TraitPools[poolIndex].Traits = nil
	}
	return system
}

func ActorStateSchemaContext(system StoryDirectorActorStateSystem, limitBytes int) string {
	data, err := json.MarshalIndent(storyDirectorActorStateSchemaSummary(system), "", "  ")
	if err != nil {
		return ""
	}
	return trimBytes(string(data), limitBytes)
}

func NormalizeStoryDirectorID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.ReplaceAll(id, "_", "-")
	id = strings.ReplaceAll(id, " ", "-")
	var sb strings.Builder
	lastDash := false
	for _, r := range id {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if allowed {
			sb.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' && !lastDash {
			sb.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(sb.String(), "-")
}

func validateStoryDirectorID(id string) error {
	if id == "" {
		return fmt.Errorf("游戏预设 ID 不能为空 / Game Preset ID is required")
	}
	if id != NormalizeStoryDirectorID(id) {
		return fmt.Errorf("游戏预设 ID 只能包含小写字母、数字和连字符 / Game Preset ID may contain only lowercase letters, digits, and hyphens: %s", id)
	}
	return nil
}

func IsBuiltinStoryDirectorID(id string) bool {
	return NormalizeStoryDirectorID(id) == DefaultStoryDirectorID
}

func isBuiltinStoryDirectorFile(path string) bool {
	return IsBuiltinStoryDirectorID(strings.TrimSuffix(filepath.Base(path), ".json"))
}

func newStoryDirectorID(name string) string {
	base := NormalizeStoryDirectorID(name)
	if base == "" {
		base = "story-director"
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}
