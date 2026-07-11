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
	storyDirectorVersion   = 3
	DefaultStoryDirectorID = "default"

	maxStoryDirectorRules               = 64
	MaxStoryDirectorStrategyPromptBytes = DirectorContextMaxBytes
	DefaultDirectorAgentMode            = DirectorAgentModeTriggered
	DirectorAgentModeTriggered          = "triggered"
	DirectorAgentModeEveryTurn          = "every_turn"
	DirectorAgentModeOff                = "off"
	DefaultRuleVisibilityMode           = RuleVisibilityModeAuditOnly
	RuleVisibilityModeAuditOnly         = "audit_only"
	RuleVisibilityModePublicRoll        = "public_roll"
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
	EventPackages     []TellerEventPackage          `json:"event_packages,omitempty"`
	EventSystem       StoryDirectorEventSystem      `json:"-"`
	TRPGSystem        StoryDirectorTRPGSystem       `json:"trpg_system"`
	ActorState        StoryDirectorActorStateSystem `json:"actor_state,omitempty"`
	OpeningSelector   StoryDirectorOpeningSelector  `json:"opening_selector,omitempty"`
	MigrationWarnings []string                      `json:"migration_warnings,omitempty"`
	ResolvedSnapshot  StoryDirectorResolvedSnapshot `json:"resolved_snapshot,omitempty"`
	Path              string                        `json:"path,omitempty"`
	Custom            bool                          `json:"custom"`
	BuiltinOverridden bool                          `json:"builtin_overridden,omitempty"`
	Invalid           bool                          `json:"invalid,omitempty"`
	Error             string                        `json:"error,omitempty"`
	CreatedAt         string                        `json:"created_at,omitempty"`
	UpdatedAt         string                        `json:"updated_at,omitempty"`
}

type StoryDirectorStrategy struct {
	Enabled                  bool                           `json:"enabled"`
	MainlineStrength         string                         `json:"mainline_strength,omitempty"`
	FailurePolicy            string                         `json:"failure_policy,omitempty"`
	PacingCurve              string                         `json:"pacing_curve,omitempty"`
	RandomEventRate          float64                        `json:"random_event_rate,omitempty"`
	DirectorAgentMode        string                         `json:"director_agent_mode,omitempty"`
	RuleStateConsumptionMode string                         `json:"rule_state_consumption_mode,omitempty"`
	RuleVisibilityMode       string                         `json:"rule_visibility_mode,omitempty"`
	PromptMarkdown           string                         `json:"prompt_markdown,omitempty"`
	BranchPlanningTurns      int                            `json:"branch_planning_turns,omitempty"`
	PlanningTemplates        StoryDirectorPlanningTemplates `json:"planning_templates,omitempty"`
}

