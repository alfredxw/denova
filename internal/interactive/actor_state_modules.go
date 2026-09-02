package interactive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"denova/internal/presetlayout"
	"denova/internal/revisionfile"
	"denova/internal/revisionjson"
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
	return listDirectorModuleFiles(l.dir(), l.ensureBuiltins, parseActorStateFile,
		func(id, path string, err error) ActorStateModule {
			return ActorStateModule{ID: id, Path: path, Invalid: true, Error: err.Error(), Custom: !IsBuiltinActorStateID(id)}
		}, applyActorStateOwnership, sortActorStates)
}

func (l *ActorStateLibrary) Get(id string) (ActorStateModule, error) {
	id = normalizeDirectorModuleID(id)
	if id == "" {
		id = DefaultActorStateModuleID
	}
	if err := validateDirectorModuleID(id, "状态系统"); err != nil {
		return ActorStateModule{}, err
	}
	return getDirectorModuleFile(l.dir(), id, l.ensureBuiltins, parseActorStateFile, applyActorStateOwnership)
}

func (l *ActorStateLibrary) Create(item ActorStateModule) (ActorStateModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return ActorStateModule{}, err
	}
	if err := validateDirectorModuleWriteBounds(item.Name, item.Description); err != nil {
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = now
	item.UpdatedAt = now
	document, err := actorStateFileStore(path).Create(context.Background(), item)
	if errors.Is(err, revisionjson.ErrAlreadyExists) {
		return ActorStateModule{}, fmt.Errorf("状态系统已存在: %s", item.ID)
	}
	if err != nil {
		return ActorStateModule{}, err
	}
	item = document.Value
	item.Path, item.Revision = path, document.Revision
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
	if err := validateDirectorModuleWriteBounds(item.Name, item.Description); err != nil {
		return ActorStateModule{}, err
	}
	item = normalizeActorStateModule(item)
	item.ID = id
	if err := validateActorStateModule(item); err != nil {
		return ActorStateModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	document, err := actorStateFileStore(path).Update(context.Background(), baseRevision, func(current ActorStateModule) (ActorStateModule, error) {
		item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
		item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		item.BuiltinOverridden = IsBuiltinActorStateID(id)
		return item, validateActorStateModule(item)
	})
	if err != nil {
		if errors.Is(err, revisionfile.ErrRevisionConflict) || errors.Is(err, revisionjson.ErrRevisionRequired) {
			return ActorStateModule{}, fmt.Errorf("%w: %v", ErrActorStateRevisionConflict, err)
		}
		return ActorStateModule{}, err
	}
	item = document.Value
	item.Path, item.Revision = path, document.Revision
	return applyActorStateOwnership(item), nil
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
	return presetlayout.ActorStates(l.novaDir)
}

func (l *ActorStateLibrary) ensureBuiltins() error {
	return ensureBuiltinModuleFiles(l.dir(), builtinActorStateModules(),
		func(item ActorStateModule) string { return item.ID }, parseActorStateFile,
		func(current, builtin ActorStateModule) bool {
			return current.BuiltinOverridden || current.ID == builtin.ID && current.Version == storyDirectorModuleVersion && !actorStateDiffersFromBuiltin(current)
		}, writeActorStateFile)
}
