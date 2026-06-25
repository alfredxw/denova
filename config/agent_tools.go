package config

const (
	AgentToolFileRead         = "file_read"
	AgentToolFileWrite        = "file_write"
	AgentToolShellExecute     = "shell_execute"
	AgentToolSkills           = "skills"
	AgentToolLoreRead         = "lore_read"
	AgentToolLoreWrite        = "lore_write"
	AgentToolTodo             = "todo"
	AgentToolWebSearch        = "web_search"
	AgentToolAgentConfigRead  = "agent_config_read"
	AgentToolAgentConfigWrite = "agent_config_write"
)

// AgentToolSettings 保存各类 Agent 的工具能力开关。
type AgentToolSettings struct {
	Default               AgentToolOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentToolOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentToolOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager         AgentToolOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveState      AgentToolOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentToolOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentToolOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentToolOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Automation            AgentToolOverride `toml:"automation,omitempty" json:"automation,omitempty"`
	ContextCompaction     AgentToolOverride `toml:"context_compaction,omitempty" json:"context_compaction,omitempty"`
}

// AgentToolOverride 的指针字段用于区分继承与显式关闭。
type AgentToolOverride struct {
	FileRead         *bool `toml:"file_read,omitempty" json:"file_read,omitempty"`
	FileWrite        *bool `toml:"file_write,omitempty" json:"file_write,omitempty"`
	ShellExecute     *bool `toml:"shell_execute,omitempty" json:"shell_execute,omitempty"`
	Skills           *bool `toml:"skills,omitempty" json:"skills,omitempty"`
	LoreRead         *bool `toml:"lore_read,omitempty" json:"lore_read,omitempty"`
	LoreWrite        *bool `toml:"lore_write,omitempty" json:"lore_write,omitempty"`
	Todo             *bool `toml:"todo,omitempty" json:"todo,omitempty"`
	WebSearch        *bool `toml:"web_search,omitempty" json:"web_search,omitempty"`
	AgentConfigRead  *bool `toml:"agent_config_read,omitempty" json:"agent_config_read,omitempty"`
	AgentConfigWrite *bool `toml:"agent_config_write,omitempty" json:"agent_config_write,omitempty"`
}

type ResolvedAgentToolSettings struct {
	FileRead         bool `json:"file_read"`
	FileWrite        bool `json:"file_write"`
	ShellExecute     bool `json:"shell_execute"`
	Skills           bool `json:"skills"`
	LoreRead         bool `json:"lore_read"`
	LoreWrite        bool `json:"lore_write"`
	Todo             bool `json:"todo"`
	WebSearch        bool `json:"web_search"`
	AgentConfigRead  bool `json:"agent_config_read"`
	AgentConfigWrite bool `json:"agent_config_write"`
}

func DefaultAgentToolSettings() AgentToolSettings {
	on := boolPtr(true)
	off := boolPtr(false)
	return AgentToolSettings{
		Default: AgentToolOverride{
			FileRead:         on,
			FileWrite:        on,
			ShellExecute:     on,
			Skills:           on,
			LoreRead:         on,
			LoreWrite:        on,
			Todo:             on,
			WebSearch:        on,
			AgentConfigRead:  off,
			AgentConfigWrite: off,
		},
		InteractiveStory: AgentToolOverride{
			LoreWrite: off,
			Todo:      off,
			WebSearch: off,
		},
		ConfigManager: AgentToolOverride{
			ShellExecute:     off,
			AgentConfigRead:  on,
			AgentConfigWrite: on,
		},
		InteractiveState:      noToolAgentOverride(),
		InteractiveHotChoices: noToolAgentOverride(),
		VersionSummary:        noToolAgentOverride(),
		ToolAgent:             noToolAgentOverride(),
		ContextCompaction:     noToolAgentOverride(),
		Automation: AgentToolOverride{
			FileRead:         on,
			FileWrite:        on,
			ShellExecute:     off,
			Skills:           on,
			LoreRead:         on,
			LoreWrite:        on,
			Todo:             on,
			WebSearch:        on,
			AgentConfigRead:  off,
			AgentConfigWrite: off,
		},
	}
}

