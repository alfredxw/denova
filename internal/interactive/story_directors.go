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
)

const (
	storyDirectorVersion   = 2
	DefaultStoryDirectorID = "default"

	maxStoryDirectorAttributes          = 64
	maxStoryDirectorRules               = 64
	MaxStoryDirectorStrategyPromptBytes = maxTurnBriefTextBytes
	DefaultDirectorAgentMode            = DirectorAgentModeTriggered
	DirectorAgentModeTriggered          = "triggered"
	DirectorAgentModeEveryTurn          = "every_turn"
	DirectorAgentModeOff                = "off"
)

var ErrStoryDirectorRevisionConflict = errors.New("故事导演已被其他操作更新，请重新加载后再保存")

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
	EventSystem       StoryDirectorEventSystem      `json:"event_system"`
	StatSystem        StoryDirectorStatSystem       `json:"stat_system"`
	TRPGSystem        StoryDirectorTRPGSystem       `json:"trpg_system"`
	OpeningSelector   StoryDirectorOpeningSelector  `json:"opening_selector"`
	ResolvedSnapshot  StoryDirectorResolvedSnapshot `json:"resolved_snapshot,omitempty"`
	Tags              []string                      `json:"tags"`
	Path              string                        `json:"path,omitempty"`
	Custom            bool                          `json:"custom"`
	BuiltinOverridden bool                          `json:"builtin_overridden,omitempty"`
	Invalid           bool                          `json:"invalid,omitempty"`
	Error             string                        `json:"error,omitempty"`
	CreatedAt         string                        `json:"created_at,omitempty"`
	UpdatedAt         string                        `json:"updated_at,omitempty"`
}

type StoryDirectorStrategy struct {
	Enabled             bool                           `json:"enabled"`
	MainlineStrength    string                         `json:"mainline_strength,omitempty"`
	FailurePolicy       string                         `json:"failure_policy,omitempty"`
	PacingCurve         string                         `json:"pacing_curve,omitempty"`
	RandomEventRate     float64                        `json:"random_event_rate,omitempty"`
	DirectorAgentMode   string                         `json:"director_agent_mode,omitempty"`
	PromptMarkdown      string                         `json:"prompt_markdown,omitempty"`
	BranchPlanningTurns int                            `json:"branch_planning_turns,omitempty"`
	PlanningTemplates   StoryDirectorPlanningTemplates `json:"planning_templates,omitempty"`
}

type StoryDirectorEventSystem struct {
	EventPackages []TellerEventPackage `json:"event_packages,omitempty"`
	CustomEvents  []DirectorEvent      `json:"custom_events,omitempty"`
}

type StoryDirectorStatSystem struct {
	Attributes []StoryDirectorAttribute `json:"attributes,omitempty"`
}

type StoryDirectorAttribute struct {
	ID          string  `json:"id,omitempty"`
	Path        string  `json:"path"`
	Name        string  `json:"name"`
	Type        string  `json:"type,omitempty"`
	Default     float64 `json:"default,omitempty"`
	Min         float64 `json:"min,omitempty"`
	Max         float64 `json:"max,omitempty"`
	Visibility  string  `json:"visibility,omitempty"`
	Description string  `json:"description,omitempty"`
}

type StoryDirectorTRPGSystem struct {
	RuleTemplates []RuleCheck `json:"rule_templates,omitempty"`
}

type StoryDirectorOpeningSelector struct {
	Enabled         bool               `json:"enabled"`
	TraitPools      []OpeningTraitPool `json:"trait_pools,omitempty"`
	InitialStateOps []StateOp          `json:"initial_state_ops,omitempty"`
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
		persistResolvedStoryDirectorSnapshot(file, director)
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
	persistResolvedStoryDirectorSnapshot(filepath.Join(l.dir(), id+".json"), director)
	return director, nil
}

