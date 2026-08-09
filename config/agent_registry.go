package config

import (
	"runtime"

	agent "github.com/alfredxw/denova/agent"
)

const (
	AgentKindIDE                 = "ide"
	AgentKindGeneral             = "general"
	AgentKindInteractiveStory    = "interactive_story"
	AgentKindConfigManager       = "config_manager"
	AgentKindInteractiveDirector = "interactive_director"
	AgentKindVersionSummary      = "version_summary"
	AgentKindToolAgent           = "tool_agent"
	AgentKindImage               = "image"
	// AgentKindAutomation is retained only to decode Beta runtime journals.
	// New automation turns always run as their owning Project Agent.
	AgentKindAutomation        = "automation"
	AgentKindContextCompaction = "context_compaction"
)

// AgentKindDefinition is the registry entry for one runtime Agent kind.
// Config accessors keep the persisted TOML/JSON shape stable while avoiding
// scattered agent-kind switches for model, tool, prompt and session behavior.
type AgentKindDefinition struct {
	Kind      string
	SessionID string
	// ToolCapabilities is the authoritative capability ceiling for this
	// Agent kind, as well as the stable display order of its tool manifest.
	ToolCapabilities []string
	ModelOverride    func(AgentModelSettings) AgentModelOverride
	SetModelOverride func(*AgentModelSettings, AgentModelOverride)
	ToolOverride     func(AgentToolSettings) AgentToolOverride
	PromptOverride   func(AgentPromptSettings) AgentPromptOverride
	SkillOverride    func(AgentSkillSettings) AgentSkillOverride
	ContextOverride  func(AgentContextSettings) AgentContextOverride
}

var agentKindRegistry = []AgentKindDefinition{
	{
		Kind: AgentKindGeneral,
		ToolCapabilities: []string{
			AgentToolWorkspaceRead, AgentToolWorkspaceWrite, AgentToolShell,
			AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolAsk, AgentToolTodo, AgentToolGoal, AgentToolSkills, AgentToolDelegation,
		},
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.General },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.General = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.General },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.General },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.General },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.General },
	},
	{
		Kind: AgentKindIDE,
		ToolCapabilities: []string{
			AgentToolWorkspaceRead, AgentToolWorkspaceWrite, AgentToolShell,
			AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolAsk, AgentToolTodo, AgentToolGoal, AgentToolSkills, AgentToolDelegation,
			AgentToolLoreRead, AgentToolLoreWrite, AgentToolImageGeneration,
		},
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.IDE },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.IDE = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.IDE },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.IDE },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.IDE },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.IDE },
	},
	{
		Kind: AgentKindInteractiveStory,
		ToolCapabilities: []string{
			AgentToolWorkspaceRead, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolSkills, AgentToolDelegation, AgentToolLoreRead,
		},
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveStory },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.InteractiveStory = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveStory },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveStory },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveStory },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.InteractiveStory },
	},
	{
		Kind:      AgentKindConfigManager,
		SessionID: "config-manager-agent",
		ToolCapabilities: []string{
			AgentToolWorkspaceRead, AgentToolWorkspaceWrite, AgentToolShell,
			AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolAsk, AgentToolTodo, AgentToolSkills, AgentToolDelegation,
			AgentToolConfigRead, AgentToolConfigApply,
		},
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.ConfigManager },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.ConfigManager = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.ConfigManager },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.ConfigManager },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.ConfigManager },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.ConfigManager },
	},
	{
		Kind:      AgentKindInteractiveDirector,
		SessionID: "interactive-director-agent",
		ToolCapabilities: []string{
			AgentToolEventRead, AgentToolLoreRead,
		},
		ModelOverride: func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveDirector },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) {
			settings.InteractiveDirector = override
		},
		ToolOverride:    func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveDirector },
		PromptOverride:  func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveDirector },
		SkillOverride:   func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveDirector },
		ContextOverride: func(settings AgentContextSettings) AgentContextOverride { return settings.InteractiveDirector },
	},
	{
		Kind:             AgentKindVersionSummary,
		SessionID:        "version-summary-agent",
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.VersionSummary },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.VersionSummary = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.VersionSummary },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.VersionSummary },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.VersionSummary },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.VersionSummary },
	},
	{
		Kind:             AgentKindToolAgent,
		SessionID:        "tool-agent",
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.ToolAgent },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.ToolAgent = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.ToolAgent },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.ToolAgent },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.ToolAgent },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.ToolAgent },
	},
	{
		Kind:      AgentKindImage,
		SessionID: "image-agent",
		ToolCapabilities: []string{
			AgentToolWorkspaceRead, AgentToolWorkspaceWrite, AgentToolShell,
			AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolSkills, AgentToolImageGeneration,
		},
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.Image },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.Image = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.Image },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.Image },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.Image },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.Image },
	},
	{
		Kind:             AgentKindContextCompaction,
		SessionID:        "context-compaction-agent",
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.ContextCompaction },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.ContextCompaction = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.ContextCompaction },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.ContextCompaction },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.ContextCompaction },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.ContextCompaction },
	},
}

