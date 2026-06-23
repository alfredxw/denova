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

// CompactContextPart describes one bounded, model-visible workspace state source.
type CompactContextPart struct {
	ID          string
	Source      string
	Title       string
	Content     string
	PromptTitle string
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

// IdeasFileName 创作灵感文件名，存于 workspace 根目录，承载新书构思阶段的阶段性结论。
const IdeasFileName = "ideas.md"

// LegacyBrainstormFileName 是旧版顶层定调文件名，仅用于初始化时迁移旧工作区。
const LegacyBrainstormFileName = "brainstorm.md"

const ideasContextMaxRunes = 2000

// CharacterStatesFileName 角色状态文件名，存于 setting/，用于追踪当前连续性状态。
const CharacterStatesFileName = "character-states.md"

// InitWorkspace 初始化作品工作目录结构，并在缺失时写入 ideas.md 创作灵感模板。
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

	if err := s.ensureIdeasFile(); err != nil {
		return err
	}

	if err := ensureCreatorTemplate(s.workspace); err != nil {
		return err
	}
	if err := NewLoreStore(s.workspace).Ensure(); err != nil {
		return fmt.Errorf("初始化资料库失败: %w", err)
	}
	return nil
}

// IdeasPath 返回创作灵感文件绝对路径。
func (s *State) IdeasPath() string {
	return filepath.Join(s.workspace, IdeasFileName)
}

// CompactContextParts 读取作品状态和结构化资料库，保留每个注入片段的真实来源。
func (s *State) CompactContextParts() []CompactContextPart {
	parts := make([]CompactContextPart, 0, 7)
	loreContext := s.LoreContext()

	if ideasContext := s.IdeasContext(); ideasContext != "" {
		parts = append(parts, CompactContextPart{
			ID:          "ideas",
			Source:      IdeasFileName,
			Title:       "创作灵感",
			PromptTitle: fmt.Sprintf("创作灵感（%s，最多 %d 字）", IdeasFileName, ideasContextMaxRunes),
			Content:     ideasContext,
		})
	}

	sections := []struct {
		file  string
		title string
		id    string
	}{
		{"outline.md", "当前大纲", "outline"},
		{"progress.md", "当前进度", "progress"},
		{CharacterStatesFileName, "角色状态", "character_states"},
	}

	for _, sec := range sections {
		content := s.readSettingFile(sec.file)
		if content == "" {
			continue
		}
		parts = append(parts, CompactContextPart{
			ID:          sec.id,
			Source:      filepath.ToSlash(filepath.Join("setting", sec.file)),
			Title:       sec.title,
			PromptTitle: sec.title,
			Content:     strings.TrimSpace(content),
		})
	}

	if chapterContext := s.ChapterPathContext(12); chapterContext != "" {
		parts = append(parts, CompactContextPart{
			ID:          "chapter_paths",
			Source:      "chapters/",
			Title:       "章节目录概览",
			PromptTitle: "章节目录概览",
			Content:     chapterContext,
		})
	}

	if loreContext != "" {
		parts = append(parts, CompactContextPart{
			ID:      "lore",
			Source:  ".nova/lore/items.json",
			Title:   "资料库",
			Content: loreContext,
		})
	}

	if groupContext := s.ChapterGroupContext(2); groupContext != "" {
		parts = append(parts, CompactContextPart{
			ID:          "chapter_groups",
			Source:      "setting/chapter-groups/",
			Title:       "章节组细纲",
			PromptTitle: "章节组细纲",
			Content:     groupContext,
		})
	}

	return parts
}

// CompactContext 读取作品状态和结构化资料库，构建分级注入的上下文字符串。
func (s *State) CompactContext() string {
	var sb strings.Builder
	for _, part := range s.CompactContextParts() {
		content := strings.TrimSpace(part.Content)
		if content == "" {
			continue
		}
		if title := strings.TrimSpace(part.PromptTitle); title != "" {
			sb.WriteString("## ")
			sb.WriteString(title)
			sb.WriteString("\n\n")
		}
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}
	return sb.String()
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

	if err := os.WriteFile(ideasPath, []byte(prompts.IdeasTemplate), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", IdeasFileName, err)
	}
	return nil
}

// IdeasContext 返回有界的创作灵感上下文；空模板不参与模型注入。
func (s *State) IdeasContext() string {
	data, err := os.ReadFile(s.IdeasPath())
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == "" || content == strings.TrimSpace(prompts.IdeasTemplate) {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= ideasContextMaxRunes {
		return content
	}
	return strings.TrimSpace(string(runes[:ideasContextMaxRunes])) + fmt.Sprintf("\n\n（已按 %d 字上限截断；如需全文请显式读取 %s。）", ideasContextMaxRunes, IdeasFileName)
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

// ChapterPathContext 返回最近章节路径和分卷概览，帮助 Agent 在续写时选择正确目录。
func (s *State) ChapterPathContext(limit int) string {
	if limit <= 0 {
		limit = 12
	}
	root := filepath.Join(s.workspace, "chapters")
	if _, err := os.Stat(root); err != nil {
		return ""
	}

	type chapterEntry struct {
		Path   string
		Index  int
		Volume string
	}
	var entries []chapterEntry
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".txt" {
			return nil
		}
		rel, err := filepath.Rel(s.workspace, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		volume, _ := chapterVolume(rel)
		entries = append(entries, chapterEntry{Path: rel, Index: chapterIndex(filepath.Base(rel)), Volume: volume})
		return nil
	})
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if cmp := compareChapterLikeNames(filepath.Base(left.Path), filepath.Base(right.Path)); cmp != 0 {
			return cmp < 0
		}
		return left.Path < right.Path
	})
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	var sb strings.Builder
	for _, entry := range entries {
		sb.WriteString("- ")
		sb.WriteString(entry.Path)
		if entry.Volume != "" {
			sb.WriteString("（分卷：")
			sb.WriteString(entry.Volume)
			sb.WriteString("）")
		}
		sb.WriteString("\n")
	}
	latest := entries[len(entries)-1]
	sb.WriteString("\n最近定稿章节：")
	sb.WriteString(latest.Path)
	if latest.Volume != "" && latest.Volume != "未分卷" {
		sb.WriteString("；若大纲仍处于该卷，下一章应继续写入 chapters/")
		sb.WriteString(latest.Volume)
		sb.WriteString("/。")
	}
	return strings.TrimSpace(sb.String())
}

// LoreContext 返回结构化资料库的渐进式 Markdown 上下文。
func (s *State) LoreContext() string {
	context, err := NewLoreStore(s.workspace).ProgressiveContextMarkdown()
	if err != nil {
		return ""
	}
	return context
}

// HasState 检查作品是否已有大纲、进度、角色状态或资料库内容。
func (s *State) HasState() bool {
	files := []string{"outline.md", "progress.md", CharacterStatesFileName}
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