func (l *StoryDirectorLibrary) Create(director StoryDirector) (StoryDirector, error) {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
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
	if _, err := os.Stat(path); err == nil {
		return StoryDirector{}, fmt.Errorf("故事导演已存在: %s", director.ID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	director.CreatedAt = firstNonEmptyString(director.CreatedAt, now)
	director.UpdatedAt = now
	director = ResolveStoryDirectorModules(l.novaDir, director)
	if err := writeStoryDirectorFile(path, director); err != nil {
		return StoryDirector{}, err
	}
	director.Path = path
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
	current, err := parseStoryDirectorFile(path)
	if err != nil {
		return StoryDirector{}, err
	}
	isBuiltin := IsBuiltinStoryDirectorID(id)
	if strings.TrimSpace(baseRevision) != "" && strings.TrimSpace(current.UpdatedAt) != strings.TrimSpace(baseRevision) {
		return StoryDirector{}, ErrStoryDirectorRevisionConflict
	}
	director = normalizeStoryDirector(director)
	director.ID = id
	director.CreatedAt = firstNonEmptyString(current.CreatedAt, director.CreatedAt)
	director.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	director.BuiltinOverridden = isBuiltin
	director = ResolveStoryDirectorModules(l.novaDir, director)
	if err := writeStoryDirectorFile(path, director); err != nil {
		return StoryDirector{}, err
	}
	director.Path = path
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
	if err := NewEventSystemLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := NewRuleSystemLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := NewOpeningSelectorLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(l.dir(), DefaultStoryDirectorID+".json")
	version, versionErr := readStoryDirectorFileVersion(path)
	current, parseErr := parseStoryDirectorFile(path)
	if parseErr == nil && current.BuiltinOverridden {
		return l.migrateStoryDirectorResources()
	}
	if versionErr == nil && parseErr == nil && current.Version == storyDirectorVersion && version == storyDirectorVersion {
		return l.migrateStoryDirectorResources()
	}
	if err := writeStoryDirectorFile(path, DefaultStoryDirector()); err != nil {
		return err
	}
	return l.migrateStoryDirectorResources()
}

func (l *StoryDirectorLibrary) migrateStoryDirectorResources() error {
	if err := l.migrateLegacyTellerOrchestrations(); err != nil {
		return err
	}
	return l.migrateEmbeddedStoryDirectorModules()
}

func (l *StoryDirectorLibrary) migrateLegacyTellerOrchestrations() error {
	files, err := filepath.Glob(filepath.Join(l.novaDir, "story-tellers", "*.json"))
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, file := range files {
		if isBuiltinTellerFile(file) {
			continue
		}
		teller, err := parseTellerFile(file)
		if err != nil || teller.Orchestration == nil {
			continue
		}
		directorID := NormalizeStoryDirectorID(teller.ID)
		if directorID == "" {
			directorID = NormalizeStoryDirectorID(teller.Name)
		}
		if directorID == "" {
			continue
		}
		if IsBuiltinStoryDirectorID(directorID) {
			directorID = "teller-" + directorID
		}
		path := filepath.Join(l.dir(), directorID+".json")
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		directorName := strings.TrimSpace(teller.Name)
		if directorName != "" {
			directorName += " 故事导演"
		} else {
			directorName = directorID
		}
		director := StoryDirectorFromTellerOrchestration(
			directorID,
			directorName,
			"由旧叙事风格中的 orchestration 配置迁移生成。",
			teller.RandomEventRate,
			*teller.Orchestration,
		)
		director.CreatedAt = firstNonEmptyString(teller.CreatedAt, now)
		director.UpdatedAt = now
		director.Tags = normalizeStringListLimit(append(director.Tags, "迁移"), maxTurnBriefListItems)
		if err := writeStoryDirectorFile(path, director); err != nil {
			return err
		}
	}
	return nil
}

func (l *StoryDirectorLibrary) migrateEmbeddedStoryDirectorModules() error {
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return err
	}
	eventLibrary := NewEventSystemLibrary(l.novaDir)
	ruleLibrary := NewRuleSystemLibrary(l.novaDir)
	openingLibrary := NewOpeningSelectorLibrary(l.novaDir)
	for _, file := range files {
		if isBuiltinStoryDirectorFile(file) {
			continue
		}
		raw, err := parseRawStoryDirectorFile(file)
		if err != nil {
			continue
		}
		if !StoryDirectorModuleRefsEmpty(raw.ModuleRefs) || !storyDirectorHasEmbeddedModules(raw) {
			continue
		}
		director := normalizeStoryDirector(raw)
		refs := NormalizeStoryDirectorModuleRefs(raw.ModuleRefs)
		if refs.NarrativeStyleID == "" {
			refs.NarrativeStyleID = "classic"
		}
		if refs.ImagePresetID == "" {
			refs.ImagePresetID = "game-cg"
		}
		if !eventSystemEmpty(raw.EventSystem) {
			id, err := ensureMigratedEventSystem(eventLibrary, director)
			if err != nil {
				return err
			}
			refs.EventSystemID = id
		}
		if !ruleSystemEmpty(raw.StatSystem, raw.TRPGSystem) {
			id, err := ensureMigratedRuleSystem(ruleLibrary, director)
			if err != nil {
				return err
			}
			refs.RuleSystemID = id
		}
		if !openingSelectorEmpty(raw.OpeningSelector) {
			id, err := ensureMigratedOpeningSelector(openingLibrary, director)
			if err != nil {
				return err
			}
			refs.OpeningSelectorID = id
		}
		director.ModuleRefs = refs
		director = ResolveStoryDirectorModules(l.novaDir, director)
		if err := writeStoryDirectorFile(file, director); err != nil {
			return err
		}
	}
	return nil
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

func parseRawStoryDirectorFile(path string) (StoryDirector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StoryDirector{}, err
	}
	var director StoryDirector
	if err := json.Unmarshal(data, &director); err != nil {
		return StoryDirector{}, fmt.Errorf("解析故事导演 JSON 失败: %w", err)
	}
	director.Path = path
	return applyStoryDirectorOwnership(director), nil
}

func parseStoryDirectorFile(path string) (StoryDirector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StoryDirector{}, err
	}
	var director StoryDirector
	if err := json.Unmarshal(data, &director); err != nil {
		return StoryDirector{}, fmt.Errorf("解析故事导演 JSON 失败: %w", err)
	}
	director = normalizeStoryDirector(director)
	director.Path = path
	return applyStoryDirectorOwnership(director), nil
}

