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
	storyDirectorModuleVersion = 1
	DefaultEventPackageID      = "default"
	DefaultEventSystemID       = "default"
	DefaultRuleSystemID        = "default"
	DefaultOpeningSelectorID   = "default"
)

var (
	ErrEventPackageRevisionConflict    = errors.New("事件包已被其他操作更新，请重新加载后再保存")
	ErrEventSystemRevisionConflict     = errors.New("事件系统已被其他操作更新，请重新加载后再保存")
	ErrRuleSystemRevisionConflict      = errors.New("数值规则系统已被其他操作更新，请重新加载后再保存")
	ErrOpeningSelectorRevisionConflict = errors.New("开局选择器已被其他操作更新，请重新加载后再保存")
	ErrActorStateRevisionConflict      = errors.New("Actor 状态系统已被其他操作更新，请重新加载后再保存")
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
	StatSystem            StoryDirectorStatSystem       `json:"stat_system,omitempty"`
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
	Tags              []string          `json:"tags"`
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
	StatSystem        StoryDirectorStatSystem `json:"stat_system"`
	TRPGSystem        StoryDirectorTRPGSystem `json:"trpg_system"`
	Tags              []string                `json:"tags"`
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
	Tags              []string                      `json:"tags"`
	Path              string                        `json:"path,omitempty"`
	Custom            bool                          `json:"custom"`
	BuiltinOverridden bool                          `json:"builtin_overridden,omitempty"`
	Invalid           bool                          `json:"invalid,omitempty"`
	Error             string                        `json:"error,omitempty"`
	CreatedAt         string                        `json:"created_at,omitempty"`
	UpdatedAt         string                        `json:"updated_at,omitempty"`
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

type EventSystemLibrary struct {
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

func NewEventSystemLibrary(novaDir string) *EventSystemLibrary {
	return &EventSystemLibrary{novaDir: novaDir}
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

func (l *EventSystemLibrary) List() ([]EventSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]EventSystemModule, 0, len(files))
	for _, file := range files {
		item, err := parseEventSystemFile(file)
		if err != nil {
			items = append(items, EventSystemModule{ID: strings.TrimSuffix(filepath.Base(file), ".json"), Path: file, Invalid: true, Error: err.Error(), Custom: !IsBuiltinEventSystemID(strings.TrimSuffix(filepath.Base(file), ".json"))})
			continue
		}
		item.Path = file
		item = applyEventSystemOwnership(item)
		items = append(items, item)
	}
	sortEventSystems(items)
	return items, nil
}

func (l *EventSystemLibrary) Get(id string) (EventSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventSystemModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if id == "" {
		id = DefaultEventSystemID
	}
	if err := validateDirectorModuleID(id, "事件系统"); err != nil {
		return EventSystemModule{}, err
	}
	item, err := parseEventSystemFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return EventSystemModule{}, err
	}
	return applyEventSystemOwnership(item), nil
}

func (l *EventSystemLibrary) Create(item EventSystemModule) (EventSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventSystemModule{}, err
	}
	item = normalizeEventSystemModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("event-system")
	}
	item.BuiltinOverridden = false
	if err := validateEventSystemModule(item); err != nil {
		return EventSystemModule{}, err
	}
	path := filepath.Join(l.dir(), item.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return EventSystemModule{}, fmt.Errorf("事件系统已存在: %s", item.ID)
	} else if !os.IsNotExist(err) {
		return EventSystemModule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := writeEventSystemFile(path, item); err != nil {
		return EventSystemModule{}, err
	}
	item.Path = path
	return applyEventSystemOwnership(item), nil
}

func (l *EventSystemLibrary) Update(id string, item EventSystemModule, baseRevision string) (EventSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventSystemModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "事件系统"); err != nil {
		return EventSystemModule{}, err
	}
	isBuiltin := IsBuiltinEventSystemID(id)
	current, err := l.Get(id)
	if err != nil {
		return EventSystemModule{}, err
	}
	if strings.TrimSpace(baseRevision) != "" && strings.TrimSpace(current.UpdatedAt) != strings.TrimSpace(baseRevision) {
		return EventSystemModule{}, ErrEventSystemRevisionConflict
	}
	item = normalizeEventSystemModule(item)
	item.ID = id
	item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	item.BuiltinOverridden = isBuiltin
	if err := validateEventSystemModule(item); err != nil {
		return EventSystemModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeEventSystemFile(path, item); err != nil {
		return EventSystemModule{}, err
	}
	item.Path = path
	return applyEventSystemOwnership(item), nil
}

