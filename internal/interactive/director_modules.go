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

	"denova/internal/imagepreset"
)

const (
	storyDirectorModuleVersion = 1
	DefaultEventSystemID       = "default"
	DefaultRuleSystemID        = "default"
	DefaultOpeningSelectorID   = "default"
)

var (
	ErrEventSystemRevisionConflict     = errors.New("事件系统已被其他操作更新，请重新加载后再保存")
	ErrRuleSystemRevisionConflict      = errors.New("数值规则系统已被其他操作更新，请重新加载后再保存")
	ErrOpeningSelectorRevisionConflict = errors.New("开局选择器已被其他操作更新，请重新加载后再保存")
)

// StoryDirectorModuleRefs declares the reusable resources a story director
// combines at runtime. Changing a referenced module affects future resolution.
type StoryDirectorModuleRefs struct {
	NarrativeStyleID        string `json:"narrative_style_id,omitempty"`
	NarrativeStyleDisabled  bool   `json:"narrative_style_disabled,omitempty"`
	EventSystemID           string `json:"event_system_id,omitempty"`
	EventSystemDisabled     bool   `json:"event_system_disabled,omitempty"`
	RuleSystemID            string `json:"rule_system_id,omitempty"`
	RuleSystemDisabled      bool   `json:"rule_system_disabled,omitempty"`
	OpeningSelectorID       string `json:"opening_selector_id,omitempty"`
	OpeningSelectorDisabled bool   `json:"opening_selector_disabled,omitempty"`
	ImagePresetID           string `json:"image_preset_id,omitempty"`
	ImagePresetDisabled     bool   `json:"image_preset_disabled,omitempty"`
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
	Version          int                          `json:"version"`
	ResolvedAt       string                       `json:"resolved_at,omitempty"`
	Status           string                       `json:"status,omitempty"`
	Warnings         []StoryDirectorModuleWarning `json:"warnings,omitempty"`
	ModuleRefs       StoryDirectorModuleRefs      `json:"module_refs"`
	NarrativeStyleID string                       `json:"narrative_style_id,omitempty"`
	ImagePresetID    string                       `json:"image_preset_id,omitempty"`
	EventSystem      StoryDirectorEventSystem     `json:"event_system,omitempty"`
	StatSystem       StoryDirectorStatSystem      `json:"stat_system,omitempty"`
	TRPGSystem       StoryDirectorTRPGSystem      `json:"trpg_system,omitempty"`
	OpeningSelector  StoryDirectorOpeningSelector `json:"opening_selector,omitempty"`
}

type EventSystemModule struct {
	Version     int                      `json:"version"`
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	EventSystem StoryDirectorEventSystem `json:"event_system"`
	Tags        []string                 `json:"tags"`
	Path        string                   `json:"path,omitempty"`
	Custom      bool                     `json:"custom"`
	Invalid     bool                     `json:"invalid,omitempty"`
	Error       string                   `json:"error,omitempty"`
	CreatedAt   string                   `json:"created_at,omitempty"`
	UpdatedAt   string                   `json:"updated_at,omitempty"`
}

type RuleSystemModule struct {
	Version     int                     `json:"version"`
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	StatSystem  StoryDirectorStatSystem `json:"stat_system"`
	TRPGSystem  StoryDirectorTRPGSystem `json:"trpg_system"`
	Tags        []string                `json:"tags"`
	Path        string                  `json:"path,omitempty"`
	Custom      bool                    `json:"custom"`
	Invalid     bool                    `json:"invalid,omitempty"`
	Error       string                  `json:"error,omitempty"`
	CreatedAt   string                  `json:"created_at,omitempty"`
	UpdatedAt   string                  `json:"updated_at,omitempty"`
}

