package agentruntime

import (
	"denova/config"
	"denova/internal/agents"
	"runtime"
)

// EngineCapabilities describe product operations, independently of the
// engine's own configurable fields and current connection health.
type EngineCapabilities struct {
	AskUser             bool `json:"ask_user"`
	Cancel              bool `json:"cancel"`
	InteractiveApproval bool `json:"interactive_approval"`
	Delegation          bool `json:"delegation"`
	Goal                bool `json:"goal"`
	Queue               bool `json:"queue"`
	Steer               bool `json:"steer"`
	Pause               bool `json:"pause"`
}

type EngineDescriptor struct {
	ID                    config.RuntimeID       `json:"id"`
	NameKey               string                 `json:"name_key"`
	Status                string                 `json:"status"`
	ReasonKey             string                 `json:"reason_key,omitempty"`
	Capabilities          EngineCapabilities     `json:"capabilities"`
	ConfigurationSections []ConfigurationSection `json:"configuration_sections"`
}

func engineDescriptor(id config.RuntimeID) EngineDescriptor {
	item := EngineDescriptor{ID: id}
	switch id {
	case config.RuntimeNative:
		item.NameKey, item.Status = "agentRuntime.native", "ready"
		item.Capabilities = EngineCapabilities{AskUser: true, Cancel: true, InteractiveApproval: true, Delegation: true, Goal: true, Queue: true, Steer: true, Pause: true}
	case config.RuntimeCodex, config.RuntimeClaude:
		item.NameKey, item.Status = "agentRuntime."+string(id), "unchecked"
		item.Capabilities = EngineCapabilities{AskUser: true, Cancel: true}
	}
	item.ConfigurationSections = EngineConfigurationSections(id)
	return item
}

// ConfigurationSection is a finite, typed UI component identifier. Values stay
// in their original Settings paths; this projection is never persisted.
type ConfigurationSection struct {
	ID        ConfigurationSectionID `json:"id"`
	Owner     string                 `json:"owner"`
	State     string                 `json:"state"`
	ReasonKey string                 `json:"reason_key,omitempty"`
}

type ConfigurationSectionID string

const (
	SectionInstructions      ConfigurationSectionID = "shared.instructions"
	SectionSkills            ConfigurationSectionID = "shared.skills"
	SectionContextSources    ConfigurationSectionID = "shared.context_sources"
	SectionInputBudget       ConfigurationSectionID = "shared.input_budget"
	SectionNativeModel       ConfigurationSectionID = "native.model"
	SectionNativePermissions ConfigurationSectionID = "native.permissions"
	SectionNativeContext     ConfigurationSectionID = "native.context_policy"
	SectionNativeCheckpoint  ConfigurationSectionID = "native.checkpoint"
	SectionNativeSubagents   ConfigurationSectionID = "native.subagents"
	SectionClaudeModel       ConfigurationSectionID = "claude.model"
	SectionClaudePolicy      ConfigurationSectionID = "claude.execution_policy"
	SectionCodexModel        ConfigurationSectionID = "codex.model"
	SectionCodexPolicy       ConfigurationSectionID = "codex.execution_policy"
)

// AgentConfiguration is an inspection projection, never a second configuration
// store. Missing model settings and disconnected engines remain editable.
type AgentConfiguration struct {
	Selected     config.RuntimeID                     `json:"selected"`
	Sections     []ConfigurationSection               `json:"sections"`
	ToolManifest []config.ResolvedAgentToolCapability `json:"tool_manifest,omitempty"`
}

func ProjectAgentConfiguration(settings config.Settings) map[string]AgentConfiguration {
	result := make(map[string]AgentConfiguration)
	add := func(id, kind string, preferences config.RuntimePreferences) {
		selected := preferences.Selected
		if selected == "" {
			selected = config.RuntimeNative
		}
		item := AgentConfiguration{Selected: selected, Sections: EngineConfigurationSections(selected)}
		if selected != config.RuntimeNative {
			item.ToolManifest = config.ResolveAgentToolManifestForGOOS(agents.ExternalConversationToolSettings(kind), kind, runtime.GOOS)
		}
		result[id] = item
	}
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral} {
		add(kind, kind, settings.AgentRuntimes.ForAgent(kind))
	}
	for _, definition := range settings.CustomAgents {
		kind := config.CustomAgentRuntimeKind(definition)
		if kind != config.AgentKindIDE && kind != config.AgentKindGeneral {
			continue
		}
		preferences := config.RuntimePreferences{}
		if definition.Runtime != nil {
			preferences = *definition.Runtime
		}
		add(definition.ID, kind, preferences)
	}
	return result
}

func EngineConfigurationSections(selected config.RuntimeID) []ConfigurationSection {
	sections := []ConfigurationSection{
		{ID: "shared.instructions", Owner: "shared", State: "editable"},
		{ID: "shared.skills", Owner: "shared", State: "editable"},
		{ID: "shared.context_sources", Owner: "shared", State: "editable"},
		{ID: "shared.input_budget", Owner: "shared", State: "editable"},
	}
	for _, id := range []ConfigurationSectionID{SectionNativeModel, SectionNativePermissions, SectionNativeContext, SectionNativeCheckpoint, SectionNativeSubagents} {
		section := ConfigurationSection{ID: id, Owner: "native", State: "editable"}
		if selected != config.RuntimeNative {
			section.State, section.ReasonKey = "inactive", "agentRuntime.configuration.otherRuntime"
		}
		sections = append(sections, section)
	}
	for _, engine := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude} {
		for _, suffix := range []string{"model", "execution_policy"} {
			section := ConfigurationSection{ID: ConfigurationSectionID(string(engine) + "." + suffix), Owner: string(engine), State: "editable"}
			if suffix == "execution_policy" && engine != config.RuntimeCodex {
				section.State, section.ReasonKey = "read_only", "agentRuntime.configuration.managedPolicy"
			}
			if selected != engine {
				section.State, section.ReasonKey = "inactive", "agentRuntime.configuration.otherRuntime"
			}
			sections = append(sections, section)
		}
	}

	return sections
}
