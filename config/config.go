package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Config 保存 Nova 的全局配置
type Config struct {
	OpenAIAPIKey                string                       `toml:"openai_api_key"`
	OpenAIBaseURL               string                       `toml:"openai_base_url"`
	OpenAIModel                 string                       `toml:"openai_model"`
	OpenAIContextWindowTokens   int                          `toml:"openai_context_window_tokens"`
	ModelProfiles               []ModelProfileSettings       `toml:"model_profiles"`
	ImageAPIKey                 string                       `toml:"image_api_key"`
	ImageAPIBaseURL             string                       `toml:"image_api_base_url"`
	ImageAPIModel               string                       `toml:"image_api_model"`
	DefaultImageAPIProfileID    string                       `toml:"default_image_api_profile_id"`
	ImageAPIProfiles            []ImageAPIProfileSettings    `toml:"image_api_profiles"`
	AgentModels                 AgentModelSettings           `toml:"agent_models"`
	AgentTools                  AgentToolSettings            `toml:"agent_tools"`
	AgentPrompts                AgentPromptSettings          `toml:"agent_prompts"`
	AgentSkills                 AgentSkillSettings           `toml:"agent_skills"`
	AgentContexts               AgentContextSettings         `toml:"agent_context"`
	GeneralSubAgents            AgentGeneralSubAgentSettings `toml:"general_sub_agents"`
	SubAgents                   []SubAgentConfig             `toml:"sub_agents"`
	SkillsDir                   string                       `toml:"skills_dir"`
	BackendPort                 int                          `toml:"backend_port"`
	FrontendPort                int                          `toml:"frontend_port"`
	AllowLANAccess              bool                         `toml:"allow_lan_access"`
	RemoteAccessUsername        string                       `toml:"remote_access_username"`
	RemoteAccessPasswordHash    string                       `toml:"remote_access_password_hash"`
	Language                    string                       `toml:"language"`
	NovaDir                     string                       `toml:"nova_dir"`
	Workspace                   string                       `toml:"workspace"`
	RuntimeWebPort              int                          `toml:"-"`
	IDEStoryTellerID            string                       `toml:"-"`
	IDEImagePresetID            string                       `toml:"-"`
	ImagePresetToolPrompt       string                       `toml:"-"`
	WritingSkillDefault         string                       `toml:"writing_skill_default"`
	MaxIteration                int                          `toml:"max_iteration"`
	ModelMaxRetries             int                          `toml:"model_max_retries"`
	AgentIdleTimeoutSeconds     int                          `toml:"agent_idle_timeout_seconds"`
	AgentToolResultLimitKB      int                          `toml:"agent_tool_result_limit_kb"`
	ChapterFilenameFormat       string                       `toml:"-"`
	VolumeDirFormat             string                       `toml:"-"`
	HideChapterBodyLiveOutput   bool                         `toml:"-"`
	ChapterGroupMin             int                          `toml:"-"`
	ChapterGroupMax             int                          `toml:"-"`
	VersionTimedEnabled         bool                         `toml:"-"`
	VersionTimedIntervalMinutes int                          `toml:"-"`
	VersionAgentEnabled         bool                         `toml:"-"`
	VersionAgentCharThreshold   int                          `toml:"-"`
	InteractiveReplyTargetChars int                          `toml:"-"`
	InteractiveHotChoices       bool                         `toml:"-"`
	ResumeLastWorkspace         bool                         `toml:"-"`
	UpdateCheckEnabled          bool                         `toml:"-"`
}

