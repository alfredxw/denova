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

const DefaultStoryMemoryStructureModuleID = "default"

var ErrStoryMemoryStructureRevisionConflict = errors.New("故事记忆结构预设已被其他操作更新，请重新加载后再保存")

type StoryMemoryStructureModule struct {
	Version           int                    `json:"version"`
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Structures        []StoryMemoryStructure `json:"structures"`
	Tags              []string               `json:"tags"`
	Path              string                 `json:"path,omitempty"`
	Custom            bool                   `json:"custom"`
	BuiltinOverridden bool                   `json:"builtin_overridden,omitempty"`
	Invalid           bool                   `json:"invalid,omitempty"`
	Error             string                 `json:"error,omitempty"`
	CreatedAt         string                 `json:"created_at,omitempty"`
	UpdatedAt         string                 `json:"updated_at,omitempty"`
}

type StoryMemoryStructureLibrary struct {
	novaDir string
}

func NewStoryMemoryStructureLibrary(novaDir string) *StoryMemoryStructureLibrary {
	return &StoryMemoryStructureLibrary{novaDir: novaDir}
}

func (l *StoryMemoryStructureLibrary) List() ([]StoryMemoryStructureModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]StoryMemoryStructureModule, 0, len(files))
	for _, file := range files {
		item, err := parseStoryMemoryStructureFile(file)
		if err != nil {
			id := strings.TrimSuffix(filepath.Base(file), ".json")
			items = append(items, StoryMemoryStructureModule{ID: id, Path: file, Invalid: true, Error: err.Error(), Custom: !IsBuiltinStoryMemoryStructureID(id)})
			continue
		}
		item.Path = file
		item = applyStoryMemoryStructureOwnership(item)
		items = append(items, item)
	}
	sortStoryMemoryStructureModules(items)
	return items, nil
}

func (l *StoryMemoryStructureLibrary) Get(id string) (StoryMemoryStructureModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if id == "" {
		id = DefaultStoryMemoryStructureModuleID
	}
	if err := validateDirectorModuleID(id, "故事记忆结构预设"); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	item, err := parseStoryMemoryStructureFile(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return StoryMemoryStructureModule{}, err
	}
	return applyStoryMemoryStructureOwnership(item), nil
}

func (l *StoryMemoryStructureLibrary) Create(item StoryMemoryStructureModule) (StoryMemoryStructureModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	item = normalizeStoryMemoryStructureModule(item)
	if item.ID == "" {
		item.ID = newDirectorModuleID("memory-structure")
	}
	item.BuiltinOverridden = false
	if err := validateStoryMemoryStructureModule(item); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	path := filepath.Join(l.dir(), item.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return StoryMemoryStructureModule{}, fmt.Errorf("故事记忆结构预设已存在: %s", item.ID)
	} else if !os.IsNotExist(err) {
		return StoryMemoryStructureModule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := writeStoryMemoryStructureFile(path, item); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	item.Path = path
	return applyStoryMemoryStructureOwnership(item), nil
}

func (l *StoryMemoryStructureLibrary) Update(id string, item StoryMemoryStructureModule, baseRevision string) (StoryMemoryStructureModule, error) {
	if err := l.ensureBuiltins(); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "故事记忆结构预设"); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	isBuiltin := IsBuiltinStoryMemoryStructureID(id)
	current, err := l.Get(id)
	if err != nil {
		return StoryMemoryStructureModule{}, err
	}
	if strings.TrimSpace(baseRevision) != "" && strings.TrimSpace(current.UpdatedAt) != strings.TrimSpace(baseRevision) {
		return StoryMemoryStructureModule{}, ErrStoryMemoryStructureRevisionConflict
	}
	item = normalizeStoryMemoryStructureModule(item)
	item.ID = id
	item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	item.BuiltinOverridden = isBuiltin
	if err := validateStoryMemoryStructureModule(item); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	path := filepath.Join(l.dir(), id+".json")
	if err := writeStoryMemoryStructureFile(path, item); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	item.Path = path
	return applyStoryMemoryStructureOwnership(item), nil
}

