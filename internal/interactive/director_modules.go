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

	"denova/internal/imagepreset"
)

const (
	storyDirectorModuleVersion     = 6
	DefaultEventPackageID          = "default"
	DefaultEventSystemID           = "default"
	DefaultRuleSystemID            = "default"
	RuleSystemFailForwardID        = "dm-fail-forward"
	RuleSystemOSRPlayerSkillID     = "dm-osr-player-skill"
	RuleSystemCinematicHeroicID    = "dm-cinematic-heroic"
	RuleSystemGrittySurvivalID     = "dm-gritty-survival"
	RuleSystemMysteryClueForwardID = "dm-mystery-clue-forward"
	RuleSystemDramaStakesID        = "dm-drama-stakes"
	DefaultOpeningSelectorID       = "default"
)

var (
	ErrEventPackageRevisionConflict    = errors.New("事件包已被其他操作更新，请重新加载后再保存")
	ErrRuleSystemRevisionConflict      = errors.New("TRPG 检定已被其他操作更新，请重新加载后再保存")
	ErrOpeningSelectorRevisionConflict = errors.New("开局选择器已被其他操作更新，请重新加载后再保存")
	ErrActorStateRevisionConflict      = errors.New("状态系统已被其他操作更新，请重新加载后再保存")
)

// StoryDirectorModuleRefs declares the reusable resources a story director
// combines at runtime. Changing a referenced module affects future resolution.
type StoryDirectorModuleRefs struct {
	NarrativeStyleID        string   `json:"narrative_style_id,omitempty"`
	NarrativeStyleDisabled  bool     `json:"narrative_style_disabled,omitempty"`
	EventPackageIDs         []string `json:"event_package_ids,omitempty"`
	EventPackagesDisabled   bool     `json:"event_packages_disabled,omitempty"`
	EventSystemID           string   `json:"event_system_id,omitempty"`
	EventSystemDisabled     bool     `json:"event_system_disabled,omitempty"`
	RuleSystemID            string   `json:"rule_system_id,omitempty"`
	RuleSystemDisabled      bool     `json:"rule_system_disabled,omitempty"`
	ActorStateID            string   `json:"actor_state_id,omitempty"`
	ActorStateDisabled      bool     `json:"actor_state_disabled,omitempty"`
	MemoryStructureID       string   `json:"memory_structure_id,omitempty"`
	MemoryStructureDisabled bool     `json:"memory_structure_disabled,omitempty"`
	OpeningSelectorID       string   `json:"opening_selector_id,omitempty"`
	OpeningSelectorDisabled bool     `json:"opening_selector_disabled,omitempty"`
	ImagePresetID           string   `json:"image_preset_id,omitempty"`
	ImagePresetDisabled     bool     `json:"image_preset_disabled,omitempty"`
}

type StoryDirectorModuleWarning struct {
	Module  string `json:"module"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`
}

// StoryDirectorResolvedSnapshot is the last known-good expanded module graph.
// It lets directors and stories keep working when a referenced module is
// deleted, renamed, or temporarily invalid.
type StoryDirectorResolvedSnapshot struct {
	Version               int                           `json:"version"`
	ResolvedAt            string                        `json:"resolved_at,omitempty"`
	Status                string                        `json:"status,omitempty"`
	Warnings              []StoryDirectorModuleWarning  `json:"warnings,omitempty"`
	ModuleRefs            StoryDirectorModuleRefs       `json:"module_refs"`
	NarrativeStyleID      string                        `json:"narrative_style_id,omitempty"`
	ImagePresetID         string                        `json:"image_preset_id,omitempty"`
	EventPackages         []TellerEventPackage          `json:"event_packages,omitempty"`
	EventSystem           StoryDirectorEventSystem      `json:"-"`
	TRPGSystem            StoryDirectorTRPGSystem       `json:"trpg_system,omitempty"`
	ActorState            StoryDirectorActorStateSystem `json:"actor_state,omitempty"`
	StoryMemoryStructures []StoryMemoryStructure        `json:"story_memory_structures,omitempty"`
	OpeningSelector       StoryDirectorOpeningSelector  `json:"opening_selector,omitempty"`
}

type EventPackageModule struct {
	Version           int               `json:"version"`
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Events            []TellerEventCard `json:"events,omitempty"`
	Path              string            `json:"path,omitempty"`
	Custom            bool              `json:"custom"`
	BuiltinOverridden bool              `json:"builtin_overridden,omitempty"`
	Invalid           bool              `json:"invalid,omitempty"`
	Error             string            `json:"error,omitempty"`
	CreatedAt         string            `json:"created_at,omitempty"`
	UpdatedAt         string            `json:"updated_at,omitempty"`
}

type EventSystemModule struct {
	Version           int                      `json:"version"`
	ID                string                   `json:"id"`
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	EventSystem       StoryDirectorEventSystem `json:"event_system"`
	Tags              []string                 `json:"tags"`
	Path              string                   `json:"path,omitempty"`
	Custom            bool                     `json:"custom"`
	BuiltinOverridden bool                     `json:"builtin_overridden,omitempty"`
	Invalid           bool                     `json:"invalid,omitempty"`
	Error             string                   `json:"error,omitempty"`
	CreatedAt         string                   `json:"created_at,omitempty"`
	UpdatedAt         string                   `json:"updated_at,omitempty"`
}

type RuleSystemModule struct {
	Version           int                     `json:"version"`
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	Description       string                  `json:"description"`
	ActorStateID      string                  `json:"actor_state_id,omitempty"`
	TRPGSystem        StoryDirectorTRPGSystem `json:"trpg_system"`
	Path              string                  `json:"path,omitempty"`
	Custom            bool                    `json:"custom"`
	BuiltinOverridden bool                    `json:"builtin_overridden,omitempty"`
	Invalid           bool                    `json:"invalid,omitempty"`
	Error             string                  `json:"error,omitempty"`
	CreatedAt         string                  `json:"created_at,omitempty"`
	UpdatedAt         string                  `json:"updated_at,omitempty"`
}

type ActorStateModule struct {
	Version           int                           `json:"version"`
	ID                string                        `json:"id"`
	Name              string                        `json:"name"`
	Description       string                        `json:"description"`
	ActorState        StoryDirectorActorStateSystem `json:"actor_state"`
	OpeningSelector   StoryDirectorOpeningSelector  `json:"opening_selector,omitempty"`
	MigrationWarnings []string                      `json:"migration_warnings,omitempty"`
	Path              string                        `json:"path,omitempty"`
	Custom            bool                          `json:"custom"`
	BuiltinOverridden bool                          `json:"builtin_overridden,omitempty"`
	Invalid           bool                          `json:"invalid,omitempty"`
	Error             string                        `json:"error,omitempty"`
	CreatedAt         string                        `json:"created_at,omitempty"`
	UpdatedAt         string                        `json:"updated_at,omitempty"`
	NeedsMigration    bool                          `json:"-"`
	SourceVersion     int                           `json:"-"`
}

type OpeningSelectorModule struct {
	Version           int                          `json:"version"`
	ID                string                       `json:"id"`
	Name              string                       `json:"name"`
	Description       string                       `json:"description"`
	OpeningSelector   StoryDirectorOpeningSelector `json:"opening_selector"`
	Tags              []string                     `json:"tags"`
	Path              string                       `json:"path,omitempty"`
	Custom            bool                         `json:"custom"`
	BuiltinOverridden bool                         `json:"builtin_overridden,omitempty"`
	Invalid           bool                         `json:"invalid,omitempty"`
	Error             string                       `json:"error,omitempty"`
	CreatedAt         string                       `json:"created_at,omitempty"`
	UpdatedAt         string                       `json:"updated_at,omitempty"`
}

type EventPackageLibrary struct {
	novaDir string
}

type RuleSystemLibrary struct {
	novaDir string
}

type ActorStateLibrary struct {
	novaDir string
}

type OpeningSelectorLibrary struct {
	novaDir string
}

func NewEventPackageLibrary(novaDir string) *EventPackageLibrary {
	return &EventPackageLibrary{novaDir: novaDir}
}

func NewRuleSystemLibrary(novaDir string) *RuleSystemLibrary {
	return &RuleSystemLibrary{novaDir: novaDir}
}

func NewActorStateLibrary(novaDir string) *ActorStateLibrary {
	return &ActorStateLibrary{novaDir: novaDir}
}

func NewOpeningSelectorLibrary(novaDir string) *OpeningSelectorLibrary {
	return &OpeningSelectorLibrary{novaDir: novaDir}
}

