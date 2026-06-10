package config

import "strings"

// AgentPromptSettings 保存各类 Agent 的自定义系统提示。
type AgentPromptSettings struct {
	Default               AgentPromptOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentPromptOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentPromptOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	LoreEditor            AgentPromptOverride `toml:"lore_editor,omitempty" json:"lore_editor,omitempty"`
	TellerEditor          AgentPromptOverride `toml:"teller_editor,omitempty" json:"teller_editor,omitempty"`
	InteractiveState      AgentPromptOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentPromptOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentPromptOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentPromptOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
}

type AgentPromptOverride struct {
	SystemPrompt string `toml:"system_prompt,omitempty" json:"system_prompt,omitempty"`
}

type ResolvedAgentPromptSettings struct {
	SystemPrompt string `json:"system_prompt"`
}

func MergeAgentPromptSettings(parent, child AgentPromptSettings) AgentPromptSettings {
	return AgentPromptSettings{
		Default:               mergeAgentPromptOverride(parent.Default, child.Default),
		IDE:                   mergeAgentPromptOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentPromptOverride(parent.InteractiveStory, child.InteractiveStory),
		LoreEditor:            mergeAgentPromptOverride(parent.LoreEditor, child.LoreEditor),
		TellerEditor:          mergeAgentPromptOverride(parent.TellerEditor, child.TellerEditor),
		InteractiveState:      mergeAgentPromptOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentPromptOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentPromptOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentPromptOverride(parent.ToolAgent, child.ToolAgent),
	}
}

func ResolveAgentPrompt(cfg *Config, agentKind string) ResolvedAgentPromptSettings {
	if cfg == nil {
		return ResolvedAgentPromptSettings{}
	}
	override := mergeAgentPromptOverride(cfg.AgentPrompts.Default, agentPromptOverrideFor(cfg.AgentPrompts, agentKind))
	return ResolvedAgentPromptSettings{
		SystemPrompt: strings.TrimSpace(override.SystemPrompt),
	}
}

func mergeAgentPromptOverride(parent, child AgentPromptOverride) AgentPromptOverride {
	out := parent
	if strings.TrimSpace(child.SystemPrompt) != "" {
		out.SystemPrompt = child.SystemPrompt
	}
	return out
}

func agentPromptOverrideFor(settings AgentPromptSettings, agentKind string) AgentPromptOverride {
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
		return AgentPromptOverride{}
	}
}

func sanitizeAgentPromptSettings(settings AgentPromptSettings) AgentPromptSettings {
	settings.Default = sanitizeAgentPromptOverride(settings.Default)
	settings.IDE = sanitizeAgentPromptOverride(settings.IDE)
	settings.InteractiveStory = sanitizeAgentPromptOverride(settings.InteractiveStory)
	settings.LoreEditor = sanitizeAgentPromptOverride(settings.LoreEditor)
	settings.TellerEditor = sanitizeAgentPromptOverride(settings.TellerEditor)
	settings.InteractiveState = sanitizeAgentPromptOverride(settings.InteractiveState)
	settings.InteractiveHotChoices = sanitizeAgentPromptOverride(settings.InteractiveHotChoices)
	settings.VersionSummary = sanitizeAgentPromptOverride(settings.VersionSummary)
	settings.ToolAgent = sanitizeAgentPromptOverride(settings.ToolAgent)
	return settings
}

func sanitizeAgentPromptOverride(override AgentPromptOverride) AgentPromptOverride {
	if strings.TrimSpace(override.SystemPrompt) == "" {
		override.SystemPrompt = ""
	}
	return override
}
