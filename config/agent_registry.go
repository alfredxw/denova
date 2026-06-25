package config

const (
	AgentKindIDE                   = "ide"
	AgentKindInteractiveStory      = "interactive_story"
	AgentKindConfigManager         = "config_manager"
	AgentKindInteractiveState      = "interactive_state"
	AgentKindInteractiveHotChoices = "interactive_hot_choices"
	AgentKindVersionSummary        = "version_summary"
	AgentKindToolAgent             = "tool_agent"
	AgentKindAutomation            = "automation"
	AgentKindContextCompaction     = "context_compaction"
)

// AgentKindDefinition is the registry entry for one runtime Agent kind.
// Config accessors keep the persisted TOML/JSON shape stable while avoiding
// scattered agent-kind switches for model, tool, prompt and session behavior.
type AgentKindDefinition struct {
	Kind            string
	SessionID       string
	ModelOverride   func(AgentModelSettings) AgentModelOverride
	ToolOverride    func(AgentToolSettings) AgentToolOverride
	PromptOverride  func(AgentPromptSettings) AgentPromptOverride
	SkillOverride   func(AgentSkillSettings) AgentSkillOverride
	ContextOverride func(AgentContextSettings) AgentContextOverride
}

var agentKindRegistry = []AgentKindDefinition{
	{
		Kind:            AgentKindIDE,
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.IDE },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.IDE },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.IDE },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.IDE },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.IDE },
	},
	{
		Kind:            AgentKindInteractiveStory,
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveStory },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveStory },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveStory },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveStory },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.InteractiveStory },
	},
	{
		Kind:            AgentKindConfigManager,
		SessionID:       "config-manager-agent",
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.ConfigManager },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.ConfigManager },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.ConfigManager },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.ConfigManager },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.ConfigManager },
	},
	{
		Kind:            AgentKindInteractiveState,
		SessionID:       "interactive-state-agent",
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveState },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveState },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveState },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveState },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.InteractiveState },
	},
	{
		Kind:            AgentKindInteractiveHotChoices,
		SessionID:       "interactive-hot-choices-agent",
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveHotChoices },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveHotChoices },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveHotChoices },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveHotChoices },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.InteractiveHotChoices },
	},
	{
		Kind:            AgentKindVersionSummary,
		SessionID:       "version-summary-agent",
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.VersionSummary },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.VersionSummary },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.VersionSummary },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.VersionSummary },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.VersionSummary },
	},
	{
		Kind:            AgentKindToolAgent,
		SessionID:       "tool-agent",
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.ToolAgent },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.ToolAgent },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.ToolAgent },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.ToolAgent },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.ToolAgent },
	},
	{
		Kind:            AgentKindAutomation,
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.Automation },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.Automation },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.Automation },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.Automation },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.Automation },
	},
	{
		Kind:            AgentKindContextCompaction,
		SessionID:       "context-compaction-agent",
		ModelOverride:   func(settings AgentModelSettings) AgentModelOverride { return settings.ContextCompaction },
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.ContextCompaction },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.ContextCompaction },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.ContextCompaction },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.ContextCompaction },
	},
}

// AgentKindDefinitions returns all registered Agent kinds in stable UI/runtime order.
func AgentKindDefinitions() []AgentKindDefinition {
	out := make([]AgentKindDefinition, len(agentKindRegistry))
	copy(out, agentKindRegistry)
	return out
}

func LookupAgentKind(kind string) (AgentKindDefinition, bool) {
	for _, definition := range agentKindRegistry {
		if definition.Kind == kind {
			return definition, true
		}
	}
	return AgentKindDefinition{}, false
}

// AgentToolCapability describes one configurable model-callable tool family.
type AgentToolCapability struct {
	Source string
}

var agentToolCapabilities = []AgentToolCapability{
	{Source: AgentToolFileRead},
	{Source: AgentToolFileWrite},
	{Source: AgentToolShellExecute},
	{Source: AgentToolSkills},
	{Source: AgentToolLoreRead},
	{Source: AgentToolLoreWrite},
	{Source: AgentToolTodo},
	{Source: AgentToolWebSearch},
	{Source: AgentToolAgentConfigRead},
	{Source: AgentToolAgentConfigWrite},
}

func AgentToolCapabilities() []AgentToolCapability {
	out := make([]AgentToolCapability, len(agentToolCapabilities))
	copy(out, agentToolCapabilities)
	return out
}

type ResolvedAgentToolCapability struct {
	Source  string `json:"source"`
	Allowed bool   `json:"allowed"`
}

func ResolveAgentToolManifest(settings ResolvedAgentToolSettings) []ResolvedAgentToolCapability {
	result := make([]ResolvedAgentToolCapability, 0, len(agentToolCapabilities))
	for _, capability := range agentToolCapabilities {
		result = append(result, ResolvedAgentToolCapability{
			Source:  capability.Source,
			Allowed: AgentToolAllowed(settings, capability.Source),
		})
	}
	return result
}

func AgentToolAllowed(settings ResolvedAgentToolSettings, source string) bool {
	switch source {
	case AgentToolFileRead:
		return settings.FileRead
	case AgentToolFileWrite:
		return settings.FileWrite
	case AgentToolShellExecute:
		return settings.ShellExecute
	case AgentToolSkills:
		return settings.Skills
	case AgentToolLoreRead:
		return settings.LoreRead
	case AgentToolLoreWrite:
		return settings.LoreWrite
	case AgentToolTodo:
		return settings.Todo
	case AgentToolWebSearch:
		return settings.WebSearch
	case AgentToolAgentConfigRead:
		return settings.AgentConfigRead
	case AgentToolAgentConfigWrite:
		return settings.AgentConfigWrite
	default:
		return false
	}
}