func (l *EventSystemLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "事件系统"); err != nil {
		return err
	}
	if IsBuiltinEventSystemID(id) {
		item, ok := builtinEventSystemModuleByID(id)
		if !ok {
			return fmt.Errorf("内置事件系统不存在: %s", id)
		}
		return writeEventSystemFile(filepath.Join(l.dir(), id+".json"), item)
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *EventSystemLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-director-modules", "event-systems")
}

func (l *EventSystemLibrary) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	for _, item := range builtinEventSystemModules() {
		path := filepath.Join(l.dir(), item.ID+".json")
		if current, err := parseEventSystemFile(path); err == nil && current.BuiltinOverridden {
			continue
		} else if err == nil && current.Version == item.Version {
			continue
		}
		if err := writeEventSystemFile(path, item); err != nil {
			return err
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
	if err := validateDirectorModuleID(id, "数值规则系统"); err != nil {
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
		return RuleSystemModule{}, fmt.Errorf("数值规则系统已存在: %s", item.ID)
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
	if err := validateDirectorModuleID(id, "数值规则系统"); err != nil {
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
	if err := validateDirectorModuleID(id, "数值规则系统"); err != nil {
		return err
	}
	if IsBuiltinRuleSystemID(id) {
		return writeRuleSystemFile(filepath.Join(l.dir(), id+".json"), DefaultRuleSystemModule())
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
	path := filepath.Join(l.dir(), DefaultRuleSystemID+".json")
	if current, err := parseRuleSystemFile(path); err == nil && current.BuiltinOverridden {
		return nil
	} else if err == nil && current.Version == storyDirectorModuleVersion {
		return nil
	}
	return writeRuleSystemFile(path, DefaultRuleSystemModule())
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
		OpeningSelectorID: DefaultOpeningSelectorID,
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
		OpeningSelectorID:       normalizeDirectorModuleID(refs.OpeningSelectorID),
		OpeningSelectorDisabled: refs.OpeningSelectorDisabled,
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
		refs.OpeningSelectorID == "" &&
		refs.ImagePresetID == "" &&
		!refs.NarrativeStyleDisabled &&
		!refs.EventPackagesDisabled &&
		!refs.RuleSystemDisabled &&
		!refs.ActorStateDisabled &&
		!refs.MemoryStructureDisabled &&
		!refs.OpeningSelectorDisabled &&
		!refs.ImagePresetDisabled
}

func StoryDirectorNarrativeStyleEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).NarrativeStyleDisabled
}

func StoryDirectorEventSystemEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).EventPackagesDisabled
}

func StoryDirectorRuleSystemEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).RuleSystemDisabled
}

func StoryDirectorActorStateEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).ActorStateDisabled
}

func StoryDirectorMemoryStructureEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).MemoryStructureDisabled
}

func StoryDirectorOpeningSelectorEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).OpeningSelectorDisabled
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
		effective.StatSystem = StoryDirectorStatSystem{Attributes: []StoryDirectorAttribute{}}
		effective.TRPGSystem = StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{}}
	} else if refs.RuleSystemID != "" {
		if module, err := NewRuleSystemLibrary(novaDir).Get(refs.RuleSystemID); err == nil {
			effective.StatSystem = module.StatSystem
			effective.TRPGSystem = module.TRPGSystem
		} else if !ruleSystemEmpty(snapshot.StatSystem, snapshot.TRPGSystem) {
			effective.StatSystem = snapshot.StatSystem
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
	if refs.OpeningSelectorDisabled {
		effective.OpeningSelector = StoryDirectorOpeningSelector{Enabled: false, TraitPools: []OpeningTraitPool{}, InitialStateOps: []StateOp{}}
	} else if refs.OpeningSelectorID != "" {
		if module, err := NewOpeningSelectorLibrary(novaDir).Get(refs.OpeningSelectorID); err == nil {
			effective.OpeningSelector = module.OpeningSelector
		} else if !openingSelectorEmpty(snapshot.OpeningSelector) {
			effective.OpeningSelector = snapshot.OpeningSelector
			warnings = append(warnings, moduleWarning("opening_selector", refs.OpeningSelectorID, err))
		} else {
			warnings = append(warnings, moduleWarning("opening_selector", refs.OpeningSelectorID, err))
		}
	}
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
		StatSystem:            director.StatSystem,
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
		Tags:        []string{"内置", "事件"},
	})
}