func writeStoryDirectorFile(path string, director StoryDirector) error {
	director = normalizeStoryDirector(director)
	data, err := json.MarshalIndent(director, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ensureMigratedEventSystem(library *EventSystemLibrary, director StoryDirector) (string, error) {
	id := normalizeDirectorModuleID(director.ID + "-events")
	if _, err := library.Get(id); err == nil {
		return id, nil
	}
	module, err := library.Create(EventSystemModule{
		ID:          id,
		Name:        director.Name + " 事件系统",
		Description: "由旧故事导演内嵌 event_system 迁移生成。",
		EventSystem: director.EventSystem,
		Tags:        migratedDirectorModuleTags(director.Tags),
	})
	if err != nil {
		return "", err
	}
	return module.ID, nil
}

func ensureMigratedRuleSystem(library *RuleSystemLibrary, director StoryDirector) (string, error) {
	id := normalizeDirectorModuleID(director.ID + "-rules")
	if _, err := library.Get(id); err == nil {
		return id, nil
	}
	module, err := library.Create(RuleSystemModule{
		ID:          id,
		Name:        director.Name + " 数值与TRPG系统",
		Description: "由旧故事导演内嵌 stat_system/trpg_system 迁移生成。",
		StatSystem:  director.StatSystem,
		TRPGSystem:  director.TRPGSystem,
		Tags:        migratedDirectorModuleTags(director.Tags),
	})
	if err != nil {
		return "", err
	}
	return module.ID, nil
}

func ensureMigratedOpeningSelector(library *OpeningSelectorLibrary, director StoryDirector) (string, error) {
	id := normalizeDirectorModuleID(director.ID + "-opening")
	if _, err := library.Get(id); err == nil {
		return id, nil
	}
	module, err := library.Create(OpeningSelectorModule{
		ID:              id,
		Name:            director.Name + " 开局选择器",
		Description:     "由旧故事导演内嵌 opening_selector 迁移生成。",
		OpeningSelector: director.OpeningSelector,
		Tags:            migratedDirectorModuleTags(director.Tags),
	})
	if err != nil {
		return "", err
	}
	return module.ID, nil
}

func migratedDirectorModuleTags(tags []string) []string {
	return normalizeStringListLimit(append(append([]string{}, tags...), "迁移"), maxTurnBriefListItems)
}

func persistResolvedStoryDirectorSnapshot(path string, director StoryDirector) {
	if path == "" || IsBuiltinStoryDirectorID(director.ID) || director.Invalid {
		return
	}
	_ = writeStoryDirectorFile(path, director)
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
	director.Path = ""
	director.Custom = false
	director.BuiltinOverridden = false
	director.Invalid = false
	director.Error = ""
	director.CreatedAt = ""
	director.UpdatedAt = ""
	director.ResolvedSnapshot = StoryDirectorResolvedSnapshot{}
	return director
}

func DefaultStoryDirector() StoryDirector {
	refs := DefaultStoryDirectorModuleRefs()
	return normalizeStoryDirector(StoryDirector{
		Version:     storyDirectorVersion,
		ID:          DefaultStoryDirectorID,
		Name:        "默认故事导演",
		Description: "通用互动故事导演，提供软主线、可逆失败、递进节奏、事件系统、基础数值和开局选择器。",
		ModuleRefs:  refs,
		Strategy: StoryDirectorStrategy{
			Enabled:             true,
			MainlineStrength:    "soft_guidance",
			FailurePolicy:       "reversible",
			PacingCurve:         "progressive",
			RandomEventRate:     0.15,
			DirectorAgentMode:   DefaultDirectorAgentMode,
			BranchPlanningTurns: defaultBranchPlanningTurns,
			PlanningTemplates:   DefaultStoryDirectorPlanningTemplates(),
		},
		EventSystem:     DefaultEventSystemModule().EventSystem,
		StatSystem:      DefaultRuleSystemModule().StatSystem,
		TRPGSystem:      DefaultRuleSystemModule().TRPGSystem,
		OpeningSelector: DefaultOpeningSelectorModule().OpeningSelector,
		Tags:            []string{"内置", "导演"},
	})
}

func StoryDirectorFromTellerOrchestration(id, name, description string, randomEventRate float64, config TellerOrchestrationConfig) StoryDirector {
	return normalizeStoryDirector(StoryDirector{
		Version:     storyDirectorVersion,
		ID:          NormalizeStoryDirectorID(id),
		Name:        name,
		Description: description,
		ModuleRefs:  StoryDirectorModuleRefs{},
		Strategy: StoryDirectorStrategy{
			Enabled:             config.Enabled,
			MainlineStrength:    config.MainlineStrength,
			FailurePolicy:       config.FailurePolicy,
			PacingCurve:         config.PacingCurve,
			RandomEventRate:     randomEventRate,
			DirectorAgentMode:   DefaultDirectorAgentMode,
			BranchPlanningTurns: defaultBranchPlanningTurns,
			PlanningTemplates:   DefaultStoryDirectorPlanningTemplates(),
		},
		EventSystem: StoryDirectorEventSystem{
			EventPackages: config.EventPackages,
			CustomEvents:  config.CustomEvents,
		},
		StatSystem: StoryDirectorStatSystem{
			Attributes: defaultStoryDirectorAttributes(),
		},
		TRPGSystem: StoryDirectorTRPGSystem{
			RuleTemplates: config.RuleTemplates,
		},
		OpeningSelector: StoryDirectorOpeningSelector{
			Enabled:         config.Opening.Enabled,
			TraitPools:      config.Opening.TraitPools,
			InitialStateOps: config.Opening.InitialStateOps,
		},
		Tags: []string{"内置", "导演"},
	})
}

func normalizeStoryDirector(director StoryDirector) StoryDirector {
	director.Version = storyDirectorVersion
	director.ID = NormalizeStoryDirectorID(director.ID)
	director.Name = trimBytes(firstNonEmptyString(director.Name, director.ID, "故事导演"), 256)
	director.Description = trimBytes(director.Description, 1024)
	director.ModuleRefs = NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
	if StoryDirectorModuleRefsEmpty(director.ModuleRefs) && !storyDirectorHasEmbeddedModules(director) {
		director.ModuleRefs = DefaultStoryDirectorModuleRefs()
	}
	director.Strategy = normalizeStoryDirectorStrategy(director.Strategy)
	if director.ModuleRefs.EventSystemDisabled {
		director.EventSystem.EventPackages = normalizeTellerEventPackagesNoDefault(director.EventSystem.EventPackages)
	} else {
		director.EventSystem.EventPackages = normalizeTellerEventPackages(director.EventSystem.EventPackages)
	}
	director.EventSystem.CustomEvents = normalizeDirectorEvents(director.EventSystem.CustomEvents)
	if director.ModuleRefs.RuleSystemDisabled {
		director.StatSystem.Attributes = normalizeStoryDirectorAttributesNoDefault(director.StatSystem.Attributes)
	} else {
		director.StatSystem.Attributes = normalizeStoryDirectorAttributes(director.StatSystem.Attributes)
	}
	director.TRPGSystem.RuleTemplates = normalizeRuleChecks(director.TRPGSystem.RuleTemplates)
	director.OpeningSelector = normalizeStoryDirectorOpeningSelector(director.OpeningSelector)
	director.ResolvedSnapshot = normalizeStoryDirectorResolvedSnapshot(director.ResolvedSnapshot)
	director.Tags = normalizeStringListLimit(director.Tags, maxTurnBriefListItems)
	return director
}

func NormalizeStoryDirectorStrategy(strategy StoryDirectorStrategy) StoryDirectorStrategy {
	return normalizeStoryDirectorStrategy(strategy)
}

func normalizeStoryDirectorStrategy(strategy StoryDirectorStrategy) StoryDirectorStrategy {
	strategy.MainlineStrength = normalizeOrchestrationOption(strategy.MainlineStrength, "soft_guidance")
	strategy.FailurePolicy = normalizeOrchestrationOption(strategy.FailurePolicy, "reversible")
	strategy.PacingCurve = normalizeOrchestrationOption(strategy.PacingCurve, "progressive")
	strategy.DirectorAgentMode = normalizeDirectorAgentMode(strategy.DirectorAgentMode)
	strategy.PromptMarkdown = trimBytes(strategy.PromptMarkdown, MaxStoryDirectorStrategyPromptBytes)
	strategy.BranchPlanningTurns = NormalizeBranchPlanningTurns(strategy.BranchPlanningTurns)
	strategy.PlanningTemplates = NormalizeStoryDirectorPlanningTemplates(strategy.PlanningTemplates)
	if strategy.RandomEventRate < 0 {
		strategy.RandomEventRate = 0
	}
	if strategy.RandomEventRate > 1 {
		strategy.RandomEventRate = 1
	}
	return strategy
}

func normalizeDirectorAgentMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case DirectorAgentModeEveryTurn:
		return DirectorAgentModeEveryTurn
	case DirectorAgentModeOff:
		return DirectorAgentModeOff
	case DirectorAgentModeTriggered, "":
		return DirectorAgentModeTriggered
	default:
		return DirectorAgentModeTriggered
	}
}