// LoadWithWorkspace 在已知 workspace 时读取分层配置（默认 < 用户级 < 工作区级 < 环境变量）。
func LoadWithWorkspace(workspace string) (*Config, LayeredSettings, error) {
	novaDir := startupNovaDir()
	layered, err := LoadLayeredWithStartupConfig(novaDir, workspace)
	if err != nil {
		return nil, LayeredSettings{}, err
	}

	s := layered.Effective
	cfg := &Config{
		OpenAIAPIKey:                s.OpenAIAPIKey,
		OpenAIBaseURL:               s.OpenAIBaseURL,
		OpenAIModel:                 s.OpenAIModel,
		OpenAIContextWindowTokens:   settingsInt(s.OpenAIContextWindowTokens, DefaultContextWindowTokens),
		ModelProfiles:               s.ModelProfiles,
		ImageAPIKey:                 s.ImageAPIKey,
		ImageAPIBaseURL:             s.ImageAPIBaseURL,
		ImageAPIModel:               s.ImageAPIModel,
		DefaultImageAPIProfileID:    s.DefaultImageAPIProfileID,
		ImageAPIProfiles:            s.ImageAPIProfiles,
		AgentModels:                 s.AgentModels,
		AgentTools:                  s.AgentTools,
		AgentPrompts:                s.AgentPrompts,
		AgentSkills:                 s.AgentSkills,
		AgentContexts:               s.AgentContexts,
		GeneralSubAgents:            s.GeneralSubAgents,
		SubAgents:                   s.SubAgents,
		SkillsDir:                   s.SkillsDir,
		BackendPort:                 settingsInt(s.BackendPort, 8080),
		FrontendPort:                settingsInt(s.FrontendPort, 5173),
		AllowLANAccess:              settingsBool(s.AllowLANAccess, false),
		RemoteAccessUsername:        s.RemoteAccessUsername,
		RemoteAccessPasswordHash:    s.RemoteAccessPasswordHash,
		Language:                    s.Language,
		NovaDir:                     novaDir,
		Workspace:                   workspace,
		IDEStoryTellerID:            s.IDEStoryTellerID,
		IDEImagePresetID:            s.IDEImagePresetID,
		WritingSkillDefault:         s.WritingSkillDefault,
		MaxIteration:                settingsInt(s.MaxIteration, 0),
		ModelMaxRetries:             settingsInt(s.ModelMaxRetries, 5),
		AgentIdleTimeoutSeconds:     settingsAgentIdleTimeoutSeconds(s.AgentIdleTimeoutSeconds),
		AgentToolResultLimitKB:      settingsAgentToolResultLimitKB(s.AgentToolResultLimitKB),
		ChapterFilenameFormat:       s.ChapterFilenameFormat,
		VolumeDirFormat:             s.VolumeDirFormat,
		HideChapterBodyLiveOutput:   settingsBool(s.HideChapterBodyLiveOutput, false),
		ChapterGroupMin:             settingsInt(s.ChapterGroupMin, 3),
		ChapterGroupMax:             settingsInt(s.ChapterGroupMax, 8),
		VersionTimedEnabled:         settingsBool(s.VersionTimedEnabled, true),
		VersionTimedIntervalMinutes: settingsInt(s.VersionTimedIntervalMinutes, 10),
		VersionAgentEnabled:         settingsBool(s.VersionAgentEnabled, true),
		VersionAgentCharThreshold:   settingsInt(s.VersionAgentCharThreshold, 3000),
		InteractiveReplyTargetChars: 2000,
		InteractiveHotChoices:       settingsBool(s.InteractiveHotChoices, true),
		ResumeLastWorkspace:         true,
		UpdateCheckEnabled:          settingsBool(s.UpdateCheckEnabled, true),
	}

	// 环境变量始终最高优先级
	overrideFromEnv(cfg)

	if cfg.Workspace != "" {
		if abs, err := filepath.Abs(cfg.Workspace); err == nil {
			cfg.Workspace = abs
		}
	}
	if cfg.SkillsDir != "" {
		cfg.SkillsDir = normalizePath(cfg.SkillsDir)
	}
	if cfg.NovaDir == "" {
		cfg.NovaDir = normalizePath(defaultNovaDir())
	} else {
		cfg.NovaDir = normalizePath(cfg.NovaDir)
	}
	return cfg, layered, nil
}

// LoadLayeredWithStartupConfig reads layered settings with the same global
// startup config layer used by LoadWithWorkspace.
func LoadLayeredWithStartupConfig(novaDir, workspace string) (LayeredSettings, error) {
	if strings.TrimSpace(novaDir) == "" {
		novaDir = startupNovaDir()
	} else {
		novaDir = normalizePath(novaDir)
	}
	globalSettings := settingsFromConfig(loadGlobalConfig())
	globalSettings.NovaDir = novaDir
	return LoadLayeredWithGlobal(novaDir, workspace, globalSettings)
}

func startupNovaDir() string {
	global := loadGlobalConfig()
	novaDir := global.NovaDir
	if novaDir == "" {
		novaDir = defaultNovaDir()
	}
	if v := os.Getenv("NOVA_DIR"); v != "" {
		novaDir = v
	}
	if novaDir == "" {
		novaDir = defaultNovaDir()
	}
	return normalizePath(novaDir)
}

func loadGlobalConfig() *Config {
	cfg := &Config{AgentIdleTimeoutSeconds: -1, AgentToolResultLimitKB: -1}
	for _, path := range globalConfigCandidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := toml.Unmarshal(data, cfg); err != nil {
			continue
		}
		return cfg
	}
	return cfg
}

