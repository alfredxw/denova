package config

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

	// Agent
	MaxIteration    *int  `toml:"max_iteration,omitempty" json:"max_iteration,omitempty"`
	ModelMaxRetries *int  `toml:"model_max_retries,omitempty" json:"model_max_retries,omitempty"`
	PlanModeDefault *bool `toml:"plan_mode_default,omitempty" json:"plan_mode_default,omitempty"`
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// DefaultSettings 返回内置默认配置（最低优先级）。
func DefaultSettings() Settings {
	return Settings{
		OpenAIBaseURL:         "https://api.deepseek.com",
		OpenAIModel:           "deepseek-v4-pro",
		SkillsDir:             "./skills",
		NovaDir:               "~/.nova",
		AutoSaveEnabled:       boolPtr(true),
		AutoSaveIntervalMs:    intPtr(1500),
		ChapterFilenameFormat: "ch{NN}-{title}.md",
		MaxIteration:          intPtr(50),
		ModelMaxRetries:       intPtr(5),
		PlanModeDefault:       boolPtr(false),
	}
}
