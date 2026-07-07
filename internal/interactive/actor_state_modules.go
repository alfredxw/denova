package interactive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	if err := writeActorStateFile(path, item); err != nil {
		return ActorStateModule{}, err
	}
	item.Path = path
	return applyActorStateOwnership(item), nil
}

func (l *ActorStateLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "状态系统"); err != nil {
		return err
	}
	if IsBuiltinActorStateID(id) {
		return writeActorStateFile(filepath.Join(l.dir(), id+".json"), DefaultActorStateModule())
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
	path := filepath.Join(l.dir(), DefaultActorStateModuleID+".json")
	if current, err := parseActorStateFile(path); err == nil && current.BuiltinOverridden {
		return nil
	} else if err == nil && current.Version == storyDirectorModuleVersion {
		return nil
	}
	return writeActorStateFile(path, DefaultActorStateModule())
}