func settingsFromConfig(cfg *Config) Settings {
	if cfg == nil {
		return Settings{}
	}
	settings := Settings{
		OpenAIAPIKey:             cfg.OpenAIAPIKey,
		OpenAIBaseURL:            cfg.OpenAIBaseURL,
		OpenAIModel:              cfg.OpenAIModel,
		ModelProfiles:            cfg.ModelProfiles,
		ImageAPIKey:              cfg.ImageAPIKey,
		ImageAPIBaseURL:          cfg.ImageAPIBaseURL,
		ImageAPIModel:            cfg.ImageAPIModel,
		DefaultImageAPIProfileID: cfg.DefaultImageAPIProfileID,
		ImageAPIProfiles:         cfg.ImageAPIProfiles,
		AgentModels:              cfg.AgentModels,
		AgentTools:               cfg.AgentTools,
		AgentPrompts:             cfg.AgentPrompts,
		AgentSkills:              cfg.AgentSkills,
		AgentContexts:            cfg.AgentContexts,
		GeneralSubAgents:         cfg.GeneralSubAgents,
		SubAgents:                cfg.SubAgents,
		SkillsDir:                cfg.SkillsDir,
		NovaDir:                  cfg.NovaDir,
		RemoteAccessUsername:     cfg.RemoteAccessUsername,
		RemoteAccessPasswordHash: cfg.RemoteAccessPasswordHash,
		Language:                 cfg.Language,
		ChapterFilenameFormat:    cfg.ChapterFilenameFormat,
		VolumeDirFormat:          cfg.VolumeDirFormat,
		IDEImagePresetID:         cfg.IDEImagePresetID,
		WritingSkillDefault:      cfg.WritingSkillDefault,
	}
	if cfg.HideChapterBodyLiveOutput {
		settings.HideChapterBodyLiveOutput = &cfg.HideChapterBodyLiveOutput
	}
	if cfg.BackendPort > 0 {
		settings.BackendPort = &cfg.BackendPort
	}
	if cfg.FrontendPort > 0 {
		settings.FrontendPort = &cfg.FrontendPort
	}
	settings.AllowLANAccess = &cfg.AllowLANAccess
	if cfg.MaxIteration > 0 {
		settings.MaxIteration = &cfg.MaxIteration
	}
	if cfg.ModelMaxRetries > 0 {
		settings.ModelMaxRetries = &cfg.ModelMaxRetries
	}
	if cfg.AgentIdleTimeoutSeconds >= 0 {
		settings.AgentIdleTimeoutSeconds = &cfg.AgentIdleTimeoutSeconds
	}
	if cfg.AgentToolResultLimitKB >= 0 {
		settings.AgentToolResultLimitKB = &cfg.AgentToolResultLimitKB
	}
	if cfg.OpenAIContextWindowTokens > 0 {
		settings.OpenAIContextWindowTokens = &cfg.OpenAIContextWindowTokens
	}
	return settings
}

func globalConfigCandidates() []string {
	candidates := []string{"config.toml"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "config.toml"))
	}
	return candidates
}

// Load 加载启动配置；默认不指定 workspace，让 App 恢复上次打开的书籍或进入无书籍状态。
func Load() *Config {
	cfg, _, err := LoadWithWorkspace("")
	if err != nil || cfg == nil {
		// fallback：返回纯默认值 + env，保持启动不挂
		d := DefaultSettings()
		cfg = &Config{
			OpenAIBaseURL:               d.OpenAIBaseURL,
			OpenAIModel:                 d.OpenAIModel,
			OpenAIContextWindowTokens:   settingsInt(d.OpenAIContextWindowTokens, DefaultContextWindowTokens),
			ModelProfiles:               d.ModelProfiles,
			ImageAPIKey:                 d.ImageAPIKey,
			ImageAPIBaseURL:             d.ImageAPIBaseURL,
			ImageAPIModel:               d.ImageAPIModel,
			DefaultImageAPIProfileID:    d.DefaultImageAPIProfileID,
			ImageAPIProfiles:            d.ImageAPIProfiles,
			AgentModels:                 d.AgentModels,
			AgentTools:                  d.AgentTools,
			AgentPrompts:                d.AgentPrompts,
			AgentSkills:                 d.AgentSkills,
			AgentContexts:               d.AgentContexts,
			GeneralSubAgents:            d.GeneralSubAgents,
			SubAgents:                   d.SubAgents,
			SkillsDir:                   d.SkillsDir,
			BackendPort:                 settingsInt(d.BackendPort, 8080),
			FrontendPort:                settingsInt(d.FrontendPort, 5173),
			AllowLANAccess:              settingsBool(d.AllowLANAccess, false),
			RemoteAccessUsername:        d.RemoteAccessUsername,
			RemoteAccessPasswordHash:    d.RemoteAccessPasswordHash,
			Language:                    d.Language,
			NovaDir:                     normalizePath(d.NovaDir),
			IDEStoryTellerID:            d.IDEStoryTellerID,
			IDEImagePresetID:            d.IDEImagePresetID,
			WritingSkillDefault:         d.WritingSkillDefault,
			MaxIteration:                settingsInt(d.MaxIteration, 0),
			ModelMaxRetries:             settingsInt(d.ModelMaxRetries, 5),
			AgentIdleTimeoutSeconds:     settingsAgentIdleTimeoutSeconds(d.AgentIdleTimeoutSeconds),
			AgentToolResultLimitKB:      settingsAgentToolResultLimitKB(d.AgentToolResultLimitKB),
			ChapterFilenameFormat:       d.ChapterFilenameFormat,
			VolumeDirFormat:             d.VolumeDirFormat,
			HideChapterBodyLiveOutput:   settingsBool(d.HideChapterBodyLiveOutput, false),
			ChapterGroupMin:             settingsInt(d.ChapterGroupMin, 3),
			ChapterGroupMax:             settingsInt(d.ChapterGroupMax, 8),
			VersionTimedEnabled:         settingsBool(d.VersionTimedEnabled, true),
			VersionTimedIntervalMinutes: settingsInt(d.VersionTimedIntervalMinutes, 10),
			VersionAgentEnabled:         settingsBool(d.VersionAgentEnabled, true),
			VersionAgentCharThreshold:   settingsInt(d.VersionAgentCharThreshold, 3000),
			InteractiveReplyTargetChars: 2000,
			InteractiveHotChoices:       settingsBool(d.InteractiveHotChoices, true),
			ResumeLastWorkspace:         true,
			UpdateCheckEnabled:          settingsBool(d.UpdateCheckEnabled, true),
		}
		overrideFromEnv(cfg)
		if cfg.Workspace != "" {
			if abs, err := filepath.Abs(cfg.Workspace); err == nil {
				cfg.Workspace = abs
			}
		}
		if cfg.SkillsDir != "" {
			cfg.SkillsDir = normalizePath(cfg.SkillsDir)
		}
	}
	return cfg
}