// AgentKindDefinitions returns all registered Agent kinds in stable UI/runtime order.
func AgentKindDefinitions() []AgentKindDefinition {
	out := make([]AgentKindDefinition, len(agentKindRegistry))
	for index, definition := range agentKindRegistry {
		out[index] = cloneAgentKindDefinition(definition)
	}
	return out
}

func LookupAgentKind(kind string) (AgentKindDefinition, bool) {
	for _, definition := range agentKindRegistry {
		if definition.Kind == kind {
			return cloneAgentKindDefinition(definition), true
		}
	}
	return AgentKindDefinition{}, false
}

func cloneAgentKindDefinition(definition AgentKindDefinition) AgentKindDefinition {
	definition.ToolCapabilities = append([]string(nil), definition.ToolCapabilities...)
	return definition
}

// AgentToolCapability describes one configurable model-callable tool family.
type AgentToolCapability struct {
	Source         string
	TitleKey       string
	DescriptionKey string
	Descriptor     AgentToolDescriptorSummary

	toolNames           []string
	windowsToolNames    []string
	runtimeAvailability bool
	subAgentUnavailable bool
}

var agentToolCapabilities = []AgentToolCapability{
	capabilityDefinition(AgentToolWorkspaceRead, "agents.tool.workspaceRead.title", "agents.tool.workspaceRead.subtitle", []string{"read", "glob", "grep"}, readOnlyDescriptor()),
	capabilityDefinition(AgentToolWorkspaceWrite, "agents.tool.workspaceWrite.title", "agents.tool.workspaceWrite.subtitle", []string{"write", "edit"}, workspaceWriteDescriptor(agent.ToolRecoveryReconcilable)),
	runtimePlatformCapabilityDefinition(AgentToolShell, "agents.tool.shell.title", "agents.tool.shell.subtitle", []string{"bash"}, []string{"pwsh"}, descriptorSummary(
		agent.ToolExecutionWorkspaceExclusive, agent.ToolMutationExternal, agent.ToolPostCheckExternalReceipt,
		agent.ToolRecoveryNonIdempotent, agent.SteeringFinishCurrent,
	)),
	capabilityDefinition(AgentToolWebSearch, "agents.tool.webSearch.title", "agents.tool.webSearch.subtitle", []string{"web_search"}, interruptibleReadDescriptor()),
	capabilityDefinition(AgentToolWebFetch, "agents.tool.webFetch.title", "agents.tool.webFetch.subtitle", []string{"web_fetch"}, interruptibleReadDescriptor()),
	runtimeCapabilityDefinition(AgentToolBrowser, "agents.tool.browser.title", "agents.tool.browser.subtitle", []string{"browser"}, descriptorSummary(
		agent.ToolExecutionSessionExclusive, agent.ToolMutationExternal, agent.ToolPostCheckExternalReceipt,
		agent.ToolRecoveryNonIdempotent, agent.SteeringFinishCurrent,
	)),
	runtimeSubAgentUnavailableCapabilityDefinition(AgentToolAsk, "agents.tool.ask.title", "agents.tool.ask.subtitle", []string{"ask"}, transientDescriptorSummary(descriptorSummary(
		agent.ToolExecutionInteractiveWait, agent.ToolMutationSession, agent.ToolPostCheckSessionState,
		agent.ToolRecoveryReconcilable, agent.SteeringFinishCurrent,
	))),
	subAgentUnavailableCapabilityDefinition(AgentToolTodo, "agents.tool.todo.title", "agents.tool.todo.subtitle", []string{"todo"}, transientDescriptorSummary(descriptorSummary(
		agent.ToolExecutionSessionExclusive, agent.ToolMutationSession, agent.ToolPostCheckSessionState,
		agent.ToolRecoveryIdempotent, agent.SteeringFinishCurrent,
	))),
	subAgentUnavailableCapabilityDefinition(AgentToolGoal, "agents.tool.goal.title", "agents.tool.goal.subtitle", []string{"goal_finish"}, transientDescriptorSummary(descriptorSummary(
		agent.ToolExecutionSessionExclusive, agent.ToolMutationSession, agent.ToolPostCheckSessionState,
		agent.ToolRecoveryIdempotent, agent.SteeringFinishCurrent,
	))),
	runtimeCapabilityDefinition(AgentToolSkills, "agents.tool.skills.title", "agents.tool.skills.subtitle", []string{"skill", "read"}, readOnlyDescriptor()),
	runtimeSubAgentUnavailableCapabilityDefinition(AgentToolDelegation, "agents.tool.delegation.title", "agents.tool.delegation.subtitle", []string{"task"}, transientDescriptorSummary(descriptorSummary(
		agent.ToolExecutionChild, agent.ToolMutationNone, agent.ToolPostCheckNone,
		agent.ToolRecoveryNonIdempotent, agent.SteeringFinishCurrent,
	))),
	capabilityDefinition(AgentToolConfigRead, "agents.tool.configRead.title", "agents.tool.configRead.subtitle", []string{"config_read"}, readOnlyDescriptor()),
	capabilityDefinition(AgentToolConfigApply, "agents.tool.configApply.title", "agents.tool.configApply.subtitle", []string{"config_apply"}, descriptorSummary(
		agent.ToolExecutionConfigExclusive, agent.ToolMutationConfig, agent.ToolPostCheckConfigRevision,
		agent.ToolRecoveryReconcilable, agent.SteeringFinishCurrent,
	)),
	runtimeSubAgentUnavailableCapabilityDefinition(AgentToolEventRead, "agents.tool.eventRead.title", "agents.tool.eventRead.subtitle", []string{"read"}, readOnlyDescriptor()),
	capabilityDefinition(AgentToolLoreRead, "agents.tool.loreRead.title", "agents.tool.loreRead.subtitle", []string{"list_lore_items", "read_lore_items"}, readOnlyDescriptor()),
	capabilityDefinition(AgentToolLoreWrite, "agents.tool.loreWrite.title", "agents.tool.loreWrite.subtitle", []string{"write_lore_items"}, workspaceWriteDescriptor(agent.ToolRecoveryReconcilable)),
	capabilityDefinition(AgentToolImageGeneration, "agents.tool.imageGeneration.title", "agents.tool.imageGeneration.subtitle", []string{"generate_image"}, workspaceWriteDescriptor(agent.ToolRecoveryNonIdempotent)),
}

