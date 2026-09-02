package book

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"denova/internal/book/lore"

	workspacelayout "denova/internal/workspace"
)

// State 管理作品状态文件和内部目录。
type State struct {
	workspace              string
	chapterPathMu          sync.Mutex
	chapterPathDirty       bool
	chapterPathEntries     []chapterPathEntry
	chapterPathDirectories map[string][]chapterDirectoryEntry
}

// NewState 创建作品状态管理器。
func NewState(workspace string) *State {
	return &State{workspace: workspace, chapterPathDirty: true}
}

// Workspace 返回作品工作目录。
func (s *State) Workspace() string {
	return s.workspace
}

// NovaDir 返回工作区内部数据目录路径（用户不需要关注）。
func (s *State) NovaDir() string {
	return workspacelayout.Dir(s.workspace)
}

// SessionDir returns the released workspace-private session path for migration
// and recovery only. New Project runtimes store sessions in the Project Store.
func (s *State) SessionDir() string {
	return workspacelayout.Path(s.workspace, "sessions")
}

// BackupDir returns the released workspace-private backup path. It remains a
// read/migration locator and is not created for new Project runtimes.
func (s *State) BackupDir() string {
	return workspacelayout.Path(s.workspace, "backups")
}

// LoreDir 返回用户可见的 setting/lore/ 目录路径（结构化资料库）。
func (s *State) LoreDir() string {
	return filepath.Join(s.SettingDir(), "lore")
}

// SettingDir 返回 setting/ 目录路径（作品设定，用户可查看和编辑）。
func (s *State) SettingDir() string {
	return filepath.Join(s.workspace, "setting")
}

// ChapterGroupDir 返回章节组细纲目录路径（用户可查看和编辑）。
func (s *State) ChapterGroupDir() string {
	return filepath.Join(s.SettingDir(), "chapter-groups")
}

// IdeasFileName 创作灵感文件名，存于 workspace 根目录，承载新书构思阶段的阶段性结论。
const IdeasFileName = "ideas.md"

// LegacyBrainstormFileName 是旧版顶层定调文件名，仅用于初始化时迁移旧工作区。
const LegacyBrainstormFileName = "brainstorm.md"

// CharacterStatesFileName 角色状态文件名，存于 setting/，用于追踪当前连续性状态。
const CharacterStatesFileName = "character-states.md"

// InitWorkspace 初始化作品工作目录结构，并在缺失时写入 ideas.md 创作灵感模板。
func (s *State) InitWorkspace() error {
	dirs := []string{
		s.NovaDir(),
		s.LoreDir(),
		s.SettingDir(),
		s.ChapterGroupDir(),
		filepath.Join(s.workspace, "chapters"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}

	if err := s.ensureIdeasFile(); err != nil {
		return err
	}

	if err := ensureCreatorTemplate(s.workspace); err != nil {
		return err
	}
	if err := lore.NewStore(s.workspace).Ensure(); err != nil {
		return fmt.Errorf("初始化资料库失败: %w", err)
	}
	return nil
}

// IdeasPath 返回创作灵感文件绝对路径。
func (s *State) IdeasPath() string {
	return filepath.Join(s.workspace, IdeasFileName)
}

// HasState reports whether the workspace contains established writing state or
// enabled Lore. ideas.md intentionally remains an ideation source, not proof
// that an outline-backed work has been initialized.
func (s *State) HasState() bool {
	if s == nil {
		return false
	}
	for _, name := range []string{"outline.md", "progress.md", CharacterStatesFileName} {
		if _, err := os.Stat(filepath.Join(s.SettingDir(), name)); err == nil {
			return true
		}
	}
	items, err := lore.NewStore(s.workspace).List()
	return err == nil && len(items) > 0
}

func (s *State) ensureIdeasFile() error {
	ideasPath := filepath.Join(s.workspace, IdeasFileName)
	if _, err := os.Stat(ideasPath); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查 %s 失败: %w", IdeasFileName, err)
	}

	legacyPath := filepath.Join(s.workspace, LegacyBrainstormFileName)
	if _, err := os.Stat(legacyPath); err == nil {
		if renameErr := os.Rename(legacyPath, ideasPath); renameErr != nil {
			return fmt.Errorf("迁移 %s 到 %s 失败: %w", LegacyBrainstormFileName, IdeasFileName, renameErr)
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查 %s 失败: %w", LegacyBrainstormFileName, err)
	}

	if err := os.WriteFile(ideasPath, []byte(IdeasTemplate), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", IdeasFileName, err)
	}
	return nil
}

// BookMeta 书籍元信息，存储在工作区根目录的 book.json 中。
type BookMeta struct {
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ReadBookMeta 读取工作区的 book.json 元信息。文件不存在时返回默认值（Title 取目录名）。
func (s *State) ReadBookMeta() BookMeta {
	return ReadBookMetaFromDir(s.workspace)
}

// WriteBookMeta 写入工作区的 book.json 元信息。自动设置 UpdatedAt，CreatedAt 为空时也自动设置。
func (s *State) WriteBookMeta(meta BookMeta) error {
	now := time.Now().Format(time.RFC3339)
	if meta.CreatedAt == "" {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 book.json 失败: %w", err)
	}
	p := filepath.Join(s.workspace, "book.json")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("写入 book.json 失败: %w", err)
	}
	return nil
}

// ReadBookMetaFromDir 从指定目录读取 book.json，文件不存在时返回默认值。
func ReadBookMetaFromDir(dir string) BookMeta {
	data, err := os.ReadFile(filepath.Join(dir, "book.json"))
	if err != nil {
		return BookMeta{Title: filepath.Base(dir)}
	}
	var meta BookMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return BookMeta{Title: filepath.Base(dir)}
	}
	return meta
}