func noToolAgentOverride() AgentToolOverride {
	off := boolPtr(false)
	return AgentToolOverride{
		FileRead:         off,
		FileWrite:        off,
		ShellExecute:     off,
		Skills:           off,
		LoreRead:         off,
		LoreWrite:        off,
		Todo:             off,
		WebSearch:        off,
		AgentConfigRead:  off,
		AgentConfigWrite: off,
	}
}

func MergeAgentToolSettings(parent, child AgentToolSettings) AgentToolSettings {
	return AgentToolSettings{
		Default:               mergeAgentToolOverride(parent.Default, child.Default),
		IDE:                   mergeAgentToolOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentToolOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:         mergeAgentToolOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveState:      mergeAgentToolOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentToolOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentToolOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentToolOverride(parent.ToolAgent, child.ToolAgent),
		Automation:            mergeAgentToolOverride(parent.Automation, child.Automation),
		ContextCompaction:     mergeAgentToolOverride(parent.ContextCompaction, child.ContextCompaction),
	}
}

func ResolveAgentTools(cfg *Config, agentKind string) ResolvedAgentToolSettings {
	return resolveAgentTools(cfg, agentKind)
}

func resolveAgentToolsForGOOS(cfg *Config, agentKind, _ string) ResolvedAgentToolSettings {
	return resolveAgentTools(cfg, agentKind)
}

func resolveAgentTools(cfg *Config, agentKind string) ResolvedAgentToolSettings {
	settings := DefaultAgentToolSettings()
	if cfg != nil {
		settings = MergeAgentToolSettings(settings, cfg.AgentTools)
	}
	override := mergeAgentToolOverride(settings.Default, agentToolOverrideFor(settings, agentKind))
	resolved := ResolvedAgentToolSettings{
		FileRead:         boolValue(override.FileRead, true),
		FileWrite:        boolValue(override.FileWrite, true),
		ShellExecute:     boolValue(override.ShellExecute, true),
		Skills:           boolValue(override.Skills, true),
		LoreRead:         boolValue(override.LoreRead, true),
		LoreWrite:        boolValue(override.LoreWrite, true),
		Todo:             boolValue(override.Todo, true),
		WebSearch:        boolValue(override.WebSearch, true),
		AgentConfigRead:  boolValue(override.AgentConfigRead, false),
		AgentConfigWrite: boolValue(override.AgentConfigWrite, false),
	}
	return resolved
}

func mergeAgentToolOverride(parent, child AgentToolOverride) AgentToolOverride {
	out := parent
	if child.FileRead != nil {
		out.FileRead = child.FileRead
	}
	if child.FileWrite != nil {
		out.FileWrite = child.FileWrite
	}
	if child.ShellExecute != nil {
		out.ShellExecute = child.ShellExecute
	}
	if child.Skills != nil {
		out.Skills = child.Skills
	}
	if child.LoreRead != nil {
		out.LoreRead = child.LoreRead
	}
	if child.LoreWrite != nil {
		out.LoreWrite = child.LoreWrite
	}
	if child.Todo != nil {
		out.Todo = child.Todo
	}
	if child.WebSearch != nil {
		out.WebSearch = child.WebSearch
	}
	if child.AgentConfigRead != nil {
		out.AgentConfigRead = child.AgentConfigRead
	}
	if child.AgentConfigWrite != nil {
		out.AgentConfigWrite = child.AgentConfigWrite
	}
	return out
}

func agentToolOverrideFor(settings AgentToolSettings, agentKind string) AgentToolOverride {
	if definition, ok := LookupAgentKind(agentKind); ok && definition.ToolOverride != nil {
		return definition.ToolOverride(settings)
	}
	return AgentToolOverride{}
}

func boolValue(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}
