package config

const (
	AgentToolFileRead     = "file_read"
	AgentToolFileWrite    = "file_write"
	AgentToolShellExecute = "shell_execute"
	AgentToolSkills       = "skills"
	AgentToolLoreRead     = "lore_read"
	AgentToolLoreWrite    = "lore_write"
	AgentToolTodo         = "todo"
)

// AgentToolSettings 保存各类 Agent 的工具能力开关。
type AgentToolSettings struct {
	Default               AgentToolOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentToolOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentToolOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	LoreEditor            AgentToolOverride `toml:"lore_editor,omitempty" json:"lore_editor,omitempty"`
	TellerEditor          AgentToolOverride `toml:"teller_editor,omitempty" json:"teller_editor,omitempty"`
	InteractiveState      AgentToolOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentToolOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentToolOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentToolOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
}

// AgentToolOverride 的指针字段用于区分继承与显式关闭。
type AgentToolOverride struct {
	FileRead     *bool `toml:"file_read,omitempty" json:"file_read,omitempty"`
	FileWrite    *bool `toml:"file_write,omitempty" json:"file_write,omitempty"`
	ShellExecute *bool `toml:"shell_execute,omitempty" json:"shell_execute,omitempty"`
	Skills       *bool `toml:"skills,omitempty" json:"skills,omitempty"`
	LoreRead     *bool `toml:"lore_read,omitempty" json:"lore_read,omitempty"`
	LoreWrite    *bool `toml:"lore_write,omitempty" json:"lore_write,omitempty"`
	Todo         *bool `toml:"todo,omitempty" json:"todo,omitempty"`
}

type ResolvedAgentToolSettings struct {
	FileRead     bool `json:"file_read"`
	FileWrite    bool `json:"file_write"`
	ShellExecute bool `json:"shell_execute"`
	Skills       bool `json:"skills"`
	LoreRead     bool `json:"lore_read"`
	LoreWrite    bool `json:"lore_write"`
	Todo         bool `json:"todo"`
}

func DefaultAgentToolSettings() AgentToolSettings {
	on := boolPtr(true)
	off := boolPtr(false)
	return AgentToolSettings{
		Default: AgentToolOverride{
			FileRead:     on,
			FileWrite:    on,
			ShellExecute: on,
			Skills:       on,
			LoreRead:     on,
			LoreWrite:    on,
			Todo:         on,
		},
		InteractiveStory: AgentToolOverride{
			Skills:    off,
			LoreWrite: off,
			Todo:      off,
		},
		LoreEditor: AgentToolOverride{
			ShellExecute: off,
			Todo:         off,
		},
		TellerEditor:          noToolAgentOverride(),
		InteractiveState:      noToolAgentOverride(),
		InteractiveHotChoices: noToolAgentOverride(),
		VersionSummary:        noToolAgentOverride(),
		ToolAgent:             noToolAgentOverride(),
	}
}

func noToolAgentOverride() AgentToolOverride {
	off := boolPtr(false)
	return AgentToolOverride{
		FileRead:     off,
		FileWrite:    off,
		ShellExecute: off,
		Skills:       off,
		LoreRead:     off,
		LoreWrite:    off,
		Todo:         off,
	}
}

func MergeAgentToolSettings(parent, child AgentToolSettings) AgentToolSettings {
	return AgentToolSettings{
		Default:               mergeAgentToolOverride(parent.Default, child.Default),
		IDE:                   mergeAgentToolOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentToolOverride(parent.InteractiveStory, child.InteractiveStory),
		LoreEditor:            mergeAgentToolOverride(parent.LoreEditor, child.LoreEditor),
		TellerEditor:          mergeAgentToolOverride(parent.TellerEditor, child.TellerEditor),
		InteractiveState:      mergeAgentToolOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentToolOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentToolOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentToolOverride(parent.ToolAgent, child.ToolAgent),
	}
}

func ResolveAgentTools(cfg *Config, agentKind string) ResolvedAgentToolSettings {
	settings := DefaultAgentToolSettings()
	if cfg != nil {
		settings = MergeAgentToolSettings(settings, cfg.AgentTools)
	}
	override := mergeAgentToolOverride(settings.Default, agentToolOverrideFor(settings, agentKind))
	return ResolvedAgentToolSettings{
		FileRead:     boolValue(override.FileRead, true),
		FileWrite:    boolValue(override.FileWrite, true),
		ShellExecute: boolValue(override.ShellExecute, true),
		Skills:       boolValue(override.Skills, true),
		LoreRead:     boolValue(override.LoreRead, true),
		LoreWrite:    boolValue(override.LoreWrite, true),
		Todo:         boolValue(override.Todo, true),
	}
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
	return out
}

func agentToolOverrideFor(settings AgentToolSettings, agentKind string) AgentToolOverride {
	switch agentKind {
	case AgentKindIDE:
		return settings.IDE
	case AgentKindInteractiveStory:
		return settings.InteractiveStory
	case AgentKindLoreEditor:
		return settings.LoreEditor
	case AgentKindTellerEditor:
		return settings.TellerEditor
	case AgentKindInteractiveState:
		return settings.InteractiveState
	case AgentKindInteractiveHotChoices:
		return settings.InteractiveHotChoices
	case AgentKindVersionSummary:
		return settings.VersionSummary
	case AgentKindToolAgent:
		return settings.ToolAgent
	default:
		return AgentToolOverride{}
	}
}

func boolValue(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}