func AgentToolCapabilities() []AgentToolCapability {
	out := make([]AgentToolCapability, len(agentToolCapabilities))
	for index, capability := range agentToolCapabilities {
		out[index] = cloneAgentToolCapability(capability)
	}
	return out
}

// AgentToolDescriptorSummary is the stable scheduling and recovery contract
// shared by every concrete tool represented by one capability family.
type AgentToolDescriptorSummary struct {
	Execution        agent.ToolExecutionClass      `json:"execution"`
	MutationScope    agent.ToolMutationScope       `json:"mutation_scope"`
	PostCheck        agent.ToolPostCheckPolicy     `json:"post_check"`
	Recovery         agent.ToolRecoveryClass       `json:"recovery"`
	ResultProjection agent.ToolResultProjection    `json:"result_projection"`
	ResultRetention  agent.ToolResultRetentionMode `json:"result_retention"`
	Steering         agent.SteeringPolicy          `json:"steering"`
}

// AgentToolCapabilityCatalogEntry is one platform-resolved capability. Its
// order comes from the registry and its concrete tool names match the target
// GOOS (bash on Unix-like hosts, pwsh on Windows).
type AgentToolCapabilityCatalogEntry struct {
	Capability           string                     `json:"capability"`
	TitleKey             string                     `json:"title_key"`
	DescriptionKey       string                     `json:"description_key"`
	ToolNames            []string                   `json:"tool_names"`
	Descriptor           AgentToolDescriptorSummary `json:"descriptor"`
	AvailableToSubAgents bool                       `json:"available_to_subagents"`
}

