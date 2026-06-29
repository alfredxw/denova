package config

import "strings"

// AgentPromptSettings 保存各类 Agent 的自定义系统提示。
type AgentPromptSettings struct {
	Default               AgentPromptOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentPromptOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentPromptOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager         AgentPromptOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveState      AgentPromptOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentPromptOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentPromptOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentPromptOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Image                 AgentPromptOverride `toml:"image,omitempty" json:"image,omitempty"`
	Automation            AgentPromptOverride `toml:"automation,omitempty" json:"automation,omitempty"`
	ContextCompaction     AgentPromptOverride `toml:"context_compaction,omitempty" json:"context_compaction,omitempty"`
}

type AgentPromptOverride struct {
	FlowPrompt   string `toml:"flow_prompt,omitempty" json:"flow_prompt,omitempty"`
	SystemPrompt string `toml:"system_prompt,omitempty" json:"system_prompt,omitempty"`
}

type AgentPromptSourceSettings struct {
	Default               AgentPromptSourceList `json:"default,omitempty"`
	IDE                   AgentPromptSourceList `json:"ide,omitempty"`
	InteractiveStory      AgentPromptSourceList `json:"interactive_story,omitempty"`
	ConfigManager         AgentPromptSourceList `json:"config_manager,omitempty"`
	InteractiveState      AgentPromptSourceList `json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentPromptSourceList `json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentPromptSourceList `json:"version_summary,omitempty"`
	ToolAgent             AgentPromptSourceList `json:"tool_agent,omitempty"`
	Image                 AgentPromptSourceList `json:"image,omitempty"`
	Automation            AgentPromptSourceList `json:"automation,omitempty"`
	ContextCompaction     AgentPromptSourceList `json:"context_compaction,omitempty"`
}

type AgentPromptSourceList struct {
	Sources []AgentPromptSource `json:"sources,omitempty"`
}

type AgentPromptSource struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Source   string `json:"source"`
	Content  string `json:"content"`
	Editable bool   `json:"editable,omitempty"`
	Field    string `json:"field,omitempty"`
}

type AgentPromptBlockSettings struct {
	Default               AgentPromptBlocks `json:"default,omitempty"`
	IDE                   AgentPromptBlocks `json:"ide,omitempty"`
	InteractiveStory      AgentPromptBlocks `json:"interactive_story,omitempty"`
	ConfigManager         AgentPromptBlocks `json:"config_manager,omitempty"`
	InteractiveState      AgentPromptBlocks `json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentPromptBlocks `json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentPromptBlocks `json:"version_summary,omitempty"`
	ToolAgent             AgentPromptBlocks `json:"tool_agent,omitempty"`
	Image                 AgentPromptBlocks `json:"image,omitempty"`
	Automation            AgentPromptBlocks `json:"automation,omitempty"`
	ContextCompaction     AgentPromptBlocks `json:"context_compaction,omitempty"`
}

type AgentPromptBlocks struct {
	RuntimeContract      string `json:"runtime_contract,omitempty"`
	OutputProtocol       string `json:"output_protocol,omitempty"`
	EditableSystemPrompt string `json:"editable_system_prompt,omitempty"`
}

type ResolvedAgentPromptSettings struct {
	FlowPrompt   string `json:"flow_prompt"`
	SystemPrompt string `json:"system_prompt"`
}

func MergeAgentPromptSettings(parent, child AgentPromptSettings) AgentPromptSettings {
	return AgentPromptSettings{
		Default:               mergeAgentPromptOverride(parent.Default, child.Default),
		IDE:                   mergeAgentPromptOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentPromptOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:         mergeAgentPromptOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveState:      mergeAgentPromptOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentPromptOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentPromptOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentPromptOverride(parent.ToolAgent, child.ToolAgent),
		Image:                 mergeAgentPromptOverride(parent.Image, child.Image),
		Automation:            mergeAgentPromptOverride(parent.Automation, child.Automation),
		ContextCompaction:     mergeAgentPromptOverride(parent.ContextCompaction, child.ContextCompaction),
	}
}

func ResolveAgentPrompt(cfg *Config, agentKind string) ResolvedAgentPromptSettings {
	if cfg == nil {
		return ResolvedAgentPromptSettings{}
	}
	override := mergeAgentPromptOverride(cfg.AgentPrompts.Default, agentPromptOverrideFor(cfg.AgentPrompts, agentKind))
	return ResolvedAgentPromptSettings{
		FlowPrompt:   strings.TrimSpace(override.FlowPrompt),
		SystemPrompt: strings.TrimSpace(override.SystemPrompt),
	}
}

func mergeAgentPromptOverride(parent, child AgentPromptOverride) AgentPromptOverride {
	out := parent
	if strings.TrimSpace(child.FlowPrompt) != "" {
		out.FlowPrompt = child.FlowPrompt
	}
	if strings.TrimSpace(child.SystemPrompt) != "" {
		out.SystemPrompt = child.SystemPrompt
	}
	return out
}

func agentPromptOverrideFor(settings AgentPromptSettings, agentKind string) AgentPromptOverride {
	if definition, ok := LookupAgentKind(agentKind); ok && definition.PromptOverride != nil {
		return definition.PromptOverride(settings)
	}
	return AgentPromptOverride{}
}

func sanitizeAgentPromptSettings(settings AgentPromptSettings) AgentPromptSettings {
	settings.Default = sanitizeAgentPromptOverride(settings.Default)
	settings.IDE = sanitizeAgentPromptOverride(settings.IDE)
	settings.InteractiveStory = sanitizeAgentPromptOverride(settings.InteractiveStory)
	settings.ConfigManager = sanitizeAgentPromptOverride(settings.ConfigManager)
	settings.InteractiveState = sanitizeAgentPromptOverride(settings.InteractiveState)
	settings.InteractiveHotChoices = sanitizeAgentPromptOverride(settings.InteractiveHotChoices)
	settings.VersionSummary = sanitizeAgentPromptOverride(settings.VersionSummary)
	settings.ToolAgent = sanitizeAgentPromptOverride(settings.ToolAgent)
	settings.Image = sanitizeAgentPromptOverride(settings.Image)
	settings.Automation = sanitizeAgentPromptOverride(settings.Automation)
	settings.ContextCompaction = sanitizeAgentPromptOverride(settings.ContextCompaction)
	return settings
}

func sanitizeAgentPromptOverride(override AgentPromptOverride) AgentPromptOverride {
	if strings.TrimSpace(override.FlowPrompt) == "" {
		override.FlowPrompt = ""
	}
	if strings.TrimSpace(override.SystemPrompt) == "" {
		override.SystemPrompt = ""
	}
	return override
}