type StoryDirectorEventSystem struct {
	EventPackages []TellerEventPackage `json:"event_packages,omitempty"`
	CustomEvents  []DirectorEvent      `json:"custom_events,omitempty"`
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
	if err := NewEventPackageLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := NewRuleSystemLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := NewActorStateLibrary(l.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	if err := NewStoryMemoryStructureLibrary(l.novaDir).ensureBuiltins(); err != nil {
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
	eventLibrary := NewEventPackageLibrary(l.novaDir)
	ruleLibrary := NewRuleSystemLibrary(l.novaDir)
	actorStateLibrary := NewActorStateLibrary(l.novaDir)
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
		if len(raw.EventPackages) > 0 || !eventSystemEmpty(raw.EventSystem) {
			ids, err := ensureMigratedEventPackages(eventLibrary, director)
			if err != nil {
				return err
			}
			refs.EventPackageIDs = ids
		}
		if !ruleSystemEmpty(raw.TRPGSystem) {
			id, err := ensureMigratedRuleSystem(ruleLibrary, director)
			if err != nil {
				return err
			}
			refs.RuleSystemID = id
		}
		if !actorStateEmpty(raw.ActorState) || !openingSelectorEmpty(raw.OpeningSelector) {
			id, err := ensureMigratedActorState(actorStateLibrary, director)
			if err != nil {
				return err
			}
			refs.ActorStateID = id
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
	director, err := decodeStoryDirectorJSON(data)
	if err != nil {
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
	director, err := decodeStoryDirectorJSON(data)
	if err != nil {
		return StoryDirector{}, fmt.Errorf("解析故事导演 JSON 失败: %w", err)
	}
	director = normalizeStoryDirector(director)
	director.Path = path
	return applyStoryDirectorOwnership(director), nil
}

func decodeStoryDirectorJSON(data []byte) (StoryDirector, error) {
	var director StoryDirector
	if err := json.Unmarshal(data, &director); err != nil {
		return StoryDirector{}, err
	}
	var legacy struct {
		EventSystem      StoryDirectorEventSystem `json:"event_system"`
		ResolvedSnapshot struct {
			EventSystem StoryDirectorEventSystem `json:"event_system"`
		} `json:"resolved_snapshot"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return StoryDirector{}, err
	}
	if len(director.EventPackages) == 0 && !eventSystemEmpty(legacy.EventSystem) {
		director.EventSystem = legacy.EventSystem
	}
	if len(director.ResolvedSnapshot.EventPackages) == 0 && !eventSystemEmpty(legacy.ResolvedSnapshot.EventSystem) {
		director.ResolvedSnapshot.EventSystem = legacy.ResolvedSnapshot.EventSystem
	}
	return director, nil
}

func writeStoryDirectorFile(path string, director StoryDirector) error {
	director = normalizeStoryDirector(director)
	data, err := marshalJSONWithoutFields(director, "opening_selector")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ensureMigratedEventPackages(library *EventPackageLibrary, director StoryDirector) ([]string, error) {
	packages := director.EventPackages
	if len(packages) == 0 && !eventSystemEmpty(director.EventSystem) {
		packages = eventPackagesFromLegacyEventSystem(director.EventSystem, director.ID)
	}
	ids := make([]string, 0, len(packages))
	for i, pkg := range packages {
		id := normalizeDirectorModuleID(pkg.ID)
		if id == "" {
			id = normalizeDirectorModuleID(fmt.Sprintf("%s-events-%d", director.ID, i+1))
		}
		if _, err := library.Get(id); err == nil {
			ids = append(ids, id)
			continue
		}
		module, err := library.Create(EventPackageModule{
			ID:          id,
			Name:        firstNonEmptyString(pkg.Name, director.Name+" 事件包"),
			Description: "由旧故事导演内嵌事件配置迁移生成。",
			Events:      pkg.Events,
		})
		if err != nil {
			return nil, err
		}
		ids = append(ids, module.ID)
	}
	return normalizeEventPackageIDs(ids), nil
}

func ensureMigratedRuleSystem(library *RuleSystemLibrary, director StoryDirector) (string, error) {
	id := normalizeDirectorModuleID(director.ID + "-rules")
	if _, err := library.Get(id); err == nil {
		return id, nil
	}
	module, err := library.Create(RuleSystemModule{
		ID:          id,
		Name:        director.Name + " TRPG 检定",
		Description: "由旧故事导演内嵌 trpg_system 迁移生成。",
		TRPGSystem:  director.TRPGSystem,
	})
	if err != nil {
		return "", err
	}
	return module.ID, nil
}

func ensureMigratedActorState(library *ActorStateLibrary, director StoryDirector) (string, error) {
	id := normalizeDirectorModuleID(director.ID + "-actor-state")
	if existing, err := library.Get(id); err == nil {
		changed := false
		if !openingSelectorHasContent(existing.OpeningSelector) && openingSelectorHasContent(director.OpeningSelector) {
			existing.OpeningSelector = director.OpeningSelector
			existing.Description = firstNonEmptyString(existing.Description, "由旧故事导演内嵌 actor_state 和 opening_selector 迁移生成。")
			changed = true
		}
		if len(director.MigrationWarnings) > 0 {
			existing.MigrationWarnings = normalizeStringListLimit(append(existing.MigrationWarnings, director.MigrationWarnings...), maxTurnBriefListItems)
			changed = true
		}
		if changed {
			if _, updateErr := library.Update(id, existing, existing.UpdatedAt); updateErr != nil {
				return "", updateErr
			}
		}
		return id, nil
	}
	actorState := director.ActorState
	if actorStateEmpty(actorState) {
		actorState = defaultActorStateSystem()
	}
	module, err := library.Create(ActorStateModule{
		ID:                id,
		Name:              director.Name + " 状态系统",
		Description:       "由旧故事导演内嵌 actor_state 和 opening_selector 迁移生成。",
		ActorState:        actorState,
		OpeningSelector:   director.OpeningSelector,
		MigrationWarnings: director.MigrationWarnings,
	})
	if err != nil {
		return "", err
	}
	return module.ID, nil
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
	director.ResolvedSnapshot = StoryDirectorResolvedSnapshot{}
	return director
}

func DefaultStoryDirector() StoryDirector {
	refs := DefaultStoryDirectorModuleRefs()
	defaultActorState := DefaultActorStateModule()
	return normalizeStoryDirector(StoryDirector{
		Version:     storyDirectorVersion,
		ID:          DefaultStoryDirectorID,
		Name:        "默认故事导演",
		Description: "通用互动故事导演，提供软主线、可逆失败、递进节奏、事件包、状态系统和图像方案。",
		ModuleRefs:  refs,
		Strategy: StoryDirectorStrategy{
			Enabled:                  true,
			MainlineStrength:         "soft_guidance",
			FailurePolicy:            "reversible",
			PacingCurve:              "progressive",
			RandomEventRate:          0.15,
			DirectorAgentMode:        DefaultDirectorAgentMode,
			RuleStateConsumptionMode: DefaultRuleStateConsumptionMode,
			RuleVisibilityMode:       DefaultRuleVisibilityMode,
			BranchPlanningTurns:      defaultBranchPlanningTurns,
			PlanningTemplates:        DefaultStoryDirectorPlanningTemplates(),
		},
		EventPackages: []TellerEventPackage{tellerEventPackageFromModule(DefaultEventPackageModule())},
		TRPGSystem:    DefaultRuleSystemModule().TRPGSystem,
		ActorState:    defaultActorState.ActorState,
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
			Enabled:                  config.Enabled,
			MainlineStrength:         config.MainlineStrength,
			FailurePolicy:            config.FailurePolicy,
			PacingCurve:              config.PacingCurve,
			RandomEventRate:          randomEventRate,
			DirectorAgentMode:        DefaultDirectorAgentMode,
			RuleStateConsumptionMode: DefaultRuleStateConsumptionMode,
			RuleVisibilityMode:       DefaultRuleVisibilityMode,
			BranchPlanningTurns:      defaultBranchPlanningTurns,
			PlanningTemplates:        DefaultStoryDirectorPlanningTemplates(),
		},
		EventPackages: eventPackagesFromLegacyEventSystem(StoryDirectorEventSystem{EventPackages: config.EventPackages, CustomEvents: config.CustomEvents}, id),
		TRPGSystem: StoryDirectorTRPGSystem{
			RuleTemplates: config.RuleTemplates,
		},
		ActorState: defaultActorStateSystem(),
		OpeningSelector: StoryDirectorOpeningSelector{
			Enabled:         config.Opening.Enabled,
			TraitPools:      config.Opening.TraitPools,
			InitialStateOps: config.Opening.InitialStateOps,
		},
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
	if len(director.EventPackages) == 0 && !eventSystemEmpty(director.EventSystem) {
		director.EventPackages = eventPackagesFromLegacyEventSystem(director.EventSystem, director.ID)
	}
	if director.ModuleRefs.EventPackagesDisabled {
		director.EventPackages = normalizeTellerEventPackagesNoDefault(director.EventPackages)
	} else {
		director.EventPackages = normalizeTellerEventPackagesNoDefault(director.EventPackages)
	}
	director.EventSystem = StoryDirectorEventSystem{}
	director.TRPGSystem.RuleTemplates = normalizeRuleChecks(director.TRPGSystem.RuleTemplates)
	if director.ModuleRefs.ActorStateDisabled {
		director.ActorState = normalizeActorStateSystem(StoryDirectorActorStateSystem{})
	} else {
		director.ActorState = normalizeActorStateSystem(director.ActorState)
	}
	director.OpeningSelector = normalizeStoryDirectorOpeningSelector(director.OpeningSelector)
	if !director.ModuleRefs.ActorStateDisabled && openingSelectorHasContent(director.OpeningSelector) {
		var warnings []string
		director.ActorState, warnings = migrateLegacyOpeningTraits(director.ActorState, director.OpeningSelector)
		director.MigrationWarnings = normalizeStringListLimit(append(director.MigrationWarnings, warnings...), maxTurnBriefListItems)
		director.OpeningSelector = StoryDirectorOpeningSelector{}
	}
	director.TRPGSystem = resolveRuleStateFieldIDs(director.ActorState, director.TRPGSystem)
	director.ResolvedSnapshot = normalizeStoryDirectorResolvedSnapshot(director.ResolvedSnapshot)
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
	strategy.RuleStateConsumptionMode = normalizeRuleStateConsumptionMode(strategy.RuleStateConsumptionMode)
	strategy.RuleVisibilityMode = normalizeRuleVisibilityMode(strategy.RuleVisibilityMode)
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

func StoryDirectorInitialStateOps(director StoryDirector) []StateOp {
	director = normalizeStoryDirector(director)
	return actorStateInitialOps(director.ActorState)
}

func BuildStoryDirectorInitialStateOps(director StoryDirector, rolls []InitialActorTraitRoll) ([]StateOp, error) {
	director = normalizeStoryDirector(director)
	return BuildActorStateInitialOps(director.ActorState, rolls)
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
	for _, pkg := range director.EventPackages {
		for _, eventCard := range pkg.Events {
			if !eventCard.Enabled {
				continue
			}
			events = upsertDirectorEvent(events, directorEventFromTellerEventCard(eventCard))
		}
	}
	return appendDefaultDirectorEventTemplates(events)
}

func StoryDirectorRuleSummary(director StoryDirector, limitBytes int) string {
	director = normalizeStoryDirector(director)
	payload := map[string]any{
		"source": map[string]string{
			"kind":              "story_director_rule_summary",
			"story_director_id": director.ID,
			"name":              director.Name,
		},
		"limits":       map[string]int{"max_bytes": limitBytes},
		"strategy":     storyDirectorStructuredStrategySummary(director.Strategy),
		"state_system": storyDirectorActorStateSchemaSummary(director.ActorState),
		"trpg_system":  director.TRPGSystem,
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
		"limits":         map[string]int{"max_bytes": limitBytes},
		"strategy":       storyDirectorStructuredStrategySummary(director.Strategy),
		"event_packages": director.EventPackages,
		"state_system":   storyDirectorActorStateSchemaSummary(director.ActorState),
		"trpg_system":    director.TRPGSystem,
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
	for templateIndex := range system.Templates {
		fields := system.Templates[templateIndex].Fields
		visibleFields := make([]ActorStateField, 0, len(fields))
		for _, field := range fields {
			if field.Visibility == "hidden" {
				continue
			}
			visibleFields = append(visibleFields, field)
		}
		system.Templates[templateIndex].Fields = visibleFields
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

type storyDirectorStructuredStrategy struct {
	Enabled                  bool    `json:"enabled"`
	MainlineStrength         string  `json:"mainline_strength,omitempty"`
	FailurePolicy            string  `json:"failure_policy,omitempty"`
	PacingCurve              string  `json:"pacing_curve,omitempty"`
	RandomEventRate          float64 `json:"random_event_rate,omitempty"`
	DirectorAgentMode        string  `json:"director_agent_mode,omitempty"`
	RuleStateConsumptionMode string  `json:"rule_state_consumption_mode,omitempty"`
	RuleVisibilityMode       string  `json:"rule_visibility_mode,omitempty"`
	BranchPlanningTurns      int     `json:"branch_planning_turns,omitempty"`
}

func storyDirectorStructuredStrategySummary(strategy StoryDirectorStrategy) storyDirectorStructuredStrategy {
	return storyDirectorStructuredStrategy{
		Enabled:                  strategy.Enabled,
		MainlineStrength:         strategy.MainlineStrength,
		FailurePolicy:            strategy.FailurePolicy,
		PacingCurve:              strategy.PacingCurve,
		RandomEventRate:          strategy.RandomEventRate,
		DirectorAgentMode:        strategy.DirectorAgentMode,
		RuleStateConsumptionMode: strategy.RuleStateConsumptionMode,
		RuleVisibilityMode:       strategy.RuleVisibilityMode,
		BranchPlanningTurns:      strategy.BranchPlanningTurns,
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