func (l *EventPackageLibrary) List() ([]EventPackageModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]EventPackageModule, 0, len(files))
	for _, file := range files {
		item, err := parseEventPackageFile(file)
		if err != nil {
			id := strings.TrimSuffix(filepath.Base(file), ".json")
			items = append(items, EventPackageModule{ID: id, Path: file, Invalid: true, Error: err.Error(), Custom: !IsBuiltinEventPackageID(id)})
			continue
		}
		item.Path = file
		item = applyEventPackageOwnership(item)
		items = append(items, item)
	}
	sortEventPackages(items)
	return items, nil
}

func (l *EventPackageLibrary) Get(id string) (EventPackageModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventPackageModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if id == "" {
		id = DefaultEventPackageID
	}
	if err := validateDirectorModuleID(id, "事件包"); err != nil {
		return EventPackageModule{}, err
	}
	item, err := parseEventPackageFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return EventPackageModule{}, err
	}
	return applyEventPackageOwnership(item), nil
}

func (l *EventPackageLibrary) Create(item EventPackageModule) (EventPackageModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventPackageModule{}, err
	}
	item = normalizeEventPackageModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("event-package")
	}
	item.BuiltinOverridden = false
	if err := validateEventPackageModule(item); err != nil {
		return EventPackageModule{}, err
	}
	path := filepath.Join(l.dir(), item.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return EventPackageModule{}, fmt.Errorf("事件包已存在: %s", item.ID)
	} else if !os.IsNotExist(err) {
		return EventPackageModule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := writeEventPackageFile(path, item); err != nil {
		return EventPackageModule{}, err
	}
	item.Path = path
	return applyEventPackageOwnership(item), nil
}

func (l *EventPackageLibrary) Update(id string, item EventPackageModule, baseRevision string) (EventPackageModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventPackageModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "事件包"); err != nil {
		return EventPackageModule{}, err
	}
	isBuiltin := IsBuiltinEventPackageID(id)
	current, err := l.Get(id)
	if err != nil {
		return EventPackageModule{}, err
	}
	if strings.TrimSpace(baseRevision) != "" && strings.TrimSpace(current.UpdatedAt) != strings.TrimSpace(baseRevision) {
		return EventPackageModule{}, ErrEventPackageRevisionConflict
	}
	item = normalizeEventPackageModule(item)
	item.ID = id
	item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	item.BuiltinOverridden = isBuiltin
	if err := validateEventPackageModule(item); err != nil {
		return EventPackageModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeEventPackageFile(path, item); err != nil {
		return EventPackageModule{}, err
	}
	item.Path = path
	return applyEventPackageOwnership(item), nil
}

func (l *EventPackageLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "事件包"); err != nil {
		return err
	}
	if IsBuiltinEventPackageID(id) {
		item, ok := builtinEventPackageModuleByID(id)
		if !ok {
			return fmt.Errorf("内置事件包不存在: %s", id)
		}
		return writeEventPackageFile(filepath.Join(l.dir(), id+".json"), item)
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *EventPackageLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-director-modules", "event-packages")
}

func (l *EventPackageLibrary) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	for _, item := range builtinEventPackageModules() {
		path := filepath.Join(l.dir(), item.ID+".json")
		if current, err := parseEventPackageFile(path); err == nil && current.BuiltinOverridden {
			continue
		} else if err == nil && current.Version == item.Version {
			continue
		}
		if err := writeEventPackageFile(path, item); err != nil {
			return err
		}
	}
	return l.migrateLegacyEventSystems()
}

func (l *EventPackageLibrary) migrateLegacyEventSystems() error {
	files, err := filepath.Glob(filepath.Join(l.novaDir, "story-director-modules", "event-systems", "*.json"))
	if err != nil {
		return err
	}
	for _, file := range files {
		legacy, err := parseEventSystemFile(file)
		if err != nil {
			continue
		}
		for _, item := range eventPackageModulesFromLegacyEventSystem(legacy) {
			path := filepath.Join(l.dir(), item.ID+".json")
			current, currentErr := parseEventPackageFile(path)
			if currentErr == nil {
				if !legacy.BuiltinOverridden || current.BuiltinOverridden || !IsBuiltinEventPackageID(item.ID) {
					continue
				}
			} else if !os.IsNotExist(currentErr) {
				return currentErr
			}
			if err := writeEventPackageFile(path, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *RuleSystemLibrary) List() ([]RuleSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]RuleSystemModule, 0, len(files))
	for _, file := range files {
		item, err := parseRuleSystemFile(file)
		if err != nil {
			items = append(items, RuleSystemModule{ID: strings.TrimSuffix(filepath.Base(file), ".json"), Path: file, Invalid: true, Error: err.Error(), Custom: !IsBuiltinRuleSystemID(strings.TrimSuffix(filepath.Base(file), ".json"))})
			continue
		}
		item.Path = file
		item = applyRuleSystemOwnership(item)
		items = append(items, item)
	}
	sortRuleSystems(items)
	return items, nil
}

func (l *RuleSystemLibrary) Get(id string) (RuleSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return RuleSystemModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if id == "" {
		id = DefaultRuleSystemID
	}
	if err := validateDirectorModuleID(id, "TRPG 检定"); err != nil {
		return RuleSystemModule{}, err
	}
	item, err := parseRuleSystemFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return RuleSystemModule{}, err
	}
	return applyRuleSystemOwnership(item), nil
}

func (l *RuleSystemLibrary) Create(item RuleSystemModule) (RuleSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return RuleSystemModule{}, err
	}
	item = normalizeRuleSystemModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("rule-system")
	}
	item.BuiltinOverridden = false
	if err := validateRuleSystemModule(item); err != nil {
		return RuleSystemModule{}, err
	}
	path := filepath.Join(l.dir(), item.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return RuleSystemModule{}, fmt.Errorf("TRPG 检定已存在: %s", item.ID)
	} else if !os.IsNotExist(err) {
		return RuleSystemModule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := writeRuleSystemFile(path, item); err != nil {
		return RuleSystemModule{}, err
	}
	item.Path = path
	return applyRuleSystemOwnership(item), nil
}

func (l *RuleSystemLibrary) Update(id string, item RuleSystemModule, baseRevision string) (RuleSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return RuleSystemModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "TRPG 检定"); err != nil {
		return RuleSystemModule{}, err
	}
	isBuiltin := IsBuiltinRuleSystemID(id)
	current, err := l.Get(id)
	if err != nil {
		return RuleSystemModule{}, err
	}
	if strings.TrimSpace(baseRevision) != "" && strings.TrimSpace(current.UpdatedAt) != strings.TrimSpace(baseRevision) {
		return RuleSystemModule{}, ErrRuleSystemRevisionConflict
	}
	item = normalizeRuleSystemModule(item)
	item.ID = id
	item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	item.BuiltinOverridden = isBuiltin
	if err := validateRuleSystemModule(item); err != nil {
		return RuleSystemModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeRuleSystemFile(path, item); err != nil {
		return RuleSystemModule{}, err
	}
	item.Path = path
	return applyRuleSystemOwnership(item), nil
}

func (l *RuleSystemLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "TRPG 检定"); err != nil {
		return err
	}
	if builtin, ok := builtinRuleSystemModuleByID(id); ok {
		return writeRuleSystemFile(filepath.Join(l.dir(), id+".json"), builtin)
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *RuleSystemLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-director-modules", "rule-systems")
}

func (l *RuleSystemLibrary) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	for _, builtin := range builtinRuleSystemModules() {
		path := filepath.Join(l.dir(), builtin.ID+".json")
		if current, err := parseRuleSystemFile(path); err == nil && current.BuiltinOverridden {
			continue
		} else if err == nil && current.Version == storyDirectorModuleVersion && !ruleSystemDiffersFromBuiltin(current) {
			continue
		}
		if err := writeRuleSystemFile(path, builtin); err != nil {
			return err
		}
	}
	return nil
}

func (l *OpeningSelectorLibrary) List() ([]OpeningSelectorModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]OpeningSelectorModule, 0, len(files))
	for _, file := range files {
		item, err := parseOpeningSelectorFile(file)
		if err != nil {
			items = append(items, OpeningSelectorModule{ID: strings.TrimSuffix(filepath.Base(file), ".json"), Path: file, Invalid: true, Error: err.Error(), Custom: !IsBuiltinOpeningSelectorID(strings.TrimSuffix(filepath.Base(file), ".json"))})
			continue
		}
		item.Path = file
		item = applyOpeningSelectorOwnership(item)
		items = append(items, item)
	}
	sortOpeningSelectors(items)
	return items, nil
}