func normalizeStoryDirectorAttributes(attributes []StoryDirectorAttribute) []StoryDirectorAttribute {
	if attributes == nil {
		return defaultStoryDirectorAttributes()
	}
	return normalizeStoryDirectorAttributesNoDefault(attributes)
}

func normalizeStoryDirectorAttributesNoDefault(attributes []StoryDirectorAttribute) []StoryDirectorAttribute {
	if attributes == nil {
		return []StoryDirectorAttribute{}
	}
	if len(attributes) > maxStoryDirectorAttributes {
		attributes = attributes[:maxStoryDirectorAttributes]
	}
	out := make([]StoryDirectorAttribute, 0, len(attributes))
	seen := map[string]bool{}
	for i, attribute := range attributes {
		attribute.ID = trimBytes(attribute.ID, 128)
		attribute.Path = strings.TrimSpace(attribute.Path)
		if attribute.Path == "" || !validStatePathSyntax(attribute.Path) {
			continue
		}
		if attribute.ID == "" {
			attribute.ID = fmt.Sprintf("attr_%d", i+1)
		}
		if seen[attribute.ID] {
			continue
		}
		seen[attribute.ID] = true
		attribute.Name = trimBytes(firstNonEmptyString(attribute.Name, attribute.ID), 128)
		attribute.Type = trimBytes(firstNonEmptyString(attribute.Type, "resource"), 64)
		attribute.Visibility = normalizeStoryDirectorVisibility(attribute.Visibility)
		attribute.Description = trimBytes(attribute.Description, maxTurnBriefTextBytes)
		out = append(out, attribute)
	}
	return out
}

