package config

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Settings 是用户可见且可在三层配置中持久化的字段。
// 指针类型用于区分 "未设置"（继承上层）与 "显式置零"。
type Settings struct {
	// 模型
	OpenAIAPIKey  string `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	OpenAIBaseURL string `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	OpenAIModel   string `toml:"openai_model,omitempty" json:"openai_model,omitempty"`

	// 路径
	SkillsDir string `toml:"skills_dir,omitempty" json:"skills_dir,omitempty"`
	NovaDir   string `toml:"nova_dir,omitempty" json:"nova_dir,omitempty"`

	// 编辑器
	AutoSaveEnabled       *bool  `toml:"auto_save_enabled,omitempty" json:"auto_save_enabled,omitempty"`
	AutoSaveIntervalMs    *int   `toml:"auto_save_interval_ms,omitempty" json:"auto_save_interval_ms,omitempty"`
	ChapterFilenameFormat string `toml:"chapter_filename_format,omitempty" json:"chapter_filename_format,omitempty"`
	MaxOpenTabs           *int   `toml:"max_open_tabs,omitempty" json:"max_open_tabs,omitempty"`

	// 外观
	UIFontFamily      string `toml:"ui_font_family,omitempty" json:"ui_font_family,omitempty"`
	ReadingFontFamily string `toml:"reading_font_family,omitempty" json:"reading_font_family,omitempty"`

	// Agent
	MaxIteration    *int  `toml:"max_iteration,omitempty" json:"max_iteration,omitempty"`
	ModelMaxRetries *int  `toml:"model_max_retries,omitempty" json:"model_max_retries,omitempty"`
	PlanModeDefault *bool `toml:"plan_mode_default,omitempty" json:"plan_mode_default,omitempty"`

	// 互动模式
	InteractiveReplyTargetChars *int     `toml:"interactive_reply_target_chars,omitempty" json:"interactive_reply_target_chars,omitempty"`
	InteractiveMaxTokens        *int     `toml:"interactive_max_tokens,omitempty" json:"interactive_max_tokens,omitempty"`
	InteractiveStageFontSize    *int     `toml:"interactive_stage_font_size,omitempty" json:"interactive_stage_font_size,omitempty"`
	InteractiveStageLineHeight  *float64 `toml:"interactive_stage_line_height,omitempty" json:"interactive_stage_line_height,omitempty"`

	// 风格：场景化默认风格规则（仅工作区级生效）。
	// 每条规则关联一个自然语言场景描述与若干 setting/styles/ 下的风格文件，
	// 由 Agent 基于本轮章节内容自动匹配场景并选择对应风格文件。
	// 当用户本轮通过 # 指定了任意风格参考时，本轮覆盖默认规则。
	StyleRules []StyleRule `toml:"style_rules,omitempty" json:"style_rules,omitempty"`
}

// StyleRule 表示一条「场景 → 风格文件」映射。
// Scene 使用自然语言描述触发条件（如「激烈打斗」「日常对话」「宏大世界观铺陈」）；
// Styles 是 setting/styles/ 下的相对路径列表。
type StyleRule struct {
	Scene  string   `toml:"scene" json:"scene"`
	Styles []string `toml:"styles" json:"styles"`
}

