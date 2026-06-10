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
	OpenAIAPIKey  string                 `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	OpenAIBaseURL string                 `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	OpenAIModel   string                 `toml:"openai_model,omitempty" json:"openai_model,omitempty"`
	ModelProfiles []ModelProfileSettings `toml:"model_profiles,omitempty" json:"model_profiles,omitempty"`
	AgentModels   AgentModelSettings     `toml:"agent_models,omitempty" json:"agent_models,omitempty"`
	AgentTools    AgentToolSettings      `toml:"agent_tools,omitempty" json:"agent_tools,omitempty"`
	AgentPrompts  AgentPromptSettings    `toml:"agent_prompts,omitempty" json:"agent_prompts,omitempty"`

	// 路径
	SkillsDir string `toml:"skills_dir,omitempty" json:"skills_dir,omitempty"`
	NovaDir   string `toml:"nova_dir,omitempty" json:"nova_dir,omitempty"`

	// 编辑器
	AutoSaveEnabled             *bool  `toml:"auto_save_enabled,omitempty" json:"auto_save_enabled,omitempty"`
	AutoSaveIntervalMs          *int   `toml:"auto_save_interval_ms,omitempty" json:"auto_save_interval_ms,omitempty"`
	ChapterFilenameFormat       string `toml:"chapter_filename_format,omitempty" json:"chapter_filename_format,omitempty"`
	MaxOpenTabs                 *int   `toml:"max_open_tabs,omitempty" json:"max_open_tabs,omitempty"`
	DraftFlowEnabled            *bool  `toml:"draft_flow_enabled,omitempty" json:"draft_flow_enabled,omitempty"`
	ChapterGroupMin             *int   `toml:"chapter_group_min,omitempty" json:"chapter_group_min,omitempty"`
	ChapterGroupMax             *int   `toml:"chapter_group_max,omitempty" json:"chapter_group_max,omitempty"`
	VersionTimedEnabled         *bool  `toml:"version_timed_enabled,omitempty" json:"version_timed_enabled,omitempty"`
	VersionTimedIntervalMinutes *int   `toml:"version_timed_interval_minutes,omitempty" json:"version_timed_interval_minutes,omitempty"`
	VersionAgentEnabled         *bool  `toml:"version_agent_enabled,omitempty" json:"version_agent_enabled,omitempty"`
	VersionAgentCharThreshold   *int   `toml:"version_agent_char_threshold,omitempty" json:"version_agent_char_threshold,omitempty"`

	// 外观
	UIFontFamily      string `toml:"ui_font_family,omitempty" json:"ui_font_family,omitempty"`
	UIFontSize        *int   `toml:"ui_font_size,omitempty" json:"ui_font_size,omitempty"`
	ReadingFontFamily string `toml:"reading_font_family,omitempty" json:"reading_font_family,omitempty"`
	ReadingFontSize   *int   `toml:"reading_font_size,omitempty" json:"reading_font_size,omitempty"`
	Language          string `toml:"language,omitempty" json:"language,omitempty"`

	// Agent
	MaxIteration     *int   `toml:"max_iteration,omitempty" json:"max_iteration,omitempty"`
	ModelMaxRetries  *int   `toml:"model_max_retries,omitempty" json:"model_max_retries,omitempty"`
	PlanModeDefault  *bool  `toml:"plan_mode_default,omitempty" json:"plan_mode_default,omitempty"`
	IDEStoryTellerID string `toml:"ide_story_teller_id,omitempty" json:"ide_story_teller_id,omitempty"`

	// 互动模式
	InteractiveMaxTokens       *int     `toml:"interactive_max_tokens,omitempty" json:"interactive_max_tokens,omitempty"`
	InteractiveHotChoices      *bool    `toml:"interactive_hot_choices_enabled,omitempty" json:"interactive_hot_choices_enabled,omitempty"`
	InteractiveStageFontSize   *int     `toml:"interactive_stage_font_size,omitempty" json:"interactive_stage_font_size,omitempty"`
	InteractiveStageLineHeight *float64 `toml:"interactive_stage_line_height,omitempty" json:"interactive_stage_line_height,omitempty"`
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
		ChapterFilenameFormat:       "第{N}章-{title}.md",
		MaxOpenTabs:                 intPtr(5),
		DraftFlowEnabled:            boolPtr(false),
		ChapterGroupMin:             intPtr(3),
		ChapterGroupMax:             intPtr(8),
		VersionTimedEnabled:         boolPtr(true),
		VersionTimedIntervalMinutes: intPtr(10),
		VersionAgentEnabled:         boolPtr(true),
		VersionAgentCharThreshold:   intPtr(3000),
		UIFontFamily:                "system-sans",
		UIFontSize:                  intPtr(12),
		ReadingFontFamily:           "source-han-serif",
		ReadingFontSize:             intPtr(18),
		Language:                    "auto",
		MaxIteration:                intPtr(50),
		ModelMaxRetries:             intPtr(5),
		AgentModels: AgentModelSettings{
			InteractiveHotChoices: AgentModelOverride{EnableThinking: boolPtr(false)},
			VersionSummary:        AgentModelOverride{EnableThinking: boolPtr(false)},
			ToolAgent:             AgentModelOverride{EnableThinking: boolPtr(false)},
		},
		AgentTools:                 DefaultAgentToolSettings(),
		PlanModeDefault:            boolPtr(false),
		IDEStoryTellerID:           "classic",
		InteractiveHotChoices:      boolPtr(true),
		InteractiveStageFontSize:   intPtr(16),
		InteractiveStageLineHeight: floatPtr(1.78),
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
	out.ModelProfiles = mergeModelProfiles(out.ModelProfiles, child.ModelProfiles)
	out.AgentModels = MergeAgentModelSettings(out.AgentModels, child.AgentModels)
	out.AgentTools = MergeAgentToolSettings(out.AgentTools, child.AgentTools)
	out.AgentPrompts = MergeAgentPromptSettings(out.AgentPrompts, child.AgentPrompts)
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
	if child.DraftFlowEnabled != nil {
		out.DraftFlowEnabled = child.DraftFlowEnabled
	}
	if child.ChapterGroupMin != nil {
		out.ChapterGroupMin = child.ChapterGroupMin
	}
	if child.ChapterGroupMax != nil {
		out.ChapterGroupMax = child.ChapterGroupMax
	}
	if child.VersionTimedEnabled != nil {
		out.VersionTimedEnabled = child.VersionTimedEnabled
	}
	if child.VersionTimedIntervalMinutes != nil {
		out.VersionTimedIntervalMinutes = child.VersionTimedIntervalMinutes
	}
	if child.VersionAgentEnabled != nil {
		out.VersionAgentEnabled = child.VersionAgentEnabled
	}
	if child.VersionAgentCharThreshold != nil {
		out.VersionAgentCharThreshold = child.VersionAgentCharThreshold
	}
	if child.UIFontFamily != "" {
		out.UIFontFamily = child.UIFontFamily
	}
	if child.UIFontSize != nil {
		out.UIFontSize = child.UIFontSize
	}
	if child.ReadingFontFamily != "" {
		out.ReadingFontFamily = child.ReadingFontFamily
	}
	if child.ReadingFontSize != nil {
		out.ReadingFontSize = child.ReadingFontSize
	}
	if child.Language != "" {
		out.Language = child.Language
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
	if child.IDEStoryTellerID != "" {
		out.IDEStoryTellerID = child.IDEStoryTellerID
	}
	if child.InteractiveMaxTokens != nil {
		out.InteractiveMaxTokens = child.InteractiveMaxTokens
	}
	if child.InteractiveHotChoices != nil {
		out.InteractiveHotChoices = child.InteractiveHotChoices
	}
	if child.InteractiveStageFontSize != nil {
		out.InteractiveStageFontSize = child.InteractiveStageFontSize
	}
	if child.InteractiveStageLineHeight != nil {
		out.InteractiveStageLineHeight = child.InteractiveStageLineHeight
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
	s.Language = normalizeLanguage(s.Language)
	s.AgentPrompts = sanitizeAgentPromptSettings(s.AgentPrompts)
	return s
}

func normalizeLanguage(language string) string {
	switch language {
	case "", "auto", "zh-CN", "en-US":
		return language
	default:
		return ""
	}
}
