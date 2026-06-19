package config

// AgentContextSettings stores per-agent context window settings.
type AgentContextSettings struct {
	Default               AgentContextOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentContextOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentContextOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager         AgentContextOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveState      AgentContextOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentContextOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentContextOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentContextOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Automation            AgentContextOverride `toml:"automation,omitempty" json:"automation,omitempty"`
}

type AgentContextOverride struct {
	RecentTurns *int `toml:"recent_turns,omitempty" json:"recent_turns,omitempty"`
}

type ResolvedAgentContextSettings struct {
	RecentTurns int `json:"recent_turns"`
}

func DefaultAgentContextSettings() AgentContextSettings {
	return AgentContextSettings{
		Default: AgentContextOverride{RecentTurns: intPtr(30)},
	}
}

func MergeAgentContextSettings(parent, child AgentContextSettings) AgentContextSettings {
	return AgentContextSettings{
		Default:               mergeAgentContextOverride(parent.Default, child.Default),
		IDE:                   mergeAgentContextOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentContextOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:         mergeAgentContextOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveState:      mergeAgentContextOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentContextOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentContextOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentContextOverride(parent.ToolAgent, child.ToolAgent),
		Automation:            mergeAgentContextOverride(parent.Automation, child.Automation),
	}
}

func ResolveAgentContext(cfg *Config, agentKind string) ResolvedAgentContextSettings {
	settings := DefaultAgentContextSettings()
	if cfg != nil {
		settings = MergeAgentContextSettings(settings, cfg.AgentContexts)
	}
	override := mergeAgentContextOverride(settings.Default, agentContextOverrideFor(settings, agentKind))
	recentTurns := 30
	if override.RecentTurns != nil && *override.RecentTurns > 0 {
		recentTurns = *override.RecentTurns
	}
	if recentTurns > 30 {
		recentTurns = 30
	}
	return ResolvedAgentContextSettings{RecentTurns: recentTurns}
}

func mergeAgentContextOverride(parent, child AgentContextOverride) AgentContextOverride {
	out := parent
	if child.RecentTurns != nil {
		out.RecentTurns = child.RecentTurns
	}
	return out
}

func agentContextOverrideFor(settings AgentContextSettings, agentKind string) AgentContextOverride {
	if definition, ok := LookupAgentKind(agentKind); ok && definition.ContextOverride != nil {
		return definition.ContextOverride(settings)
	}
	return AgentContextOverride{}
}

func sanitizeAgentContextSettings(settings AgentContextSettings) AgentContextSettings {
	settings.Default = sanitizeAgentContextOverride(settings.Default)
	settings.IDE = sanitizeAgentContextOverride(settings.IDE)
	settings.InteractiveStory = sanitizeAgentContextOverride(settings.InteractiveStory)
	settings.ConfigManager = sanitizeAgentContextOverride(settings.ConfigManager)
	settings.InteractiveState = sanitizeAgentContextOverride(settings.InteractiveState)
	settings.InteractiveHotChoices = sanitizeAgentContextOverride(settings.InteractiveHotChoices)
	settings.VersionSummary = sanitizeAgentContextOverride(settings.VersionSummary)
	settings.ToolAgent = sanitizeAgentContextOverride(settings.ToolAgent)
	settings.Automation = sanitizeAgentContextOverride(settings.Automation)
	return settings
}

func sanitizeAgentContextOverride(override AgentContextOverride) AgentContextOverride {
	if override.RecentTurns == nil {
		return override
	}
	if *override.RecentTurns <= 0 {
		override.RecentTurns = nil
		return override
	}
	if *override.RecentTurns > 30 {
		*override.RecentTurns = 30
	}
	return override
}
