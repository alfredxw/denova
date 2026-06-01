package book

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nova/internal/prompts"
)

// State 管理作品状态文件和内部目录。
type State struct {
	workspace string
}

// NewState 创建作品状态管理器。
func NewState(workspace string) *State {
	return &State{workspace: workspace}
}

// Workspace 返回作品工作目录。
func (s *State) Workspace() string {
	return s.workspace
}

// NovaDir 返回 .nova/ 目录路径（内部数据，用户不需要关注）。
func (s *State) NovaDir() string {
	return filepath.Join(s.workspace, ".nova")
}

// SessionDir 返回 .nova/sessions/ 目录路径（会话存储）。
func (s *State) SessionDir() string {
	return filepath.Join(s.NovaDir(), "sessions")
}

// BackupDir 返回 .nova/backups/ 目录路径。
func (s *State) BackupDir() string {
	return filepath.Join(s.NovaDir(), "backups")
}

// LoreDir 返回 .nova/lore/ 目录路径（结构化资料库）。
func (s *State) LoreDir() string {
	return filepath.Join(s.NovaDir(), "lore")
}

// SettingDir 返回 setting/ 目录路径（作品设定，用户可查看和编辑）。
func (s *State) SettingDir() string {
	return filepath.Join(s.workspace, "setting")
}

// ChapterGroupDir 返回章节组细纲目录路径（用户可查看和编辑）。
func (s *State) ChapterGroupDir() string {
	return filepath.Join(s.SettingDir(), "chapter-groups")
}

// BrainstormFileName 顶层定调文件名，存于 workspace 根目录。
const BrainstormFileName = "脑暴.md"

// InitWorkspace 初始化作品工作目录结构，并在缺失时写入「脑暴.md」顶层定调模板。
func (s *State) InitWorkspace() error {
	dirs := []string{
		s.NovaDir(),
		s.BackupDir(),
		s.SessionDir(),
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

	brainstormPath := filepath.Join(s.workspace, BrainstormFileName)
	if _, err := os.Stat(brainstormPath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(brainstormPath, []byte(prompts.BrainstormTemplate), 0o644); writeErr != nil {
			return fmt.Errorf("写入 %s 失败: %w", BrainstormFileName, writeErr)
		}
	} else if err != nil {
		return fmt.Errorf("检查 %s 失败: %w", BrainstormFileName, err)
	}

	if err := ensureCreatorTemplate(s.workspace); err != nil {
		return err
	}
	if err := NewLoreStore(s.workspace).Ensure(); err != nil {
		return fmt.Errorf("初始化资料库失败: %w", err)
	}
	return nil
}

// BrainstormPath 返回脑暴文件绝对路径。
func (s *State) BrainstormPath() string {
	return filepath.Join(s.workspace, BrainstormFileName)
}

// CompactContext 读取作品状态和结构化资料库，构建分级注入的上下文字符串。
func (s *State) CompactContext() string {
	var sb strings.Builder
	loreContext := s.LoreContext()

	sections := []struct {
		file  string
		title string
	}{
		{"outline.md", "当前大纲"},
		{"progress.md", "当前进度"},
	}

	for _, sec := range sections {
		content := s.readSettingFile(sec.file)
		if content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("## %s\n\n", sec.title))
		sb.WriteString(strings.TrimSpace(content))
		sb.WriteString("\n\n")
	}

	if loreContext != "" {
		sb.WriteString("## 资料库\n\n")
		sb.WriteString(loreContext)
		sb.WriteString("\n\n")
	}

	if groupContext := s.ChapterGroupContext(2); groupContext != "" {
		sb.WriteString("## 章节组细纲\n\n")
		sb.WriteString(groupContext)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// ChapterGroupContext 返回最近的章节组细纲内容，用于提示 Agent 关注当前短期写作计划。
func (s *State) ChapterGroupContext(limit int) string {
	if limit <= 0 {
		limit = 2
	}
	root := s.ChapterGroupDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".txt" {
			continue
		}
		files = append(files, name)
	}
	if len(files) == 0 {
		return ""
	}
	sort.Strings(files)
	if len(files) > limit {
		files = files[len(files)-limit:]
	}
	var sb strings.Builder
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		sb.WriteString("### ")
		sb.WriteString(name)
		sb.WriteString("\n\n")
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// LoreContext 返回结构化资料库中的 Markdown 上下文。
func (s *State) LoreContext() string {
	context, err := NewLoreStore(s.workspace).ContextMarkdown()
	if err != nil {
		return ""
	}
	return context
}

// HasState 检查作品是否已有大纲、进度或资料库内容。
func (s *State) HasState() bool {
	files := []string{"outline.md", "progress.md"}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(s.SettingDir(), f)); err == nil {
			return true
		}
	}
	return strings.TrimSpace(s.LoreContext()) != ""
}

func (s *State) readSettingFile(name string) string {
	data, err := os.ReadFile(filepath.Join(s.SettingDir(), name))
	if err != nil {
		return ""
	}
	return string(data)
}

// ReadCreatorPrompt 读取 workspace 根目录下的 CREATOR.md 自定义指令。
func (s *State) ReadCreatorPrompt() string {
	data, err := os.ReadFile(filepath.Join(s.workspace, "CREATOR.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
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