type OpeningSelectorModule struct {
	Version         int                          `json:"version"`
	ID              string                       `json:"id"`
	Name            string                       `json:"name"`
	Description     string                       `json:"description"`
	OpeningSelector StoryDirectorOpeningSelector `json:"opening_selector"`
	Tags            []string                     `json:"tags"`
	Path            string                       `json:"path,omitempty"`
	Custom          bool                         `json:"custom"`
	Invalid         bool                         `json:"invalid,omitempty"`
	Error           string                       `json:"error,omitempty"`
	CreatedAt       string                       `json:"created_at,omitempty"`
	UpdatedAt       string                       `json:"updated_at,omitempty"`
}

type EventSystemLibrary struct {
	novaDir string
}

type RuleSystemLibrary struct {
	novaDir string
}

type OpeningSelectorLibrary struct {
	novaDir string
}

func NewEventSystemLibrary(novaDir string) *EventSystemLibrary {
	return &EventSystemLibrary{novaDir: novaDir}
}

func NewRuleSystemLibrary(novaDir string) *RuleSystemLibrary {
	return &RuleSystemLibrary{novaDir: novaDir}
}

func NewOpeningSelectorLibrary(novaDir string) *OpeningSelectorLibrary {
	return &OpeningSelectorLibrary{novaDir: novaDir}
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
		item.Custom = !IsBuiltinEventSystemID(item.ID)
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
	item.Custom = !IsBuiltinEventSystemID(item.ID)
	return item, nil
}

func (l *EventSystemLibrary) Create(item EventSystemModule) (EventSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventSystemModule{}, err
	}
	item = normalizeEventSystemModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("event-system")
	}
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
	item.Custom = !IsBuiltinEventSystemID(item.ID)
	return item, nil
}

func (l *EventSystemLibrary) Update(id string, item EventSystemModule, baseRevision string) (EventSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return EventSystemModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "事件系统"); err != nil {
		return EventSystemModule{}, err
	}
	if IsBuiltinEventSystemID(id) {
		return EventSystemModule{}, errors.New("内置事件系统不能修改，请复制后编辑")
	}
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
	if err := validateEventSystemModule(item); err != nil {
		return EventSystemModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeEventSystemFile(path, item); err != nil {
		return EventSystemModule{}, err
	}
	item.Path = path
	item.Custom = true
	return item, nil
}

func (l *EventSystemLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "事件系统"); err != nil {
		return err
	}
	if IsBuiltinEventSystemID(id) {
		return errors.New("内置事件系统不能删除")
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
		if current, err := parseEventSystemFile(path); err == nil && current.Version == item.Version {
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
		item.Custom = !IsBuiltinRuleSystemID(item.ID)
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
	item.Custom = !IsBuiltinRuleSystemID(item.ID)
	return item, nil
}

func (l *RuleSystemLibrary) Create(item RuleSystemModule) (RuleSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return RuleSystemModule{}, err
	}
	item = normalizeRuleSystemModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("rule-system")
	}
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
	item.Custom = !IsBuiltinRuleSystemID(item.ID)
	return item, nil
}

func (l *RuleSystemLibrary) Update(id string, item RuleSystemModule, baseRevision string) (RuleSystemModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return RuleSystemModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "数值规则系统"); err != nil {
		return RuleSystemModule{}, err
	}
	if IsBuiltinRuleSystemID(id) {
		return RuleSystemModule{}, errors.New("内置数值规则系统不能修改，请复制后编辑")
	}
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
	if err := validateRuleSystemModule(item); err != nil {
		return RuleSystemModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeRuleSystemFile(path, item); err != nil {
		return RuleSystemModule{}, err
	}
	item.Path = path
	item.Custom = true
	return item, nil
}

func (l *RuleSystemLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "数值规则系统"); err != nil {
		return err
	}
	if IsBuiltinRuleSystemID(id) {
		return errors.New("内置数值规则系统不能删除")
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
	if current, err := parseRuleSystemFile(path); err == nil && current.Version == storyDirectorModuleVersion {
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
		item.Custom = !IsBuiltinOpeningSelectorID(item.ID)
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
	item.Custom = !IsBuiltinOpeningSelectorID(item.ID)
	return item, nil
}

func (l *OpeningSelectorLibrary) Create(item OpeningSelectorModule) (OpeningSelectorModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return OpeningSelectorModule{}, err
	}
	item = normalizeOpeningSelectorModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("opening-selector")
	}
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
	item.Custom = !IsBuiltinOpeningSelectorID(item.ID)
	return item, nil
}