func (l *OpeningSelectorLibrary) Get(id string) (OpeningSelectorModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return OpeningSelectorModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if id == "" {
		id = DefaultOpeningSelectorID
	}
	if err := validateDirectorModuleID(id, "开局选择器"); err != nil {
		return OpeningSelectorModule{}, err
	}
	item, err := parseOpeningSelectorFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return OpeningSelectorModule{}, err
	}
	return applyOpeningSelectorOwnership(item), nil
}

func (l *OpeningSelectorLibrary) Create(item OpeningSelectorModule) (OpeningSelectorModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return OpeningSelectorModule{}, err
	}
	item = normalizeOpeningSelectorModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("opening-selector")
	}
	item.BuiltinOverridden = false
	if err := validateOpeningSelectorModule(item); err != nil {
		return OpeningSelectorModule{}, err
	}
	path := filepath.Join(l.dir(), item.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return OpeningSelectorModule{}, fmt.Errorf("开局选择器已存在: %s", item.ID)
	} else if !os.IsNotExist(err) {
		return OpeningSelectorModule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := writeOpeningSelectorFile(path, item); err != nil {
		return OpeningSelectorModule{}, err
	}
	item.Path = path
	return applyOpeningSelectorOwnership(item), nil
}

func (l *OpeningSelectorLibrary) Update(id string, item OpeningSelectorModule, baseRevision string) (OpeningSelectorModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return OpeningSelectorModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "开局选择器"); err != nil {
		return OpeningSelectorModule{}, err
	}
	isBuiltin := IsBuiltinOpeningSelectorID(id)
	current, err := l.Get(id)
	if err != nil {
		return OpeningSelectorModule{}, err
	}
	if strings.TrimSpace(baseRevision) != "" && strings.TrimSpace(current.UpdatedAt) != strings.TrimSpace(baseRevision) {
		return OpeningSelectorModule{}, ErrOpeningSelectorRevisionConflict
	}
	item = normalizeOpeningSelectorModule(item)
	item.ID = id
	item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	item.BuiltinOverridden = isBuiltin
	if err := validateOpeningSelectorModule(item); err != nil {
		return OpeningSelectorModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeOpeningSelectorFile(path, item); err != nil {
		return OpeningSelectorModule{}, err
	}
	item.Path = path
	return applyOpeningSelectorOwnership(item), nil
}

func (l *OpeningSelectorLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "开局选择器"); err != nil {
		return err
	}
	if IsBuiltinOpeningSelectorID(id) {
		return writeOpeningSelectorFile(filepath.Join(l.dir(), id+".json"), DefaultOpeningSelectorModule())
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *OpeningSelectorLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-director-modules", "opening-selectors")
}

func (l *OpeningSelectorLibrary) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(l.dir(), DefaultOpeningSelectorID+".json")
	if current, err := parseOpeningSelectorFile(path); err == nil && current.BuiltinOverridden {
		return nil
	} else if err == nil && current.Version == storyDirectorModuleVersion {
		return nil
	}
	return writeOpeningSelectorFile(path, DefaultOpeningSelectorModule())
}

func DefaultStoryDirectorModuleRefs() StoryDirectorModuleRefs {
	return StoryDirectorModuleRefs{
		NarrativeStyleID:  "classic",
		EventPackageIDs:   []string{DefaultEventPackageID},
		RuleSystemID:      DefaultRuleSystemID,
		ActorStateID:      DefaultActorStateModuleID,
		MemoryStructureID: DefaultStoryMemoryStructureModuleID,
		ImagePresetID:     imagepreset.DefaultID,
	}
}

func NormalizeStoryDirectorModuleRefs(refs StoryDirectorModuleRefs) StoryDirectorModuleRefs {
	eventPackageIDs := normalizeEventPackageIDs(refs.EventPackageIDs)
	if len(eventPackageIDs) == 0 && strings.TrimSpace(refs.EventSystemID) != "" {
		eventPackageIDs = []string{normalizeDirectorModuleID(refs.EventSystemID)}
	}
	return StoryDirectorModuleRefs{
		NarrativeStyleID:        strings.TrimSpace(refs.NarrativeStyleID),
		NarrativeStyleDisabled:  refs.NarrativeStyleDisabled,
		EventPackageIDs:         eventPackageIDs,
		EventPackagesDisabled:   refs.EventPackagesDisabled || refs.EventSystemDisabled,
		RuleSystemID:            normalizeDirectorModuleID(refs.RuleSystemID),
		RuleSystemDisabled:      refs.RuleSystemDisabled,
		ActorStateID:            normalizeDirectorModuleID(refs.ActorStateID),
		ActorStateDisabled:      refs.ActorStateDisabled,
		MemoryStructureID:       normalizeDirectorModuleID(refs.MemoryStructureID),
		MemoryStructureDisabled: refs.MemoryStructureDisabled,
		ImagePresetID:           imagepreset.NormalizeID(refs.ImagePresetID),
		ImagePresetDisabled:     refs.ImagePresetDisabled,
	}
}

func StoryDirectorModuleRefsEmpty(refs StoryDirectorModuleRefs) bool {
	refs = NormalizeStoryDirectorModuleRefs(refs)
	return refs.NarrativeStyleID == "" &&
		len(refs.EventPackageIDs) == 0 &&
		refs.RuleSystemID == "" &&
		refs.ActorStateID == "" &&
		refs.MemoryStructureID == "" &&
		refs.ImagePresetID == "" &&
		!refs.NarrativeStyleDisabled &&
		!refs.EventPackagesDisabled &&
		!refs.RuleSystemDisabled &&
		!refs.ActorStateDisabled &&
		!refs.MemoryStructureDisabled &&
		!refs.ImagePresetDisabled
}

func StoryDirectorNarrativeStyleEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).NarrativeStyleDisabled
}

func StoryDirectorEventSystemEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).EventPackagesDisabled
}

func StoryDirectorMemoryStructureEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).MemoryStructureDisabled
}

func StoryDirectorImagePresetEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).ImagePresetDisabled
}

func ResolveStoryDirectorModules(novaDir string, director StoryDirector) StoryDirector {
	director = normalizeStoryDirector(director)
	refs := NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
	refs.EventPackageIDs = expandLegacyEventPackageRefs(novaDir, refs.EventPackageIDs)
	if StoryDirectorModuleRefsEmpty(refs) {
		if storyDirectorHasEmbeddedModules(director) {
			refs.MemoryStructureID = DefaultStoryMemoryStructureModuleID
			director.ModuleRefs = refs
		} else {
			refs = DefaultStoryDirectorModuleRefs()
			director.ModuleRefs = refs
		}
	} else if refs.MemoryStructureID == "" && !refs.MemoryStructureDisabled {
		refs.MemoryStructureID = DefaultStoryMemoryStructureModuleID
		director.ModuleRefs = refs
	}
	if refs.ActorStateID == "" && !refs.ActorStateDisabled && actorStateEmpty(director.ActorState) && openingSelectorEmpty(director.OpeningSelector) {
		refs.ActorStateID = DefaultActorStateModuleID
		director.ModuleRefs = refs
	}

	warnings := []StoryDirectorModuleWarning{}
	snapshot := normalizeStoryDirectorResolvedSnapshot(director.ResolvedSnapshot)
	effective := director
	effective.ModuleRefs = refs
	storyMemoryStructures := snapshot.StoryMemoryStructures

	if refs.EventPackagesDisabled {
		effective.EventPackages = []TellerEventPackage{}
	} else if len(refs.EventPackageIDs) > 0 {
		packages, packageWarnings := resolveEventPackages(novaDir, refs.EventPackageIDs)
		if len(packageWarnings) > 0 {
			warnings = append(warnings, packageWarnings...)
		}
		if len(packages) > 0 && len(packageWarnings) == 0 {
			effective.EventPackages = packages
		} else if len(snapshot.EventPackages) > 0 {
			effective.EventPackages = snapshot.EventPackages
		} else {
			effective.EventPackages = packages
		}
	}
	if refs.RuleSystemDisabled {
		effective.TRPGSystem = StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{}}
	} else if refs.RuleSystemID != "" {
		if module, err := NewRuleSystemLibrary(novaDir).Get(refs.RuleSystemID); err == nil {
			effective.TRPGSystem = module.TRPGSystem
			if module.ActorStateID != "" {
				refs.ActorStateID = module.ActorStateID
				refs.ActorStateDisabled = false
				effective.ModuleRefs = refs
			}
		} else if !ruleSystemEmpty(snapshot.TRPGSystem) {
			effective.TRPGSystem = snapshot.TRPGSystem
			warnings = append(warnings, moduleWarning("rule_system", refs.RuleSystemID, err))
		} else {
			warnings = append(warnings, moduleWarning("rule_system", refs.RuleSystemID, err))
		}
	}
	if refs.ActorStateDisabled {
		effective.ActorState = StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{}, InitialActors: []ActorStateInitialActor{}}
	} else if refs.ActorStateID != "" {
		if module, err := NewActorStateLibrary(novaDir).Get(refs.ActorStateID); err == nil {
			effective.ActorState = module.ActorState
		} else if !actorStateEmpty(snapshot.ActorState) {
			effective.ActorState = snapshot.ActorState
			warnings = append(warnings, moduleWarning("actor_state", refs.ActorStateID, err))
		} else {
			warnings = append(warnings, moduleWarning("actor_state", refs.ActorStateID, err))
		}
	}
	if refs.MemoryStructureDisabled {
		storyMemoryStructures = []StoryMemoryStructure{}
	} else if refs.MemoryStructureID != "" {
		if module, err := NewStoryMemoryStructureLibrary(novaDir).Get(refs.MemoryStructureID); err == nil {
			storyMemoryStructures = module.Structures
		} else if len(snapshot.StoryMemoryStructures) > 0 {
			storyMemoryStructures = snapshot.StoryMemoryStructures
			warnings = append(warnings, moduleWarning("story_memory_structure", refs.MemoryStructureID, err))
		} else {
			storyMemoryStructures = []StoryMemoryStructure{}
			warnings = append(warnings, moduleWarning("story_memory_structure", refs.MemoryStructureID, err))
		}
	}
	effective.OpeningSelector = StoryDirectorOpeningSelector{}
	if !refs.NarrativeStyleDisabled && refs.NarrativeStyleID != "" {
		if _, err := NewTellerLibrary(novaDir).Get(refs.NarrativeStyleID); err != nil {
			warnings = append(warnings, moduleWarning("narrative_style", refs.NarrativeStyleID, err))
		}
	}
	if !refs.ImagePresetDisabled && refs.ImagePresetID != "" {
		if _, err := imagepreset.NewLibrary(novaDir).Get(refs.ImagePresetID); err != nil {
			warnings = append(warnings, moduleWarning("image_preset", refs.ImagePresetID, err))
		}
	}
	effective.ResolvedSnapshot = snapshotFromEffectiveDirector(effective, refs, warnings, storyMemoryStructures)
	return normalizeStoryDirector(effective)
}

