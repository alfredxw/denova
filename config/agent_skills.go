package config

import "strings"

// AgentSkillSettings stores per-agent skill availability overrides.
type AgentSkillSettings struct {
	Default               AgentSkillOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentSkillOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentSkillOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	LoreEditor            AgentSkillOverride `toml:"lore_editor,omitempty" json:"lore_editor,omitempty"`
	TellerEditor          AgentSkillOverride `toml:"teller_editor,omitempty" json:"teller_editor,omitempty"`
	InteractiveState      AgentSkillOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentSkillOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentSkillOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentSkillOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Automation            AgentSkillOverride `toml:"automation,omitempty" json:"automation,omitempty"`
}

// AgentSkillOverride maps skill name to an explicit availability override.
type AgentSkillOverride map[string]bool

func MergeAgentSkillSettings(parent, child AgentSkillSettings) AgentSkillSettings {
	return AgentSkillSettings{
		Default:               mergeAgentSkillOverride(parent.Default, child.Default),
		IDE:                   mergeAgentSkillOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentSkillOverride(parent.InteractiveStory, child.InteractiveStory),
		LoreEditor:            mergeAgentSkillOverride(parent.LoreEditor, child.LoreEditor),
		TellerEditor:          mergeAgentSkillOverride(parent.TellerEditor, child.TellerEditor),
		InteractiveState:      mergeAgentSkillOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentSkillOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentSkillOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentSkillOverride(parent.ToolAgent, child.ToolAgent),
		Automation:            mergeAgentSkillOverride(parent.Automation, child.Automation),
	}
}

func ResolveAgentSkillOverrides(cfg *Config, agentKind string) map[string]bool {
	settings := AgentSkillSettings{}
	if cfg != nil {
		settings = cfg.AgentSkills
	}
	override := mergeAgentSkillOverride(settings.Default, agentSkillOverrideFor(settings, agentKind))
	result := make(map[string]bool, len(override))
	for name, enabled := range override {
		name = normalizeSkillName(name)
		if name != "" {
			result[name] = enabled
		}
	}
	return result
}

func mergeAgentSkillOverride(parent, child AgentSkillOverride) AgentSkillOverride {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	out := make(AgentSkillOverride, len(parent)+len(child))
	for name, enabled := range parent {
		if normalized := normalizeSkillName(name); normalized != "" {
			out[normalized] = enabled
		}
	}
	for name, enabled := range child {
		if normalized := normalizeSkillName(name); normalized != "" {
			out[normalized] = enabled
		}
	}
	return out
}

func agentSkillOverrideFor(settings AgentSkillSettings, agentKind string) AgentSkillOverride {
	if definition, ok := LookupAgentKind(agentKind); ok && definition.SkillOverride != nil {
		return definition.SkillOverride(settings)
	}
	return nil
}

func normalizeSkillName(name string) string {
	return strings.TrimSpace(name)
}