func (l *OpeningSelectorLibrary) Update(id string, item OpeningSelectorModule, baseRevision string) (OpeningSelectorModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return OpeningSelectorModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "开局选择器"); err != nil {
		return OpeningSelectorModule{}, err
	}
	if IsBuiltinOpeningSelectorID(id) {
		return OpeningSelectorModule{}, errors.New("内置开局选择器不能修改，请复制后编辑")
	}
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
	if err := validateOpeningSelectorModule(item); err != nil {
		return OpeningSelectorModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeOpeningSelectorFile(path, item); err != nil {
		return OpeningSelectorModule{}, err
	}
	item.Path = path
	item.Custom = true
	return item, nil
}

func (l *OpeningSelectorLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "开局选择器"); err != nil {
		return err
	}
	if IsBuiltinOpeningSelectorID(id) {
		return errors.New("内置开局选择器不能删除")
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
	if current, err := parseOpeningSelectorFile(path); err == nil && current.Version == storyDirectorModuleVersion {
		return nil
	}
	return writeOpeningSelectorFile(path, DefaultOpeningSelectorModule())
}

func DefaultStoryDirectorModuleRefs() StoryDirectorModuleRefs {
	return StoryDirectorModuleRefs{
		NarrativeStyleID:  "classic",
		EventSystemID:     DefaultEventSystemID,
		RuleSystemID:      DefaultRuleSystemID,
		OpeningSelectorID: DefaultOpeningSelectorID,
		ImagePresetID:     imagepreset.DefaultID,
	}
}

func NormalizeStoryDirectorModuleRefs(refs StoryDirectorModuleRefs) StoryDirectorModuleRefs {
	return StoryDirectorModuleRefs{
		NarrativeStyleID:        strings.TrimSpace(refs.NarrativeStyleID),
		NarrativeStyleDisabled:  refs.NarrativeStyleDisabled,
		EventSystemID:           normalizeDirectorModuleID(refs.EventSystemID),
		EventSystemDisabled:     refs.EventSystemDisabled,
		RuleSystemID:            normalizeDirectorModuleID(refs.RuleSystemID),
		RuleSystemDisabled:      refs.RuleSystemDisabled,
		OpeningSelectorID:       normalizeDirectorModuleID(refs.OpeningSelectorID),
		OpeningSelectorDisabled: refs.OpeningSelectorDisabled,
		ImagePresetID:           imagepreset.NormalizeID(refs.ImagePresetID),
		ImagePresetDisabled:     refs.ImagePresetDisabled,
	}
}

func StoryDirectorModuleRefsEmpty(refs StoryDirectorModuleRefs) bool {
	refs = NormalizeStoryDirectorModuleRefs(refs)
	return refs.NarrativeStyleID == "" &&
		refs.EventSystemID == "" &&
		refs.RuleSystemID == "" &&
		refs.OpeningSelectorID == "" &&
		refs.ImagePresetID == "" &&
		!refs.NarrativeStyleDisabled &&
		!refs.EventSystemDisabled &&
		!refs.RuleSystemDisabled &&
		!refs.OpeningSelectorDisabled &&
		!refs.ImagePresetDisabled
}

func StoryDirectorNarrativeStyleEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).NarrativeStyleDisabled
}

func StoryDirectorEventSystemEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).EventSystemDisabled
}