func snapshotFromEffectiveDirector(director StoryDirector, refs StoryDirectorModuleRefs, warnings []StoryDirectorModuleWarning, storyMemoryStructures []StoryMemoryStructure) StoryDirectorResolvedSnapshot {
	status := "ready"
	if len(warnings) > 0 {
		status = "warning"
	}
	return normalizeStoryDirectorResolvedSnapshot(StoryDirectorResolvedSnapshot{
		Version:               storyDirectorModuleVersion,
		ResolvedAt:            time.Now().UTC().Format(time.RFC3339Nano),
		Status:                status,
		Warnings:              warnings,
		ModuleRefs:            refs,
		NarrativeStyleID:      refs.NarrativeStyleID,
		ImagePresetID:         refs.ImagePresetID,
		EventPackages:         director.EventPackages,
		TRPGSystem:            director.TRPGSystem,
		ActorState:            director.ActorState,
		StoryMemoryStructures: storyMemoryStructures,
		OpeningSelector:       director.OpeningSelector,
	})
}

func moduleWarning(module, id string, err error) StoryDirectorModuleWarning {
	message := "模块不可用，已尝试使用最近可用快照。"
	if err != nil {
		message = err.Error()
	}
	return StoryDirectorModuleWarning{Module: module, ID: id, Message: trimBytes(message, 512)}
}

func DefaultEventPackageModule() EventPackageModule {
	config := DefaultTellerOrchestrationConfig()
	pkg := config.EventPackages[0]
	return normalizeEventPackageModule(EventPackageModule{
		Version:     storyDirectorModuleVersion,
		ID:          DefaultEventPackageID,
		Name:        "默认事件包",
		Description: "通用爽文与互动叙事事件卡，覆盖打脸、奇遇、冲突、恋爱、伏笔回收等基础事件。",
		Events:      pkg.Events,
	})
}

func DefaultRuleSystemModule() RuleSystemModule {
	module, _ := builtinRuleSystemModuleByID(DefaultRuleSystemID)
	return module
}