func DefaultEventSystemModule() EventSystemModule {
	config := DefaultTellerOrchestrationConfig()
	return normalizeEventSystemModule(EventSystemModule{
		Version:     storyDirectorModuleVersion,
		ID:          DefaultEventSystemID,
		Name:        "默认事件系统",
		Description: "通用爽文与互动叙事事件包，覆盖打脸、奇遇、冲突、恋爱、伏笔回收等基础事件。",
		EventSystem: StoryDirectorEventSystem{EventPackages: config.EventPackages, CustomEvents: config.CustomEvents},
		Tags:        []string{"内置", "事件"},
	})
}

func DefaultRuleSystemModule() RuleSystemModule {
	config := DefaultTellerOrchestrationConfig()
	return normalizeRuleSystemModule(RuleSystemModule{
		Version:     storyDirectorModuleVersion,
		ID:          DefaultRuleSystemID,
		Name:        "默认数值与TRPG系统",
		Description: "提供生命、体力、好感等基础属性和可扩展 TRPG 检定模板。",
		StatSystem:  StoryDirectorStatSystem{Attributes: defaultStoryDirectorAttributes()},
		TRPGSystem:  StoryDirectorTRPGSystem{RuleTemplates: config.RuleTemplates},
		Tags:        []string{"内置", "规则"},
	})
}