func boolPtr(v bool) *bool        { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

// DefaultSettings 返回内置默认配置（最低优先级）。
func DefaultSettings() Settings {
	return Settings{
		OpenAIBaseURL:               "https://api.deepseek.com",
		OpenAIModel:                 "deepseek-v4-pro",
		SkillsDir:                   "./skills",
		NovaDir:                     "./.nova",
		AutoSaveEnabled:             boolPtr(true),
		AutoSaveIntervalMs:          intPtr(1500),
		ChapterFilenameFormat:       "ch{NN}-{title}.md",
		MaxOpenTabs:                 intPtr(5),
		UIFontFamily:                "system-sans",
		ReadingFontFamily:           "source-han-serif",
		MaxIteration:                intPtr(50),
		ModelMaxRetries:             intPtr(5),
		PlanModeDefault:             boolPtr(false),
		InteractiveReplyTargetChars: intPtr(1200),
		InteractiveStageFontSize:    intPtr(16),
		InteractiveStageLineHeight:  floatPtr(1.78),
	}
}

// Merge 用 child 的非零字段覆盖 parent 后返回新值。
// 字符串：空串视为未设置；指针：nil 视为未设置。
func Merge(parent, child Settings) Settings {
	out := parent
	if child.OpenAIAPIKey != "" {
		out.OpenAIAPIKey = child.OpenAIAPIKey
	}
	if child.OpenAIBaseURL != "" {
		out.OpenAIBaseURL = child.OpenAIBaseURL
	}
	if child.OpenAIModel != "" {
		out.OpenAIModel = child.OpenAIModel
	}
	if child.SkillsDir != "" {
		out.SkillsDir = child.SkillsDir
	}
	if child.NovaDir != "" {
		out.NovaDir = child.NovaDir
	}
	if child.AutoSaveEnabled != nil {
		out.AutoSaveEnabled = child.AutoSaveEnabled
	}
	if child.AutoSaveIntervalMs != nil {
		out.AutoSaveIntervalMs = child.AutoSaveIntervalMs
	}
	if child.ChapterFilenameFormat != "" {
		out.ChapterFilenameFormat = child.ChapterFilenameFormat
	}
	if child.MaxOpenTabs != nil {
		out.MaxOpenTabs = child.MaxOpenTabs
	}
	if child.UIFontFamily != "" {
		out.UIFontFamily = child.UIFontFamily
	}
	if child.ReadingFontFamily != "" {
		out.ReadingFontFamily = child.ReadingFontFamily
	}
	if child.MaxIteration != nil {
		out.MaxIteration = child.MaxIteration
	}
	if child.ModelMaxRetries != nil {
		out.ModelMaxRetries = child.ModelMaxRetries
	}
	if child.PlanModeDefault != nil {
		out.PlanModeDefault = child.PlanModeDefault
	}
	if child.InteractiveReplyTargetChars != nil {
		out.InteractiveReplyTargetChars = child.InteractiveReplyTargetChars
	}
	if child.InteractiveMaxTokens != nil {
		out.InteractiveMaxTokens = child.InteractiveMaxTokens
	}
	if child.InteractiveStageFontSize != nil {
		out.InteractiveStageFontSize = child.InteractiveStageFontSize
	}
	if child.InteractiveStageLineHeight != nil {
		out.InteractiveStageLineHeight = child.InteractiveStageLineHeight
	}
	// 场景化风格规则：工作区级覆盖，nil 视为未设置；空切片表示显式清空。
	if child.StyleRules != nil {
		out.StyleRules = child.StyleRules
	}
	return out
}

const (
	// UserConfigFilename 是用户级配置文件名（位于 NovaDir 下）。
	UserConfigFilename = "config.toml"
	// WorkspaceConfigDir 是工作区级配置目录（相对于 workspace）。
	WorkspaceConfigDir = ".nova"
	// WorkspaceConfigFilename 是工作区级配置文件名。
	WorkspaceConfigFilename = "config.toml"
)

// LayeredSettings 暴露三层快照及合并后的 effective 值。
type LayeredSettings struct {
	Default   Settings      `json:"default"`
	Global    Settings      `json:"global"`
	User      Settings      `json:"user"`
	Workspace Settings      `json:"workspace"`
	Effective Settings      `json:"effective"`
	Paths     SettingsPaths `json:"paths"`
}

// SettingsPaths 是设置页只读展示的真实配置路径。
type SettingsPaths struct {
	NovaDir         string `json:"nova_dir"`
	UserConfig      string `json:"user_config"`
	WorkspaceConfig string `json:"workspace_config"`
}

// ReadSettingsFile 读取 TOML，文件不存在时返回零值且无错误。
func ReadSettingsFile(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	var s Settings
	if err := toml.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return sanitizeEditableSettings(s), nil
}

// WriteSettingsFile 写入 TOML，自动创建父目录。
func WriteSettingsFile(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := toml.Marshal(sanitizeEditableSettings(s))
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", path, err)
	}
	return nil
}

// UserConfigPath 计算用户级配置路径。novaDir 已经过 normalizePath 处理。
func UserConfigPath(novaDir string) string {
	if novaDir == "" {
		novaDir = normalizePath("./.nova")
	}
	return filepath.Join(novaDir, UserConfigFilename)
}

// WorkspaceConfigPath 计算工作区级配置路径。
func WorkspaceConfigPath(workspace string) string {
	return filepath.Join(workspace, WorkspaceConfigDir, WorkspaceConfigFilename)
}

// LoadLayered 读取用户级 + 工作区级配置并与默认值合并。
// novaDir 为空时使用默认 ./.nova（后端运行目录下）。
func LoadLayered(novaDir, workspace string) (LayeredSettings, error) {
	return LoadLayeredWithGlobal(novaDir, workspace, Settings{})
}

// LoadLayeredWithGlobal 读取用户级 + 工作区级配置，并加入全局启动配置层。
func LoadLayeredWithGlobal(novaDir, workspace string, global Settings) (LayeredSettings, error) {
	novaDir = normalizePath(novaDir)
	user, err := ReadSettingsFile(UserConfigPath(novaDir))
	if err != nil {
		return LayeredSettings{}, err
	}
	var ws Settings
	if workspace != "" {
		ws, err = ReadSettingsFile(WorkspaceConfigPath(workspace))
		if err != nil {
			return LayeredSettings{}, err
		}
	}
	def := DefaultSettings()
	def.NovaDir = novaDir
	if global.NovaDir == "" {
		global.NovaDir = novaDir
	} else {
		global.NovaDir = normalizePath(global.NovaDir)
	}
	eff := Merge(Merge(Merge(def, global), user), ws)
	return LayeredSettings{
		Default:   def,
		Global:    global,
		User:      user,
		Workspace: ws,
		Effective: eff,
		Paths: SettingsPaths{
			NovaDir:         novaDir,
			UserConfig:      UserConfigPath(novaDir),
			WorkspaceConfig: WorkspaceConfigPath(workspace),
		},
	}, nil
}

func sanitizeEditableSettings(s Settings) Settings {
	// nova_dir 是启动级定位参数，不能由用户级/工作区级配置反向修改自身位置。
	s.NovaDir = ""
	return s
}