func builtinRuleSystemModules() []RuleSystemModule {
	return []RuleSystemModule{
		builtinRuleSystemModule(
			DefaultRuleSystemID,
			"均衡 DM 检定",
			"通用均衡裁定风格：有风险和不确定性时掷骰，失败保留推进空间并给出明确代价。",
			DefaultRuleCheckTemplates()[0],
		),
		builtinRuleSystemModule(
			RuleSystemFailForwardID,
			"推进型 DM：失败也前进",
			"失败不会让故事停住；检定决定推进方式、代价、压力和新选择。",
			RuleCheck{
				ID:                  RuleSystemFailForwardID,
				Label:               "推进型 DM：失败也前进",
				Dice:                "1d20",
				FailurePolicy:       "fail_forward",
				DifficultyGuidance:  "默认 normal。计划充分、资源合适、处境有利时降一档；时间压力、敌对环境、信息不足或连续失败后升一档。",
				StateEffectGuidance: "失败优先增加时间压力、敌意、警戒度、资源消耗、关系损伤或后续劣势，而不是直接否定行动。",
				Trigger:             "当玩家行动有风险和不确定性，但故事不应该因为一次失败停住时使用。检定用于决定推进方式和代价，而不是决定剧情是否继续。",
				MustCheckExamples:   []string{"玩家强行穿过守卫封锁线，失败也会进入新局面。", "玩家尝试说服关键 NPC，失败会改变条件而不是关闭剧情。", "玩家在危险现场搜索线索，失败仍能得到方向但会带来压力。"},
				SkipCheckExamples:   []string{"行动没有明确风险或代价。", "失败只会让剧情卡住且没有有趣后果。", "玩家提出了足够合理且无阻碍的解决方案。"},
				SuccessHint:         "成功时让玩家达成目标，并给出清楚的新信息、新位置或新机会。",
				FailureHint:         "失败时仍推进局势，但附加代价、暴露、误导、延迟或更糟的选择。",
			},
		),
		builtinRuleSystemModule(
			RuleSystemOSRPlayerSkillID,
			"OSR 型 DM：玩家技巧优先",
			"优先奖励具体方案和谨慎探索；只有风险仍未解除时才掷骰，失败后果较硬。",
			RuleCheck{
				ID:                  RuleSystemOSRPlayerSkillID,
				Label:               "OSR 型 DM：玩家技巧优先",
				Dice:                "1d20",
				FailurePolicy:       "blocked",
				DifficultyGuidance:  "根据风险和信息差设定。方法粗糙或信息不足升一档；准备充分、工具合适、描述具体降一档。玩家方案直接解决问题时不要检定。",
				StateEffectGuidance: "失败可以触发陷阱、消耗工具、浪费时间、暴露位置或封锁当前路径。代价应具体且和玩家选择相关。",
				Trigger:             "当玩法重点是探索、谨慎决策、描述细节和规避风险时使用。玩家说清楚方法且方法合理时尽量不掷骰；只有方法不足、风险仍未解除时才检定。",
				MustCheckExamples:   []string{"玩家只说“我检查陷阱”，但没有说明检查哪里或怎么检查。", "玩家在不了解机关原理的情况下强行拆除。", "玩家冒险尝试未经验证的计划。"},
				SkipCheckExamples:   []string{"玩家明确描述检查铰链、地缝、线孔和压力板。", "玩家用长杆安全触发可疑地砖。", "玩家找到正确钥匙并确认门没有额外机关。"},
				SuccessHint:         "成功时确认玩家方案有效，并奖励谨慎观察、工具使用或环境互动。",
				FailureHint:         "失败时后果较硬，但要让玩家明白风险来自自己的选择。",
			},
		),
		builtinRuleSystemModule(
			RuleSystemCinematicHeroicID,
			"电影英雄型 DM：高光优先",
			"优先保护角色高光和类型片节奏；检定决定高光是否完美以及是否付出代价。",
			RuleCheck{
				ID:                  RuleSystemCinematicHeroicID,
				Label:               "电影英雄型 DM：高光优先",
				Dice:                "1d20",
				Modifier:            -1,
				FailurePolicy:       "success_at_cost",
				DifficultyGuidance:  "默认 easy 或 normal。符合角色专长、场面高光、前文铺垫充分时降一档；挑战远超能力或连续冒险时升一档。",
				StateEffectGuidance: "代价偏叙事化：装备受损、体力消耗、暴露身份、欠下人情、留下伤痕或引出更强敌人。避免轻易阻断高光。",
				Trigger:             "当玩家行动符合角色高光、类型片节奏或英雄幻想时使用。检定重点不是惩罚失败，而是决定高光是否完美、是否付出代价。",
				MustCheckExamples:   []string{"主角从爆炸边缘跃出并救下同伴。", "主角在众目睽睽下完成逆转式发言。", "主角以冒险方式突破强敌封锁。"},
				SkipCheckExamples:   []string{"普通移动、普通对话或没有戏剧张力的动作。", "角色能力明显足够且失败不会产生戏剧价值。", "只是补充帅气描述，不改变局势。"},
				SuccessHint:         "成功时放大角色魅力和场面反馈，让玩家感到行动确实改变局势。",
				FailureHint:         "失败时也允许完成部分目标，但附带明显代价或新的危机。",
			},
		),
		builtinRuleSystemModule(
			RuleSystemGrittySurvivalID,
			"硬核生存型 DM：资源与后果",
			"强调危险、稀缺和长期状态；失败会明确消耗资源或恶化处境。",
			RuleCheck{
				ID:                  RuleSystemGrittySurvivalID,
				Label:               "硬核生存型 DM：资源与后果",
				Dice:                "1d20",
				Modifier:            1,
				FailurePolicy:       "hard_failure",
				DifficultyGuidance:  "默认 normal。装备、防护、休息和补给充足时降一档；疲劳、伤病、恶劣天气、黑暗、饥饿、追兵或缺工具时升一档。",
				StateEffectGuidance: "失败应落到资源和状态：体力、生命、补给、伤势、感染、疲劳、寒冷、士气或装备耐久。连续失败会累积后果。",
				Trigger:             "当故事强调危险、稀缺、伤病、疲劳、补给和长期后果时使用。检定用于让风险真实落地，失败可以明显恶化处境。",
				MustCheckExamples:   []string{"玩家在饥饿和受伤状态下继续赶路。", "玩家冒雨攀爬湿滑峭壁。", "玩家在缺少工具时处理感染伤口。"},
				SkipCheckExamples:   []string{"角色在安全营地完成常规休整。", "资源充足且行动没有压力。", "失败不会消耗资源或改变处境。"},
				SuccessHint:         "成功时渡过当前危险，但仍保留环境压力。",
				FailureHint:         "失败时明确扣减资源或施加状态，不要只给轻描淡写的叙事惩罚。",
			},
		),
		builtinRuleSystemModule(
			RuleSystemMysteryClueForwardID,
			"悬疑调查型 DM：线索不断线",
			"核心线索不因失败消失；检定决定信息质量、误导、时间压力和调查代价。",
			RuleCheck{
				ID:                  RuleSystemMysteryClueForwardID,
				Label:               "悬疑调查型 DM：线索不断线",
				Dice:                "1d20",
				FailurePolicy:       "fail_forward",
				DifficultyGuidance:  "线索新鲜、现场完整、推理合理时降一档；线索被伪装、时间久远、证人撒谎、现场被污染时升一档。",
				StateEffectGuidance: "失败不删除核心线索，而是增加误导、时间压力、敌人警觉、线索噪音或调查资源消耗。",
				Trigger:             "当检定关系到线索、真相、调查方向或谜题推进时使用。核心线索不应因失败完全消失，检定决定信息质量、代价和误导程度。",
				MustCheckExamples:   []string{"玩家在混乱现场寻找凶手留下的关键痕迹。", "玩家判断证词中的矛盾是否有意义。", "玩家试图从残缺记录里还原事件顺序。"},
				SkipCheckExamples:   []string{"玩家查看明摆在桌上的信件。", "NPC 已经明确告诉玩家的信息。", "玩家提出的推理已经足以连接已知证据。"},
				SuccessHint:         "成功时给出清晰、可行动、能推进判断的线索。",
				FailureHint:         "失败时给出不完整或带偏差的信息，并制造新的调查压力。",
			},
		),
		builtinRuleSystemModule(
			RuleSystemDramaStakesID,
			"戏剧张力型 DM：只为重大赌注掷骰",
			"只在关系、信念、承诺、身份或剧情方向会被改变时掷骰。",
			RuleCheck{
				ID:                  RuleSystemDramaStakesID,
				Label:               "戏剧张力型 DM：只为重大赌注掷骰",
				Dice:                "1d20",
				FailurePolicy:       "success_at_cost",
				DifficultyGuidance:  "赌注越大、关系越紧张、对方立场越坚定，难度越高。已有信任、共同利益、真诚让步或强证据可降低难度。",
				StateEffectGuidance: "结果应影响关系、信任、债务、名声、阵营态度、秘密暴露或人物承诺。代价可以是情感、人情或长期剧情负担。",
				Trigger:             "当行动关系到人物关系、信念、承诺、身份暴露、道德选择或剧情转折时使用。只有结果会改变角色关系或故事方向时才检定。",
				MustCheckExamples:   []string{"玩家向背叛过自己的盟友再次求助。", "玩家为了保护同伴公开暴露身份。", "玩家试图说服敌人放弃复仇。"},
				SkipCheckExamples:   []string{"普通寒暄或交换已知信息。", "没有关系变化的礼貌请求。", "玩家只是表达情绪但不承担行动后果。"},
				SuccessHint:         "成功时让关系或立场发生可见变化。",
				FailureHint:         "失败时不一定完全拒绝，但会附加条件、伤害关系、暴露弱点或制造长期后果。",
			},
		),
	}
}

func builtinRuleSystemModule(id, name, description string, check RuleCheck) RuleSystemModule {
	return normalizeRuleSystemModule(RuleSystemModule{
		Version:     storyDirectorModuleVersion,
		ID:          id,
		Name:        name,
		Description: description,
		TRPGSystem:  StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{check}},
	})
}

func DefaultActorStateModule() ActorStateModule {
	return normalizeActorStateModule(ActorStateModule{
		Version:     storyDirectorModuleVersion,
		ID:          DefaultActorStateModuleID,
		Name:        "默认状态系统",
		Description: "以主角等关键状态对象为起点维护结构化字段和可复用词条库，供规则检定、资源消耗和长期承接读取；可按作品需要扩展其他状态表模板。",
		ActorState:  defaultActorStateSystem(),
	})
}

func DefaultOpeningSelectorModule() OpeningSelectorModule {
	config := DefaultTellerOrchestrationConfig()
	return normalizeOpeningSelectorModule(OpeningSelectorModule{
		Version:         storyDirectorModuleVersion,
		ID:              DefaultOpeningSelectorID,
		Name:            "默认开局选择器",
		Description:     "提供可跳过的开局词条和初始状态变更入口。",
		OpeningSelector: StoryDirectorOpeningSelector{Enabled: config.Opening.Enabled, TraitPools: config.Opening.TraitPools, InitialStateOps: config.Opening.InitialStateOps},
		Tags:            []string{"内置", "开局"},
	})
}

func IsBuiltinEventPackageID(id string) bool {
	_, ok := builtinEventPackageModuleByID(id)
	return ok
}

func IsBuiltinRuleSystemID(id string) bool {
	_, ok := builtinRuleSystemModuleByID(id)
	return ok
}

func IsBuiltinActorStateID(id string) bool {
	_, ok := builtinActorStateModuleByID(id)
	return ok
}

func IsBuiltinOpeningSelectorID(id string) bool {
	return normalizeDirectorModuleID(id) == DefaultOpeningSelectorID
}

func builtinEventPackageModuleByID(id string) (EventPackageModule, bool) {
	id = normalizeDirectorModuleID(id)
	for _, item := range builtinEventPackageModules() {
		if item.ID == id {
			return item, true
		}
	}
	return EventPackageModule{}, false
}

func builtinRuleSystemModuleByID(id string) (RuleSystemModule, bool) {
	id = normalizeDirectorModuleID(id)
	for _, item := range builtinRuleSystemModules() {
		if item.ID == id {
			return item, true
		}
	}
	return RuleSystemModule{}, false
}

func applyEventPackageOwnership(item EventPackageModule) EventPackageModule {
	if !IsBuiltinEventPackageID(item.ID) {
		item.Custom = true
		item.BuiltinOverridden = false
		return item
	}
	item.Custom = false
	item.BuiltinOverridden = item.BuiltinOverridden || eventPackageDiffersFromBuiltin(item)
	return item
}