type AgentToolAvailability string

const (
	AgentToolAvailabilityAvailable    AgentToolAvailability = "available"
	AgentToolAvailabilityRuntimeCheck AgentToolAvailability = "runtime_check"
	AgentToolAvailabilityUnavailable  AgentToolAvailability = "unavailable"

	AgentToolUnavailableDisabledByPolicy = "agents.tool.unavailable.disabledByPolicy"
)

type ResolvedAgentToolCapability struct {
	AgentToolCapabilityCatalogEntry
	Allowed              bool                  `json:"allowed"`
	Availability         AgentToolAvailability `json:"availability"`
	UnavailableReasonKey string                `json:"unavailable_reason_key,omitempty"`
}

// AgentToolCapabilityCatalogForGOOS returns the complete capability catalog
// in stable display order. It is metadata only; use an Agent manifest for the
// capabilities that a concrete Agent can actually assemble.
func AgentToolCapabilityCatalogForGOOS(goos string) []AgentToolCapabilityCatalogEntry {
	capabilities := AgentToolCapabilities()
	result := make([]AgentToolCapabilityCatalogEntry, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, catalogEntryForCapability(capability, goos))
	}
	return result
}

// ResolveAgentToolManifest retains the complete catalog view for callers that
// do not select an Agent kind. Product settings should use the per-Agent form.
func ResolveAgentToolManifest(settings ResolvedAgentToolSettings) []ResolvedAgentToolCapability {
	return resolveAgentToolCapabilities(settings, AgentToolCapabilities(), runtime.GOOS)
}

// ResolveAgentToolManifestForGOOS resolves only capabilities supported by the
// selected Agent assembly. Dynamic host dependencies remain runtime_check
// instead of being guessed by the config package.
func ResolveAgentToolManifestForGOOS(settings ResolvedAgentToolSettings, agentKind, goos string) []ResolvedAgentToolCapability {
	definition, ok := LookupAgentKind(agentKind)
	if !ok {
		return []ResolvedAgentToolCapability{}
	}
	capabilities := make([]AgentToolCapability, 0, len(definition.ToolCapabilities))
	for _, source := range definition.ToolCapabilities {
		if capability, found := lookupAgentToolCapability(source); found {
			capabilities = append(capabilities, capability)
		}
	}
	return resolveAgentToolCapabilities(settings, capabilities, goos)
}

// ResolveAgentToolManifestsForGOOS builds the settings API projection for all
// registered Agent kinds, including explicit empty manifests for model-only
// Agents.
func ResolveAgentToolManifestsForGOOS(cfg *Config, goos string) map[string][]ResolvedAgentToolCapability {
	result := make(map[string][]ResolvedAgentToolCapability, len(agentKindRegistry))
	for _, definition := range AgentKindDefinitions() {
		settings := ResolveAgentTools(cfg, definition.Kind)
		result[definition.Kind] = ResolveAgentToolManifestForGOOS(settings, definition.Kind, goos)
	}
	return result
}

func AgentToolAllowed(settings ResolvedAgentToolSettings, source string) bool {
	for _, capability := range agentToolCapabilities {
		if capability.Source == source {
			return settings[source]
		}
	}
	return false
}

func resolveAgentToolCapabilities(settings ResolvedAgentToolSettings, capabilities []AgentToolCapability, goos string) []ResolvedAgentToolCapability {
	result := make([]ResolvedAgentToolCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		allowed := AgentToolAllowed(settings, capability.Source)
		availability := AgentToolAvailabilityAvailable
		reasonKey := ""
		switch {
		case !allowed:
			availability = AgentToolAvailabilityUnavailable
			reasonKey = AgentToolUnavailableDisabledByPolicy
		case capability.runtimeAvailability:
			availability = AgentToolAvailabilityRuntimeCheck
		}
		result = append(result, ResolvedAgentToolCapability{
			AgentToolCapabilityCatalogEntry: catalogEntryForCapability(capability, goos),
			Allowed:                         allowed,
			Availability:                    availability,
			UnavailableReasonKey:            reasonKey,
		})
	}
	return result
}

