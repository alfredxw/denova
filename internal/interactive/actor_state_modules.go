package interactive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func attachBuiltinActorStateLegacyPaths(id string, system StoryDirectorActorStateSystem) StoryDirectorActorStateSystem {
	builtin, ok := builtinActorStateModuleByID(id)
	if !ok {
		return system
	}
	legacyByTemplateAndName := map[string]map[string]string{}
	for _, template := range builtin.ActorState.Templates {
		legacyByTemplateAndName[template.ID] = map[string]string{}
		for _, field := range template.Fields {
			legacyByTemplateAndName[template.ID][actorStateFieldNameKey(field.Name)] = field.LegacyPath
		}
	}
	for templateIndex := range system.Templates {
		template := &system.Templates[templateIndex]
		for fieldIndex := range template.Fields {
			field := &template.Fields[fieldIndex]
			if legacyPath := legacyByTemplateAndName[template.ID][actorStateFieldNameKey(field.Name)]; legacyPath != "" {
				field.LegacyPath = legacyPath
				field.Path = legacyPath
			}
		}
	}
	return system
}

func (l *ActorStateLibrary) List() ([]ActorStateModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]ActorStateModule, 0, len(files))
	for _, file := range files {
		item, err := parseActorStateFile(file)
		if err != nil {
			items = append(items, ActorStateModule{ID: strings.TrimSuffix(filepath.Base(file), ".json"), Path: file, Invalid: true, Error: err.Error(), Custom: !IsBuiltinActorStateID(strings.TrimSuffix(filepath.Base(file), ".json"))})
			continue
		}
		item.Path = file
		item = applyActorStateOwnership(item)
		items = append(items, item)
	}
	sortActorStates(items)
	return items, nil
}

func (l *ActorStateLibrary) Get(id string) (ActorStateModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return ActorStateModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if id == "" {
		id = DefaultActorStateModuleID
	}
	if err := validateDirectorModuleID(id, "状态系统"); err != nil {
		return ActorStateModule{}, err
	}
	item, err := parseActorStateFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return ActorStateModule{}, err
	}
	return applyActorStateOwnership(item), nil
}

func (l *ActorStateLibrary) Create(item ActorStateModule) (ActorStateModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return ActorStateModule{}, err
	}
	item = normalizeActorStateModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("actor-state")
	}
	item.BuiltinOverridden = false
	if err := validateActorStateModule(item); err != nil {
		return ActorStateModule{}, err
	}
	path := filepath.Join(l.dir(), item.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return ActorStateModule{}, fmt.Errorf("状态系统已存在: %s", item.ID)
	} else if !os.IsNotExist(err) {
		return ActorStateModule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := writeActorStateFile(path, item); err != nil {
		return ActorStateModule{}, err
	}
	item.Path = path
	return applyActorStateOwnership(item), nil
}

func (l *ActorStateLibrary) Update(id string, item ActorStateModule, baseRevision string) (ActorStateModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return ActorStateModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "状态系统"); err != nil {
		return ActorStateModule{}, err
	}
	isBuiltin := IsBuiltinActorStateID(id)
	current, err := l.Get(id)
	if err != nil {
		return ActorStateModule{}, err
	}
	if strings.TrimSpace(baseRevision) != "" && strings.TrimSpace(current.UpdatedAt) != strings.TrimSpace(baseRevision) {
		return ActorStateModule{}, ErrActorStateRevisionConflict
	}
	item = normalizeActorStateModule(item)
	item.ID = id
	item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	item.BuiltinOverridden = isBuiltin
	if err := validateActorStateModule(item); err != nil {
		return ActorStateModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if current.NeedsMigration && !isBuiltin {
		if err := l.backupActorStateBeforeMigration(path); err != nil {
			return ActorStateModule{}, fmt.Errorf("备份旧状态系统失败: %w", err)
		}
	}
	if err := writeActorStateFile(path, item); err != nil {
		return ActorStateModule{}, err
	}
	item.Path = path
	return applyActorStateOwnership(item), nil
}

func (l *ActorStateLibrary) backupActorStateBeforeMigration(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	backupDir := filepath.Join(l.novaDir, "backups", "state-system-v6", timestamp)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(backupDir, filepath.Base(path)), data, 0o644)
}

func (l *ActorStateLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "状态系统"); err != nil {
		return err
	}
	if IsBuiltinActorStateID(id) {
		item, ok := builtinActorStateModuleByID(id)
		if !ok {
			return fmt.Errorf("内置状态系统不存在: %s", id)
		}
		return writeActorStateFile(filepath.Join(l.dir(), id+".json"), item)
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *ActorStateLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-director-modules", "actor-states")
}

func (l *ActorStateLibrary) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	for _, builtin := range builtinActorStateModules() {
		path := filepath.Join(l.dir(), builtin.ID+".json")
		if current, err := parseActorStateFile(path); err == nil && current.BuiltinOverridden {
			continue
		} else if err == nil && current.ID == builtin.ID && current.Version == storyDirectorModuleVersion && !actorStateDiffersFromBuiltin(current) {
			continue
		}
		if err := writeActorStateFile(path, builtin); err != nil {
			return err
		}
	}
	return nil
}