func normalizeStoryDirectorVisibility(value string) string {
	switch strings.TrimSpace(value) {
	case "visible", "hidden", "spoiler":
		return strings.TrimSpace(value)
	default:
		return "visible"
	}
}

func normalizeStoryDirectorOpeningSelector(config StoryDirectorOpeningSelector) StoryDirectorOpeningSelector {
	config.TraitPools = normalizeOpeningTraitPools(config.TraitPools)
	config.InitialStateOps = normalizeStateOpsForRule(config.InitialStateOps)
	return config
}

func defaultStoryDirectorAttributes() []StoryDirectorAttribute {
	return []StoryDirectorAttribute{
		{ID: "hp", Path: "resources.hp", Name: "生命", Type: "resource", Default: 10, Min: 0, Max: 10, Visibility: "visible", Description: "主角当前生命或伤势承受能力。"},
		{ID: "stamina", Path: "resources.stamina", Name: "体力", Type: "resource", Default: 5, Min: 0, Max: 5, Visibility: "visible", Description: "奔跑、战斗、潜入等高消耗行动的资源。"},
		{ID: "affection", Path: "relations.affection", Name: "好感", Type: "relation", Default: 0, Min: -100, Max: 100, Visibility: "spoiler", Description: "重要角色或势力对主角的亲近度，可按对象拆分。"},
	}
}