func eventPackageDiffersFromBuiltin(item EventPackageModule) bool {
	builtin, ok := builtinEventPackageModuleByID(item.ID)
	if !ok {
		return false
	}
	return !reflect.DeepEqual(eventPackageComparable(item), eventPackageComparable(builtin))
}

func eventPackageComparable(item EventPackageModule) EventPackageModule {
	item = normalizeEventPackageModule(item)
	item.Path = ""
	item.Custom = false
	item.BuiltinOverridden = false
	item.Invalid = false
	item.Error = ""
	item.CreatedAt = ""
	item.UpdatedAt = ""
	return item
}

func applyRuleSystemOwnership(item RuleSystemModule) RuleSystemModule {
	if !IsBuiltinRuleSystemID(item.ID) {
		item.Custom = true
		item.BuiltinOverridden = false
		return item
	}
	item.Custom = false
	item.BuiltinOverridden = item.BuiltinOverridden || ruleSystemDiffersFromBuiltin(item)
	return item
}

func ruleSystemDiffersFromBuiltin(item RuleSystemModule) bool {
	builtin, ok := builtinRuleSystemModuleByID(item.ID)
	if !ok {
		return false
	}
	return !reflect.DeepEqual(ruleSystemComparable(item), ruleSystemComparable(builtin))
}

func ruleSystemComparable(item RuleSystemModule) RuleSystemModule {
	item = normalizeRuleSystemModule(item)
	item.Path = ""
	item.Custom = false
	item.BuiltinOverridden = false
	item.Invalid = false
	item.Error = ""
	item.CreatedAt = ""
	item.UpdatedAt = ""
	return item
}

func applyActorStateOwnership(item ActorStateModule) ActorStateModule {
	if !IsBuiltinActorStateID(item.ID) {
		item.Custom = true
		item.BuiltinOverridden = false
		return item
	}
	item.Custom = false
	item.BuiltinOverridden = item.BuiltinOverridden || actorStateDiffersFromBuiltin(item)
	return item
}

func actorStateDiffersFromBuiltin(item ActorStateModule) bool {
	builtin, ok := builtinActorStateModuleByID(item.ID)
	if !ok {
		return false
	}
	return !reflect.DeepEqual(actorStateComparable(item), actorStateComparable(builtin))
}

func actorStateComparable(item ActorStateModule) ActorStateModule {
	item = normalizeActorStateModule(item)
	item.Path = ""
	item.Custom = false
	item.BuiltinOverridden = false
	item.Invalid = false
	item.Error = ""
	item.CreatedAt = ""
	item.UpdatedAt = ""
	item.NeedsMigration = false
	item.SourceVersion = 0
	return item
}

func applyOpeningSelectorOwnership(item OpeningSelectorModule) OpeningSelectorModule {
	if !IsBuiltinOpeningSelectorID(item.ID) {
		item.Custom = true
		item.BuiltinOverridden = false
		return item
	}
	item.Custom = false
	item.BuiltinOverridden = item.BuiltinOverridden || openingSelectorDiffersFromBuiltin(item)
	return item
}

func openingSelectorDiffersFromBuiltin(item OpeningSelectorModule) bool {
	return !reflect.DeepEqual(openingSelectorComparable(item), openingSelectorComparable(DefaultOpeningSelectorModule()))
}

func openingSelectorComparable(item OpeningSelectorModule) OpeningSelectorModule {
	item = normalizeOpeningSelectorModule(item)
	item.Path = ""
	item.Custom = false
	item.BuiltinOverridden = false
	item.Invalid = false
	item.Error = ""
	item.CreatedAt = ""
	item.UpdatedAt = ""
	return item
}

func normalizeEventPackageModule(item EventPackageModule) EventPackageModule {
	item.Version = storyDirectorModuleVersion
	item.ID = normalizeDirectorModuleID(item.ID)
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "事件包"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.Events = normalizeTellerEventCards(item.Events, item.ID)
	return item
}

func normalizeEventSystemModule(item EventSystemModule) EventSystemModule {
	item.Version = storyDirectorModuleVersion
	item.ID = normalizeDirectorModuleID(item.ID)
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "事件系统"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.EventSystem.EventPackages = normalizeTellerEventPackages(item.EventSystem.EventPackages)
	item.EventSystem.CustomEvents = normalizeDirectorEvents(item.EventSystem.CustomEvents)
	item.Tags = normalizeStringListLimit(item.Tags, maxTurnBriefListItems)
	return item
}

func normalizeRuleSystemModule(item RuleSystemModule) RuleSystemModule {
	item.Version = storyDirectorModuleVersion
	item.ID = normalizeDirectorModuleID(item.ID)
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "TRPG 检定"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.ActorStateID = normalizeDirectorModuleID(item.ActorStateID)
	item.TRPGSystem.RuleTemplates = normalizeRuleChecks(item.TRPGSystem.RuleTemplates)
	if len(item.TRPGSystem.RuleTemplates) == 0 {
		item.TRPGSystem.RuleTemplates = DefaultRuleCheckTemplates()
	}
	return item
}

func normalizeActorStateModule(item ActorStateModule) ActorStateModule {
	item.Version = storyDirectorModuleVersion
	item.ID = normalizeDirectorModuleID(item.ID)
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "状态系统"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.ActorState = normalizeActorStateSystem(item.ActorState)
	item.OpeningSelector = normalizeStoryDirectorOpeningSelector(item.OpeningSelector)
	if openingSelectorHasContent(item.OpeningSelector) {
		var warnings []string
		item.ActorState, warnings = migrateLegacyOpeningTraits(item.ActorState, item.OpeningSelector)
		item.MigrationWarnings = normalizeStringListLimit(append(item.MigrationWarnings, warnings...), maxTurnBriefListItems)
		item.OpeningSelector = StoryDirectorOpeningSelector{}
		item.NeedsMigration = true
	}
	return item
}

func normalizeOpeningSelectorModule(item OpeningSelectorModule) OpeningSelectorModule {
	item.Version = storyDirectorModuleVersion
	item.ID = normalizeDirectorModuleID(item.ID)
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "开局选择器"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.OpeningSelector = normalizeStoryDirectorOpeningSelector(item.OpeningSelector)
	item.Tags = normalizeStringListLimit(item.Tags, maxTurnBriefListItems)
	return item
}

func normalizeStoryDirectorResolvedSnapshot(snapshot StoryDirectorResolvedSnapshot) StoryDirectorResolvedSnapshot {
	if snapshot.Version <= 0 {
		snapshot.Version = storyDirectorModuleVersion
	}
	snapshot.ResolvedAt = trimBytes(snapshot.ResolvedAt, 128)
	snapshot.Status = trimBytes(firstNonEmptyString(snapshot.Status, "ready"), 128)
	snapshot.ModuleRefs = NormalizeStoryDirectorModuleRefs(snapshot.ModuleRefs)
	snapshot.NarrativeStyleID = strings.TrimSpace(firstNonEmptyString(snapshot.NarrativeStyleID, snapshot.ModuleRefs.NarrativeStyleID))
	snapshot.ImagePresetID = imagepreset.NormalizeID(firstNonEmptyString(snapshot.ImagePresetID, snapshot.ModuleRefs.ImagePresetID))
	if len(snapshot.EventPackages) == 0 && !eventSystemEmpty(snapshot.EventSystem) {
		snapshot.EventPackages = eventPackagesFromLegacyEventSystem(snapshot.EventSystem, "snapshot")
	}
	if snapshot.ModuleRefs.EventPackagesDisabled {
		snapshot.EventPackages = normalizeTellerEventPackagesNoDefault(snapshot.EventPackages)
	} else {
		snapshot.EventPackages = normalizeTellerEventPackagesNoDefault(snapshot.EventPackages)
	}
	snapshot.EventSystem = StoryDirectorEventSystem{}
	snapshot.TRPGSystem.RuleTemplates = normalizeRuleChecks(snapshot.TRPGSystem.RuleTemplates)
	if snapshot.ModuleRefs.ActorStateDisabled {
		snapshot.ActorState = normalizeActorStateSystem(StoryDirectorActorStateSystem{})
	} else {
		snapshot.ActorState = normalizeActorStateSystem(snapshot.ActorState)
	}
	if snapshot.ModuleRefs.MemoryStructureDisabled {
		snapshot.StoryMemoryStructures = []StoryMemoryStructure{}
	} else {
		snapshot.StoryMemoryStructures = normalizeStoryMemoryStructuresForModule(snapshot.StoryMemoryStructures)
	}
	snapshot.OpeningSelector = normalizeStoryDirectorOpeningSelector(snapshot.OpeningSelector)
	outWarnings := make([]StoryDirectorModuleWarning, 0, len(snapshot.Warnings))
	for _, warning := range snapshot.Warnings {
		warning.Module = trimBytes(warning.Module, 128)
		warning.ID = trimBytes(warning.ID, 128)
		warning.Message = trimBytes(warning.Message, 512)
		if warning.Module != "" || warning.Message != "" {
			outWarnings = append(outWarnings, warning)
		}
		if len(outWarnings) >= maxTurnBriefListItems {
			break
		}
	}
	snapshot.Warnings = outWarnings
	return snapshot
}