func settingsInt(v *int, fallback int) int {
	if v == nil || *v <= 0 {
		return fallback
	}
	return *v
}

func settingsAgentIdleTimeoutSeconds(v *int) int {
	if v == nil || *v < 0 {
		return DefaultAgentIdleTimeoutSeconds
	}
	return *v
}

func settingsAgentToolResultLimitKB(v *int) int {
	if v == nil || *v < 0 {
		return DefaultAgentToolResultLimitKB
	}
	return *v
}

func settingsBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

// LoadForWorkspace 加载配置并明确指定 workspace，用于 CLI 参数场景。
func LoadForWorkspace(workspace string) *Config {
	cfg, _, err := LoadWithWorkspace(workspace)
	if err != nil || cfg == nil {
		cfg = Load()
		cfg.Workspace = workspace
	}
	if cfg.Workspace != "" {
		if abs, err := filepath.Abs(cfg.Workspace); err == nil {
			cfg.Workspace = abs
		}
	}
	return cfg
}

// overrideFromEnv 用环境变量覆盖配置
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.OpenAIAPIKey = v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.OpenAIBaseURL = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" {
		cfg.OpenAIModel = v
	}
	if v := os.Getenv("OPENAI_IMAGE_API_KEY"); v != "" {
		cfg.ImageAPIKey = v
	}
	if v := os.Getenv("OPENAI_IMAGE_BASE_URL"); v != "" {
		cfg.ImageAPIBaseURL = v
	}
	if v := os.Getenv("OPENAI_IMAGE_MODEL"); v != "" {
		cfg.ImageAPIModel = v
	}
	if v := os.Getenv("NOVA_SKILLS_DIR"); v != "" {
		cfg.SkillsDir = v
	}
	if v := os.Getenv("NOVA_DIR"); v != "" {
		cfg.NovaDir = v
	}
	if v := os.Getenv("NOVA_WORKSPACE"); v != "" {
		cfg.Workspace = v
	}
	if v := os.Getenv("NOVA_BACKEND_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port >= 1 && port <= 65535 {
			cfg.BackendPort = port
		}
	}
	if v := os.Getenv("NOVA_FRONTEND_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port >= 1 && port <= 65535 {
			cfg.FrontendPort = port
		}
	}
	if v := os.Getenv("NOVA_AGENT_IDLE_TIMEOUT_SECONDS"); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds >= 0 {
			cfg.AgentIdleTimeoutSeconds = seconds
		}
	}
}

func (cfg *Config) RemoteAccessConfig() RemoteAccessConfig {
	if cfg == nil {
		return RemoteAccessConfig{}
	}
	return RemoteAccessConfig{
		AllowLANAccess: cfg.AllowLANAccess,
		Username:       cfg.RemoteAccessUsername,
		PasswordHash:   cfg.RemoteAccessPasswordHash,
	}
}

func defaultNovaDir() string {
	return "./.nova"
}

func normalizePath(path string) string {
	path = expandHome(path)
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
		return path
	}
	if len(path) > 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