func StoryDirectorInitialStateOps(director StoryDirector) []StateOp {
	director = normalizeStoryDirector(director)
	ops := make([]StateOp, 0, len(director.StatSystem.Attributes)+len(director.OpeningSelector.InitialStateOps))
	for _, attribute := range director.StatSystem.Attributes {
		ops = append(ops, StateOp{Op: "set", Path: attribute.Path, Value: attribute.Default})
		if attribute.Max > 0 {
			ops = append(ops, StateOp{Op: "set", Path: attribute.Path + "_max", Value: attribute.Max})
		}
	}
	ops = append(ops, director.OpeningSelector.InitialStateOps...)
	return normalizeStateOps(ops)
}

func StoryDirectorStrategyPromptMarkdown(director StoryDirector) string {
	director = normalizeStoryDirector(director)
	return director.Strategy.PromptMarkdown
}

func DirectorEventCatalogFromStoryDirector(director StoryDirector) []DirectorEvent {
	director = normalizeStoryDirector(director)
	if !StoryDirectorEventSystemEnabled(director) {
		return []DirectorEvent{}
	}
	events := []DirectorEvent{}
	for _, pkg := range director.EventSystem.EventPackages {
		for _, eventCard := range pkg.Events {
			if !eventCard.Enabled {
				continue
			}
			events = upsertDirectorEvent(events, directorEventFromTellerEventCard(eventCard))
		}
	}
	for _, event := range director.EventSystem.CustomEvents {
		events = upsertDirectorEvent(events, event)
	}
	return appendDefaultDirectorEventTemplates(events)
}