func (l *StoryMemoryStructureLibrary) Delete(id string) error {
	id = normalizeDirectorModuleID(id)
	if err := validateDirectorModuleID(id, "故事记忆结构预设"); err != nil {
		return err
	}
	if IsBuiltinStoryMemoryStructureID(id) {
		return writeStoryMemoryStructureFile(filepath.Join(l.dir(), id+".json"), DefaultStoryMemoryStructureModule())
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *StoryMemoryStructureLibrary) dir() string {
	return filepath.Join(l.novaDir, "story-director-modules", "story-memory-structures")
}

func (l *StoryMemoryStructureLibrary) ensureBuiltins() error {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(l.dir(), DefaultStoryMemoryStructureModuleID+".json")
	if current, err := parseStoryMemoryStructureFile(path); err == nil && current.BuiltinOverridden {
		return nil
	} else if err == nil && current.Version == storyDirectorModuleVersion {
		return nil
	}
	return writeStoryMemoryStructureFile(path, DefaultStoryMemoryStructureModule())
}

func DefaultStoryMemoryStructureModule() StoryMemoryStructureModule {
	return normalizeStoryMemoryStructureModule(StoryMemoryStructureModule{
		Version:     storyDirectorModuleVersion,
		ID:          DefaultStoryMemoryStructureModuleID,
		Name:        "默认故事记忆结构",
		Description: "面向互动故事长期承接的默认结构定义。运行时记录仍按具体故事和分支保存。",
		Structures:  defaultStoryMemoryStructures(),
		Tags:        []string{"内置", "记忆"},
	})
}

func IsBuiltinStoryMemoryStructureID(id string) bool {
	return normalizeDirectorModuleID(id) == DefaultStoryMemoryStructureModuleID
}

func applyStoryMemoryStructureOwnership(item StoryMemoryStructureModule) StoryMemoryStructureModule {
	if !IsBuiltinStoryMemoryStructureID(item.ID) {
		item.Custom = true
		item.BuiltinOverridden = false
		return item
	}
	item.Custom = false
	item.BuiltinOverridden = item.BuiltinOverridden || storyMemoryStructureModuleDiffersFromBuiltin(item)
	return item
}

func storyMemoryStructureModuleDiffersFromBuiltin(item StoryMemoryStructureModule) bool {
	return !reflect.DeepEqual(storyMemoryStructureModuleComparable(item), storyMemoryStructureModuleComparable(DefaultStoryMemoryStructureModule()))
}

func storyMemoryStructureModuleComparable(item StoryMemoryStructureModule) StoryMemoryStructureModule {
	item = normalizeStoryMemoryStructureModule(item)
	item.Path = ""
	item.Custom = false
	item.BuiltinOverridden = false
	item.Invalid = false
	item.Error = ""
	item.CreatedAt = ""
	item.UpdatedAt = ""
	item.Structures = storyMemoryStructuresComparable(item.Structures)
	return item
}

func normalizeStoryMemoryStructureModule(item StoryMemoryStructureModule) StoryMemoryStructureModule {
	item.Version = storyDirectorModuleVersion
	item.ID = normalizeDirectorModuleID(item.ID)
	item.Name = trimBytes(firstNonEmptyString(item.Name, item.ID, "故事记忆结构预设"), 256)
	item.Description = trimBytes(item.Description, 1024)
	item.Structures = normalizeStoryMemoryStructuresForModule(item.Structures)
	item.Tags = normalizeStringListLimit(item.Tags, maxTurnBriefListItems)
	return item
}

func normalizeStoryMemoryStructuresForModule(structures []StoryMemoryStructure) []StoryMemoryStructure {
	if structures == nil {
		return []StoryMemoryStructure{}
	}
	out := make([]StoryMemoryStructure, 0, len(structures))
	for i, structure := range structures {
		structure = normalizeStoryMemoryStructureFromStored(structure)
		if structure.ID == "" {
			structure.ID = fmt.Sprintf("structure_%d", i+1)
		}
		if structure.Name == "" {
			structure.Name = structure.ID
		}
		if structure.Order == 0 {
			structure.Order = (i + 1) * 10
		}
		out = append(out, structure)
	}
	sortStoryMemoryStructures(out)
	return out
}

func storyMemoryStructuresComparable(structures []StoryMemoryStructure) []StoryMemoryStructure {
	out := normalizeStoryMemoryStructuresForModule(structures)
	for i := range out {
		out[i].CreatedAt = ""
		out[i].UpdatedAt = ""
	}
	return out
}

func storyMemoryStructuresEqual(a, b []StoryMemoryStructure) bool {
	return reflect.DeepEqual(storyMemoryStructuresComparable(a), storyMemoryStructuresComparable(b))
}

func validateStoryMemoryStructureModule(item StoryMemoryStructureModule) error {
	if err := validateDirectorModuleID(item.ID, "故事记忆结构预设"); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("故事记忆结构预设名称不能为空")
	}
	if len(item.Structures) == 0 {
		return errors.New("故事记忆结构预设至少需要一个结构")
	}
	seen := map[string]bool{}
	for _, structure := range item.Structures {
		if err := validateStoryMemoryStructure(structure); err != nil {
			return err
		}
		if seen[structure.ID] {
			return fmt.Errorf("故事记忆结构 ID 重复: %s", structure.ID)
		}
		seen[structure.ID] = true
	}
	return nil
}

func parseStoryMemoryStructureFile(path string) (StoryMemoryStructureModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StoryMemoryStructureModule{}, err
	}
	var item StoryMemoryStructureModule
	if err := json.Unmarshal(data, &item); err != nil {
		return StoryMemoryStructureModule{}, fmt.Errorf("解析故事记忆结构预设 JSON 失败: %w", err)
	}
	item = normalizeStoryMemoryStructureModule(item)
	if err := validateStoryMemoryStructureModule(item); err != nil {
		return StoryMemoryStructureModule{}, err
	}
	item.Path = path
	return item, nil
}

func writeStoryMemoryStructureFile(path string, item StoryMemoryStructureModule) error {
	item = normalizeStoryMemoryStructureModule(item)
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func sortStoryMemoryStructureModules(items []StoryMemoryStructureModule) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Custom != items[j].Custom {
			return !items[i].Custom
		}
		return items[i].ID < items[j].ID
	})
}