func DefaultActorStateModule() ActorStateModule {
	return normalizeActorStateModule(ActorStateModule{
		Version:     storyDirectorModuleVersion,
		ID:          DefaultActorStateModuleID,
		Name:        "默认 Actor 状态系统",
		Description: "以主角等关键 Actor 为中心维护结构化状态，供规则检定、资源消耗和长期承接读取。",
		ActorState:  defaultActorStateSystem(),
		Tags:        []string{"内置", "状态"},
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

func IsBuiltinEventSystemID(id string) bool {
	switch normalizeDirectorModuleID(id) {
	case DefaultEventSystemID,
		GenreXuanhuanEventSystemID,
		GenreXiuxianEventSystemID,
		GenreApocalypseEventSystemID,
		GenreWesternEventSystemID,
		GenreUrbanEventSystemID,
		GenreTRPGEventSystemID:
		return true
	default:
		return false
	}
}

func IsBuiltinRuleSystemID(id string) bool {
	return normalizeDirectorModuleID(id) == DefaultRuleSystemID
}

func IsBuiltinActorStateID(id string) bool {
	return normalizeDirectorModuleID(id) == DefaultActorStateModuleID
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

func builtinEventSystemModuleByID(id string) (EventSystemModule, bool) {
	id = normalizeDirectorModuleID(id)
	for _, item := range builtinEventSystemModules() {
		if item.ID == id {
			return item, true
		}
	}
	return EventSystemModule{}, false
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

func applyEventSystemOwnership(item EventSystemModule) EventSystemModule {
	if !IsBuiltinEventSystemID(item.ID) {
		item.Custom = true
		item.BuiltinOverridden = false
		return item
	}
	item.Custom = false
	item.BuiltinOverridden = item.BuiltinOverridden || eventSystemDiffersFromBuiltin(item)
	return item
}

func eventSystemDiffersFromBuiltin(item EventSystemModule) bool {
	builtin, ok := builtinEventSystemModuleByID(item.ID)
	if !ok {
		return false
	}
	return !reflect.DeepEqual(eventSystemComparable(item), eventSystemComparable(builtin))
}

func eventSystemComparable(item EventSystemModule) EventSystemModule {
	item = normalizeEventSystemModule(item)
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
	return !reflect.DeepEqual(ruleSystemComparable(item), ruleSystemComparable(DefaultRuleSystemModule()))
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
	return !reflect.DeepEqual(actorStateComparable(item), actorStateComparable(DefaultActorStateModule()))
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
	item.Tags = normalizeStringListLimit(item.Tags, maxTurnBriefListItems)
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
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "数值规则系统"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.StatSystem.Attributes = normalizeStoryDirectorAttributes(item.StatSystem.Attributes)
	item.TRPGSystem.RuleTemplates = normalizeRuleChecks(item.TRPGSystem.RuleTemplates)
	item.Tags = normalizeStringListLimit(item.Tags, maxTurnBriefListItems)
	return item
}

func normalizeActorStateModule(item ActorStateModule) ActorStateModule {
	item.Version = storyDirectorModuleVersion
	item.ID = normalizeDirectorModuleID(item.ID)
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "Actor 状态系统"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.ActorState = normalizeActorStateSystem(item.ActorState)
	item.Tags = normalizeStringListLimit(item.Tags, maxTurnBriefListItems)
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
	if snapshot.ModuleRefs.RuleSystemDisabled {
		snapshot.StatSystem.Attributes = normalizeStoryDirectorAttributesNoDefault(snapshot.StatSystem.Attributes)
	} else {
		snapshot.StatSystem.Attributes = normalizeStoryDirectorAttributes(snapshot.StatSystem.Attributes)
	}
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
	if err := validateDirectorModuleID(item.ID, "数值规则系统"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("数值规则系统名称不能为空")
	}
	for _, check := range item.TRPGSystem.RuleTemplates {
		if err := validateRuleCheck(check); err != nil {
			return err
		}
	}
	return nil
}

func validateActorStateModule(item ActorStateModule) error {
	if err := validateDirectorModuleID(item.ID, "Actor 状态系统"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("Actor 状态系统名称不能为空")
	}
	if len(item.ActorState.Templates) == 0 {
		return errors.New("Actor 状态系统至少需要一个 actor 类型模板")
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
	for _, op := range item.OpeningSelector.InitialStateOps {
		if err := validateStateOp(op); err != nil {
			return err
		}
	}
	for _, pool := range item.OpeningSelector.TraitPools {
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
		return RuleSystemModule{}, fmt.Errorf("解析数值规则系统 JSON 失败: %w", err)
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
		return ActorStateModule{}, fmt.Errorf("解析 Actor 状态系统 JSON 失败: %w", err)
	}
	item = normalizeActorStateModule(item)
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

func writeEventSystemFile(path string, item EventSystemModule) error {
	item = normalizeEventSystemModule(item)
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
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
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

func sortEventSystems(items []EventSystemModule) {
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
		return items[i].ID < items[j].ID
	})
}

func sortActorStates(items []ActorStateModule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Custom != items[j].Custom {
			return !items[i].Custom
		}
		return items[i].ID < items[j].ID
	})
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
		Tags:              source.Tags,
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
		Tags:        append(normalizeStringListLimit(source.Tags, maxTurnBriefListItems), "迁移"),
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
		!ruleSystemEmpty(director.StatSystem, director.TRPGSystem) ||
		!actorStateEmpty(director.ActorState) ||
		!openingSelectorEmpty(director.OpeningSelector)
}

func eventSystemEmpty(system StoryDirectorEventSystem) bool {
	return len(system.EventPackages) == 0 && len(system.CustomEvents) == 0
}

func ruleSystemEmpty(stat StoryDirectorStatSystem, trpg StoryDirectorTRPGSystem) bool {
	return len(stat.Attributes) == 0 && len(trpg.RuleTemplates) == 0
}

func openingSelectorEmpty(selector StoryDirectorOpeningSelector) bool {
	return !selector.Enabled && len(selector.TraitPools) == 0 && len(selector.InitialStateOps) == 0
}
