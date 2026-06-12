package config

const (
	AgentKindIDE                   = "ide"
	AgentKindInteractiveStory      = "interactive_story"
	AgentKindLoreEditor            = "lore_editor"
	AgentKindTellerEditor          = "teller_editor"
	AgentKindInteractiveState      = "interactive_state"
	AgentKindInteractiveHotChoices = "interactive_hot_choices"
	AgentKindVersionSummary        = "version_summary"
	AgentKindToolAgent             = "tool_agent"
	AgentKindAutomation            = "automation"
)

// AgentKindDefinition is the registry entry for one runtime Agent kind.
// Config accessors keep the persisted TOML/JSON shape stable while avoiding
// scattered agent-kind switches for model, tool, prompt and session behavior.
type AgentKindDefinition struct {
	Kind           string
	SessionID      string
	ModelOverride  func(AgentModelSettings) AgentModelOverride
	ToolOverride   func(AgentToolSettings) AgentToolOverride
	PromptOverride func(AgentPromptSettings) AgentPromptOverride
	SkillOverride  func(AgentSkillSettings) AgentSkillOverride
}

var agentKindRegistry = []AgentKindDefinition{
	{
		Kind:           AgentKindIDE,
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.IDE },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.IDE },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.IDE },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.IDE },
	},
	{
		Kind:           AgentKindInteractiveStory,
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveStory },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveStory },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveStory },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveStory },
	},
	{
		Kind:           AgentKindLoreEditor,
		SessionID:      "lore-agent",
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.LoreEditor },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.LoreEditor },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.LoreEditor },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.LoreEditor },
	},
	{
		Kind:           AgentKindTellerEditor,
		SessionID:      "teller-agent",
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.TellerEditor },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.TellerEditor },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.TellerEditor },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.TellerEditor },
	},
	{
		Kind:           AgentKindInteractiveState,
		SessionID:      "interactive-state-agent",
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveState },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveState },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveState },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveState },
	},
	{
		Kind:           AgentKindInteractiveHotChoices,
		SessionID:      "interactive-hot-choices-agent",
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveHotChoices },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveHotChoices },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveHotChoices },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveHotChoices },
	},
	{
		Kind:           AgentKindVersionSummary,
		SessionID:      "version-summary-agent",
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.VersionSummary },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.VersionSummary },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.VersionSummary },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.VersionSummary },
	},
	{
		Kind:           AgentKindToolAgent,
		SessionID:      "tool-agent",
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.ToolAgent },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.ToolAgent },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.ToolAgent },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.ToolAgent },
	},
	{
		Kind:           AgentKindAutomation,
		ModelOverride:  func(settings AgentModelSettings) AgentModelOverride { return settings.Automation },
		ToolOverride:   func(settings AgentToolSettings) AgentToolOverride { return settings.Automation },
		PromptOverride: func(settings AgentPromptSettings) AgentPromptOverride { return settings.Automation },
		SkillOverride:  func(settings AgentSkillSettings) AgentSkillOverride { return settings.Automation },
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
	default:
		return false
	}
}