func lookupAgentToolCapability(source string) (AgentToolCapability, bool) {
	for _, capability := range agentToolCapabilities {
		if capability.Source == source {
			return cloneAgentToolCapability(capability), true
		}
	}
	return AgentToolCapability{}, false
}

func catalogEntryForCapability(capability AgentToolCapability, goos string) AgentToolCapabilityCatalogEntry {
	names := capability.toolNames
	if normalizedGOOS(goos) == "windows" && len(capability.windowsToolNames) != 0 {
		names = capability.windowsToolNames
	}
	return AgentToolCapabilityCatalogEntry{
		Capability:           capability.Source,
		TitleKey:             capability.TitleKey,
		DescriptionKey:       capability.DescriptionKey,
		ToolNames:            append([]string{}, names...),
		Descriptor:           capability.Descriptor,
		AvailableToSubAgents: !capability.subAgentUnavailable,
	}
}

func normalizedGOOS(goos string) string {
	if goos == "" {
		return runtime.GOOS
	}
	return goos
}

func cloneAgentToolCapability(capability AgentToolCapability) AgentToolCapability {
	capability.toolNames = append([]string(nil), capability.toolNames...)
	capability.windowsToolNames = append([]string(nil), capability.windowsToolNames...)
	return capability
}

func capabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor AgentToolDescriptorSummary) AgentToolCapability {
	return AgentToolCapability{
		Source: source, TitleKey: titleKey, DescriptionKey: descriptionKey,
		toolNames: append([]string(nil), toolNames...), Descriptor: descriptor,
	}
}

func platformCapabilityDefinition(source, titleKey, descriptionKey string, toolNames, windowsToolNames []string, descriptor AgentToolDescriptorSummary) AgentToolCapability {
	definition := capabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.windowsToolNames = append([]string(nil), windowsToolNames...)
	return definition
}

func runtimePlatformCapabilityDefinition(source, titleKey, descriptionKey string, toolNames, windowsToolNames []string, descriptor AgentToolDescriptorSummary) AgentToolCapability {
	definition := platformCapabilityDefinition(source, titleKey, descriptionKey, toolNames, windowsToolNames, descriptor)
	definition.runtimeAvailability = true
	return definition
}

func runtimeCapabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor AgentToolDescriptorSummary) AgentToolCapability {
	definition := capabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.runtimeAvailability = true
	return definition
}

func subAgentUnavailableCapabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor AgentToolDescriptorSummary) AgentToolCapability {
	definition := capabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.subAgentUnavailable = true
	return definition
}

func runtimeSubAgentUnavailableCapabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor AgentToolDescriptorSummary) AgentToolCapability {
	definition := runtimeCapabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.subAgentUnavailable = true
	return definition
}

func descriptorSummary(execution agent.ToolExecutionClass, mutation agent.ToolMutationScope, postCheck agent.ToolPostCheckPolicy, recovery agent.ToolRecoveryClass, steering agent.SteeringPolicy) AgentToolDescriptorSummary {
	retention := agent.ToolResultDeferred
	if mutation != agent.ToolMutationNone || recovery == agent.ToolRecoveryNonIdempotent {
		retention = agent.ToolResultProtected
	}
	return AgentToolDescriptorSummary{
		Execution: execution, MutationScope: mutation, PostCheck: postCheck,
		Recovery: recovery, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: retention, Steering: steering,
	}
}

func transientDescriptorSummary(summary AgentToolDescriptorSummary) AgentToolDescriptorSummary {
	summary.ResultRetention = agent.ToolResultDeferred
	return summary
}

func readOnlyDescriptor() AgentToolDescriptorSummary {
	return descriptorSummary(
		agent.ToolExecutionParallelRead, agent.ToolMutationNone, agent.ToolPostCheckNone,
		agent.ToolRecoveryReadOnly, agent.SteeringFinishCurrent,
	)
}

func interruptibleReadDescriptor() AgentToolDescriptorSummary {
	descriptor := readOnlyDescriptor()
	descriptor.Steering = agent.SteeringInterruptibleWait
	return descriptor
}

func workspaceWriteDescriptor(recovery agent.ToolRecoveryClass) AgentToolDescriptorSummary {
	return descriptorSummary(
		agent.ToolExecutionWorkspaceExclusive, agent.ToolMutationWorkspace, agent.ToolPostCheckWorkspaceChange,
		recovery, agent.SteeringFinishCurrent,
	)
}