func StoryDirectorRuleSystemEnabled(director StoryDirector) bool {
	return !NormalizeStoryDirectorModuleRefs(director.ModuleRefs).RuleSystemDisabled
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
	if StoryDirectorModuleRefsEmpty(refs) {
		if storyDirectorHasEmbeddedModules(director) {
			director.ResolvedSnapshot = snapshotFromEffectiveDirector(director, refs, nil)
			return director
		}
		refs = DefaultStoryDirectorModuleRefs()
		director.ModuleRefs = refs
	}

	warnings := []StoryDirectorModuleWarning{}
	snapshot := normalizeStoryDirectorResolvedSnapshot(director.ResolvedSnapshot)
	effective := director
	effective.ModuleRefs = refs

	if refs.EventSystemDisabled {
		effective.EventSystem = StoryDirectorEventSystem{EventPackages: []TellerEventPackage{}, CustomEvents: []DirectorEvent{}}
	} else if refs.EventSystemID != "" {
		if module, err := NewEventSystemLibrary(novaDir).Get(refs.EventSystemID); err == nil {
			effective.EventSystem = module.EventSystem
		} else if !eventSystemEmpty(snapshot.EventSystem) {
			effective.EventSystem = snapshot.EventSystem
			warnings = append(warnings, moduleWarning("event_system", refs.EventSystemID, err))
		} else {
			warnings = append(warnings, moduleWarning("event_system", refs.EventSystemID, err))
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
	effective.ResolvedSnapshot = snapshotFromEffectiveDirector(effective, refs, warnings)
	return normalizeStoryDirector(effective)
}

func snapshotFromEffectiveDirector(director StoryDirector, refs StoryDirectorModuleRefs, warnings []StoryDirectorModuleWarning) StoryDirectorResolvedSnapshot {
	status := "ready"
	if len(warnings) > 0 {
		status = "warning"
	}
	return normalizeStoryDirectorResolvedSnapshot(StoryDirectorResolvedSnapshot{
		Version:          storyDirectorModuleVersion,
		ResolvedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Status:           status,
		Warnings:         warnings,
		ModuleRefs:       refs,
		NarrativeStyleID: refs.NarrativeStyleID,
		ImagePresetID:    refs.ImagePresetID,
		EventSystem:      director.EventSystem,
		StatSystem:       director.StatSystem,
		TRPGSystem:       director.TRPGSystem,
		OpeningSelector:  director.OpeningSelector,
	})
}

func moduleWarning(module, id string, err error) StoryDirectorModuleWarning {
	message := "模块不可用，已尝试使用最近可用快照。"
	if err != nil {
		message = err.Error()
	}
	return StoryDirectorModuleWarning{Module: module, ID: id, Message: trimBytes(message, 512)}
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

func IsBuiltinOpeningSelectorID(id string) bool {
	return normalizeDirectorModuleID(id) == DefaultOpeningSelectorID
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
	if snapshot.ModuleRefs.EventSystemDisabled {
		snapshot.EventSystem.EventPackages = normalizeTellerEventPackagesNoDefault(snapshot.EventSystem.EventPackages)
	} else {
		snapshot.EventSystem.EventPackages = normalizeTellerEventPackages(snapshot.EventSystem.EventPackages)
	}
	snapshot.EventSystem.CustomEvents = normalizeDirectorEvents(snapshot.EventSystem.CustomEvents)
	if snapshot.ModuleRefs.RuleSystemDisabled {
		snapshot.StatSystem.Attributes = normalizeStoryDirectorAttributesNoDefault(snapshot.StatSystem.Attributes)
	} else {
		snapshot.StatSystem.Attributes = normalizeStoryDirectorAttributes(snapshot.StatSystem.Attributes)
	}
	snapshot.TRPGSystem.RuleTemplates = normalizeRuleChecks(snapshot.TRPGSystem.RuleTemplates)
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

func sortOpeningSelectors(items []OpeningSelectorModule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Custom != items[j].Custom {
			return !items[i].Custom
		}
		return items[i].ID < items[j].ID
	})
}

func storyDirectorHasEmbeddedModules(director StoryDirector) bool {
	return !eventSystemEmpty(director.EventSystem) ||
		!ruleSystemEmpty(director.StatSystem, director.TRPGSystem) ||
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