func StoryDirectorRuleSummary(director StoryDirector, limitBytes int) string {
	director = normalizeStoryDirector(director)
	type attribute struct {
		ID          string  `json:"id,omitempty"`
		Path        string  `json:"path"`
		Name        string  `json:"name"`
		Type        string  `json:"type,omitempty"`
		Default     float64 `json:"default,omitempty"`
		Min         float64 `json:"min,omitempty"`
		Max         float64 `json:"max,omitempty"`
		Visibility  string  `json:"visibility,omitempty"`
		Description string  `json:"description,omitempty"`
	}
	attrs := make([]attribute, 0, len(director.StatSystem.Attributes))
	for _, item := range director.StatSystem.Attributes {
		if item.Visibility == "hidden" {
			continue
		}
		attrs = append(attrs, attribute{
			ID: item.ID, Path: item.Path, Name: item.Name, Type: item.Type, Default: item.Default, Min: item.Min, Max: item.Max, Visibility: item.Visibility, Description: item.Description,
		})
	}
	payload := map[string]any{
		"source": map[string]string{
			"kind":              "story_director_rule_summary",
			"story_director_id": director.ID,
			"name":              director.Name,
		},
		"limits":      map[string]int{"max_bytes": limitBytes},
		"strategy":    storyDirectorStructuredStrategySummary(director.Strategy),
		"stat_system": map[string]any{"attributes": attrs},
		"trpg_system": director.TRPGSystem,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return trimBytes(string(data), limitBytes)
}

func StoryDirectorPlanningSummary(director StoryDirector, limitBytes int) string {
	director = normalizeStoryDirector(director)
	payload := map[string]any{
		"source": map[string]string{
			"kind":              "story_director_planning_summary",
			"story_director_id": director.ID,
			"name":              director.Name,
		},
		"limits":       map[string]int{"max_bytes": limitBytes},
		"strategy":     storyDirectorStructuredStrategySummary(director.Strategy),
		"event_system": director.EventSystem,
		"stat_system":  director.StatSystem,
		"trpg_system":  director.TRPGSystem,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return trimBytes(string(data), limitBytes)
}

type storyDirectorStructuredStrategy struct {
	Enabled             bool    `json:"enabled"`
	MainlineStrength    string  `json:"mainline_strength,omitempty"`
	FailurePolicy       string  `json:"failure_policy,omitempty"`
	PacingCurve         string  `json:"pacing_curve,omitempty"`
	RandomEventRate     float64 `json:"random_event_rate,omitempty"`
	DirectorAgentMode   string  `json:"director_agent_mode,omitempty"`
	BranchPlanningTurns int     `json:"branch_planning_turns,omitempty"`
}

func storyDirectorStructuredStrategySummary(strategy StoryDirectorStrategy) storyDirectorStructuredStrategy {
	return storyDirectorStructuredStrategy{
		Enabled:             strategy.Enabled,
		MainlineStrength:    strategy.MainlineStrength,
		FailurePolicy:       strategy.FailurePolicy,
		PacingCurve:         strategy.PacingCurve,
		RandomEventRate:     strategy.RandomEventRate,
		DirectorAgentMode:   strategy.DirectorAgentMode,
		BranchPlanningTurns: strategy.BranchPlanningTurns,
	}
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
		return fmt.Errorf("故事导演 ID 不能为空")
	}
	if id != NormalizeStoryDirectorID(id) {
		return fmt.Errorf("故事导演 ID 只能包含小写字母、数字和连字符: %s", id)
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