func validateEventPackageModule(item EventPackageModule) error {
	if err := validateDirectorModuleID(item.ID, "事件包"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("事件包名称不能为空")
	}
	return nil
}

func validateEventSystemModule(item EventSystemModule) error {
	if err := validateDirectorModuleID(item.ID, "事件系统"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("事件系统名称不能为空")
	}
	return nil
}

func validateRuleSystemModule(item RuleSystemModule) error {
	if err := validateDirectorModuleID(item.ID, "TRPG 检定"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("TRPG 检定名称不能为空")
	}
	if ruleChecksHaveStateBindings(item.TRPGSystem.RuleTemplates) && item.ActorStateID == "" {
		return errors.New("配置 state_bindings 的 TRPG 检定必须绑定状态系统 actor_state_id")
	}
	for _, check := range item.TRPGSystem.RuleTemplates {
		if err := validateRuleCheck(check); err != nil {
			return err
		}
	}
	return nil
}

func validateActorStateModule(item ActorStateModule) error {
	if err := validateDirectorModuleID(item.ID, "状态系统"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("状态系统名称不能为空")
	}
	if len(item.ActorState.Templates) == 0 {
		return errors.New("状态系统至少需要一个 actor 类型模板")
	}
	if err := validateActorStateSystem(item.ActorState); err != nil {
		return err
	}
	if err := validateActorTraitSystem(item.ActorState); err != nil {
		return err
	}
	return nil
}

func validateOpeningSelectorModule(item OpeningSelectorModule) error {
	if err := validateDirectorModuleID(item.ID, "开局选择器"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("开局选择器名称不能为空")
	}
	return validateOpeningSelectorConfig(item.OpeningSelector)
}

func validateOpeningSelectorConfig(selector StoryDirectorOpeningSelector) error {
	for _, op := range selector.InitialStateOps {
		if err := validateStateOp(op); err != nil {
			return err
		}
	}
	for _, pool := range selector.TraitPools {
		for _, trait := range pool.Traits {
			for _, op := range trait.Ops {
				if err := validateStateOp(op); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func parseEventPackageFile(path string) (EventPackageModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EventPackageModule{}, err
	}
	var item EventPackageModule
	if err := json.Unmarshal(data, &item); err != nil {
		return EventPackageModule{}, fmt.Errorf("解析事件包 JSON 失败: %w", err)
	}
	item = normalizeEventPackageModule(item)
	if err := validateEventPackageModule(item); err != nil {
		return EventPackageModule{}, err
	}
	item.Path = path
	return item, nil
}

func parseEventSystemFile(path string) (EventSystemModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EventSystemModule{}, err
	}
	var item EventSystemModule
	if err := json.Unmarshal(data, &item); err != nil {
		return EventSystemModule{}, fmt.Errorf("解析事件系统 JSON 失败: %w", err)
	}
	item = normalizeEventSystemModule(item)
	if err := validateEventSystemModule(item); err != nil {
		return EventSystemModule{}, err
	}
	item.Path = path
	return item, nil
}

func parseRuleSystemFile(path string) (RuleSystemModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuleSystemModule{}, err
	}
	var item RuleSystemModule
	if err := json.Unmarshal(data, &item); err != nil {
		return RuleSystemModule{}, fmt.Errorf("解析 TRPG 检定 JSON 失败: %w", err)
	}
	item = normalizeRuleSystemModule(item)
	if err := validateRuleSystemModule(item); err != nil {
		return RuleSystemModule{}, err
	}
	item.Path = path
	return item, nil
}

func parseActorStateFile(path string) (ActorStateModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ActorStateModule{}, err
	}
	var item ActorStateModule
	if err := json.Unmarshal(data, &item); err != nil {
		return ActorStateModule{}, fmt.Errorf("解析状态系统 JSON 失败: %w", err)
	}
	sourceVersion := item.Version
	hadLegacyOpening := openingSelectorHasContent(item.OpeningSelector)
	item = normalizeActorStateModule(item)
	item.ActorState = attachBuiltinActorStateLegacyPaths(item.ID, item.ActorState)
	item.SourceVersion = sourceVersion
	item.NeedsMigration = item.NeedsMigration || sourceVersion < storyDirectorModuleVersion || hadLegacyOpening
	if err := validateActorStateModule(item); err != nil {
		return ActorStateModule{}, err
	}
	item.Path = path
	return item, nil
}

func parseOpeningSelectorFile(path string) (OpeningSelectorModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return OpeningSelectorModule{}, err
	}
	var item OpeningSelectorModule
	if err := json.Unmarshal(data, &item); err != nil {
		return OpeningSelectorModule{}, fmt.Errorf("解析开局选择器 JSON 失败: %w", err)
	}
	item = normalizeOpeningSelectorModule(item)
	if err := validateOpeningSelectorModule(item); err != nil {
		return OpeningSelectorModule{}, err
	}
	item.Path = path
	return item, nil
}

func writeEventPackageFile(path string, item EventPackageModule) error {
	item = normalizeEventPackageModule(item)
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeRuleSystemFile(path string, item RuleSystemModule) error {
	item = normalizeRuleSystemModule(item)
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeActorStateFile(path string, item ActorStateModule) error {
	item = normalizeActorStateModule(item)
	item.NeedsMigration = false
	item.SourceVersion = 0
	data, err := marshalJSONWithoutFields(item, "opening_selector")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func marshalJSONWithoutFields(value any, fields ...string) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	for _, field := range fields {
		delete(payload, field)
	}
	return json.MarshalIndent(payload, "", "  ")
}

func writeOpeningSelectorFile(path string, item OpeningSelectorModule) error {
	item = normalizeOpeningSelectorModule(item)
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func normalizeDirectorModuleID(id string) string {
	return NormalizeStoryDirectorID(id)
}

func validateDirectorModuleID(id, label string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s ID 不能为空", label)
	}
	if id != normalizeDirectorModuleID(id) {
		return fmt.Errorf("%s ID 只能包含小写字母、数字和连字符: %s", label, id)
	}
	return nil
}

func newDirectorModuleID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().Unix())
}

func sortEventPackages(items []EventPackageModule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Custom != items[j].Custom {
			return !items[i].Custom
		}
		return items[i].ID < items[j].ID
	})
}

func sortRuleSystems(items []RuleSystemModule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Custom != items[j].Custom {
			return !items[i].Custom
		}
		if !items[i].Custom {
			leftRank := ruleSystemBuiltinSortRank(items[i].ID)
			rightRank := ruleSystemBuiltinSortRank(items[j].ID)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
		}
		return items[i].ID < items[j].ID
	})
}

func ruleSystemBuiltinSortRank(id string) int {
	id = normalizeDirectorModuleID(id)
	for index, item := range builtinRuleSystemModules() {
		if item.ID == id {
			return index
		}
	}
	return len(builtinRuleSystemModules())
}

func sortActorStates(items []ActorStateModule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Custom != items[j].Custom {
			return !items[i].Custom
		}
		if !items[i].Custom {
			leftRank := actorStateBuiltinSortRank(items[i].ID)
			rightRank := actorStateBuiltinSortRank(items[j].ID)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
		}
		return items[i].ID < items[j].ID
	})
}

func actorStateBuiltinSortRank(id string) int {
	id = normalizeDirectorModuleID(id)
	for index, item := range builtinActorStateModules() {
		if item.ID == id {
			return index
		}
	}
	return len(builtinActorStateModules())
}

func sortOpeningSelectors(items []OpeningSelectorModule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Custom != items[j].Custom {
			return !items[i].Custom
		}
		return items[i].ID < items[j].ID
	})
}

func normalizeEventPackageIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = normalizeDirectorModuleID(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= maxTurnBriefListItems {
			break
		}
	}
	return out
}

