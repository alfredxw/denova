package config

// Agent tool capabilities are stable policy names, not concrete tool names.
// A capability may authorize several tools (for example read/glob/grep), while
// every model-visible tool declares exactly one capability in its descriptor.
const (
	AgentToolWorkspaceRead   = "workspace_read"
	AgentToolWorkspaceWrite  = "workspace_write"
	AgentToolShell           = "shell"
	AgentToolWebSearch       = "web_search"
	AgentToolWebFetch        = "web_fetch"
	AgentToolBrowser         = "browser"
	AgentToolAsk             = "ask"
	AgentToolTodo            = "todo"
	AgentToolSkills          = "skills"
	AgentToolDelegation      = "delegation"
	AgentToolConfigRead      = "config_read"
	AgentToolConfigApply     = "config_apply"
	AgentToolEventRead       = "event_read"
	AgentToolLoreRead        = "lore_read"
	AgentToolLoreWrite       = "lore_write"
	AgentToolImageGeneration = "image_generation"
	AgentToolContextRewind   = "context_rewind"
)

// AgentToolSettings stores capability overrides for every Agent kind.
type AgentToolSettings struct {
	Default             AgentToolOverride `toml:"default,omitempty" json:"default,omitempty"`
	General             AgentToolOverride `toml:"general,omitempty" json:"general,omitempty"`
	IDE                 AgentToolOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory    AgentToolOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager       AgentToolOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveDirector AgentToolOverride `toml:"interactive_director,omitempty" json:"interactive_director,omitempty"`
	VersionSummary      AgentToolOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent           AgentToolOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Image               AgentToolOverride `toml:"image,omitempty" json:"image,omitempty"`
	Automation          AgentToolOverride `toml:"automation,omitempty" json:"automation,omitempty"`
	ContextCompaction   AgentToolOverride `toml:"context_compaction,omitempty" json:"context_compaction,omitempty"`
}

// AgentToolOverride is a sparse capability set. A missing key inherits from
// the parent layer; an explicit false value disables it. Adding a capability
// therefore changes only the registry and defaults, not every settings type.
type AgentToolOverride map[string]bool

// ResolvedAgentToolSettings is a complete, fail-closed capability set.
type ResolvedAgentToolSettings map[string]bool

// Allows reports whether one registered capability is enabled.
func (settings ResolvedAgentToolSettings) Allows(capability string) bool {
	return AgentToolAllowed(settings, capability)
}

func DefaultAgentToolSettings() AgentToolSettings {
	on := func(capabilities ...string) AgentToolOverride {
		values := make(AgentToolOverride, len(capabilities))
		for _, capability := range capabilities {
			values[capability] = true
		}
		return values
	}
	off := func(capabilities ...string) AgentToolOverride {
		values := make(AgentToolOverride, len(capabilities))
		for _, capability := range capabilities {
			values[capability] = false
		}
		return values
	}

	defaults := on(
		AgentToolWorkspaceRead,
		AgentToolWorkspaceWrite,
		AgentToolShell,
		AgentToolWebSearch,
		AgentToolWebFetch,
		AgentToolBrowser,
		AgentToolAsk,
		AgentToolTodo,
		AgentToolSkills,
		AgentToolDelegation,
		AgentToolLoreRead,
		AgentToolLoreWrite,
	)
	defaults[AgentToolConfigRead] = false
	defaults[AgentToolConfigApply] = false
	defaults[AgentToolEventRead] = false
	defaults[AgentToolImageGeneration] = false
	defaults[AgentToolContextRewind] = false

	return AgentToolSettings{
		Default: defaults,
		General: on(
			AgentToolContextRewind,
		),
		IDE: on(
			AgentToolImageGeneration,
		),
		InteractiveStory: off(
			AgentToolWorkspaceWrite,
			AgentToolShell,
			AgentToolWebSearch,
			AgentToolWebFetch,
			AgentToolBrowser,
			AgentToolAsk,
			AgentToolTodo,
			AgentToolDelegation,
			AgentToolLoreWrite,
			AgentToolImageGeneration,
			AgentToolConfigRead,
			AgentToolConfigApply,
			AgentToolContextRewind,
		),
		ConfigManager: mergeAgentToolOverride(noToolAgentOverride(), on(
			AgentToolWorkspaceRead,
			AgentToolAsk,
			AgentToolSkills,
			AgentToolConfigRead,
			AgentToolConfigApply,
		)),
		InteractiveDirector: mergeAgentToolOverride(noToolAgentOverride(), on(
			AgentToolEventRead,
			AgentToolLoreRead,
		)),
		VersionSummary:    noToolAgentOverride(),
		ToolAgent:         noToolAgentOverride(),
		ContextCompaction: noToolAgentOverride(),
		Image: mergeAgentToolOverride(noToolAgentOverride(), on(
			AgentToolSkills,
			AgentToolImageGeneration,
		)),
		Automation: mergeAgentToolOverride(noToolAgentOverride(), on(
			AgentToolWorkspaceRead,
			AgentToolWorkspaceWrite,
			AgentToolWebSearch,
			AgentToolWebFetch,
			AgentToolTodo,
			AgentToolSkills,
			AgentToolDelegation,
			AgentToolLoreRead,
			AgentToolLoreWrite,
		)),
	}
}

func noToolAgentOverride() AgentToolOverride {
	values := make(AgentToolOverride, len(agentToolCapabilities))
	for _, capability := range agentToolCapabilities {
		values[capability.Source] = false
	}
	return values
}

func MergeAgentToolSettings(parent, child AgentToolSettings) AgentToolSettings {
	return AgentToolSettings{
		Default:             mergeAgentToolOverride(parent.Default, child.Default),
		General:             mergeAgentToolOverride(parent.General, child.General),
		IDE:                 mergeAgentToolOverride(parent.IDE, child.IDE),
		InteractiveStory:    mergeAgentToolOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:       mergeAgentToolOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveDirector: mergeAgentToolOverride(parent.InteractiveDirector, child.InteractiveDirector),
		VersionSummary:      mergeAgentToolOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:           mergeAgentToolOverride(parent.ToolAgent, child.ToolAgent),
		Image:               mergeAgentToolOverride(parent.Image, child.Image),
		Automation:          mergeAgentToolOverride(parent.Automation, child.Automation),
		ContextCompaction:   mergeAgentToolOverride(parent.ContextCompaction, child.ContextCompaction),
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
	definition, registered := LookupAgentKind(agentKind)
	ceiling := make(map[string]struct{}, len(definition.ToolCapabilities))
	if registered {
		for _, capability := range definition.ToolCapabilities {
			ceiling[capability] = struct{}{}
		}
	}
	resolved := make(ResolvedAgentToolSettings, len(agentToolCapabilities))
	for _, capability := range agentToolCapabilities {
		_, supported := ceiling[capability.Source]
		resolved[capability.Source] = supported && override[capability.Source]
	}
	return resolved
}

func mergeAgentToolOverride(parent, child AgentToolOverride) AgentToolOverride {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	out := make(AgentToolOverride, len(parent)+len(child))
	for capability, allowed := range parent {
		out[capability] = allowed
	}
	for capability, allowed := range child {
		out[capability] = allowed
	}
	return out
}

func agentToolOverrideFor(settings AgentToolSettings, agentKind string) AgentToolOverride {
	if definition, ok := LookupAgentKind(agentKind); ok && definition.ToolOverride != nil {
		return definition.ToolOverride(settings)
	}
	return nil
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
