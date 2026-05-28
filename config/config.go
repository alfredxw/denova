package config

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Config 保存 Nova 的全局配置
type Config struct {
	OpenAIAPIKey                string `toml:"openai_api_key"`
	OpenAIBaseURL               string `toml:"openai_base_url"`
	OpenAIModel                 string `toml:"openai_model"`
	SkillsDir                   string `toml:"skills_dir"`
	NovaDir                     string `toml:"nova_dir"`
	Workspace                   string `toml:"workspace"`
	InteractiveReplyTargetChars int    `toml:"-"`
	InteractiveMaxTokens        int    `toml:"-"`
	ResumeLastWorkspace         bool   `toml:"-"`
}

// LoadWithWorkspace 在已知 workspace 时读取分层配置（默认 < 用户级 < 工作区级 < 环境变量）。
func LoadWithWorkspace(workspace string) (*Config, LayeredSettings, error) {
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
	novaDir = normalizePath(novaDir)

	globalSettings := settingsFromConfig(global)
	globalSettings.NovaDir = novaDir

	layered, err := LoadLayeredWithGlobal(novaDir, workspace, globalSettings)
	if err != nil {
		return nil, LayeredSettings{}, err
	}

	s := layered.Effective
	cfg := &Config{
		OpenAIAPIKey:                s.OpenAIAPIKey,
		OpenAIBaseURL:               s.OpenAIBaseURL,
		OpenAIModel:                 s.OpenAIModel,
		SkillsDir:                   s.SkillsDir,
		NovaDir:                     novaDir,
		Workspace:                   workspace,
		InteractiveReplyTargetChars: settingsInt(s.InteractiveReplyTargetChars, 1200),
		InteractiveMaxTokens:        settingsInt(s.InteractiveMaxTokens, 0),
		ResumeLastWorkspace:         true,
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

func loadGlobalConfig() *Config {
	cfg := &Config{}
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
	return Settings{
		OpenAIAPIKey:  cfg.OpenAIAPIKey,
		OpenAIBaseURL: cfg.OpenAIBaseURL,
		OpenAIModel:   cfg.OpenAIModel,
		SkillsDir:     cfg.SkillsDir,
		NovaDir:       cfg.NovaDir,
	}
}

func globalConfigCandidates() []string {
	candidates := []string{"config.toml"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "config.toml"))
	}
	return candidates
}

// Load 加载启动配置；默认不指定 workspace，让 App 恢复最近书籍或进入无书籍状态。
func Load() *Config {
	cfg, _, err := LoadWithWorkspace("")
	if err != nil || cfg == nil {
		// fallback：返回纯默认值 + env，保持启动不挂
		d := DefaultSettings()
		cfg = &Config{
			OpenAIBaseURL:               d.OpenAIBaseURL,
			OpenAIModel:                 d.OpenAIModel,
			SkillsDir:                   d.SkillsDir,
			NovaDir:                     normalizePath(d.NovaDir),
			InteractiveReplyTargetChars: settingsInt(d.InteractiveReplyTargetChars, 1200),
			InteractiveMaxTokens:        settingsInt(d.InteractiveMaxTokens, 0),
			ResumeLastWorkspace:         true,
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
	if v := os.Getenv("NOVA_SKILLS_DIR"); v != "" {
		cfg.SkillsDir = v
	}
	if v := os.Getenv("NOVA_DIR"); v != "" {
		cfg.NovaDir = v
	}
	if v := os.Getenv("NOVA_WORKSPACE"); v != "" {
		cfg.Workspace = v
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