func expandLegacyEventPackageRefs(novaDir string, ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range normalizeEventPackageIDs(ids) {
		expanded := eventPackageIDsFromLegacyEventSystemID(novaDir, id)
		if len(expanded) == 0 {
			expanded = []string{id}
		}
		out = append(out, expanded...)
	}
	return normalizeEventPackageIDs(out)
}

func eventPackageIDsFromLegacyEventSystemID(novaDir, id string) []string {
	switch normalizeDirectorModuleID(id) {
	case "":
		return nil
	case DefaultEventSystemID:
		return []string{DefaultEventPackageID}
	case GenreXuanhuanEventSystemID:
		return []string{GenreXuanhuanEventPackageID}
	case GenreXiuxianEventSystemID:
		return []string{GenreXiuxianEventPackageID}
	case GenreApocalypseEventSystemID:
		return []string{GenreApocalypseEventPackageID}
	case GenreWesternEventSystemID:
		return []string{GenreWesternEventPackageID}
	case GenreUrbanEventSystemID:
		return []string{GenreUrbanEventPackageID}
	case GenreTRPGEventSystemID:
		return []string{GenreTRPGEventPackageID}
	}
	if strings.TrimSpace(novaDir) == "" {
		return nil
	}
	item, err := parseEventSystemFile(filepath.Join(novaDir, "story-director-modules", "event-systems", normalizeDirectorModuleID(id)+".json"))
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(item.EventSystem.EventPackages)+1)
	for _, module := range eventPackageModulesFromLegacyEventSystem(item) {
		ids = append(ids, module.ID)
	}
	return normalizeEventPackageIDs(ids)
}

func resolveEventPackages(novaDir string, ids []string) ([]TellerEventPackage, []StoryDirectorModuleWarning) {
	library := NewEventPackageLibrary(novaDir)
	packages := make([]TellerEventPackage, 0, len(ids))
	warnings := []StoryDirectorModuleWarning{}
	for _, id := range normalizeEventPackageIDs(ids) {
		module, err := library.Get(id)
		if err != nil {
			warnings = append(warnings, moduleWarning("event_packages", id, err))
			continue
		}
		packages = append(packages, tellerEventPackageFromModule(module))
	}
	return normalizeTellerEventPackagesNoDefault(packages), warnings
}

func tellerEventPackageFromModule(module EventPackageModule) TellerEventPackage {
	module = normalizeEventPackageModule(module)
	return TellerEventPackage{
		ID:      module.ID,
		Name:    module.Name,
		Enabled: true,
		Events:  module.Events,
	}
}

func eventPackageModulesFromLegacyEventSystem(item EventSystemModule) []EventPackageModule {
	item = normalizeEventSystemModule(item)
	modules := make([]EventPackageModule, 0, len(item.EventSystem.EventPackages)+1)
	for _, pkg := range item.EventSystem.EventPackages {
		module := eventPackageModuleFromTellerPackage(pkg, item)
		if module.ID == "" {
			continue
		}
		modules = append(modules, module)
	}
	if len(item.EventSystem.CustomEvents) > 0 {
		modules = append(modules, eventPackageModuleFromCustomEvents(item))
	}
	return modules
}

func eventPackageModuleFromTellerPackage(pkg TellerEventPackage, source EventSystemModule) EventPackageModule {
	pkg.ID = legacyEventPackageIDForSystemPackage(source.ID, pkg.ID)
	if pkg.ID == "" {
		pkg.ID = normalizeDirectorModuleID(source.ID + "-events")
	}
	return normalizeEventPackageModule(EventPackageModule{
		Version:           storyDirectorModuleVersion,
		ID:                pkg.ID,
		Name:              firstNonEmptyString(pkg.Name, source.Name, pkg.ID),
		Description:       firstNonEmptyString(source.Description, "由旧事件系统迁移生成。"),
		Events:            pkg.Events,
		BuiltinOverridden: source.BuiltinOverridden && IsBuiltinEventPackageID(pkg.ID),
		CreatedAt:         source.CreatedAt,
		UpdatedAt:         source.UpdatedAt,
	})
}

func legacyEventPackageIDForSystemPackage(systemID, packageID string) string {
	systemID = normalizeDirectorModuleID(systemID)
	packageID = normalizeDirectorModuleID(packageID)
	switch systemID {
	case DefaultEventSystemID:
		return DefaultEventPackageID
	case GenreXuanhuanEventSystemID:
		return GenreXuanhuanEventPackageID
	case GenreXiuxianEventSystemID:
		return GenreXiuxianEventPackageID
	case GenreApocalypseEventSystemID:
		return GenreApocalypseEventPackageID
	case GenreWesternEventSystemID:
		return GenreWesternEventPackageID
	case GenreUrbanEventSystemID:
		return GenreUrbanEventPackageID
	case GenreTRPGEventSystemID:
		return GenreTRPGEventPackageID
	default:
		return packageID
	}
}

func eventPackageModuleFromCustomEvents(source EventSystemModule) EventPackageModule {
	id := normalizeDirectorModuleID(source.ID + "-custom-events")
	cards := make([]TellerEventCard, 0, len(source.EventSystem.CustomEvents))
	for i, event := range source.EventSystem.CustomEvents {
		card := eventCardFromDirectorEvent(event, fmt.Sprintf("%s-custom-%d", source.ID, i+1))
		if card.ID != "" {
			cards = append(cards, card)
		}
	}
	return normalizeEventPackageModule(EventPackageModule{
		Version:     storyDirectorModuleVersion,
		ID:          id,
		Name:        firstNonEmptyString(source.Name, source.ID) + " 迁移事件包",
		Description: "由旧事件系统 custom_events 自动迁移生成。",
		Events:      cards,
		CreatedAt:   source.CreatedAt,
		UpdatedAt:   source.UpdatedAt,
	})
}

func eventPackagesFromLegacyEventSystem(system StoryDirectorEventSystem, sourceID string) []TellerEventPackage {
	system.EventPackages = normalizeTellerEventPackagesNoDefault(system.EventPackages)
	system.CustomEvents = normalizeDirectorEvents(system.CustomEvents)
	packages := make([]TellerEventPackage, 0, len(system.EventPackages)+1)
	packages = append(packages, system.EventPackages...)
	if len(system.CustomEvents) > 0 {
		module := eventPackageModuleFromCustomEvents(EventSystemModule{
			ID:          firstNonEmptyString(sourceID, "legacy"),
			Name:        "迁移事件",
			EventSystem: StoryDirectorEventSystem{CustomEvents: system.CustomEvents},
		})
		packages = append(packages, tellerEventPackageFromModule(module))
	}
	return normalizeTellerEventPackagesNoDefault(packages)
}

func eventCardFromDirectorEvent(event DirectorEvent, fallbackID string) TellerEventCard {
	normalized := normalizeDirectorEvents([]DirectorEvent{event})
	if len(normalized) == 0 {
		return TellerEventCard{}
	}
	event = normalized[0]
	description := strings.TrimSpace(event.Template)
	if description == "" {
		description = strings.TrimSpace(firstNonEmptyString(event.Summary, event.PublicSummary, event.Name))
	}
	return TellerEventCard{
		ID:                  firstNonEmptyString(event.ID, normalizeSlotID(fallbackID)),
		TypeName:            firstNonEmptyString(event.Name, event.ID, fallbackID),
		DescriptionMarkdown: description,
		Enabled:             event.Enabled,
		Category:            event.Category,
		Tags:                event.CompatibleGenres,
		Weight:              event.Weight,
		CooldownTurns:       event.CooldownTurns,
		Intensity:           event.Intensity,
	}
}

func storyDirectorHasEmbeddedModules(director StoryDirector) bool {
	return len(director.EventPackages) > 0 ||
		!eventSystemEmpty(director.EventSystem) ||
		!ruleSystemEmpty(director.TRPGSystem) ||
		!actorStateEmpty(director.ActorState) ||
		!openingSelectorEmpty(director.OpeningSelector)
}

func eventSystemEmpty(system StoryDirectorEventSystem) bool {
	return len(system.EventPackages) == 0 && len(system.CustomEvents) == 0
}

func ruleSystemEmpty(trpg StoryDirectorTRPGSystem) bool {
	return len(trpg.RuleTemplates) == 0
}

func openingSelectorEmpty(selector StoryDirectorOpeningSelector) bool {
	return !selector.Enabled && len(selector.TraitPools) == 0 && len(selector.InitialStateOps) == 0
}

func openingSelectorHasContent(selector StoryDirectorOpeningSelector) bool {
	return len(selector.TraitPools) > 0 || len(selector.InitialStateOps) > 0
}
